package v1alpha1_test

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func createTestInventoryForComplexityHandler(clusterID string) []byte {
	osInfo := map[string]api.OsInfo{
		"Red Hat Enterprise Linux 9 (64-bit)": {Count: 50, Supported: true},
		"CentOS 7 (64-bit)":                   {Count: 10, Supported: false},
		"FreeBSD (64-bit)":                    {Count: 3, Supported: false},
	}
	diskSizeTier := map[string]api.DiskSizeTierSummary{
		"0-100GiB": {VmCount: 63, TotalSizeTB: 5.5},
	}
	diskComplexityTier := map[string]api.DiskSizeTierSummary{
		"0-10TiB": {VmCount: 63, TotalSizeTB: 5.5},
	}
	inventory := api.Inventory{
		Clusters: map[string]api.InventoryData{
			clusterID: {
				Vms: api.VMs{
					Total:              63,
					OsInfo:             &osInfo,
					DiskSizeTier:       &diskSizeTier,
					DiskComplexityTier: &diskComplexityTier,
					DiskGB:             api.VMResourceBreakdown{Total: 5632},
					CpuCores:           api.VMResourceBreakdown{Total: 200},
					RamGB:              api.VMResourceBreakdown{Total: 400},
				},
			},
		},
	}
	data, err := json.Marshal(inventory)
	Expect(err).ToNot(HaveOccurred())
	return data
}

func createTestAssessmentForComplexityHandler(id uuid.UUID, username, orgID, clusterID string) *model.Assessment {
	return &model.Assessment{
		ID:       id,
		Name:     "test-assessment",
		OrgID:    orgID,
		Username: username,
		Snapshots: []model.Snapshot{
			{
				ID:           1,
				CreatedAt:    time.Now(),
				Inventory:    createTestInventoryForComplexityHandler(clusterID),
				AssessmentID: id,
				Version:      2,
			},
		},
	}
}

