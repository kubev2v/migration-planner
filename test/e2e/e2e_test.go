package e2e_test

import (
	"fmt"
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
	)

	BeforeEach(func() {
		svc, err = NewPlannerService(defaultConfigPath)
		Expect(err).To(BeNil(), "Failed to create PlannerService")
		agent, err = NewPlannerAgent(defaultConfigPath, vmName)
		Expect(err).To(BeNil(), "Failed to create PlannerAgent")
		err = agent.Run()
		Expect(err).To(BeNil(), "Failed to run PlannerAgent")
		Eventually(func() string {
			agentIP, err = agent.GetIp()
			if err != nil {
				return ""
			}
			return agentIP
		}, "1m", "3s").ShouldNot(BeEmpty())
		Expect(agentIP).ToNot(BeEmpty())
		Eventually(func() bool {
			return agent.IsServiceRunning(agentIP, "planner-agent")
		}, "3m", "2s").Should(BeTrue())

		Eventually(func() string {
			s, err := svc.GetAgent(fmt.Sprintf("http://%s:3333", agentIP))
			if err != nil {
				return ""
			}
			if s.CredentialUrl != "N/A" && s.CredentialUrl != "" {
				return s.CredentialUrl
			}

			return ""
		}, "3m", "2s").Should(Equal(fmt.Sprintf("http://%s:3333", agentIP)))

		// Check that planner-agent service is running
		r := agent.IsServiceRunning(agentIP, "planner-agent")
		Expect(r).To(BeTrue())
	})

	AfterEach(func() {
		s, err := svc.GetAgent(fmt.Sprintf("http://%s:3333", agentIP))
		if err != nil {
			s = nil
		}
		_ = svc.RemoveAgent(s.Id) // remove the agent created within the 'BeforeEach' block from the DB

		_ = svc.RemoveSources()
		_ = agent.Remove()
	})

	AfterFailed(func() {
		agent.DumpLogs(agentIP)
	})

	Context("Check Vcenter login behavior", func() {

		It("should successfully login with valid credentials", func() {

			res, err := agent.Login(fmt.Sprintf("https://%s:8989/sdk", systemIP), "core", "123456")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))

		})

		It("Two test combined: should return BadRequest due to an empty username"+
			" and BadRequest due to an empty password", func() {

			res1, err1 := agent.Login(fmt.Sprintf("https://%s:8989/sdk", systemIP), "", "pass")
			Expect(err1).To(BeNil())
			Expect(res1.StatusCode).To(Equal(http.StatusBadRequest))

			res2, err2 := agent.Login(fmt.Sprintf("https://%s:8989/sdk", systemIP), "user", "")
			Expect(err2).To(BeNil())
			Expect(res2.StatusCode).To(Equal(http.StatusBadRequest))

		})

		It("should return Unauthorized due to invalid credentials", func() {

			res, err := agent.Login(fmt.Sprintf("https://%s:8989/sdk", systemIP), "invalid", "cred")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusUnauthorized))

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
			res, err := agent.Login(fmt.Sprintf("https://%s:8989/sdk", systemIP), "core", "123456")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))
			Eventually(func() bool {
				apiAgent, err := svc.GetAgent(fmt.Sprintf("http://%s:3333", agentIP))
				if err != nil {
					return false
				}
				return apiAgent.Status == v1alpha1.AgentStatusUpToDate
			}, "1m", "2s").Should(BeTrue())
		})
	})
})
