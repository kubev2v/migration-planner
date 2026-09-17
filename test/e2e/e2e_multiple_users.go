package main

import (
	"fmt"
	"strings"
	"time"

	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/test/e2e/config"

	. "github.com/kubev2v/migration-planner/test/e2e/service"
	. "github.com/kubev2v/migration-planner/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

var _ = Describe("e2e-multiple-users", func() {
	var (
		users            = []string{"user", "admin", "koko"}
		organizations    = []string{"redhat", "intel", "apple", "microsoft", "nvidia"}
		serviceInstances = make(map[string]PlannerService)
		err              error
		startTime        time.Time
	)

	BeforeEach(func() {
		startTime = time.Now()

		// Iterate over each organization and user to authenticate and create a unique source per org-user pair
		for _, org := range organizations {
			for _, user := range users {
				key := fmt.Sprintf("%s|%s", org, user)
				serviceInstances[key], err = NewPlannerService(UserAuth(user, org, config.Cfg.Test.DefaultEmailDomain))
				Expect(err).To(BeNil())
			}
		}

	})

	AfterEach(func() {
		zap.S().Info("Cleaning up after test...")
		for _, org := range organizations {
			for _, user := range users {
				key := fmt.Sprintf("%s|%s", org, user)
				err := serviceInstances[key].RemoveSources()
				Expect(err).To(BeNil(), "Failed to remove sources from DB")
				delete(serviceInstances, key)
			}
		}
		testDuration := time.Since(startTime)
		zap.S().Infof("Test completed in: %s\n", testDuration.String())
		config.Cfg.Test.TestsExecutionTime[CurrentSpecReport().LeafNodeText] = testDuration
	})

	Context("Multiple Users", func() {
		It("Users should see their sources", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Create source for each user-org pair
			for _, org := range organizations {
				for _, user := range users {
					key := fmt.Sprintf("%s|%s", org, user)
					_, err = serviceInstances[key].CreateSource(fmt.Sprintf("%s-%s", org, user))
					Expect(err).To(BeNil())
				}
			}

			// Verify that each user sees only the sources created by their own organization
			for _, org := range organizations {
				for _, user := range users {
					key := fmt.Sprintf("%s|%s", org, user)
					visibleSources, err := serviceInstances[key].GetSources()
					Expect(err).To(BeNil())
					Expect(*visibleSources).To(HaveLen(1))
					for _, source := range *visibleSources {
						Expect(strings.Split(source.Name, "-")[0]).To(Equal(org))
					}
				}
			}

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("users should see only their own assessments, even within the same organization", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			organization := organizations[0]
			owner := users[0]
			assessmentInventory := &v1alpha1.Inventory{
				VcenterId: "multiple-users-test-vcenter",
				Clusters: map[string]v1alpha1.InventoryData{
					"test-cluster": {
						Vms: v1alpha1.VMs{
							Total:                1,
							TotalMigratable:      1,
							CpuCores:             v1alpha1.VMResourceBreakdown{Total: 1},
							RamGB:                v1alpha1.VMResourceBreakdown{Total: 1},
							DiskGB:               v1alpha1.VMResourceBreakdown{Total: 1},
							DiskCount:            v1alpha1.VMResourceBreakdown{Total: 1},
							PowerStates:          map[string]int{"poweredOn": 1},
							NotMigratableReasons: []v1alpha1.MigrationIssue{},
							MigrationWarnings:    []v1alpha1.MigrationIssue{},
						},
						Infra: v1alpha1.Infra{
							TotalHosts:      1,
							HostPowerStates: map[string]int{"poweredOn": 1},
							Networks:        []v1alpha1.Network{},
							Datastores:      []v1alpha1.Datastore{},
						},
					},
				},
			}

			ownerKey := fmt.Sprintf("%s|%s", organization, owner)
			ownerService := serviceInstances[ownerKey]
			assessment, err := ownerService.CreateAssessment(
				"multiple-users-private-assessment",
				"inventory",
				nil,
				assessmentInventory,
			)
			Expect(err).To(BeNil())
			Expect(assessment).ToNot(BeNil())

			assessmentID := assessment.Id
			DeferCleanup(func() {
				cleanupErr := ownerService.RemoveAssessment(assessmentID)
				Expect(cleanupErr).To(BeNil(), "Failed to remove fresh assessment")
			})

			for _, user := range users {
				key := fmt.Sprintf("%s|%s", organization, user)
				visibleAssessments, err := serviceInstances[key].GetAssessments()
				Expect(err).To(BeNil())
				Expect(visibleAssessments).ToNot(BeNil())

				isVisible := false
				for _, visibleAssessment := range *visibleAssessments {
					if visibleAssessment.Id == assessmentID {
						isVisible = true
						break
					}
				}

				if user == owner {
					Expect(isVisible).To(BeTrue(), "user %s should see their assessment", user)
				} else {
					Expect(isVisible).To(BeFalse(), "user %s should not see assessment owned by %s in the same organization", user, owner)
				}
			}

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})
})
