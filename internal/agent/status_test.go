package agent_test

import (
	"context"
	"os"
	"path"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/agent"
	"github.com/kubev2v/migration-planner/internal/agent/client"
	"github.com/kubev2v/migration-planner/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	noCredentialsProvidedStatusInfo    = "No credentials provided"
	inventoryNotYetCollectedStatusInfo = "Inventory not yet collected"
	inventoryCollectedStatusInfo       = "Inventory successfully collected"
)

var _ = Describe("Status", func() {
	var agentID uuid.UUID
	BeforeEach(func() {
		agentID, _ = uuid.NewUUID()
	})

	Context("update status", func() {
		It("successfully updates status", func() {
			client := client.PlannerMock{
				UpdateAgentStatusFunc: func(ctx context.Context, id uuid.UUID, params v1alpha1.AgentStatusUpdate) error {
					Expect(id).To(Equal(agentID))
					Expect(params.Version).To(Equal("best_version"))
					Expect(params.Status).To(Equal("up-to-date"))
					Expect(params.CredentialUrl).To(Equal("www-cred-url"))
					Expect(params.StatusInfo).To(Equal("status_info"))
					return nil
				},
			}

			statusUpdater := agent.NewStatusUpdater(log.NewPrefixLogger(""), agentID, "best_version", "www-cred-url", &agent.Config{}, &client)
			Expect(statusUpdater.UpdateStatus(context.TODO(), api.AgentStatusUpToDate, "status_info"))
		})
	})

	Context("compute status", func() {
		var (
			dataTmpFolder string
			plannerClient *client.PlannerMock
		)
		BeforeEach(func() {
			var err error
			dataTmpFolder, err = os.MkdirTemp("", "agent-data-folder")
			Expect(err).To(BeNil())
			plannerClient = &client.PlannerMock{}
		})
		AfterEach(func() {
			os.RemoveAll(dataTmpFolder)
		})

		It("compute status returns Waiting for credentials", func() {
			statusUpdater := agent.NewStatusUpdater(log.NewPrefixLogger(""),
				agentID,
				"best_version",
				"www-cred-url",
				&agent.Config{
					DataDir: dataTmpFolder,
				},
				plannerClient,
			)

			status, status_info, inventory := statusUpdater.CalculateStatus()
			Expect(status).To(Equal(api.AgentStatusWaitingForCredentials))
			Expect(status_info).To(Equal(noCredentialsProvidedStatusInfo))
			Expect(inventory).To(BeNil())
		})

		It("compute status returns GatheringInitialInventory", func() {
			// create credentials.json
			creds, err := os.Create(path.Join(dataTmpFolder, "credentials.json"))
			Expect(err).To(BeNil())
			creds.Close()

			statusUpdater := agent.NewStatusUpdater(log.NewPrefixLogger(""),
				agentID,
				"best_version",
				"www-cred-url",
				&agent.Config{
					DataDir: dataTmpFolder,
				},
				plannerClient,
			)

			status, status_info, inventory := statusUpdater.CalculateStatus()
			Expect(status).To(Equal(api.AgentStatusGatheringInitialInventory))
			Expect(status_info).To(Equal(inventoryNotYetCollectedStatusInfo))
			Expect(inventory).To(BeNil())
		})

		It("compute status returns InventoryUptoDate", func() {
			// create credentials.json
			creds, err := os.Create(path.Join(dataTmpFolder, "credentials.json"))
			Expect(err).To(BeNil())
			creds.Close()

			inventoryFile, err := os.Create(path.Join(dataTmpFolder, "inventory.json"))
			Expect(err).To(BeNil())

			_, err = inventoryFile.Write([]byte("{\"inventory\": {}, \"error\": \"\"}"))
			Expect(err).To(BeNil())

			statusUpdater := agent.NewStatusUpdater(log.NewPrefixLogger(""),
				agentID,
				"best_version",
				"www-cred-url",
				&agent.Config{
					DataDir: dataTmpFolder,
				},
				plannerClient,
			)

			status, status_info, inventory := statusUpdater.CalculateStatus()
			Expect(status).To(Equal(api.AgentStatusUpToDate))
			Expect(status_info).To(Equal(inventoryCollectedStatusInfo))
			Expect(inventory).ToNot(BeNil())
		})
	})

})
