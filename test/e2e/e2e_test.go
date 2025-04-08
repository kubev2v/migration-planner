package e2e_test

import (
	"fmt"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"os"
)

const (
	Vsphere1Port string = "8989"
	Vsphere2Port string = "8990"
)

var (
	svc      PlannerService
	agent    PlannerAgent
	agentApi PlannerAgentAPI
	agentIP  string
	err      error
	systemIP = os.Getenv("PLANNER_IP")
	source   *v1alpha1.Source
)

var testOptions = struct {
	downloadImageByUrl      bool
	disconnectedEnvironment bool
}{}

var _ = Describe("e2e", func() {

	BeforeEach(func() {
		testOptions.downloadImageByUrl = false
		testOptions.disconnectedEnvironment = false

		svc, err = NewPlannerService(defaultConfigPath)
		Expect(err).To(BeNil(), "Failed to create PlannerService")

		source = CreateSource("source")

		agent, agentIP = CreateAgent(defaultConfigPath, defaultAgentTestID, source.Id, vmName)

		agentApi, err = agent.AgentApi()
		Expect(err).To(BeNil(), "Failed to create agent localApi")

		WaitForValidCredentialURL(source.Id, agentIP)
	})

	AfterEach(func() {
		err = svc.RemoveSources()
		Expect(err).To(BeNil(), "Failed to remove sources from DB")
		err = agent.Remove()
		Expect(err).To(BeNil(), "Failed to remove vm and iso")
	})

	AfterFailed(func() {
		agent.DumpLogs(agentIP)
	})

	Context("Check Vcenter login behavior", func() {
		It("should successfully login with valid credentials", func() {
			LoginToVsphere(agentApi, systemIP, Vsphere1Port, "core", "123456", http.StatusNoContent)
		})

		It("Two test combined: should return BadRequest due to an empty username"+
			" and BadRequest due to an empty password", func() {
			LoginToVsphere(agentApi, systemIP, Vsphere1Port, "", "pass", http.StatusBadRequest)
			LoginToVsphere(agentApi, systemIP, Vsphere1Port, "user", "", http.StatusBadRequest)
		})

		It("should return Unauthorized due to invalid credentials", func() {
			LoginToVsphere(agentApi, systemIP, Vsphere1Port, "invalid", "cred", http.StatusUnauthorized)
		})

		It("should return badRequest due to an invalid URL", func() {
			LoginToVsphere(agentApi, systemIP, "", "user", "pass", http.StatusBadRequest)
		})

	})

	Context("Flow", func() {
		It("Up to date", func() {
			LoginToVsphere(agentApi, systemIP, Vsphere1Port, "core", "123456", http.StatusNoContent)

			WaitForAgentToBeUpToDate(source.Id)
		})

		It("Source removal", func() {
			LoginToVsphere(agentApi, systemIP, Vsphere1Port, "core", "123456", http.StatusNoContent)

			WaitForAgentToBeUpToDate(source.Id)

			err = svc.RemoveSource(source.Id)
			Expect(err).To(BeNil())

			_, err = svc.GetSource(source.Id)
			Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("code: %d", http.StatusNotFound))))
		})

		It("Two agents, Two VSphere's", func() {

			LoginToVsphere(agentApi, systemIP, Vsphere1Port, "core", "123456", http.StatusNoContent)
			WaitForAgentToBeUpToDate(source.Id)

			source2 := CreateSource("source-2")

			agent2, agentIP2 := CreateAgent(defaultConfigPath, "2", source2.Id, vmName+"-2")

			agent2Api, err := agent2.AgentApi()
			Expect(err).To(BeNil())

			WaitForValidCredentialURL(source2.Id, agentIP2)

			// Login to Vcsim2
			LoginToVsphere(agent2Api, systemIP, Vsphere2Port, "core", "123456", http.StatusNoContent)

			WaitForAgentToBeUpToDate(source2.Id)

			err = agent2.Remove()
			Expect(err).To(BeNil())
		})
	})

	Context("Edge cases", func() {
		It("VM reboot", func() {
			LoginToVsphere(agentApi, systemIP, Vsphere1Port, "core", "123456", http.StatusNoContent)

			// Restarting the VM
			err = agent.Restart()
			Expect(err).To(BeNil())

			// Check that planner-agent service is running
			Eventually(func() bool {
				return agent.IsServiceRunning(agentIP, "planner-agent")
			}, "6m", "2s").Should(BeTrue())

			WaitForAgentToBeUpToDate(source.Id)
		})
	})
})