func createTestInventoryForEstimationHandler(clusterID string, totalVMs, totalDiskGB int) []byte {
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

func createTestAssessmentForEstimationHandler(id uuid.UUID, username, orgID, clusterID string) *model.Assessment {
	return &model.Assessment{
		ID:       id,
		Name:     "test-assessment",
		OrgID:    orgID,
		Username: username,
		Snapshots: []model.Snapshot{
			{
				ID:           1,
				CreatedAt:    time.Now(),
				Inventory:    createTestInventoryForEstimationHandler(clusterID, 10, 1000),
				AssessmentID: id,
				Version:      2,
			},
		},
	}
}

func createTestAssessmentForByComplexityHandler(id uuid.UUID, username, orgID, clusterID string) *model.Assessment {
	dist := map[string]api.DiskSizeTierSummary{
		"1": {VmCount: 30, TotalSizeTB: 5.0},
		"2": {VmCount: 10, TotalSizeTB: 12.0},
	}
	osInfo := map[string]api.OsInfo{
		"Red Hat Enterprise Linux 9 (64-bit)": {Count: 30},
		"Windows Server 2019":                 {Count: 10},
	}
	diskTier := map[string]api.DiskSizeTierSummary{
		"0-100GiB": {VmCount: 30, TotalSizeTB: 5.0},
		"1-2TiB":   {VmCount: 10, TotalSizeTB: 12.0},
	}
	diskComplexityTierOsDisk := map[string]api.DiskSizeTierSummary{
		"0-10TiB":  {VmCount: 30, TotalSizeTB: 5.0},
		"10-20TiB": {VmCount: 10, TotalSizeTB: 12.0},
	}
	inventory := api.Inventory{
		Clusters: map[string]api.InventoryData{
			clusterID: {
				Vms: api.VMs{
					Total:                  40,
					OsInfo:                 &osInfo,
					DiskSizeTier:           &diskTier,
					DiskComplexityTier:     &diskComplexityTierOsDisk,
					ComplexityDistribution: &dist,
					DiskGB:                 api.VMResourceBreakdown{Total: 17000},
					CpuCores:               api.VMResourceBreakdown{Total: 160},
					RamGB:                  api.VMResourceBreakdown{Total: 320},
				},
			},
		},
	}
	data, _ := json.Marshal(inventory)
	return &model.Assessment{
		ID: id, Name: "test", OrgID: orgID, Username: username,
		Snapshots: []model.Snapshot{{ID: 1, Inventory: data, AssessmentID: id, Version: 2}},
	}
}

var _ = Describe("estimation handler", func() {
	var (
		mockStore    *MockStore
		handler      *handlers.ServiceHandler
		ctx          context.Context
		user         auth.User
		assessmentID uuid.UUID
		clusterID    string
	)

	BeforeEach(func() {
		mockStore = NewMockStore()
		user = auth.User{
			Username:     "test-user",
			Organization: "test-org",
			EmailDomain:  "test.example.com",
		}
		ctx = auth.NewTokenContext(context.Background(), user)
		assessmentID = uuid.New()
		clusterID = "cluster-test-123"
	})

	Describe("CalculateMigrationEstimation", func() {
		Context("successful requests", func() {
			It("successfully returns 200 with valid request", func() {
				request := &api.MigrationEstimationRequest{
					ClusterId: clusterID,
				}

				mockStore.assessments[assessmentID] = createTestAssessmentForEstimationHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(
					nil, // sourceService
					service.NewAssessmentService(mockStore, nil, nil),
					nil, // jobService
					nil, // sizerService
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				Expect(resp).NotTo(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimation200JSONResponse)
				Expect(ok).To(BeTrue())
				// When no schema filter is supplied all schemas are returned
				Expect(response.Estimation).NotTo(BeEmpty())
				for _, schemaResult := range response.Estimation {
					Expect(schemaResult.MinTotalDuration).NotTo(BeEmpty())
					Expect(schemaResult.MaxTotalDuration).NotTo(BeEmpty())
					Expect(schemaResult.Breakdown).NotTo(BeEmpty())
				}
				// EstimationContext must reflect the schemas that actually ran
				Expect(response.EstimationContext.Schemas).NotTo(BeNil())
				Expect(*response.EstimationContext.Schemas).To(ConsistOf("network-based", "storage-offload"))
				Expect(response.EstimationContext.Params).NotTo(BeNil())
				Expect(*response.EstimationContext.Params).To(HaveKey("work_hours_per_day"))
			})

			It("returns response with breakdown containing all calculators", func() {
				request := &api.MigrationEstimationRequest{
					ClusterId: clusterID,
				}

				mockStore.assessments[assessmentID] = createTestAssessmentForEstimationHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimation200JSONResponse)
				Expect(ok).To(BeTrue())

				// Both schemas should be present when no filter is applied
				Expect(response.Estimation).To(HaveKey("network-based"))
				Expect(response.Estimation).To(HaveKey("storage-offload"))

				// Verify each schema result has MinTotalDuration, MaxTotalDuration and breakdown
				for schemaName, schemaResult := range response.Estimation {
					Expect(schemaResult.MinTotalDuration).NotTo(BeEmpty(), "schema %s should have MinTotalDuration", schemaName)
					Expect(schemaResult.MaxTotalDuration).NotTo(BeEmpty(), "schema %s should have MaxTotalDuration", schemaName)
					Expect(schemaResult.Breakdown).NotTo(BeEmpty(), "schema %s should have breakdown", schemaName)
					// Verify each breakdown entry has a Reason
					for calcName, detail := range schemaResult.Breakdown {
						Expect(detail.Reason).NotTo(BeEmpty(), "calculator %s in schema %s should have reason", calcName, schemaName)
					}
				}

				// network-based schema has Storage Migration and Post-Migration Checks
				Expect(response.Estimation["network-based"].Breakdown).To(HaveKey("Storage Migration"))
				Expect(response.Estimation["network-based"].Breakdown).To(HaveKey("Post-Migration Checks"))
			})

			It("returns only the requested schema when estimationSchema filter is provided", func() {
				schemas := []string{"network-based"}
				request := &api.MigrationEstimationRequest{
					ClusterId:        clusterID,
					EstimationSchema: &schemas,
				}

				mockStore.assessments[assessmentID] = createTestAssessmentForEstimationHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimation200JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Estimation).To(HaveLen(1))
				Expect(response.Estimation).To(HaveKey("network-based"))
				Expect(response.Estimation).NotTo(HaveKey("storage-offload"))
			})

			It("returns 400 when an unknown schema is requested", func() {
				schemas := []string{"unknown-schema"}
				request := &api.MigrationEstimationRequest{
					ClusterId:        clusterID,
					EstimationSchema: &schemas,
				}

				mockStore.assessments[assessmentID] = createTestAssessmentForEstimationHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimation400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).NotTo(BeEmpty())
			})
		})

		Context("request validation errors", func() {
			It("returns 400 when request body is nil", func() {
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: nil,
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimation400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("empty body"))
			})

			It("returns 400 when an unknown param key is provided", func() {
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id: assessmentID,
					Body: &api.MigrationEstimationRequest{
						ClusterId: clusterID,
						Params:    &map[string]interface{}{"bogus_param": 1.0},
					},
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimation400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("bogus_param"))
			})

			It("returns 400 when a param value is below its minimum", func() {
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				// transfer_rate_mbps has Min=0.1
				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id: assessmentID,
					Body: &api.MigrationEstimationRequest{
						ClusterId: clusterID,
						Params:    &map[string]interface{}{"transfer_rate_mbps": 0.0},
					},
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimation400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("transfer_rate_mbps"))
			})

			It("returns 400 when clusterId is empty", func() {
				request := &api.MigrationEstimationRequest{
					ClusterId: "",
				}

				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimation400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("clusterId is required"))
			})

			It("accepts valid clusterId format", func() {
				request := &api.MigrationEstimationRequest{
					ClusterId: "domain-c8",
				}

				mockStore.assessments[assessmentID] = createTestAssessmentForEstimationHandler(assessmentID, user.Username, user.Organization, "domain-c8")
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				_, ok := resp.(server.CalculateMigrationEstimation200JSONResponse)
				Expect(ok).To(BeTrue())
			})
		})

		Context("assessment not found errors", func() {
			It("returns 404 when assessment not found", func() {
				request := &api.MigrationEstimationRequest{
					ClusterId: clusterID,
				}

				nonExistentID := uuid.New()
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   nonExistentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimation404JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("not found"))
			})

			It("returns 500 when assessment service returns non-NotFound error", func() {
				request := &api.MigrationEstimationRequest{
					ClusterId: clusterID,
				}

				mockStore.getError = errors.New("database error")
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				_, ok := resp.(server.CalculateMigrationEstimation500JSONResponse)
				Expect(ok).To(BeTrue())
			})
		})

		Context("authorization errors", func() {
			It("returns 403 when user doesn't own assessment (different username)", func() {
				request := &api.MigrationEstimationRequest{
					ClusterId: clusterID,
				}

				handler = handlers.NewServiceHandler(
					nil,
					&ForbiddenAssessmentService{},
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimation403JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("forbidden"))
			})

			It("returns 403 when user doesn't own assessment (different organization)", func() {
				request := &api.MigrationEstimationRequest{
					ClusterId: clusterID,
				}

				handler = handlers.NewServiceHandler(
					nil,
					&ForbiddenAssessmentService{},
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimation403JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("forbidden"))
			})

			It("allows access when user owns assessment", func() {
				request := &api.MigrationEstimationRequest{
					ClusterId: clusterID,
				}

				mockStore.assessments[assessmentID] = createTestAssessmentForEstimationHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				_, ok := resp.(server.CalculateMigrationEstimation200JSONResponse)
				Expect(ok).To(BeTrue())
			})
		})

		Context("estimation service errors", func() {
			It("returns 404 when cluster ID is not found in inventory", func() {
				request := &api.MigrationEstimationRequest{
					ClusterId: "non-existent-cluster",
				}

				mockStore.assessments[assessmentID] = createTestAssessmentForEstimationHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimation404JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("not found"))
			})

			It("returns 500 when estimation service fails with no snapshots", func() {
				request := &api.MigrationEstimationRequest{
					ClusterId: clusterID,
				}

				// Create assessment without snapshots
				mockStore.assessments[assessmentID] = &model.Assessment{
					ID:        assessmentID,
					Name:      "test-assessment",
					OrgID:     user.Organization,
					Username:  user.Username,
					Snapshots: []model.Snapshot{},
				}
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
					nil,
					nil,
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				_, ok := resp.(server.CalculateMigrationEstimation500JSONResponse)
				Expect(ok).To(BeTrue())
			})
		})

		Context("when request includes params override", func() {
			It("produces a shorter Storage Migration duration when transfer_rate_mbps is increased", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForEstimationHandler(assessmentID, user.Username, user.Organization, clusterID)
				svc := service.NewEstimationService(mockStore)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil, nil),
					nil,
					nil,
					svc,
					nil,
					nil,
				)

				// Baseline: no param override, default transfer rate
				baseResp, err := handler.CalculateMigrationEstimation(ctx,
					server.CalculateMigrationEstimationRequestObject{
						Id:   assessmentID,
						Body: &api.MigrationEstimationRequest{ClusterId: clusterID},
					},
				)
				Expect(err).NotTo(HaveOccurred())
				baseResponse, ok := baseResp.(server.CalculateMigrationEstimation200JSONResponse)
				Expect(ok).To(BeTrue())
				baseDurationStr := baseResponse.Estimation["network-based"].Breakdown["Storage Migration"].Duration
				Expect(baseDurationStr).NotTo(BeNil())
				baseDuration, err := time.ParseDuration(*baseDurationStr)
				Expect(err).NotTo(HaveOccurred())

				// Override: much higher transfer rate → shorter storage migration duration
				fastResp, err := handler.CalculateMigrationEstimation(ctx,
					server.CalculateMigrationEstimationRequestObject{
						Id: assessmentID,
						Body: &api.MigrationEstimationRequest{
							ClusterId: clusterID,
							Params: &map[string]interface{}{
								"transfer_rate_mbps": 10000.0,
							},
						},
					},
				)
				Expect(err).NotTo(HaveOccurred())
				fastResponse, ok := fastResp.(server.CalculateMigrationEstimation200JSONResponse)
				Expect(ok).To(BeTrue())
				fastDurationStr := fastResponse.Estimation["network-based"].Breakdown["Storage Migration"].Duration
				Expect(fastDurationStr).NotTo(BeNil())
				fastDuration, err := time.ParseDuration(*fastDurationStr)
				Expect(err).NotTo(HaveOccurred())

				Expect(fastDuration).To(BeNumerically("<", baseDuration))
			})
		})
	})

	Describe("CalculateMigrationComplexity", func() {
		Context("successful requests", func() {
			It("returns 200 with complexityByDisk (4 entries) and complexityByOS (5 entries)", func() {
				request := &api.MigrationComplexityRequest{ClusterId: clusterID}
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexityHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationComplexity200JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.ComplexityByDisk).To(HaveLen(4))
				Expect(response.ComplexityByOS).To(HaveLen(5))
			})

			It("returns diskSizeRatings with range-only keys and correct scores", func() {
				request := &api.MigrationComplexityRequest{ClusterId: clusterID}
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexityHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response := resp.(server.CalculateMigrationComplexity200JSONResponse)
				Expect(response.DiskSizeRatings).To(HaveLen(4))
				Expect(response.DiskSizeRatings["0-10TiB"]).To(Equal(1))
				Expect(response.DiskSizeRatings["10-20TiB"]).To(Equal(2))
				Expect(response.DiskSizeRatings["20-50TiB"]).To(Equal(3))
				Expect(response.DiskSizeRatings["50+TiB"]).To(Equal(4))
			})

			It("returns osRatings with one entry per OS in the cluster inventory", func() {
				request := &api.MigrationComplexityRequest{ClusterId: clusterID}
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexityHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response := resp.(server.CalculateMigrationComplexity200JSONResponse)
				// createTestInventoryForComplexityHandler has 3 distinct OS names
				Expect(response.OsRatings).To(HaveLen(3))
				Expect(response.OsRatings["Red Hat Enterprise Linux 9 (64-bit)"]).To(Equal(1))
				Expect(response.OsRatings["CentOS 7 (64-bit)"]).To(Equal(1))
				Expect(response.OsRatings["FreeBSD (64-bit)"]).To(Equal(3))
			})

			It("returns disk scores in canonical order 1 through 4", func() {
				request := &api.MigrationComplexityRequest{ClusterId: clusterID}
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexityHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response := resp.(server.CalculateMigrationComplexity200JSONResponse)
				for i, entry := range response.ComplexityByDisk {
					Expect(entry.Score).To(Equal(i + 1))
				}
			})

			It("returns OS scores in canonical order 0 through 4", func() {
				request := &api.MigrationComplexityRequest{ClusterId: clusterID}
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexityHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response := resp.(server.CalculateMigrationComplexity200JSONResponse)
				for i, entry := range response.ComplexityByOS {
					Expect(entry.Score).To(Equal(i))
				}
			})

			It("returns complexityByOSName with one entry per distinct OS name", func() {
				request := &api.MigrationComplexityRequest{ClusterId: clusterID}
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexityHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response := resp.(server.CalculateMigrationComplexity200JSONResponse)
				// createTestInventoryForComplexityHandler has 3 distinct OS names
				Expect(response.ComplexityByOSName).To(HaveLen(3))
			})

			It("returns complexityByOSName with correct osName, score and vmCount for a known OS", func() {
				request := &api.MigrationComplexityRequest{ClusterId: clusterID}
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexityHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response := resp.(server.CalculateMigrationComplexity200JSONResponse)
				byName := map[string]api.ComplexityOSNameEntry{}
				for _, e := range response.ComplexityByOSName {
					byName[e.OsName] = e
				}
				rhel := byName["Red Hat Enterprise Linux 9 (64-bit)"]
				Expect(rhel.Score).To(Equal(1))
				Expect(rhel.VmCount).To(Equal(50))
				centos := byName["CentOS 7 (64-bit)"]
				Expect(centos.Score).To(Equal(1))
				Expect(centos.VmCount).To(Equal(10))
			})
		})

		Context("request validation errors", func() {
			It("returns 400 when request body is nil", func() {
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   assessmentID,
					Body: nil,
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationComplexity400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("empty body"))
			})

			It("returns 400 when clusterId is empty", func() {
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   assessmentID,
					Body: &api.MigrationComplexityRequest{ClusterId: ""},
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationComplexity400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("clusterId is required"))
			})
		})

		Context("assessment not found errors", func() {
			It("returns 404 when assessment does not exist", func() {
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   uuid.New(),
					Body: &api.MigrationComplexityRequest{ClusterId: clusterID},
				})

				Expect(err).To(BeNil())
				_, ok := resp.(server.CalculateMigrationComplexity404JSONResponse)
				Expect(ok).To(BeTrue())
			})

			It("returns 500 when store returns a non-NotFound error", func() {
				mockStore.getError = errors.New("database error")
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   assessmentID,
					Body: &api.MigrationComplexityRequest{ClusterId: clusterID},
				})

				Expect(err).To(BeNil())
				_, ok := resp.(server.CalculateMigrationComplexity500JSONResponse)
				Expect(ok).To(BeTrue())
			})
		})

		Context("authorization errors", func() {
			It("returns 403 when user has a different username", func() {
				handler = handlers.NewServiceHandler(nil, &ForbiddenAssessmentService{}, nil, nil, service.NewEstimationService(mockStore), nil, nil)
				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   assessmentID,
					Body: &api.MigrationComplexityRequest{ClusterId: clusterID},
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationComplexity403JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("forbidden"))
			})

			It("returns 403 when user belongs to a different organisation", func() {
				handler = handlers.NewServiceHandler(nil, &ForbiddenAssessmentService{}, nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   assessmentID,
					Body: &api.MigrationComplexityRequest{ClusterId: clusterID},
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationComplexity403JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("forbidden"))
			})
		})

		Context("complexity service errors", func() {
			It("returns 404 when cluster ID is not found in inventory", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForComplexityHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   assessmentID,
					Body: &api.MigrationComplexityRequest{ClusterId: "non-existent-cluster"},
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationComplexity404JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("not found"))
			})

			It("returns 500 when assessment has no snapshots", func() {
				mockStore.assessments[assessmentID] = &model.Assessment{
					ID:        assessmentID,
					Name:      "test-assessment",
					OrgID:     user.Organization,
					Username:  user.Username,
					Snapshots: []model.Snapshot{},
				}
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationComplexity(ctx, server.CalculateMigrationComplexityRequestObject{
					Id:   assessmentID,
					Body: &api.MigrationComplexityRequest{ClusterId: clusterID},
				})

				Expect(err).To(BeNil())
				_, ok := resp.(server.CalculateMigrationComplexity500JSONResponse)
				Expect(ok).To(BeTrue())
			})
		})
	})

	Describe("CalculateMigrationEstimationByComplexity", func() {
		Context("successful requests", func() {
			It("returns 200 with complexityByOsDisk (5 entries) and complexityMatrix", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForByComplexityHandler(
					assessmentID, user.Username, user.Organization, clusterID,
				)
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationEstimationByComplexity(ctx,
					server.CalculateMigrationEstimationByComplexityRequestObject{
						Id:   assessmentID,
						Body: &api.MigrationEstimationRequest{ClusterId: clusterID},
					})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimationByComplexity200JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.ComplexityByOsDisk).To(HaveLen(5))
				Expect(response.ComplexityMatrix).NotTo(BeEmpty())
			})

			It("populates estimation for non-empty buckets", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForByComplexityHandler(
					assessmentID, user.Username, user.Organization, clusterID,
				)
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationEstimationByComplexity(ctx,
					server.CalculateMigrationEstimationByComplexityRequestObject{
						Id:   assessmentID,
						Body: &api.MigrationEstimationRequest{ClusterId: clusterID},
					})

				Expect(err).To(BeNil())
				response := resp.(server.CalculateMigrationEstimationByComplexity200JSONResponse)
				var score1 *api.OsDiskEstimationEntry
				for i := range response.ComplexityByOsDisk {
					if response.ComplexityByOsDisk[i].Score == 1 {
						score1 = &response.ComplexityByOsDisk[i]
					}
				}
				Expect(score1).NotTo(BeNil())
				Expect(score1.Estimation).NotTo(BeNil())
			})

			It("returns estimationContext", func() {
				mockStore.assessments[assessmentID] = createTestAssessmentForByComplexityHandler(
					assessmentID, user.Username, user.Organization, clusterID,
				)
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationEstimationByComplexity(ctx,
					server.CalculateMigrationEstimationByComplexityRequestObject{
						Id:   assessmentID,
						Body: &api.MigrationEstimationRequest{ClusterId: clusterID},
					})

				Expect(err).To(BeNil())
				response := resp.(server.CalculateMigrationEstimationByComplexity200JSONResponse)
				Expect(response.EstimationContext).NotTo(BeNil())
			})

			It("returns 404 for unknown assessment", func() {
				resp, err := handler.CalculateMigrationEstimationByComplexity(ctx,
					server.CalculateMigrationEstimationByComplexityRequestObject{
						Id:   uuid.New(),
						Body: &api.MigrationEstimationRequest{ClusterId: clusterID},
					})
				Expect(err).To(BeNil())
				_, ok := resp.(server.CalculateMigrationEstimationByComplexity404JSONResponse)
				Expect(ok).To(BeTrue())
			})

			It("returns 400 for empty body", func() {
				resp, err := handler.CalculateMigrationEstimationByComplexity(ctx,
					server.CalculateMigrationEstimationByComplexityRequestObject{
						Id:   assessmentID,
						Body: nil,
					})
				Expect(err).To(BeNil())
				_, ok := resp.(server.CalculateMigrationEstimationByComplexity400JSONResponse)
				Expect(ok).To(BeTrue())
			})
		})

		Context("param validation errors", func() {
			It("returns 400 when an unknown param key is provided", func() {
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				resp, err := handler.CalculateMigrationEstimationByComplexity(ctx,
					server.CalculateMigrationEstimationByComplexityRequestObject{
						Id: assessmentID,
						Body: &api.MigrationEstimationRequest{
							ClusterId: clusterID,
							Params:    &map[string]interface{}{"bogus_param": 1.0},
						},
					})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimationByComplexity400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("bogus_param"))
			})

			It("returns 400 when a param value is below its minimum", func() {
				handler = handlers.NewServiceHandler(nil, service.NewAssessmentService(mockStore, nil, nil), nil, nil, service.NewEstimationService(mockStore), nil, nil)

				// transfer_rate_mbps has Min=0.1
				resp, err := handler.CalculateMigrationEstimationByComplexity(ctx,
					server.CalculateMigrationEstimationByComplexityRequestObject{
						Id: assessmentID,
						Body: &api.MigrationEstimationRequest{
							ClusterId: clusterID,
							Params:    &map[string]interface{}{"transfer_rate_mbps": 0.0},
						},
					})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimationByComplexity400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Message).To(ContainSubstring("transfer_rate_mbps"))
			})
		})
	})
})
