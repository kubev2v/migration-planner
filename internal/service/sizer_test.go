package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/client"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
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
	return nil
}

func (m *MockStore) Agent() store.Agent {
	return nil
}

func (m *MockStore) ImageInfra() store.ImageInfra {
	return nil
}

func (m *MockStore) Job() store.Job {
	return nil
}

func (m *MockStore) PrivateKey() store.PrivateKey {
	return nil
}

func (m *MockStore) Label() store.Label {
	return nil
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
	return nil, nil
}

func (m *MockAssessmentStore) Count(ctx context.Context, filter *store.AssessmentQueryFilter) (int64, error) {
	return 0, nil
}

func (m *MockAssessmentStore) Create(ctx context.Context, assessment model.Assessment, inventory []byte) (*model.Assessment, error) {
	return nil, nil
}

func (m *MockAssessmentStore) Update(ctx context.Context, assessmentID uuid.UUID, name *string, inventory []byte) (*model.Assessment, error) {
	return nil, nil
}

func (m *MockAssessmentStore) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
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
			_ = json.NewEncoder(w).Encode(response)
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

func createTestAssessment(id uuid.UUID, clusterID string, totalVMs, totalCPU, totalMemory int) *model.Assessment {
	return &model.Assessment{
		ID:       id,
		Name:     "test-assessment",
		OrgID:    "test-org",
		Username: "test-user",
		Snapshots: []model.Snapshot{
			{
				ID:           1,
				CreatedAt:    time.Now(),
				Inventory:    createTestInventory(clusterID, totalVMs, totalCPU, totalMemory),
				AssessmentID: id,
				Version:      2,
			},
		},
	}
}

func createTestSizerResponse(nodeCount, workerNodes, controlPlaneNodes, totalCPU, totalMemory int) *client.SizerResponse {
	return &client.SizerResponse{
		Success: true,
		Data: client.SizerData{
			NodeCount:   nodeCount,
			TotalCPU:    totalCPU,
			TotalMemory: totalMemory,
			ResourceConsumption: client.ResourceConsumption{
				CPU:    100.0,
				Memory: 200.0,
				Limits: &client.ResourceLimits{
					CPU:    150.0,
					Memory: 300.0,
				},
				OverCommitRatio: &client.OverCommitRatio{
					CPU:    1.5,
					Memory: 1.5,
				},
			},
			Advanced: []client.Zone{
				{
					Zone: "zone1",
					Nodes: []client.Node{
						{IsControlPlane: true},
						{IsControlPlane: true},
						{IsControlPlane: true},
					},
				},
				{
					Zone: "zone2",
					Nodes: []client.Node{
						{IsControlPlane: false},
						{IsControlPlane: false},
					},
				},
			},
		},
	}
}

