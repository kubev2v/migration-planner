package v1alpha1_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/client"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// MockStore is a mock implementation of store.Store
type MockStore struct {
	assessments map[uuid.UUID]*model.Assessment
	getError    error
}

func NewMockStore() *MockStore {
	return &MockStore{
		assessments: make(map[uuid.UUID]*model.Assessment),
	}
}

func (m *MockStore) Assessment() store.Assessment {
	return &MockAssessmentStore{store: m}
}

func (m *MockStore) Source() store.Source {
	panic("Source() not implemented in MockStore for this test")
}

func (m *MockStore) Agent() store.Agent {
	panic("Agent() not implemented in MockStore for this test")
}

func (m *MockStore) ImageInfra() store.ImageInfra {
	panic("ImageInfra() not implemented in MockStore for this test")
}

func (m *MockStore) Job() store.Job {
	panic("Job() not implemented in MockStore for this test")
}

func (m *MockStore) PrivateKey() store.PrivateKey {
	panic("PrivateKey() not implemented in MockStore for this test")
}

func (m *MockStore) Label() store.Label {
	panic("Label() not implemented in MockStore for this test")
}

func (m *MockStore) NewTransactionContext(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (m *MockStore) Statistics(ctx context.Context) (model.InventoryStats, error) {
	return model.InventoryStats{}, nil
}

func (m *MockStore) Close() error {
	return nil
}

type MockAssessmentStore struct {
	store *MockStore
}

func (m *MockAssessmentStore) Get(ctx context.Context, id uuid.UUID) (*model.Assessment, error) {
	if m.store.getError != nil {
		return nil, m.store.getError
	}
	assessment, exists := m.store.assessments[id]
	if !exists {
		return nil, store.ErrRecordNotFound
	}
	return assessment, nil
}

func (m *MockAssessmentStore) List(ctx context.Context, filter *store.AssessmentQueryFilter, options *store.AssessmentQueryOptions) (model.AssessmentList, error) {
	panic("List() not implemented in MockAssessmentStore for this test")
}

func (m *MockAssessmentStore) Count(ctx context.Context, filter *store.AssessmentQueryFilter) (int64, error) {
	panic("Count() not implemented in MockAssessmentStore for this test")
}

func (m *MockAssessmentStore) Create(ctx context.Context, assessment model.Assessment, inventory []byte) (*model.Assessment, error) {
	panic("Create() not implemented in MockAssessmentStore for this test")
}

func (m *MockAssessmentStore) Update(ctx context.Context, assessmentID uuid.UUID, name *string, inventory []byte) (*model.Assessment, error) {
	panic("Update() not implemented in MockAssessmentStore for this test")
}

func (m *MockAssessmentStore) Delete(ctx context.Context, id uuid.UUID) error {
	panic("Delete() not implemented in MockAssessmentStore for this test")
}

// createTestSizerServer creates an HTTP test server that mocks the sizer service
func createTestSizerServer(response *client.SizerResponse, healthStatus int, healthError bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			if healthError {
				w.WriteHeader(healthStatus)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/v1/size/custom" {
			if response == nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(response); err != nil {
				panic(fmt.Sprintf("failed to encode test response: %v", err))
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func createTestInventory(clusterID string, totalVMs, totalCPU, totalMemory int) []byte {
	inventory := api.Inventory{
		Clusters: map[string]api.InventoryData{
			clusterID: {
				Vms: api.VMs{
					Total: totalVMs,
					CpuCores: api.VMResourceBreakdown{
						Total: totalCPU,
					},
					RamGB: api.VMResourceBreakdown{
						Total: totalMemory,
					},
				},
			},
		},
	}
	data, err := json.Marshal(inventory)
	Expect(err).ToNot(HaveOccurred())
	return data
}

func createTestAssessment(id uuid.UUID, username, orgID, clusterID string) *model.Assessment {
	return &model.Assessment{
		ID:       id,
		Name:     "test-assessment",
		OrgID:    orgID,
		Username: username,
		Snapshots: []model.Snapshot{
			{
				ID:           1,
				CreatedAt:    time.Now(),
				Inventory:    createTestInventory(clusterID, 10, 40, 80),
				AssessmentID: id,
				Version:      2,
			},
		},
	}
}

func createTestSizerResponse(nodeCount, workerNodes, controlPlaneNodes, totalCPU, totalMemory int) *client.SizerResponse {
	// Build control plane nodes
	controlPlaneNodesList := make([]client.Node, controlPlaneNodes)
	for i := range controlPlaneNodesList {
		controlPlaneNodesList[i] = client.Node{IsControlPlane: true}
	}

	// Build worker nodes
	workerNodesList := make([]client.Node, workerNodes)
	for i := range workerNodesList {
		workerNodesList[i] = client.Node{IsControlPlane: false}
	}

	// Build Advanced zones
	advanced := []client.Zone{}
	if controlPlaneNodes > 0 {
		advanced = append(advanced, client.Zone{
			Zone:  "zone1",
			Nodes: controlPlaneNodesList,
		})
	}
	if workerNodes > 0 {
		zoneName := "zone1"
		if controlPlaneNodes > 0 {
			zoneName = "zone2"
		}
		advanced = append(advanced, client.Zone{
			Zone:  zoneName,
			Nodes: workerNodesList,
		})
	}

	return &client.SizerResponse{
		Success: true,
		Data: client.SizerData{
			NodeCount:   nodeCount,
			TotalCPU:    totalCPU,
			TotalMemory: totalMemory,
			ResourceConsumption: client.ResourceConsumption{
				CPU:    100.0,
				Memory: 200.0,
			},
			Advanced: advanced,
		},
	}
}

var _ = Describe("sizer handler", func() {
	var (
		mockStore    *MockStore
		testServer   *httptest.Server
		sizerClient  *client.SizerClient
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

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Describe("CalculateAssessmentClusterRequirements", func() {
		Context("successful requests", func() {
			It("successfully returns 200 with valid request", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      16,
				}

				mockStore.assessments[assessmentID] = createTestAssessment(assessmentID, user.Username, user.Organization, clusterID)
				testServer = createTestSizerServer(createTestSizerResponse(5, 2, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil, // sourceService
					service.NewAssessmentService(mockStore, nil),
					nil, // jobService
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				Expect(resp).NotTo(BeNil())
				// Check response type
				_, ok := resp.(server.CalculateAssessmentClusterRequirements200JSONResponse)
				Expect(ok).To(BeTrue())
			})

			It("returns response with all required fields", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      16,
				}

				mockStore.assessments[assessmentID] = createTestAssessment(assessmentID, user.Username, user.Organization, clusterID)
				testServer = createTestSizerServer(createTestSizerResponse(5, 2, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				Expect(resp).NotTo(BeNil())

				// Verify response type
				successResp, ok := resp.(server.CalculateAssessmentClusterRequirements200JSONResponse)
				Expect(ok).To(BeTrue())

				// Verify response contains all required fields
				Expect(successResp.ClusterSizing).NotTo(BeZero())
				Expect(successResp.ClusterSizing.TotalNodes).To(Equal(5))
				Expect(successResp.ClusterSizing.WorkerNodes).To(Equal(2))
				Expect(successResp.ClusterSizing.ControlPlaneNodes).To(Equal(3))
				Expect(successResp.ClusterSizing.TotalCPU).To(Equal(40))
				Expect(successResp.ClusterSizing.TotalMemory).To(Equal(80))

				Expect(successResp.InventoryTotals).NotTo(BeZero())
				Expect(successResp.InventoryTotals.TotalVMs).To(Equal(10))
				Expect(successResp.InventoryTotals.TotalCPU).To(Equal(40))
				Expect(successResp.InventoryTotals.TotalMemory).To(Equal(80))

				Expect(successResp.ResourceConsumption).NotTo(BeZero())
				Expect(successResp.ResourceConsumption.Cpu).To(Equal(100.0))
				Expect(successResp.ResourceConsumption.Memory).To(Equal(200.0))
			})
		})

		Context("validation errors", func() {
			It("returns 400 when request body is nil", func() {
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: nil,
				})

				Expect(err).To(BeNil())
				_, ok := resp.(server.CalculateAssessmentClusterRequirements400JSONResponse)
				Expect(ok).To(BeTrue())
			})

			It("returns 400 when clusterId is empty", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             "",
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      16,
				}

				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				errorResp, ok := resp.(server.CalculateAssessmentClusterRequirements400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(errorResp.Message).To(ContainSubstring("clusterId is required"))
			})

			It("accepts valid clusterId format", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      16,
				}

				mockStore.assessments[assessmentID] = createTestAssessment(assessmentID, user.Username, user.Organization, clusterID)
				testServer = createTestSizerServer(createTestSizerResponse(5, 2, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				_, ok := resp.(server.CalculateAssessmentClusterRequirements200JSONResponse)
				Expect(ok).To(BeTrue())
			})

			It("returns 400 when worker node CPU is zero", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         0,
					WorkerNodeMemory:      16,
				}

				mockStore.assessments[assessmentID] = createTestAssessment(assessmentID, user.Username, user.Organization, clusterID)
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				errorResp, ok := resp.(server.CalculateAssessmentClusterRequirements400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(errorResp.Message).To(ContainSubstring("worker node size must be greater than zero"))
			})

			It("returns 400 when worker node memory is zero", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      0,
				}

				mockStore.assessments[assessmentID] = createTestAssessment(assessmentID, user.Username, user.Organization, clusterID)
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				errorResp, ok := resp.(server.CalculateAssessmentClusterRequirements400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(errorResp.Message).To(ContainSubstring("worker node size must be greater than zero"))
			})

			It("returns 400 when CPU over-commit ratio is invalid", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.ClusterRequirementsRequestCpuOverCommitRatio("1:3"),
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      16,
				}

				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				errorResp, ok := resp.(server.CalculateAssessmentClusterRequirements400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(errorResp.Message).To(ContainSubstring("invalid CPU over-commit ratio"))
				Expect(errorResp.Message).To(ContainSubstring("1:3"))
			})

			It("returns 400 when memory over-commit ratio is invalid", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.ClusterRequirementsRequestMemoryOverCommitRatio("1:6"),
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      16,
				}

				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				errorResp, ok := resp.(server.CalculateAssessmentClusterRequirements400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(errorResp.Message).To(ContainSubstring("invalid memory over-commit ratio"))
				Expect(errorResp.Message).To(ContainSubstring("1:6"))
			})
		})

		Context("not found errors", func() {
			It("returns 404 when assessment not found", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      16,
				}

				// Don't add assessment to mockStore, so it will be not found
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				errorResp, ok := resp.(server.CalculateAssessmentClusterRequirements404JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(errorResp.Message).NotTo(BeEmpty())
			})
		})

		Context("authorization errors", func() {
			It("returns 403 when user doesn't own assessment (different username)", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      16,
				}

				mockStore.assessments[assessmentID] = createTestAssessment(assessmentID, "different-user", user.Organization, clusterID)
				// Create handler with mockStore
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				errorResp, ok := resp.(server.CalculateAssessmentClusterRequirements403JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(errorResp.Message).To(ContainSubstring("forbidden"))
			})

			It("returns 403 when user doesn't own assessment (different organization)", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      16,
				}

				mockStore.assessments[assessmentID] = createTestAssessment(assessmentID, user.Username, "different-org", clusterID)
				// Create handler with mockStore
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				errorResp, ok := resp.(server.CalculateAssessmentClusterRequirements403JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(errorResp.Message).To(ContainSubstring("forbidden"))
			})

			It("allows access when user owns assessment", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      16,
				}

				mockStore.assessments[assessmentID] = createTestAssessment(assessmentID, user.Username, user.Organization, clusterID)
				testServer = createTestSizerServer(createTestSizerResponse(5, 2, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				_, ok := resp.(server.CalculateAssessmentClusterRequirements200JSONResponse)
				Expect(ok).To(BeTrue())
			})
		})

		Context("service errors", func() {
			It("returns 503 when sizer service health check fails", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      16,
				}

				mockStore.assessments[assessmentID] = createTestAssessment(assessmentID, user.Username, user.Organization, clusterID)
				testServer = createTestSizerServer(nil, http.StatusServiceUnavailable, true)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				errorResp, ok := resp.(server.CalculateAssessmentClusterRequirements503JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(errorResp.Message).To(ContainSubstring("sizer service unavailable"))
			})
		})

		Context("internal errors", func() {
			It("returns 500 when assessment service returns non-NotFound error", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      16,
				}

				mockStore.getError = errors.New("database error")
				// Create handler with mockStore that has getError set
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				errorResp, ok := resp.(server.CalculateAssessmentClusterRequirements500JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(errorResp.Message).To(ContainSubstring("failed to get assessment"))
			})

			It("returns 400 when cluster has invalid inventory", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      16,
				}

				// Create assessment with empty cluster (0 VMs, 0 CPU, 0 Memory)
				assessment := createTestAssessment(assessmentID, user.Username, user.Organization, clusterID)
				// Override inventory to have empty cluster
				assessment.Snapshots[0].Inventory = []byte(`{"clusters":{"` + clusterID + `":{"vms":{"total":0,"cpu_cores":{"total":0},"ram_gb":{"total":0}}}}}`)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				errorResp, ok := resp.(server.CalculateAssessmentClusterRequirements400JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(errorResp.Message).To(ContainSubstring("invalid inventory"))
			})

			It("returns 500 when CalculateClusterRequirements returns error", func() {
				request := &api.ClusterRequirementsRequest{
					ClusterId:             clusterID,
					CpuOverCommitRatio:    api.CpuOneToFour,
					MemoryOverCommitRatio: api.MemoryOneToTwo,
					WorkerNodeCPU:         8,
					WorkerNodeMemory:      16,
				}

				mockStore.assessments[assessmentID] = createTestAssessment(assessmentID, user.Username, user.Organization, clusterID)
				testServer = createTestSizerServer(nil, http.StatusInternalServerError, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				handler = handlers.NewServiceHandler(
					nil,
					service.NewAssessmentService(mockStore, nil),
					nil,
					service.NewSizerService(sizerClient, mockStore),
				)

				resp, err := handler.CalculateAssessmentClusterRequirements(ctx, server.CalculateAssessmentClusterRequirementsRequestObject{
					Id:   assessmentID,
					Body: request,
				})

				Expect(err).To(BeNil())
				errorResp, ok := resp.(server.CalculateAssessmentClusterRequirements500JSONResponse)
				Expect(ok).To(BeTrue())
				Expect(errorResp.Message).To(ContainSubstring("failed to calculate cluster requirements"))
			})
		})
	})
})
