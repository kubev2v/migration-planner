package e2e_test

import (
	"fmt"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"net/http"
	"time"
)

var _ = Describe("e2e-disconnected-environment", func() {

	var (
		svc       PlannerService
		agent     PlannerAgent
		agentApi  PlannerAgentAPI
		agentIP   string
		err       error
		source    *v1alpha1.Source
		startTime time.Time
	)

	BeforeEach(func() {
		startTime = time.Now()
		testOptions.downloadImageByUrl = false
		testOptions.disconnectedEnvironment = true

		svc, err = NewPlannerService(defaultConfigPath)
		Expect(err).To(BeNil(), "Failed to create PlannerService")

		source, err = svc.CreateSource("source")
		Expect(err).To(BeNil())
		Expect(source).NotTo(BeNil())

		agent, err = CreateAgent(defaultConfigPath, defaultAgentTestID, source.Id, vmName)
		Expect(err).To(BeNil())

		zap.S().Info("Waiting for agent IP...")
		Eventually(func() error {
			return FindAgentIp(agent, &agentIP)
		}, "3m", "2s").Should(BeNil())
		zap.S().Infof("Agent ip is: %s", agentIP)

		zap.S().Info("Wait for planner-agent to be running...")
		Eventually(func() bool {
			return IsPlannerAgentRunning(agent, agentIP)
		}, "3m", "2s").Should(BeTrue())
		zap.S().Info("Planner-agent is now running")

		agentApi, err = agent.AgentApi()
		Expect(err).To(BeNil(), "Failed to create agent localApi")

		zap.S().Info("Wait for agent server to start...")
		Eventually(func() bool {
			if _, err := agentApi.Status(); err != nil {
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
		zap.S().Infof("Test completed in: %s\n", time.Since(startTime))
	})

	AfterFailed(func() {
		agent.DumpLogs(agentIP)
	})

	Context("Flow", func() {
		It("disconnected-environment", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Adding vcenter.com to /etc/hosts to enable connectivity to the vSphere server.
			_, err := RunSSHCommand(agentIP, fmt.Sprintf("podman exec "+
				"--user root "+
				"planner-agent "+
				"bash -c 'echo \"%s vcenter.com\" >> /etc/hosts'", systemIP))
			Expect(err).To(BeNil(), "Failed to enable connection to Vsphere")

			// Login to Vcenter
			Eventually(func() bool {
				res, err := agentApi.Login(fmt.Sprintf("https://%s:%s/sdk", "vcenter.com", Vsphere1Port),
					"core", "123456")
				return err == nil && res.StatusCode == http.StatusNoContent
			}, "3m", "2s").Should(BeTrue())
			zap.S().Info("Vcenter login completed successfully. Credentials saved.")

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				statusReply, err := agentApi.Status()
				if err != nil {
					return false
				}
				Expect(statusReply.Connected).Should(Equal("false"))
				return statusReply.Connected == "false" && statusReply.Status == string(v1alpha1.AgentStatusUpToDate)
			}, "3m", "2s").Should(BeTrue())

			// Get inventory
			inventory, err := agentApi.Inventory()
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
