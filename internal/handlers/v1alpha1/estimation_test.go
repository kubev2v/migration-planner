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
					service.NewAssessmentService(mockStore, nil),
					nil, // jobService
					nil, // sizerService
					service.NewEstimationService(mockStore),
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				Expect(resp).NotTo(BeNil())
				// Check response type
				response, ok := resp.(server.CalculateMigrationEstimation200JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(response.TotalDuration).NotTo(BeEmpty())
				Expect(response.Breakdown).NotTo(BeEmpty())
			})

			It("returns response with breakdown containing all calculators", func() {
				request := &api.MigrationEstimationRequest{
					ClusterId: clusterID,
				}

				mockStore.assessments[assessmentID] = createTestAssessmentForEstimationHandler(assessmentID, user.Username, user.Organization, clusterID)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
				)

				resp, err := handler.CalculateMigrationEstimation(ctx, server.CalculateMigrationEstimationRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				response, ok := resp.(server.CalculateMigrationEstimation200JSONResponse)
				Expect(ok).To(BeTrue())

				// Verify breakdown structure
				Expect(response.Breakdown).To(HaveKey("Storage Migration"))
				Expect(response.Breakdown).To(HaveKey("Post-Migration Checks"))

				// Verify each breakdown entry has required fields
				for calcName, detail := range response.Breakdown {
					Expect(detail.Duration).NotTo(BeEmpty(), "calculator %s should have duration", calcName)
					Expect(detail.Reason).NotTo(BeEmpty(), "calculator %s should have reason", calcName)
				}
			})
		})

		Context("request validation errors", func() {
			It("returns 400 when request body is nil", func() {
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
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

			It("returns 400 when clusterId is empty", func() {
				request := &api.MigrationEstimationRequest{
					ClusterId: "",
				}

				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
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
					service.NewAssessmentService(mockStore, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
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
					service.NewAssessmentService(mockStore, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
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
					service.NewAssessmentService(mockStore, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
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

				// Create assessment with different username
				mockStore.assessments[assessmentID] = createTestAssessmentForEstimationHandler(assessmentID, "other-user", user.Organization, clusterID)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
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

				// Create assessment with different organization
				mockStore.assessments[assessmentID] = createTestAssessmentForEstimationHandler(assessmentID, user.Username, "other-org", clusterID)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
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
					service.NewAssessmentService(mockStore, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
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
					service.NewAssessmentService(mockStore, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
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
					service.NewAssessmentService(mockStore, nil),
					nil,
					nil,
					service.NewEstimationService(mockStore),
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
	})
})
