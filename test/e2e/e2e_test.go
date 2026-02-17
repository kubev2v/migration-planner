package e2e_test

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/agent"
	. "github.com/kubev2v/migration-planner/test/e2e/helpers"
	. "github.com/kubev2v/migration-planner/test/e2e/model"
	. "github.com/kubev2v/migration-planner/test/e2e/service"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

var _ = Describe("e2e", func() {
	var (
		svc       PlannerService
		e2eAgent  E2EAgent
		agentIP   string
		err       error
		source    *v1alpha1.Source
		startTime time.Time
	)

	BeforeEach(func() {
		startTime = time.Now()

		svc, err = DefaultPlannerService()
		Expect(err).To(BeNil(), "Failed to create PlannerService")

		source, err = svc.CreateSource("source")
		Expect(err).To(BeNil())
		Expect(source).NotTo(BeNil())

		e2eAgent.Agent, err = CreateAgent(source.Id, GenerateVmName(), svc)
		Expect(err).To(BeNil())

		zap.S().Info("Waiting for agent IP...")
		Eventually(func() error {
			agentIP, err = e2eAgent.Agent.GetIp()
			if err != nil {
				return err
			}
			return nil
		}, "3m", "2s").Should(BeNil())
		zap.S().Infof("agent ip is: %s", agentIP)

		agentApiBaseUrl := fmt.Sprintf("https://%s:3333/api/v1/", agentIP)
		e2eAgent.Api = DefaultAgentApi(agentApiBaseUrl)
		zap.S().Infof("agent Api base url: %s", agentApiBaseUrl)

		zap.S().Info("Wait for planner-agent to be running...")
		var s *AgentStatus
		Eventually(func() error {
			s, err = e2eAgent.Api.Status()
			if err != nil {
				return err
			}
			return nil
		}, "3m", "2s").Should(BeNil())
		zap.S().Info("Planner-agent is now running")

		s, err = e2eAgent.Api.SetAgentMode(string(AgentModeConnected))
		Expect(err).To(BeNil())

		Eventually(func() string {
			s, err = e2eAgent.Api.Status()
			if err != nil {
				return ""
			}
			return s.ConsoleConnection
		}, "3m", "2s").Should(Equal(string(AgentModeConnected)))

		Eventually(func() v1alpha1.AgentStatus {
			s, err := svc.GetSource(source.Id)
			if err != nil {
				return ""
			}
			if s.Agent == nil {
				return ""
			}
			return s.Agent.Status
		}, "3m", "2s").
			Should(Equal(v1alpha1.AgentStatusWaitingForCredentials))

		zap.S().Info("Setup complete for test.")
	})

	AfterEach(func() {
		zap.S().Info("Cleaning up after test...")
		err = svc.RemoveSources()
		Expect(err).To(BeNil(), "Failed to remove sources from DB")
		err = e2eAgent.Agent.Remove()
		Expect(err).To(BeNil(), "Failed to remove vm and iso")
		testDuration := time.Since(startTime)
		zap.S().Infof("Test completed in: %s\n", testDuration.String())
		TestsExecutionTime[CurrentSpecReport().LeafNodeText] = testDuration
	})

	AfterFailed(func() {
		e2eAgent.Agent.DumpLogs(agentIP)
	})

	Context("collect from vsphere", func() {
		It("start collecting only when credentials are valid", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			_, resCode, err := e2eAgent.Api.StartCollector(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
				"", "pass")
			Expect(err).To(BeNil())
			Expect(resCode).To(Equal(http.StatusBadRequest))

			_, resCode, err = e2eAgent.Api.StartCollector(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
				"user", "")
			Expect(err).To(BeNil())
			Expect(resCode).To(Equal(http.StatusBadRequest))

			_, resCode, err = e2eAgent.Api.StartCollector(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
				"invalid", "cred")
			Expect(err).To(BeNil())
			Expect(resCode).To(Equal(http.StatusAccepted))

			var s *CollectorStatus
			Eventually(func() string {
				s, err = e2eAgent.Api.GetCollectorStatus()
				if err != nil {
					return ""
				}
				return s.Status
			}, "30s", "2s").Should(Equal(string(CollectorStatusError)))

			s, resCode, err = e2eAgent.Api.StartCollector(fmt.Sprintf("https://%s:%s/badUrl", SystemIP, Vsphere1Port),
				"user", "pass")
			Expect(err).To(BeNil())
			Expect(resCode).To(Equal(http.StatusAccepted))
			Eventually(func() string {
				s, err = e2eAgent.Api.GetCollectorStatus()
				if err != nil {
					return ""
				}
				return s.Status
			}, "30s", "2s").Should(Equal(string(CollectorStatusError)))

			s, resCode, err = e2eAgent.Api.StartCollector(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port), "core", "123456")
			Expect(err).To(BeNil())
			Expect(resCode).To(Equal(http.StatusAccepted))
			Expect(s.Status).ToNot(Equal(CollectorStatusError))

			Eventually(func() string {
				s, err = e2eAgent.Api.GetCollectorStatus()
				if err != nil {
					return ""
				}
				if s.Status == string(CollectorStatusError) {
					zap.S().Infof("Collector status is error: %s", s.Error)
				}
				return s.Status
			}, "30s", "2s").Should(Equal(string(CollectorStatusCollected)))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("Flow", func() {
		It("Up to date", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			s, resCode, err := e2eAgent.Api.StartCollector(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port), "core", "123456")
			Expect(err).To(BeNil())
			Expect(resCode).To(Equal(http.StatusAccepted))
			Expect(s.Status).ToNot(Equal(CollectorStatusError))

			Eventually(func() string {
				s, err = e2eAgent.Api.GetCollectorStatus()
				if err != nil {
					return ""
				}
				if s.Status == string(CollectorStatusError) {
					zap.S().Infof("Collector status is error: %s", s.Error)
				}
				return s.Status
			}, "30s", "2s").Should(Equal(string(CollectorStatusCollected)))

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				isAgentIsUpToDate, err := AgentIsUpToDate(svc, source.Id)
				Expect(err).To(BeNil())
				return isAgentIsUpToDate
			}, "3m", "2s").Should(BeTrue())

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("Source removal", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			s, resCode, err := e2eAgent.Api.StartCollector(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
				"core", "123456")
			Expect(err).To(BeNil())
			Expect(resCode).To(Equal(http.StatusAccepted))
			Expect(s.Status).ToNot(Equal(CollectorStatusError))

			Eventually(func() string {
				s, err = e2eAgent.Api.GetCollectorStatus()
				if err != nil {
					return ""
				}
				if s.Status == string(CollectorStatusError) {
					zap.S().Infof("Collector status is error: %s", s.Error)
				}
				return s.Status
			}, "30s", "2s").Should(Equal(string(CollectorStatusCollected)))
			zap.S().Info("Collector completed successfully. Status collected.")

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				isAgentIsUpToDate, err := AgentIsUpToDate(svc, source.Id)
				Expect(err).To(BeNil())
				return isAgentIsUpToDate
			}, "3m", "2s").Should(BeTrue())

			err = svc.RemoveSource(source.Id)
			Expect(err).To(BeNil())

			_, err = svc.GetSource(source.Id)
			Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("code: %d", http.StatusNotFound))))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("Two agents, Two VSphere's", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			s, resCode, err := e2eAgent.Api.StartCollector(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
				"core", "123456")
			Expect(err).To(BeNil())
			Expect(resCode).To(Equal(http.StatusAccepted))
			Expect(s.Status).ToNot(Equal(CollectorStatusError))

			Eventually(func() string {
				s, err = e2eAgent.Api.GetCollectorStatus()
				if err != nil {
					return ""
				}
				if s.Status == string(CollectorStatusError) {
					zap.S().Infof("Collector status is error: %s", s.Error)
				}
				return s.Status
			}, "30s", "2s").Should(Equal(string(CollectorStatusCollected)))
			zap.S().Info("Collector completed successfully. Status collected.")

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				isAgentIsUpToDate, err := AgentIsUpToDate(svc, source.Id)
				Expect(err).To(BeNil())
				return isAgentIsUpToDate
			}, "3m", "2s").Should(BeTrue())

			source2, err := svc.CreateSource("source-2")
			Expect(err).To(BeNil())
			Expect(source2).NotTo(BeNil())

			var agent2 E2EAgent
			agent2.Agent, err = CreateAgent(source2.Id, GenerateVmName(), svc)
			Expect(err).To(BeNil())

			var agentIP2 string
			Eventually(func() error {
				agentIP2, err = agent2.Agent.GetIp()
				if err != nil {
					return err
				}
				return nil
			}, "3m", "2s").Should(BeNil())

			agent2ApiBaseUrl := fmt.Sprintf("https://%s:3333/api/v1/", agentIP2)
			agent2.Api = DefaultAgentApi(agent2ApiBaseUrl)
			zap.S().Infof("Agent2 Api base url: %s", agent2ApiBaseUrl)

			var agent2Status *AgentStatus
			Eventually(func() error {
				agent2Status, err = agent2.Api.Status()
				if err != nil {
					return err
				}
				return nil
			}, "3m", "2s").Should(BeNil())
			zap.S().Info("agent2 is now running")

			_, err = agent2.Api.SetAgentMode(string(AgentModeConnected))
			Expect(err).To(BeNil())

			Eventually(func() string {
				agent2Status, err = agent2.Api.Status()
				if err != nil {
					return ""
				}
				return agent2Status.ConsoleConnection
			}, "1m", "2s").Should(Equal(string(AgentModeConnected)))

			Eventually(func() v1alpha1.AgentStatus {
				s, err := svc.GetSource(source2.Id)
				if err != nil {
					return ""
				}
				if s.Agent == nil {
					return ""
				}
				return s.Agent.Status
			}, "1m", "2s").
				Should(Equal(v1alpha1.AgentStatusWaitingForCredentials))

			// Start collector for Vcsim2
			s, resCode, err = agent2.Api.StartCollector(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere2Port),
				"core", "123456")
			Expect(err).To(BeNil())
			Expect(resCode).To(Equal(http.StatusAccepted))
			Expect(s.Status).ToNot(Equal(CollectorStatusError))

			Eventually(func() string {
				s, err = e2eAgent.Api.GetCollectorStatus()
				if err != nil {
					return ""
				}
				if s.Status == string(CollectorStatusError) {
					zap.S().Infof("Collector status is error: %s", s.Error)
				}
				return s.Status
			}, "30s", "2s").Should(Equal(string(CollectorStatusCollected)))
			zap.S().Info("Collector completed successfully. Status collected.")

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				isAgentIsUpToDate, err := AgentIsUpToDate(svc, source2.Id)
				Expect(err).To(BeNil())
				return isAgentIsUpToDate
			}, "3m", "2s").Should(BeTrue())

			err = agent2.Agent.Remove()
			Expect(err).To(BeNil())

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("Edge cases", func() {
		It("VM reboot", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			s, resCode, err := e2eAgent.Api.StartCollector(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
				"core", "123456")
			Expect(err).To(BeNil())
			Expect(resCode).To(Equal(http.StatusAccepted))
			Expect(s.Status).ToNot(Equal(CollectorStatusError))

			Eventually(func() string {
				s, err = e2eAgent.Api.GetCollectorStatus()
				if err != nil {
					return ""
				}
				if s.Status == string(CollectorStatusError) {
					zap.S().Infof("Collector status is error: %s", s.Error)
				}
				return s.Status
			}, "30s", "2s").Should(Equal(string(CollectorStatusCollected)))
			zap.S().Info("Collector completed successfully. Status collected.")

			// Dump data directory before reboot
			zap.S().Info("Data directory BEFORE reboot:")
			err = e2eAgent.Agent.DumpDataDir()
			Expect(err).To(BeNil())

			// Restarting the VM
			err = e2eAgent.Agent.Restart()
			Expect(err).To(BeNil())

			// wait for the agent to be up again
			zap.S().Info("Waiting for agent IP after reboot...")
			Eventually(func() error {
				agentIP, err = e2eAgent.Agent.GetIp()
				return err
			}, "3m", "2s").Should(BeNil())
			zap.S().Infof("Agent IP after reboot: %s", agentIP)

			// wait for the agent API to be up
			var agentStatus *AgentStatus
			agentApiBaseUrl := fmt.Sprintf("https://%s:3333/api/v1/", agentIP)
			e2eAgent.Api = DefaultAgentApi(agentApiBaseUrl)

			zap.S().Info("Wait for planner-agent to be running...")
			Eventually(func() error {
				_, err := e2eAgent.Api.Status()
				if err != nil {
					return err
				}
				return nil
			}, "3m", "2s").Should(BeNil())
			zap.S().Info("Planner-agent is now running")

			// Dump data directory after reboot
			zap.S().Info("Data directory AFTER reboot:")
			err = e2eAgent.Agent.DumpDataDir()
			Expect(err).To(BeNil())

			Eventually(func() string {
				agentStatus, err = e2eAgent.Api.Status()
				if err != nil {
					return ""
				}
				return agentStatus.ConsoleConnection
			}, "3m", "2s").Should(Equal(string(AgentModeConnected)))

			// Restart VM should keep the inventory
			s, err = e2eAgent.Api.GetCollectorStatus()
			Expect(err).To(BeNil())
			Expect(s.Status).To(Equal(string(CollectorStatusCollected)))

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				isAgentIsUpToDate, err := AgentIsUpToDate(svc, source.Id)
				if err != nil {
					return false
				}
				return isAgentIsUpToDate
			}, "3m", "2s").Should(BeTrue())

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("Assessments", func() {
		It("Test Assessment Endpoints With inventory", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			s, resCode, err := e2eAgent.Api.StartCollector(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
				"core", "123456")
			Expect(err).To(BeNil())
			Expect(resCode).To(Equal(http.StatusAccepted))
			Expect(s.Status).ToNot(Equal(CollectorStatusError))

			Eventually(func() string {
				s, err = e2eAgent.Api.GetCollectorStatus()
				if err != nil {
					return ""
				}
				if s.Status == string(CollectorStatusError) {
					zap.S().Infof("Collector status is error: %s", s.Error)
				}
				return s.Status
			}, "30s", "2s").Should(Equal(string(CollectorStatusCollected)))
			zap.S().Info("Collector completed successfully. Status collected.")

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				isAgentIsUpToDate, err := AgentIsUpToDate(svc, source.Id)
				Expect(err).To(BeNil())
				return isAgentIsUpToDate
			}, "3m", "2s").Should(BeTrue())

			inventory, err := e2eAgent.Api.Inventory()
			Expect(err).To(BeNil())

			// Create an assessment from an environment (source)
			assessment, err := svc.CreateAssessment("assessment", "agent", &source.Id, inventory)
			Expect(err).To(BeNil())
			Expect(assessment).NotTo(BeNil())

			assessment, err = svc.GetAssessment(assessment.Id)
			Expect(err).To(BeNil())
			Expect(assessment.Name).To(Equal("assessment"))

			assessment, err = svc.UpdateAssessment(assessment.Id, "assessment1")
			Expect(err).To(BeNil())
			Expect(assessment.Name).To(Equal("assessment1"))

			assessments, err := svc.GetAssessments()
			Expect(err).To(BeNil())
			Expect(*assessments).To(HaveLen(1))

			err = svc.RemoveAssessment(assessment.Id)
			Expect(err).To(BeNil())

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})
})
