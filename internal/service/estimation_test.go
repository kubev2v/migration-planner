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

// helpers for complexity tests

func buildOsInfo(entries map[string]int) *map[string]api.OsInfo {
	m := make(map[string]api.OsInfo, len(entries))
	for name, count := range entries {
		m[name] = api.OsInfo{Count: count}
	}
	return &m
}

func buildDiskSizeTier(entries map[string]api.DiskSizeTierSummary) *map[string]api.DiskSizeTierSummary {
	return &entries
}

func createTestInventoryForComplexity(clusterID string, osInfo *map[string]api.OsInfo, diskSizeTier *map[string]api.DiskSizeTierSummary) []byte {
	inventory := api.Inventory{
		Clusters: map[string]api.InventoryData{
			clusterID: {
				Vms: api.VMs{
					Total:        10,
					OsInfo:       osInfo,
					DiskSizeTier: diskSizeTier,
					DiskGB:       api.VMResourceBreakdown{Total: 100},
					CpuCores:     api.VMResourceBreakdown{Total: 40},
					RamGB:        api.VMResourceBreakdown{Total: 80},
				},
			},
		},
	}
	data, err := json.Marshal(inventory)
	Expect(err).ToNot(HaveOccurred())
	return data
}

func createTestAssessmentForComplexity(id uuid.UUID, username, orgID, clusterID string, osInfo *map[string]api.OsInfo, diskSizeTier *map[string]api.DiskSizeTierSummary) *model.Assessment {
	return &model.Assessment{
		ID:       id,
		Name:     "test-assessment",
		OrgID:    orgID,
		Username: username,
		Snapshots: []model.Snapshot{
			{
				ID:           1,
				CreatedAt:    time.Now(),
				Inventory:    createTestInventoryForComplexity(clusterID, osInfo, diskSizeTier),
				AssessmentID: id,
				Version:      2,
			},
		},
	}
}

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

	Describe("CalculateMigrationComplexity", func() {
		var defaultOsInfo *map[string]api.OsInfo
		var defaultDiskTier *map[string]api.DiskSizeTierSummary

		BeforeEach(func() {
			defaultOsInfo = buildOsInfo(map[string]int{
				"Red Hat Enterprise Linux 9 (64-bit)": 100,
				"CentOS 7 (64-bit)":                   20,
				"FreeBSD (64-bit)":                    5,
			})
			defaultDiskTier = buildDiskSizeTier(map[string]api.DiskSizeTierSummary{
				"Easy (0-10TB)": {VmCount: 125, TotalSizeTB: 8.5},
			})
		})

		Context("successful calculation", func() {
			It("returns a result with 5 OS entries and 4 disk entries", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexity(
					assessmentID, testUsername, testOrgID, clusterID, defaultOsInfo, defaultDiskTier,
				)

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				Expect(result.ComplexityByOS).To(HaveLen(5))
				Expect(result.ComplexityByDisk).To(HaveLen(4))
			})

			It("places OS names into the correct score buckets", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexity(
					assessmentID, testUsername, testOrgID, clusterID, defaultOsInfo, defaultDiskTier,
				)

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				// score 0: FreeBSD (5 VMs, unclassified) — always first in canonical order
				Expect(result.ComplexityByOS[0].Score).To(Equal(0))
				Expect(result.ComplexityByOS[0].VMCount).To(Equal(5))
				// score 1: Red Hat (100 VMs)
				Expect(result.ComplexityByOS[1].Score).To(Equal(1))
				Expect(result.ComplexityByOS[1].VMCount).To(Equal(100))
				// score 2: CentOS (20 VMs)
				Expect(result.ComplexityByOS[2].Score).To(Equal(2))
				Expect(result.ComplexityByOS[2].VMCount).To(Equal(20))
			})

			It("maps disk tier labels to correct scores with correct size values", func() {
				diskTier := buildDiskSizeTier(map[string]api.DiskSizeTierSummary{
					"Easy (0-10TB)":       {VmCount: 80, TotalSizeTB: 5.0},
					"Medium (10-20TB)":    {VmCount: 10, TotalSizeTB: 15.0},
					"Hard (20-50TB)":      {VmCount: 5, TotalSizeTB: 30.0},
					"White Glove (>50TB)": {VmCount: 1, TotalSizeTB: 75.0},
				})
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexity(
					assessmentID, testUsername, testOrgID, clusterID, defaultOsInfo, diskTier,
				)

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				Expect(result.ComplexityByDisk[0].Score).To(Equal(1))
				Expect(result.ComplexityByDisk[0].VMCount).To(Equal(80))
				Expect(result.ComplexityByDisk[0].TotalSizeTB).To(Equal(5.0))
				Expect(result.ComplexityByDisk[3].Score).To(Equal(4))
				Expect(result.ComplexityByDisk[3].VMCount).To(Equal(1))
				Expect(result.ComplexityByDisk[3].TotalSizeTB).To(Equal(75.0))
			})

			It("OS entries are always in canonical score order 0 through 4", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexity(
					assessmentID, testUsername, testOrgID, clusterID, defaultOsInfo, defaultDiskTier,
				)

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				for i, entry := range result.ComplexityByOS {
					Expect(entry.Score).To(Equal(i))
				}
			})

			It("disk entries are always in canonical score order 1 through 4", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexity(
					assessmentID, testUsername, testOrgID, clusterID, defaultOsInfo, defaultDiskTier,
				)

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				for i, entry := range result.ComplexityByDisk {
					Expect(entry.Score).To(Equal(i + 1))
				}
			})
		})

		Context("ComplexityByOSName field", func() {
			It("has one entry per distinct OS name in the inventory", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexity(
					assessmentID, testUsername, testOrgID, clusterID, defaultOsInfo, defaultDiskTier,
				)

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				// defaultOsInfo has 3 distinct OS names
				Expect(result.ComplexityByOSName).To(HaveLen(3))
			})

			It("assigns correct scores to each OS name", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexity(
					assessmentID, testUsername, testOrgID, clusterID, defaultOsInfo, defaultDiskTier,
				)

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				byName := map[string]int{}
				for _, e := range result.ComplexityByOSName {
					byName[e.Name] = e.Score
				}
				Expect(byName["Red Hat Enterprise Linux 9 (64-bit)"]).To(Equal(1))
				Expect(byName["CentOS 7 (64-bit)"]).To(Equal(2))
				Expect(byName["FreeBSD (64-bit)"]).To(Equal(0))
			})

			It("preserves VM counts from the inventory", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexity(
					assessmentID, testUsername, testOrgID, clusterID, defaultOsInfo, defaultDiskTier,
				)

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				byName := map[string]int{}
				for _, e := range result.ComplexityByOSName {
					byName[e.Name] = e.VMCount
				}
				Expect(byName["Red Hat Enterprise Linux 9 (64-bit)"]).To(Equal(100))
				Expect(byName["CentOS 7 (64-bit)"]).To(Equal(20))
				Expect(byName["FreeBSD (64-bit)"]).To(Equal(5))
			})

		})

		Context("assumptions fields", func() {
			It("diskSizeRatings contains exactly the 4 range keys with correct scores", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexity(
					assessmentID, testUsername, testOrgID, clusterID, defaultOsInfo, defaultDiskTier,
				)

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				Expect(result.DiskSizeRatings).To(HaveLen(4))
				Expect(result.DiskSizeRatings["0-10TB"]).To(Equal(1))
				Expect(result.DiskSizeRatings["10-20TB"]).To(Equal(2))
				Expect(result.DiskSizeRatings["20-50TB"]).To(Equal(3))
				Expect(result.DiskSizeRatings[">50TB"]).To(Equal(4))
			})

			It("osRatings contains one entry per distinct OS name from the inventory", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexity(
					assessmentID, testUsername, testOrgID, clusterID, defaultOsInfo, defaultDiskTier,
				)

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(err).To(BeNil())
				// defaultOsInfo has 3 distinct OS names
				Expect(result.OSRatings).To(HaveLen(3))
				Expect(result.OSRatings["Red Hat Enterprise Linux 9 (64-bit)"]).To(Equal(1))
				Expect(result.OSRatings["CentOS 7 (64-bit)"]).To(Equal(2))
				Expect(result.OSRatings["FreeBSD (64-bit)"]).To(Equal(0))
			})
		})

		Context("assessment not found", func() {
			It("returns ErrResourceNotFound when assessment does not exist", func() {
				result, err := estimationSrv.CalculateMigrationComplexity(ctx, uuid.New(), clusterID)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				_, ok := err.(*service.ErrResourceNotFound)
				Expect(ok).To(BeTrue())
			})

			It("returns error when store returns error", func() {
				mockStore.getError = store.ErrRecordNotFound

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})

		Context("snapshot validation", func() {
			It("returns error when assessment has no snapshots", func() {
				mockStore.assessments[assessmentID] = &model.Assessment{
					ID:        assessmentID,
					OrgID:     testOrgID,
					Username:  testUsername,
					Snapshots: []model.Snapshot{},
				}

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("no snapshots"))
			})

			It("returns error when snapshot has empty inventory", func() {
				mockStore.assessments[assessmentID] = &model.Assessment{
					ID:       assessmentID,
					OrgID:    testOrgID,
					Username: testUsername,
					Snapshots: []model.Snapshot{
						{ID: 1, Inventory: []byte{}, AssessmentID: assessmentID},
					},
				}

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("empty inventory"))
			})
		})

		Context("invalid cluster ID", func() {
			It("returns ErrResourceNotFound when cluster ID is not in inventory", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexity(
					assessmentID, testUsername, testOrgID, "other-cluster", defaultOsInfo, defaultDiskTier,
				)

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, "non-existent-cluster")

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				_, ok := err.(*service.ErrResourceNotFound)
				Expect(ok).To(BeTrue())
			})
		})

		Context("missing inventory data", func() {
			It("returns error when osInfo is nil", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexity(
					assessmentID, testUsername, testOrgID, clusterID, nil, defaultDiskTier,
				)

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("osInfo"))
			})

			It("returns error when diskSizeTier is nil", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexity(
					assessmentID, testUsername, testOrgID, clusterID, defaultOsInfo, nil,
				)

				result, err := estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("diskSizeTier"))
			})
		})
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
