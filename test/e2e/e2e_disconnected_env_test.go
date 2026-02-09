package e2e_test

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kubev2v/migration-planner/test/e2e/model"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/agent"
	. "github.com/kubev2v/migration-planner/test/e2e/helpers"
	. "github.com/kubev2v/migration-planner/test/e2e/service"
	. "github.com/kubev2v/migration-planner/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

var _ = Describe("e2e-disconnected-environment", func() {

	var (
		svc       PlannerService
		e2eAgent  model.E2EAgent
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
		Eventually(func() error {
			_, err = e2eAgent.Api.Status()
			if err != nil {
				return err
			}
			return nil
		}, "3m", "2s").Should(BeNil())
		zap.S().Info("Planner-agent is now running")

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

	Context("Flow", func() {
		It("Disconnected-environment", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Adding vcenter.com to /etc/hosts to enable connectivity to the vSphere server.
			_, err := RunSSHCommand(agentIP, fmt.Sprintf("podman exec "+
				"--user root "+
				"planner-agent "+
				"bash -c 'echo \"%s vcenter.com\" >> /etc/hosts'", SystemIP))
			Expect(err).To(BeNil(), "Failed to enable connection to Vsphere")

			s, resCode, err := e2eAgent.Api.StartCollector(fmt.Sprintf("https://%s:%s/sdk", "vcenter.com", Vsphere1Port), "core", "123456")
			Expect(err).To(BeNil())
			Expect(resCode).To(Equal(http.StatusAccepted))
			Expect(s.Status).ToNot(Equal(CollectorStatusError))

			zap.S().Infof("Wait for collecor status to be %s...", string(CollectorStatusCollected))
			Eventually(func() string {
				s, err = e2eAgent.Api.GetCollectorStatus()
				if err != nil {
					zap.S().Errorf("Failed to get collector status: %s", err)
					return ""
				}
				return s.Status
			}, "3m", "2s").Should(Equal(string(CollectorStatusCollected)))

			statusReply, err := e2eAgent.Api.Status()
			Expect(err).To(BeNil())
			Expect(statusReply.ConsoleConnection).To(Equal("disconnected"))

			// agent shouldn't be created in the api
			so, err := svc.GetSource(source.Id)
			Expect(err).To(BeNil())
			Expect(so.Agent).To(BeNil())

			// Get inventory
			inventory, err := e2eAgent.Api.Inventory()
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
