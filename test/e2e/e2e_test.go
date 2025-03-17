package e2e_test

import (
	"fmt"
	"net/http"
	"os"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	svc      PlannerService
	agent    PlannerAgent
	agentIP  string
	err      error
	systemIP = os.Getenv("PLANNER_IP")
	source   *v1alpha1.Source
)

var testOptions = struct {
	downloadImageByUrl bool
}{}

var _ = Describe("e2e", func() {

	BeforeEach(func() {
		testOptions.downloadImageByUrl = false

		svc, err = NewPlannerService(defaultConfigPath)
		Expect(err).To(BeNil(), "Failed to create PlannerService")

		source = CreateSource("source")

		agent, agentIP = CreateAgent(defaultConfigPath, defaultAgentTestID, source.Id, vmName)

		WaitForValidCredentialURL(source.Id, agentIP)

		Expect(agent.IsServiceRunning(agentIP, "planner-agent")).To(BeTrue())
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
			LoginToVsphere("core", "123456", http.StatusNoContent)
		})

		It("Two test combined: should return BadRequest due to an empty username"+
			" and BadRequest due to an empty password", func() {
			LoginToVsphere("", "pass", http.StatusBadRequest)
			LoginToVsphere("user", "", http.StatusBadRequest)
		})

		It("should return Unauthorized due to invalid credentials", func() {
			LoginToVsphere("invalid", "cred", http.StatusUnauthorized)
		})

		It("should return badRequest due to an invalid URL", func() {
			res, err := agent.Login(fmt.Sprintf("https://%s", systemIP), "user", "pass") // bad link to Vcenter environment
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusBadRequest))
		})

	})

	Context("Flow", func() {
		It("Up to date", func() {
			LoginToVsphere("core", "123456", http.StatusNoContent)

			WaitForAgentToBeUpToDate(source.Id)
		})

		It("Source removal", func() {
			LoginToVsphere("core", "123456", http.StatusNoContent)

			WaitForAgentToBeUpToDate(source.Id)

			err = svc.RemoveSource(source.Id)
			Expect(err).To(BeNil())

			_, err = svc.GetSource(source.Id)
			Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("code: %d", http.StatusNotFound))))
		})

		It("Two agents, Two VSphere's", func() {

			LoginToVsphere("core", "123456", http.StatusNoContent)
			WaitForAgentToBeUpToDate(source.Id)

			source2 := CreateSource("source-2")

			agent2, agentIP2 := CreateAgent(defaultConfigPath, "2", source2.Id, vmName+"-2")

			WaitForValidCredentialURL(source2.Id, agentIP2)

			Expect(agent2.IsServiceRunning(agentIP2, "planner-agent")).To(BeTrue())

			// Login to Vcsim2
			res, err := agent2.Login(fmt.Sprintf("https://%s:8990/sdk", systemIP), "core", "123456")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))

			WaitForAgentToBeUpToDate(source2.Id)

			err = agent2.Remove()
			Expect(err).To(BeNil())
		})
	})

	Context("Edge cases", func() {
		It("VM reboot", func() {
			LoginToVsphere("core", "123456", http.StatusNoContent)

			// Restarting the VM
			err = agent.Restart()
			Expect(err).To(BeNil())

			// Check that planner-agent service is running
			Eventually(func() bool {
				return agent.IsServiceRunning(agentIP, "planner-agent")
			}, "6m").Should(BeTrue())

			WaitForAgentToBeUpToDate(source.Id)
		})
	})
})

var _ = Describe("e2e-download-ova-from-url", func() {

	BeforeEach(func() {
		testOptions.downloadImageByUrl = true

		svc, err = NewPlannerService(defaultConfigPath)
		Expect(err).To(BeNil(), "Failed to create PlannerService")

		source = CreateSource("source")

		agent, agentIP = CreateAgent(defaultConfigPath, defaultAgentTestID, source.Id, vmName)

		WaitForValidCredentialURL(source.Id, agentIP)

		Expect(agent.IsServiceRunning(agentIP, "planner-agent")).To(BeTrue())
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
			LoginToVsphere("core", "123456", http.StatusNoContent)

			WaitForAgentToBeUpToDate(source.Id)
		})
	})
})