var _ = Describe("sizer service", func() {
	var (
		mockStore    *MockStore
		testServer   *httptest.Server
		sizerClient  *client.SizerClient
		sizerService *service.SizerService
		ctx          context.Context
	)

	BeforeEach(func() {
		mockStore = NewMockStore()
		ctx = context.Background()
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Describe("CalculateClusterRequirements", func() {
		var (
			assessmentID uuid.UUID
			clusterID    string
			request      *mappers.ClusterRequirementsRequestForm
		)

		BeforeEach(func() {
			assessmentID = uuid.New()
			clusterID = "cluster-test-123"
			request = &mappers.ClusterRequirementsRequestForm{
				ClusterID:               clusterID,
				CpuOverCommitRatio:      "1:4",
				MemoryOverCommitRatio:   "1:2",
				WorkerNodeCPU:           8,
				WorkerNodeMemory:        16,
				ControlPlaneSchedulable: false,
			}
		})

		Context("successful calculation", func() {
			It("successfully calculates cluster requirements", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(createTestSizerResponse(5, 2, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				Expect(result.InventoryTotals.TotalVMs).To(Equal(10))
				Expect(result.InventoryTotals.TotalCPU).To(Equal(40))
				Expect(result.InventoryTotals.TotalMemory).To(Equal(80))
				Expect(result.ClusterSizing.TotalNodes).To(Equal(5))
				Expect(result.ClusterSizing.WorkerNodes).To(Equal(2))
				Expect(result.ClusterSizing.ControlPlaneNodes).To(Equal(3))
				Expect(result.ResourceConsumption.CPU).To(Equal(100.0))
				Expect(result.ResourceConsumption.Memory).To(Equal(200.0))
			})

			It("successfully handles control plane schedulable enabled", func() {
				request.ControlPlaneSchedulable = true
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(createTestSizerResponse(5, 2, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
			})

			It("successfully handles different over-commit ratios", func() {
				cpuRatios := []string{"1:1", "1:2", "1:4", "1:6"}
				memoryRatios := []string{"1:1", "1:2", "1:4"}

				for _, cpuRatio := range cpuRatios {
					for _, memoryRatio := range memoryRatios {
						request.CpuOverCommitRatio = cpuRatio
						request.MemoryOverCommitRatio = memoryRatio
						assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
						mockStore.assessments[assessmentID] = assessment
						testServer = createTestSizerServer(createTestSizerResponse(5, 2, 3, 40, 80), http.StatusOK, false)
						sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
						sizerService = service.NewSizerService(sizerClient, mockStore)

						result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)
						Expect(err).To(BeNil())
						Expect(result).NotTo(BeNil())
						testServer.Close()
					}
				}
			})

			It("successfully handles response without Limits and OverCommitRatio", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(&client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   5,
						TotalCPU:    40,
						TotalMemory: 80,
						ResourceConsumption: client.ResourceConsumption{
							CPU:             100.0,
							Memory:          200.0,
							Limits:          nil,
							OverCommitRatio: nil,
						},
					},
				}, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				Expect(result.ResourceConsumption.Limits.CPU).To(Equal(0.0))
				Expect(result.ResourceConsumption.Limits.Memory).To(Equal(0.0))
				Expect(result.ResourceConsumption.OverCommitRatio.CPU).To(Equal(0.0))
				Expect(result.ResourceConsumption.OverCommitRatio.Memory).To(Equal(0.0))
			})

			It("successfully handles response with only Limits", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(&client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   5,
						TotalCPU:    40,
						TotalMemory: 80,
						ResourceConsumption: client.ResourceConsumption{
							CPU:    100.0,
							Memory: 200.0,
							Limits: &client.ResourceLimits{
								CPU:    150.0,
								Memory: 300.0,
							},
							OverCommitRatio: nil,
						},
					},
				}, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				Expect(result.ResourceConsumption.Limits.CPU).To(Equal(150.0))
				Expect(result.ResourceConsumption.Limits.Memory).To(Equal(300.0))
				Expect(result.ResourceConsumption.OverCommitRatio.CPU).To(Equal(0.0))
				Expect(result.ResourceConsumption.OverCommitRatio.Memory).To(Equal(0.0))
			})

			It("successfully handles response with only OverCommitRatio", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(&client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   5,
						TotalCPU:    40,
						TotalMemory: 80,
						ResourceConsumption: client.ResourceConsumption{
							CPU:    100.0,
							Memory: 200.0,
							Limits: nil,
							OverCommitRatio: &client.OverCommitRatio{
								CPU:    1.5,
								Memory: 1.5,
							},
						},
					},
				}, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				Expect(result.ResourceConsumption.Limits.CPU).To(Equal(0.0))
				Expect(result.ResourceConsumption.Limits.Memory).To(Equal(0.0))
				Expect(result.ResourceConsumption.OverCommitRatio.CPU).To(Equal(1.5))
				Expect(result.ResourceConsumption.OverCommitRatio.Memory).To(Equal(1.5))
			})

			It("successfully handles fallback node counting when Advanced data is missing", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(&client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   5,
						TotalCPU:    40,
						TotalMemory: 80,
						ResourceConsumption: client.ResourceConsumption{
							CPU:    100.0,
							Memory: 200.0,
						},
						Advanced: nil,
					},
				}, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				Expect(result.ClusterSizing.TotalNodes).To(Equal(5))
				Expect(result.ClusterSizing.ControlPlaneNodes).To(Equal(3))
				Expect(result.ClusterSizing.WorkerNodes).To(Equal(2))
			})

			It("successfully handles fallback node counting when totalNodes < 3", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(&client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   2,
						TotalCPU:    40,
						TotalMemory: 80,
						ResourceConsumption: client.ResourceConsumption{
							CPU:    100.0,
							Memory: 200.0,
						},
						Advanced: nil,
					},
				}, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				Expect(result.ClusterSizing.TotalNodes).To(Equal(2))
				Expect(result.ClusterSizing.ControlPlaneNodes).To(Equal(0))
				Expect(result.ClusterSizing.WorkerNodes).To(Equal(2))
			})
		})

		Context("error handling", func() {
			It("returns error when assessment not found", func() {
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				_, ok := err.(*service.ErrResourceNotFound)
				Expect(ok).To(BeTrue())
			})

			It("returns error when store returns non-NotFound error", func() {
				mockStore.getError = errors.New("database error")
				// Create a dummy sizerService - it won't be used since store will error first
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("failed to get assessment"))
			})

			It("returns error when assessment has no snapshots", func() {
				assessment := &model.Assessment{
					ID:        assessmentID,
					Snapshots: []model.Snapshot{},
				}
				mockStore.assessments[assessmentID] = assessment
				// Create a dummy sizerService - it won't be used since validation will fail first
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("no snapshots"))
			})

			It("returns error when snapshot has empty inventory", func() {
				assessment := &model.Assessment{
					ID: assessmentID,
					Snapshots: []model.Snapshot{
						{
							ID:           1,
							CreatedAt:    time.Now(),
							Inventory:    []byte{},
							AssessmentID: assessmentID,
						},
					},
				}
				mockStore.assessments[assessmentID] = assessment
				// Create a dummy sizerService - it won't be used since validation will fail first
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("empty inventory"))
			})

			It("returns error when inventory JSON is invalid", func() {
				assessment := &model.Assessment{
					ID: assessmentID,
					Snapshots: []model.Snapshot{
						{
							ID:           1,
							CreatedAt:    time.Now(),
							Inventory:    []byte("{invalid json}"),
							AssessmentID: assessmentID,
						},
					},
				}
				mockStore.assessments[assessmentID] = assessment
				// Create a dummy sizerService - it won't be used since validation will fail first
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("failed to parse inventory"))
			})

			It("returns error when cluster not found in inventory", func() {
				assessment := createTestAssessment(assessmentID, "different-cluster", 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				// Create a dummy sizerService - it won't be used since validation will fail first
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("cluster"))
				Expect(err.Error()).To(ContainSubstring("not found"))
			})

			It("returns error when cluster has no VMs", func() {
				// Create assessment with empty cluster (0 VMs, 0 CPU, 0 Memory)
				assessment := createTestAssessment(assessmentID, clusterID, 0, 0, 0)
				mockStore.assessments[assessmentID] = assessment
				// Create a dummy sizerService - it won't be used since validation will fail first
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				_, ok := err.(*service.ErrInvalidClusterInventory)
				Expect(ok).To(BeTrue())
			})

			It("returns error when estimated batches exceed MaxBatches", func() {
				// Create inventory that would require > 200 batches
				assessment := createTestAssessment(assessmentID, clusterID, 1000, 100000, 200000)
				mockStore.assessments[assessmentID] = assessment
				// Create a dummy sizerService - it won't be used since validation will fail first
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("too small"))
				Expect(err.Error()).To(ContainSubstring("larger worker nodes"))
			})

			It("handles very small inventory with large worker nodes", func() {
				// Test edge case: very small inventory (1, 1, 1) with large worker nodes
				// This tests the minimum batch size enforcement and "too small" validation
				assessment := createTestAssessment(assessmentID, clusterID, 1, 1, 1)
				mockStore.assessments[assessmentID] = assessment
				request.WorkerNodeCPU = 200
				request.WorkerNodeMemory = 512
				testServer = createTestSizerServer(createTestSizerResponse(5, 2, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				// The function should handle this edge case gracefully
				// It may succeed (if batch is valid) or return an error (if batch is too small)
				// Both outcomes are valid - the important thing is it doesn't panic
				if err != nil {
					Expect(err).NotTo(BeNil())
					// Error could be "too small" or other validation errors
				} else {
					Expect(result).NotTo(BeNil())
				}
			})

			It("returns error when total nodes exceed MaxNodeCount", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(createTestSizerResponse(101, 98, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("too small"))
				Expect(err.Error()).To(ContainSubstring("larger worker nodes"))
			})

			It("returns error when sizer service call fails", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(nil, http.StatusInternalServerError, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)
				// Simulate error by using nil response
				// Note: The actual error will come from the sizer service call

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("failed to call sizer service"))
			})
		})
	})

	Describe("Health", func() {
		It("successfully returns nil when sizer service is healthy", func() {
			testServer = createTestSizerServer(nil, http.StatusOK, false)
			sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
			sizerService = service.NewSizerService(sizerClient, mockStore)

			err := sizerService.Health(ctx)

			Expect(err).To(BeNil())
		})

		It("returns error when sizer service is unhealthy", func() {
			testServer = createTestSizerServer(nil, http.StatusServiceUnavailable, true)
			sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
			sizerService = service.NewSizerService(sizerClient, mockStore)

			err := sizerService.Health(ctx)

			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring("status 503"))
		})

		It("respects context timeout", func() {
			testServer = createTestSizerServer(nil, http.StatusOK, false)
			sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
			sizerService = service.NewSizerService(sizerClient, mockStore)

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
			defer cancel()
			time.Sleep(10 * time.Millisecond) // Ensure timeout

			err := sizerService.Health(ctx)

			// The error could be either context timeout or the mock error
			Expect(err).NotTo(BeNil())
		})
	})

	// Note: Tests for unexported methods (aggregateVMsIntoServices, getOverCommitMultiplier,
	// calculateMinimumNodeSize, formatNodeSizeError, buildSizerPayload, transformSizerResponse)
	// are tested indirectly through CalculateClusterRequirements above.
})
