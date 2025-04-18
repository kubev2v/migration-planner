package e2e_test

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_agent"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_helpers"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_service"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

var _ = Describe("e2e-disconnected-environment", func() {

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
		TestOptions.DisconnectedEnvironment = true

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

		zap.S().Info("Wait for agent server to start...")
		Eventually(func() bool {
			if _, err := agent.AgentApi().Status(); err != nil {
				return false
			}
			return true
		}, "3m", "2s").Should(BeTrue())

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
		It("Disconnected-environment", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Adding vcenter.com to /etc/hosts to enable connectivity to the vSphere server.
			_, err := RunSSHCommand(agentIP, fmt.Sprintf("podman exec "+
				"--user root "+
				"planner-agent "+
				"bash -c 'echo \"%s vcenter.com\" >> /etc/hosts'", SystemIP))
			Expect(err).To(BeNil(), "Failed to enable connection to Vsphere")

			// Login to Vcenter
			Eventually(func() bool {
				res, err := agent.AgentApi().Login(fmt.Sprintf("https://%s:%s/sdk", "vcenter.com", Vsphere1Port),
					"core", "123456")
				return err == nil && res.StatusCode == http.StatusNoContent
			}, "3m", "2s").Should(BeTrue())
			zap.S().Info("Vcenter login completed successfully. Credentials saved.")

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				statusReply, err := agent.AgentApi().Status()
				if err != nil {
					return false
				}
				Expect(statusReply.Connected).Should(Equal("false"))
				return statusReply.Connected == "false" && statusReply.Status == string(v1alpha1.AgentStatusUpToDate)
			}, "3m", "2s").Should(BeTrue())

			// Get inventory
			inventory, err := agent.AgentApi().Inventory()
			Expect(err).To(BeNil())

			// Manually upload the collected inventory data
			err = svc.UpdateSource(source.Id, inventory)
			Expect(err).To(BeNil())

			// Verify that the inventory upload was successful
			source, err = svc.GetSource(source.Id)
			Expect(err).To(BeNil())
			Expect(source.Agent).To(Not(BeNil()))
			Expect(source.Agent.Status).Should(Equal(v1alpha1.AgentStatusNotConnected))
			Expect(source.Agent.CredentialUrl).Should(BeEmpty())
			Expect(source.Inventory).To(Equal(inventory))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})
})
