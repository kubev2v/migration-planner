package service_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func createTestInventoryForEstimation(clusterID string, totalVMs, totalDiskGB int) []byte {
	inventory := api.Inventory{
		Clusters: map[string]api.InventoryData{
			clusterID: {
				Vms: api.VMs{
					Total: totalVMs,
					DiskGB: api.VMResourceBreakdown{
						Total: totalDiskGB,
					},
					CpuCores: api.VMResourceBreakdown{
						Total: 40,
					},
					RamGB: api.VMResourceBreakdown{
						Total: 80,
					},
				},
			},
		},
	}
	data, err := json.Marshal(inventory)
	Expect(err).ToNot(HaveOccurred())
	return data
}

func createTestAssessmentForEstimation(id uuid.UUID, username, orgID, clusterID string, totalVMs, totalDiskGB int) *model.Assessment {
	return &model.Assessment{
		ID:       id,
		Name:     "test-assessment",
		OrgID:    orgID,
		Username: username,
		Snapshots: []model.Snapshot{
			{
				ID:           1,
				CreatedAt:    time.Now(),
				Inventory:    createTestInventoryForEstimation(clusterID, totalVMs, totalDiskGB),
				AssessmentID: id,
				Version:      2,
			},
		},
	}
}

var _ = Describe("EstimationService", func() {
	var (
		mockStore     *MockStore
		estimationSrv *service.EstimationService
		ctx           context.Context
		assessmentID  uuid.UUID
		clusterID     string
		testUsername  string
		testOrgID     string
	)

	BeforeEach(func() {
		mockStore = NewMockStore()
		estimationSrv = service.NewEstimationService(mockStore)
		ctx = context.Background()
		assessmentID = uuid.New()
		clusterID = "cluster-test-123"
		testUsername = "test-user"
		testOrgID = "test-org"
	})

	Describe("CalculateMigrationEstimation", func() {
		Context("successful calculation", func() {
			It("successfully calculates estimation with valid data", func() {
				// Setup: 10 VMs, 1000 GB disk
				mockStore.assessments[assessmentID] = createTestAssessmentForEstimation(
					assessmentID, testUsername, testOrgID, clusterID, 10, 1000,
				)

				result, err := estimationSrv.CalculateMigrationEstimation(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				Expect(result.TotalDuration).To(BeNumerically(">", 0))
				Expect(result.Breakdown).NotTo(BeEmpty())
			})

			It("returns breakdown with all registered calculators", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForEstimation(
					assessmentID, testUsername, testOrgID, clusterID, 20, 2000,
				)

				result, err := estimationSrv.CalculateMigrationEstimation(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				Expect(result.Breakdown).To(HaveKey("Storage Migration"))
				Expect(result.Breakdown).To(HaveKey("Post-Migration Checks"))
			})

			It("calculates correct total duration as sum of all calculators", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForEstimation(
					assessmentID, testUsername, testOrgID, clusterID, 10, 1000,
				)

				result, err := estimationSrv.CalculateMigrationEstimation(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())

				// Calculate expected total
				var expectedTotal time.Duration
				for _, est := range result.Breakdown {
					expectedTotal += est.Duration
				}
				Expect(result.TotalDuration).To(Equal(expectedTotal))
			})

			It("includes reason text in each calculator result", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForEstimation(
					assessmentID, testUsername, testOrgID, clusterID, 15, 750,
				)

				result, err := estimationSrv.CalculateMigrationEstimation(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				for calcName, est := range result.Breakdown {
					Expect(est.Reason).NotTo(BeEmpty(), "calculator %s should have a reason", calcName)
					Expect(est.Duration).To(BeNumerically(">=", 0))
				}
			})
		})

		Context("assessment not found", func() {
			It("returns ErrResourceNotFound when assessment does not exist", func() {
				nonExistentID := uuid.New()

				result, err := estimationSrv.CalculateMigrationEstimation(ctx, nonExistentID, clusterID)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				_, ok := err.(*service.ErrResourceNotFound)
				Expect(ok).To(BeTrue(), "expected ErrResourceNotFound")
			})

			It("returns error when store returns error", func() {
				mockStore.getError = store.ErrRecordNotFound

				result, err := estimationSrv.CalculateMigrationEstimation(ctx, assessmentID, clusterID)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})

		Context("snapshot validation", func() {
			It("returns error when assessment has no snapshots", func() {
				// Create assessment without snapshots
				mockStore.assessments[assessmentID] = &model.Assessment{
					ID:        assessmentID,
					Name:      "test-assessment",
					OrgID:     testOrgID,
					Username:  testUsername,
					Snapshots: []model.Snapshot{}, // Empty snapshots
				}

				result, err := estimationSrv.CalculateMigrationEstimation(ctx, assessmentID, clusterID)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("no snapshots"))
			})

			It("returns error when snapshot has empty inventory", func() {
				// Create assessment with empty inventory
				mockStore.assessments[assessmentID] = &model.Assessment{
					ID:       assessmentID,
					Name:     "test-assessment",
					OrgID:    testOrgID,
					Username: testUsername,
					Snapshots: []model.Snapshot{
						{
							ID:           1,
							CreatedAt:    time.Now(),
							Inventory:    []byte{}, // Empty inventory
							AssessmentID: assessmentID,
							Version:      2,
						},
					},
				}

				result, err := estimationSrv.CalculateMigrationEstimation(ctx, assessmentID, clusterID)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("empty inventory"))
			})
		})

		Context("inventory parsing errors", func() {
			It("returns error when inventory JSON is invalid", func() {
				mockStore.assessments[assessmentID] = &model.Assessment{
					ID:       assessmentID,
					Name:     "test-assessment",
					OrgID:    testOrgID,
					Username: testUsername,
					Snapshots: []model.Snapshot{
						{
							ID:           1,
							CreatedAt:    time.Now(),
							Inventory:    []byte("invalid json {{{"),
							AssessmentID: assessmentID,
							Version:      2,
						},
					},
				}

				result, err := estimationSrv.CalculateMigrationEstimation(ctx, assessmentID, clusterID)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("failed to parse inventory"))
			})

			It("returns error when inventory has no clusters", func() {
				emptyInventory := api.Inventory{
					Clusters: map[string]api.InventoryData{}, // Empty clusters
				}
				data, _ := json.Marshal(emptyInventory)

				mockStore.assessments[assessmentID] = &model.Assessment{
					ID:       assessmentID,
					Name:     "test-assessment",
					OrgID:    testOrgID,
					Username: testUsername,
					Snapshots: []model.Snapshot{
						{
							ID:           1,
							CreatedAt:    time.Now(),
							Inventory:    data,
							AssessmentID: assessmentID,
							Version:      2,
						},
					},
				}

				result, err := estimationSrv.CalculateMigrationEstimation(ctx, assessmentID, clusterID)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("no clusters"))
			})
		})

		Context("invalid cluster ID", func() {
			It("returns error when cluster ID not found in inventory", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForEstimation(
					assessmentID, testUsername, testOrgID, "different-cluster", 10, 1000,
				)

				result, err := estimationSrv.CalculateMigrationEstimation(ctx, assessmentID, "non-existent-cluster")

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("not found"))
				Expect(err.Error()).To(ContainSubstring("non-existent-cluster"))
			})
		})

		Context("edge cases", func() {
			It("handles zero VMs correctly", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForEstimation(
					assessmentID, testUsername, testOrgID, clusterID, 0, 0,
				)

				result, err := estimationSrv.CalculateMigrationEstimation(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				// Should still have calculators run, just with 0 values
				Expect(result.Breakdown).NotTo(BeEmpty())
			})

			It("handles large values correctly", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForEstimation(
					assessmentID, testUsername, testOrgID, clusterID, 10000, 500000,
				)

				result, err := estimationSrv.CalculateMigrationEstimation(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				Expect(result.TotalDuration).To(BeNumerically(">", 0))
			})
		})
	})
})