var _ = Describe("e2e-download-ova-from-url", func() {

	BeforeEach(func() {
		testOptions.downloadImageByUrl = true
		testOptions.disconnectedEnvironment = false

		svc, err = NewPlannerService(defaultConfigPath)
		Expect(err).To(BeNil(), "Failed to create PlannerService")

		source = CreateSource("source")

		agent, agentIP = CreateAgent(defaultConfigPath, defaultAgentTestID, source.Id, vmName)

		agentApi, err = agent.AgentApi()
		Expect(err).To(BeNil(), "Failed to create agent localApi")

		WaitForValidCredentialURL(source.Id, agentIP)
	})

	AfterEach(func() {
		err = svc.RemoveSources()
		Expect(err).To(BeNil(), "Failed to remove sources from DB")
		err = agent.Remove()
		Expect(err).To(BeNil(), "Failed to remove vm and iso")
	})

	AfterFailed(func() {
		agent.DumpLogs(agentIP)
	})

	Context("Flow", func() {
		It("Downloads OVA file from URL", func() {
			LoginToVsphere(agentApi, systemIP, Vsphere1Port, "core", "123456", http.StatusNoContent)

			WaitForAgentToBeUpToDate(source.Id)
		})
	})
})
var _ = Describe("e2e-disconnected-environment", func() {

	BeforeEach(func() {
		testOptions.downloadImageByUrl = false
		testOptions.disconnectedEnvironment = true

		svc, err = NewPlannerService(defaultConfigPath)
		Expect(err).To(BeNil(), "Failed to create PlannerService")

		source = CreateSource("source")

		agent, agentIP = CreateAgent(defaultConfigPath, defaultAgentTestID, source.Id, vmName)

		agentApi, err = agent.AgentApi()
		Expect(err).To(BeNil(), "Failed to create agent localApi")

		Eventually(func() bool {
			if _, err := agentApi.Status(); err != nil {
				return false
			}
			return true
		}, "5m", "2s").Should(BeTrue())
	})

	AfterEach(func() {
		err = svc.RemoveSources()
		Expect(err).To(BeNil(), "Failed to remove sources from DB")
		err = agent.Remove()
		Expect(err).To(BeNil(), "Failed to remove vm and iso")
	})

	AfterFailed(func() {
		agent.DumpLogs(agentIP)
	})

	Context("Flow", func() {
		It("disconnected-environment", func() {

			// Adding vcenter.com to /etc/hosts to enable connectivity to the vSphere server.
			_, err := RunSSHCommand(agentIP, fmt.Sprintf("podman exec "+
				"--user root "+
				"planner-agent "+
				"bash -c 'echo \"%s vcenter.com\" >> /etc/hosts'", systemIP))
			Expect(err).To(BeNil(), "Failed to enable connection to Vsphere")

			// Login to Vcenter
			Eventually(func() bool {
				res, err := agentApi.Login(fmt.Sprintf("https://%s:%s/sdk", "vcenter.com", Vsphere1Port), "core", "123456")
				return err == nil && res.StatusCode == http.StatusNoContent
			}, "3m", "2s").Should(BeTrue())

			// Wait for the inventory collection process to complete
			Eventually(func() bool {
				statusReply, err := agentApi.Status()
				if err != nil {
					return false
				}
				Expect(statusReply.Connected).Should(Equal("false"))
				return statusReply.Connected == "false" && statusReply.Status == string(v1alpha1.AgentStatusUpToDate)
			}, "5m", "2s").Should(BeTrue())

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
		})
	})
})
