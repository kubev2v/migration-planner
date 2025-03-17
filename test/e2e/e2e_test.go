package e2e_test

import (
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"os"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("e2e", func() {

	var (
		svc      PlannerService
		agent    PlannerAgent
		agentIP  string
		err      error
		systemIP = os.Getenv("PLANNER_IP")
		source   *v1alpha1.Source
	)

	createSource := func(name string) *v1alpha1.Source {
		source, err := svc.CreateSource(name)
		Expect(err).To(BeNil())
		Expect(source).NotTo(BeNil())
		return source
	}

	createAgent := func(configPath string, idForTest string, uuid uuid.UUID, vmName string) PlannerAgent {
		agent, err := NewPlannerAgent(configPath, uuid, vmName, idForTest)
		Expect(err).To(BeNil(), "Failed to create PlannerAgent")
		err = agent.Run()
		Expect(err).To(BeNil(), "Failed to run PlannerAgent")
		Eventually(func() string {
			agentIP, err = agent.GetIp()
			if err != nil {
				return ""
			}
			return agentIP
		}, "3m").ShouldNot(BeEmpty())
		Expect(agentIP).ToNot(BeEmpty())
		Eventually(func() bool {
			return agent.IsServiceRunning(agentIP, "planner-agent")
		}, "3m").Should(BeTrue())
		return agent
	}

	loginToVsphere := func(username string, password string, expectedStatusCode int) {
		res, err := agent.Login(fmt.Sprintf("https://%s:8989/sdk", systemIP), username, password)
		Expect(err).To(BeNil())
		Expect(res.StatusCode).To(Equal(expectedStatusCode))
	}

	WaitForAgentToBeUpToDate := func(uuid uuid.UUID) {
		Eventually(func() bool {
			source, err := svc.GetSource(uuid)
			if err != nil {
				return false
			}
			return source.Agent.Status == v1alpha1.AgentStatusUpToDate
		}, "3m").Should(BeTrue())
	}

	waitForValidCredentialURL := func(uuid uuid.UUID, agentIP string) {
		Eventually(func() string {
			s, err := svc.GetSource(uuid)
			if err != nil {
				return ""
			}
			if s.Agent == nil {
				return ""
			}
			if s.Agent.CredentialUrl != "N/A" && s.Agent.CredentialUrl != "" {
				return s.Agent.CredentialUrl
			}

			return ""
		}, "3m").Should(Equal(fmt.Sprintf("https://%s:3333", agentIP)))
	}

	BeforeEach(func() {
		svc, err = NewPlannerService(defaultConfigPath)
		Expect(err).To(BeNil(), "Failed to create PlannerService")

		source = createSource("source")

		agent = createAgent(defaultConfigPath, defaultAgentTestID, source.Id, vmName)

		waitForValidCredentialURL(source.Id, agentIP)

		Expect(agent.IsServiceRunning(agentIP, "planner-agent")).To(BeTrue())
	})

	AfterEach(func() {
		_ = svc.RemoveSources()
		_ = agent.Remove()
	})

	AfterFailed(func() {
		agent.DumpLogs(agentIP)
	})

	Context("Check Vcenter login behavior", func() {
		It("should successfully login with valid credentials", func() {
			loginToVsphere("core", "123456", http.StatusNoContent)
		})

		It("Two test combined: should return BadRequest due to an empty username"+
			" and BadRequest due to an empty password", func() {
			loginToVsphere("", "pass", http.StatusBadRequest)
			loginToVsphere("user", "", http.StatusBadRequest)
		})

		It("should return Unauthorized due to invalid credentials", func() {
			loginToVsphere("invalid", "cred", http.StatusUnauthorized)
		})

		It("should return badRequest due to an invalid URL", func() {
			res, err := agent.Login(fmt.Sprintf("https://%s", systemIP), "user", "pass") // bad link to Vcenter environment
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusBadRequest))
		})

	})

	Context("Flow", func() {
		It("Up to date", func() {
			// Put the vCenter credentials and check that source is up to date eventually
			loginToVsphere("core", "123456", http.StatusNoContent)

			Eventually(func() bool {
				source, err := svc.GetSource(source.Id)
				if err != nil {
					return false
				}
				return source.Agent.Status == v1alpha1.AgentStatusUpToDate
			}, "1m", "2s").Should(BeTrue())
		})

		It("Source removal", func() {
			loginToVsphere("core", "123456", http.StatusNoContent)

			WaitForAgentToBeUpToDate(source.Id)

			err = svc.RemoveSource(source.Id)
			Expect(err).To(BeNil())

			_, err = svc.GetSource(source.Id)
			Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("code: %d", http.StatusNotFound))))
		})

		It("Two agents, Two VSphere's", func() {

			loginToVsphere("core", "123456", http.StatusNoContent)
			WaitForAgentToBeUpToDate(source.Id)

			source2 := createSource("source-2")

			agent2 := createAgent(defaultConfigPath, "2", source2.Id, vmName+"-2")

			waitForValidCredentialURL(source2.Id, agentIP)

			Expect(agent2.IsServiceRunning(agentIP, "planner-agent")).To(BeTrue())

			// Login to Vcsim2
			res, err := agent2.Login(fmt.Sprintf("https://%s:8990/sdk", systemIP), "core", "123456")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))

			WaitForAgentToBeUpToDate(source2.Id)

			_ = agent2.Remove()
		})
	})

	Context("Edge cases", func() {
		It("VM reboot", func() {
			loginToVsphere("core", "123456", http.StatusNoContent)

			// Restarting the VM
			err = agent.Restart()
			Expect(err).To(BeNil())

			// Check that planner-agent service is running
			Eventually(func() bool {
				return agent.IsServiceRunning(agentIP, "planner-agent")
			}, "3m").Should(BeTrue())

			WaitForAgentToBeUpToDate(source.Id)
		})
	})
})
