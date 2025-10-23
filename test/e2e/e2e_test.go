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
		TestOptions.DisconnectedEnvironment = false

		svc, err = DefaultPlannerService()
		Expect(err).To(BeNil(), "Failed to create PlannerService")

		source, err = svc.CreateSource("source")
		Expect(err).To(BeNil())
		Expect(source).NotTo(BeNil())

		e2eAgent.Agent, err = CreateAgent(DefaultAgentTestID, source.Id, VmName, svc)
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
		Eventually(func() bool {
			return e2eAgent.Agent.IsServiceRunning(agentIP, "planner-agent")
		}, "3m", "2s").Should(BeTrue())
		zap.S().Info("Planner-agent is now running")

		Eventually(func() string {
			credUrl, err := CredentialURL(svc, source.Id)
			if err != nil {
				return err.Error()
			}
			return credUrl
		}, "3m", "2s").
			Should(Equal(fmt.Sprintf("https://%s:3333", agentIP)))
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

	Context("Check Vcenter login behavior", func() {
		It("Succeeds login only for valid credentials", func() {

			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			res, err := e2eAgent.Api.Login(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
				"", "pass")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusBadRequest))
			zap.S().Info("Empty User. Successfully returned http status: BadRequest.")

			res, err = e2eAgent.Api.Login(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
				"user", "")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusBadRequest))
			zap.S().Info("Empty Password. Successfully returned http status: BadRequest.")

			res, err = e2eAgent.Api.Login(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
				"invalid", "cred")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusUnauthorized))
			zap.S().Info("Invalid credentials. HTTP status: Unauthorized returned.")

			res, err = e2eAgent.Api.Login(fmt.Sprintf("https://%s:%s/badUrl", SystemIP, Vsphere1Port),
				"user", "pass")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusBadRequest))
			zap.S().Info("Invalid URL. Successfully returned http status: BadRequest.")

			res, err = e2eAgent.Api.Login(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
				"core", "123456")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))
			zap.S().Info("Credentials verified successfully. HTTP status: NoContent (204) returned.")

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("Flow", func() {
		It("Up to date", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			res, err := e2eAgent.Api.Login(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
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

		It("Source removal", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			res, err := e2eAgent.Api.Login(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
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

			err = svc.RemoveSource(source.Id)
			Expect(err).To(BeNil())

			_, err = svc.GetSource(source.Id)
			Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("code: %d", http.StatusNotFound))))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("Two agents, Two VSphere's", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			res, err := e2eAgent.Api.Login(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
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

			source2, err := svc.CreateSource("source-2")
			Expect(err).To(BeNil())
			Expect(source2).NotTo(BeNil())

			var agent2 E2EAgent
			agent2.Agent, err = CreateAgent("2", source2.Id, VmName+"-2", svc)
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

			Eventually(func() bool {
				return agent2.Agent.IsServiceRunning(agentIP2, "planner-agent")
			}, "3m", "2s").Should(BeTrue())

			Eventually(func() string {
				credUrl, err := CredentialURL(svc, source2.Id)
				if err != nil {
					return err.Error()
				}
				return credUrl
			}, "3m", "2s").Should(Equal(fmt.Sprintf("https://%s:3333", agentIP2)))

			// Login to Vcsim2
			res, err = agent2.Api.Login(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere2Port),
				"core", "123456")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))
			zap.S().Info("Vcenter login completed successfully. Credentials saved.")

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

			res, err := e2eAgent.Api.Login(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
				"core", "123456")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))
			zap.S().Info("Vcenter login completed successfully. Credentials saved.")

			// Restarting the VM
			err = e2eAgent.Agent.Restart()
			Expect(err).To(BeNil())

			// Check that planner-agent service is running
			Eventually(func() bool {
				return e2eAgent.Agent.IsServiceRunning(agentIP, "planner-agent")
			}, "5m", "2s").Should(BeTrue())

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				isAgentIsUpToDate, err := AgentIsUpToDate(svc, source.Id)
				Expect(err).To(BeNil())
				return isAgentIsUpToDate
			}, "3m", "2s").Should(BeTrue())

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("Assessments", func() {
		It("Test Assessment Endpoints With inventory", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			res, err := e2eAgent.Api.Login(fmt.Sprintf("https://%s:%s/sdk", SystemIP, Vsphere1Port),
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
