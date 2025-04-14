package e2e_test

import (
	"fmt"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"net/http"
	"os"
	"time"
)

const (
	Vsphere1Port string = "8989"
	Vsphere2Port string = "8990"
)

var (
	systemIP           = os.Getenv("PLANNER_IP")
	testsExecutionTime = make(map[string]time.Duration)
)

var _ = BeforeSuite(func() {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.CallerKey = ""
	config.EncoderConfig.MessageKey = "msg"

	logger, _ := config.Build()
	if logger != nil {
		zap.ReplaceGlobals(logger)
	}
})

var _ = AfterSuite(func() {
	logExecutionSummary()
})

var _ = Describe("e2e", func() {
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
		testOptions.disconnectedEnvironment = false

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

		Eventually(func() string {
			return CredentialURL(svc, source.Id)
		}, "3m", "2s").
			Should(Equal(fmt.Sprintf("https://%s:3333", agentIP)))
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
		testsExecutionTime[CurrentSpecReport().LeafNodeText] = testDuration
	})

	AfterFailed(func() {
		agent.DumpLogs(agentIP)
	})

	Context("Check Vcenter login behavior", func() {
		It("Succeeds login only for valid credentials", func() {

			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			res, err := agentApi.Login(fmt.Sprintf("https://%s:%s/sdk", systemIP, Vsphere1Port),
				"", "pass")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusBadRequest))
			zap.S().Info("Empty User. Successfully returned http status: BadRequest.")

			res, err = agentApi.Login(fmt.Sprintf("https://%s:%s/sdk", systemIP, Vsphere1Port),
				"user", "")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusBadRequest))
			zap.S().Info("Empty Password. Successfully returned http status: BadRequest.")

			res, err = agentApi.Login(fmt.Sprintf("https://%s:%s/sdk", systemIP, Vsphere1Port),
				"invalid", "cred")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusUnauthorized))
			zap.S().Info("Invalid credentials. HTTP status: Unauthorized returned.")

			res, err = agentApi.Login(fmt.Sprintf("https://%s:%s/badUrl", systemIP, Vsphere1Port),
				"user", "pass")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusBadRequest))
			zap.S().Info("Invalid URL. Successfully returned http status: BadRequest.")

			res, err = agentApi.Login(fmt.Sprintf("https://%s:%s/sdk", systemIP, Vsphere1Port),
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

			res, err := agentApi.Login(fmt.Sprintf("https://%s:%s/sdk", systemIP, Vsphere1Port),
				"core", "123456")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))
			zap.S().Info("Vcenter login completed successfully. Credentials saved.")

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				return AgentIsUpToDate(svc, source.Id)
			}, "3m", "2s").Should(BeTrue())

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("Source removal", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			res, err := agentApi.Login(fmt.Sprintf("https://%s:%s/sdk", systemIP, Vsphere1Port),
				"core", "123456")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))
			zap.S().Info("Vcenter login completed successfully. Credentials saved.")

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				return AgentIsUpToDate(svc, source.Id)
			}, "3m", "2s").Should(BeTrue())

			err = svc.RemoveSource(source.Id)
			Expect(err).To(BeNil())

			_, err = svc.GetSource(source.Id)
			Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("code: %d", http.StatusNotFound))))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("Two agents, Two VSphere's", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			res, err := agentApi.Login(fmt.Sprintf("https://%s:%s/sdk", systemIP, Vsphere1Port),
				"core", "123456")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))
			zap.S().Info("Vcenter login completed successfully. Credentials saved.")

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				return AgentIsUpToDate(svc, source.Id)
			}, "3m", "2s").Should(BeTrue())

			source2, err := svc.CreateSource("source-2")
			Expect(err).To(BeNil())
			Expect(source2).NotTo(BeNil())

			agent2, err := CreateAgent(defaultConfigPath, "2", source2.Id, vmName+"-2")
			Expect(err).To(BeNil())

			var agentIP2 string
			Eventually(func() error {
				return FindAgentIp(agent2, &agentIP2)
			}, "3m", "2s").Should(BeNil())

			Eventually(func() bool {
				return IsPlannerAgentRunning(agent2, agentIP2)
			}, "3m", "2s").Should(BeTrue())

			agent2Api, err := agent2.AgentApi()
			Expect(err).To(BeNil())

			Eventually(func() string {
				return CredentialURL(svc, source2.Id)
			}, "3m", "2s").Should(Equal(fmt.Sprintf("https://%s:3333", agentIP2)))

			// Login to Vcsim2
			res, err = agent2Api.Login(fmt.Sprintf("https://%s:%s/sdk", systemIP, Vsphere2Port),
				"core", "123456")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))
			zap.S().Info("Vcenter login completed successfully. Credentials saved.")

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				return AgentIsUpToDate(svc, source2.Id)
			}, "3m", "2s").Should(BeTrue())

			err = agent2.Remove()
			Expect(err).To(BeNil())

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("Edge cases", func() {
		It("VM reboot", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			res, err := agentApi.Login(fmt.Sprintf("https://%s:%s/sdk", systemIP, Vsphere1Port),
				"core", "123456")
			Expect(err).To(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))
			zap.S().Info("Vcenter login completed successfully. Credentials saved.")

			// Restarting the VM
			err = agent.Restart()
			Expect(err).To(BeNil())

			// Check that planner-agent service is running
			Eventually(func() bool {
				return agent.IsServiceRunning(agentIP, "planner-agent")
			}, "5m", "2s").Should(BeTrue())

			zap.S().Infof("Wait for agent status to be %s...", string(v1alpha1.AgentStatusUpToDate))
			Eventually(func() bool {
				return AgentIsUpToDate(svc, source.Id)
			}, "3m", "2s").Should(BeTrue())

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})
})
