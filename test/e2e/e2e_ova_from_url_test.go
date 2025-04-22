package e2e_test

import (
	"fmt"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_agent"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_helpers"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_service"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"net/http"
	"time"
)

var _ = Describe("e2e-download-ova-from-url", func() {

	var (
		svc       PlannerService
		agent     PlannerAgent
		agentIP   string
		err       error
		source    *v1alpha1.Source
		startTime time.Time
	)

	BeforeEach(func() {
		startTime = time.Now()
		TestOptions.DownloadImageByUrl = true
		TestOptions.DisconnectedEnvironment = false

		svc, err = DefaultPlannerService()
		Expect(err).To(BeNil(), "Failed to create PlannerService")

		source, err = svc.CreateSource("source")
		Expect(err).To(BeNil())
		Expect(source).NotTo(BeNil())

		agent, err = CreateAgent(DefaultAgentTestID, source.Id, VmName, svc)
		Expect(err).To(BeNil())

		zap.S().Info("Waiting for agent IP...")
		Eventually(func() error {
			agentIP, err = agent.GetIp()
			if err != nil {
				return err
			}
			return nil
		}, "3m", "2s").Should(BeNil())
		zap.S().Infof("Agent ip is: %s", agentIP)

		agentApiBaseUrl := fmt.Sprintf("https://%s:3333/api/v1/", agentIP)
		agent.SetAgentApi(DefaultAgentApi(agentApiBaseUrl))
		zap.S().Infof("Agent Api base url: %s", agentApiBaseUrl)

		zap.S().Info("Wait for planner-agent to be running...")
		Eventually(func() bool {
			return agent.IsServiceRunning(agentIP, "planner-agent")
		}, "3m", "2s").Should(BeTrue())
		zap.S().Info("Planner-agent is now running")

		Eventually(func() string {
			credUrl, err := CredentialURL(svc, source.Id)
			if err != nil {
				return err.Error()
			}
			return credUrl
		}, "3m", "2s").Should(Equal(fmt.Sprintf("https://%s:3333", agentIP)))

		zap.S().Info("Setup complete for test.")
	})

	AfterEach(func() {
		zap.S().Info("Cleaning up after test...")
		err = svc.RemoveSources()
		Expect(err).To(BeNil(), "Failed to remove sources from DB")
		err = agent.Remove()
		Expect(err).To(BeNil(), "Failed to remove vm and iso")
		testDuration := time.Since(startTime)
		zap.S().Infof("Test completed in: %s\n", testDuration.String())
		TestsExecutionTime[CurrentSpecReport().LeafNodeText] = testDuration
	})

	AfterFailed(func() {
		agent.DumpLogs(agentIP)
	})

	Context("Flow", func() {
		It("Downloads OVA file from URL", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			res, err := agent.AgentApi().Login(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
				"core", "123456")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))
			zap.S().Info("Vcenter login completed successfully. Credentials saved.")

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				isAgentIsUpToDate, err := AgentIsUpToDate(svc, source.Id)
				Expect(err).To(BeNil())
				return isAgentIsUpToDate
			}, "3m", "2s").Should(BeTrue())

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})
})
