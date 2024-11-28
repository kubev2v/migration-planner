package agent_test

import (
	"context"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/agent"
	"github.com/kubev2v/migration-planner/internal/agent/client"
	"github.com/kubev2v/migration-planner/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Inventory", func() {
	var (
		agentID  uuid.UUID
		sourceID uuid.UUID
	)
	BeforeEach(func() {
		agentID, _ = uuid.NewUUID()
		sourceID, _ = uuid.NewUUID()
	})

	Context("update inventory", func() {
		It("successfully updates inventory", func() {
			client := client.PlannerMock{
				UpdateSourceStatusFunc: func(ctx context.Context, id uuid.UUID, params v1alpha1.SourceStatusUpdate) error {
					Expect(id).To(Equal(sourceID))
					Expect(params.AgentId).To(Equal(agentID))
					Expect(params.Inventory).ToNot(BeNil())
					return nil

				},
			}

			inventory := &api.Inventory{
				Vms:     api.VMs{Total: 2},
				Vcenter: api.VCenter{Id: sourceID.String()},
			}
			inventoryUpdater := agent.NewInventoryUpdater(log.NewPrefixLogger(""), agentID, &client)
			inventoryUpdater.UpdateServiceWithInventory(context.TODO(), inventory)
		})
	})
})
