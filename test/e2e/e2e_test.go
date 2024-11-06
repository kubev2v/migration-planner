package e2e_test

import (
	"fmt"
	"os"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("e2e", func() {

	var (
		svc      PlannerService
		agent    PlannerAgent
		sourceId string
		agentIP  string
		err      error
		systemIP = os.Getenv("PLANNER_IP")
	)

	BeforeEach(func() {
		svc, err = NewPlannerService(defaultConfigPath)
		Expect(err).To(BeNil())
		agent, err = NewPlannerAgent(defaultConfigPath, vmName)
		Expect(err).To(BeNil())
		sourceId, err = svc.Create("testsource")
		Expect(err).To(BeNil())
		Expect(sourceId).ToNot(BeNil())
		err = agent.Run(sourceId)
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
			s, err := svc.GetSource()
			if err != nil {
				return ""
			}
			if s.CredentialUrl != nil {
				return *s.CredentialUrl
			}

			return ""
		}, "3m", "2s").Should(Equal(fmt.Sprintf("http://%s:3333", agentIP)))
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
			err = agent.Login(fmt.Sprintf("https://%s:8989/sdk", systemIP), "user", "pass")
			Expect(err).To(BeNil())
			Eventually(func() bool {
				source, err := svc.GetSource()
				if err != nil {
					return false
				}
				return source.Status == v1alpha1.SourceStatusUpToDate
			}, "1m", "2s").Should(BeTrue())
		})
	})
})
