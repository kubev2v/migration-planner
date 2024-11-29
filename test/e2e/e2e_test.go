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
		agentID  string
		err      error
		systemIP = os.Getenv("PLANNER_IP")
	)

	BeforeEach(func() {
		svc, err = NewPlannerService(defaultConfigPath)
		Expect(err).To(BeNil())
		agent, err = NewPlannerAgent(defaultConfigPath, vmName)
		Expect(err).To(BeNil())
		err = agent.Run()
		Expect(err).To(BeNil())
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
			s, err := svc.GetAgent(agentIP)
			if err != nil || s == nil {
				return ""
			}
			agentID = s.Id
			return agentID
		}, "3m", "2s").ShouldNot(BeEmpty())
	})

	AfterEach(func() {
		_ = svc.RemoveSources()
		_ = agent.Remove()
	})

	AfterFailed(func() {
		agent.DumpLogs(agentIP)
	})

	Context("Flow", func() {
		It("Up to date", func() {
			// Check that planner-agent service is running
			r := agent.IsServiceRunning(agentIP, "planner-agent")
			Expect(r).To(BeTrue())

			// Put the vCenter credentials and check that source is up to date eventually
			res, err := agent.Login(fmt.Sprintf("https://%s:8989/sdk", systemIP), "user", "pass")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))
			Eventually(func() bool {
				apiAgent, err := svc.GetAgent(agentIP)
				if err != nil {
					return false
				}
				return apiAgent.Status == v1alpha1.AgentStatusUpToDate
			}, "1m", "2s").Should(BeTrue())
			s, err := svc.GetSource()
			Expect(err).To(BeNil())
			Expect(s).ToNot(BeNil())
			Expect(s.Inventory).ToNot(BeNil())
			Expect(s.Inventory.Vcenter.Id).To(Equal(s.Id.String()))
		})
		It("version endpoint is not empty", func() {
			version, err := agent.Version()
			Expect(err).To(BeNil())
			Expect(version).ToNot(BeEmpty())
		})
		It("Return 422 in case of wrong URL", func() {
			// Put the vCenter credentials with wrong URL and check it return HTTP 422 error code
			res, err := agent.Login("this is not URL", "user", "pass")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusUnprocessableEntity))
		})
	})
})
