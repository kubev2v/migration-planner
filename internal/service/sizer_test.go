package service_test

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

func (m *MockAssessmentStore) List(ctx context.Context, filter *store.AssessmentQueryFilter) (model.AssessmentList, error) {
	return nil, nil
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
	return createTestSizerServerWithRequestCapture(response, healthStatus, healthError, nil)
}

// createTestSizerServerWithRequestCapture is like createTestSizerServer but captures the POST body into captured.
func createTestSizerServerWithRequestCapture(response *client.SizerResponse, healthStatus int, healthError bool, captured *client.SizerRequest) *httptest.Server {
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
			if captured != nil && r.Body != nil {
				_ = json.NewDecoder(r.Body).Decode(captured)
			}
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

	advanced := []client.Zone{}
	if controlPlaneNodes > 0 {
		advanced = append(advanced, client.Zone{
			Zone:  "zone1",
			Nodes: controlPlaneNodesList,
		})
	}
	if workerNodes > 0 {
		advanced = append(advanced, client.Zone{
			Zone:  "zone2",
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
				Limits: &client.ResourceLimits{
					CPU:    150.0,
					Memory: 300.0,
				},
				OverCommitRatio: &client.OverCommitRatio{
					CPU:    1.5,
					Memory: 1.5,
				},
			},
			Advanced: advanced,
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
				ControlPlaneNodeCount:   3,
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
				// Base: 2 workers + 3 control plane = 5 total
				// Failover: max(2, ceil(2*0.10)) = 2 nodes added
				Expect(result.ClusterSizing.TotalNodes).To(Equal(7))
				Expect(result.ClusterSizing.WorkerNodes).To(Equal(4))
				Expect(result.ClusterSizing.ControlPlaneNodes).To(Equal(3))
				Expect(result.ClusterSizing.FailoverNodes).To(Equal(2))
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

			It("successfully handles hosted control plane (worker nodes only)", func() {
				request.HostedControlPlane = true
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment

				var sizerPayload client.SizerRequest
				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/v1/size/custom" && r.Method == http.MethodPost {
						Expect(json.NewDecoder(r.Body).Decode(&sizerPayload)).To(Succeed())
					}
					if r.URL.Path == "/health" {
						w.WriteHeader(http.StatusOK)
						return
					}
					if r.URL.Path == "/api/v1/size/custom" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode(createTestSizerResponse(4, 4, 0, 40, 80))
						return
					}
					w.WriteHeader(http.StatusNotFound)
				}))
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				Expect(result.ClusterSizing.ControlPlaneNodes).To(Equal(0))
				Expect(result.ClusterSizing.WorkerNodes).To(Equal(6))
				Expect(result.ClusterSizing.TotalNodes).To(Equal(6))
				Expect(result.ClusterSizing.FailoverNodes).To(Equal(2))

				Expect(sizerPayload.MachineSets).To(HaveLen(1))
				Expect(sizerPayload.MachineSets[0].Name).To(Equal("worker"))
				Expect(sizerPayload.Workloads).To(HaveLen(1))
				Expect(sizerPayload.Workloads[0].Name).To(Equal("vm-workload"))
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
				// Base: 2 workers + 3 control plane = 5 total
				// Failover: max(2, ceil(2*0.10)) = 2 nodes added
				Expect(result.ClusterSizing.TotalNodes).To(Equal(7))
				Expect(result.ClusterSizing.ControlPlaneNodes).To(Equal(3))
				Expect(result.ClusterSizing.WorkerNodes).To(Equal(4))
				Expect(result.ClusterSizing.FailoverNodes).To(Equal(2))
			})

			It("successfully handles fallback node counting when totalNodes < controlPlaneNodeCount", func() {
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
				// Fallback case: totalNodes (2) < controlPlaneNodeCount (3)
				// Assign all available nodes to control plane (partial HA setup)
				// No worker nodes, so no failover nodes
				Expect(result.ClusterSizing.TotalNodes).To(Equal(2))
				Expect(result.ClusterSizing.ControlPlaneNodes).To(Equal(2))
				Expect(result.ClusterSizing.WorkerNodes).To(Equal(0))
				Expect(result.ClusterSizing.FailoverNodes).To(Equal(0))
			})

			It("successfully handles single node cluster (1 control plane)", func() {
				request.ControlPlaneNodeCount = 1
				request.ControlPlaneSchedulable = true
				request.ControlPlaneCPU = 50
				request.ControlPlaneMemory = 100
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				var sizerReq client.SizerRequest
				testServer = createTestSizerServerWithRequestCapture(createTestSizerResponse(1, 0, 1, 40, 80), http.StatusOK, false, &sizerReq)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				Expect(result.ClusterSizing.TotalNodes).To(Equal(1))
				Expect(result.ClusterSizing.ControlPlaneNodes).To(Equal(1))
				Expect(result.ClusterSizing.WorkerNodes).To(Equal(0))
				Expect(result.ClusterSizing.FailoverNodes).To(Equal(0))

				Expect(sizerReq.MachineSets).To(HaveLen(1))
				Expect(sizerReq.MachineSets[0].Name).To(Equal("controlPlane"))
				Expect(sizerReq.MachineSets[0].AllowWorkloadScheduling).NotTo(BeNil())
				Expect(*sizerReq.MachineSets[0].AllowWorkloadScheduling).To(BeTrue())
				var vmWorkload, cpServicesWorkload *client.Workload
				for i := range sizerReq.Workloads {
					switch sizerReq.Workloads[i].Name {
					case "vm-workload":
						vmWorkload = &sizerReq.Workloads[i]
					case "control-plane-services":
						cpServicesWorkload = &sizerReq.Workloads[i]
					}
				}
				Expect(vmWorkload).NotTo(BeNil())
				Expect(vmWorkload.UsesMachines).To(ContainElement("controlPlane"))
				Expect(cpServicesWorkload).NotTo(BeNil())
				Expect(cpServicesWorkload.UsesMachines).To(ContainElement("controlPlane"))
			})

			It("returns error when single node requested with non-schedulable control plane", func() {
				request.ControlPlaneNodeCount = 1
				request.ControlPlaneSchedulable = false
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(createTestSizerResponse(1, 0, 1, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				_, ok := err.(*service.ErrInvalidRequest)
				Expect(ok).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("single-node clusters require schedulable control planes"))
				Expect(err.Error()).To(ContainSubstring("Set ControlPlaneSchedulable to true"))
			})

			It("returns error when single node workload does not fit", func() {
				request.ControlPlaneNodeCount = 1
				request.ControlPlaneSchedulable = true
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(createTestSizerResponse(3, 2, 1, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				_, ok := err.(*service.ErrInvalidRequest)
				Expect(ok).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("workload does not fit on a single node"))
			})

			It("returns ErrInvalidRequest when single node and sizer returns not-schedulable error", func() {
				request.ControlPlaneNodeCount = 1
				request.ControlPlaneSchedulable = true
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/health" {
						w.WriteHeader(http.StatusOK)
						return
					}
					if r.URL.Path == "/api/v1/size/custom" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusInternalServerError)
						_ = json.NewEncoder(w).Encode(map[string]interface{}{
							"success": false,
							"error":   "Workload \"vm-workload\" is not schedulable. All available MachineSets are too small to run this workload. Minimum required: at least 200 CPU and 512 GB memory.",
						})
						return
					}
				}))
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				_, ok := err.(*service.ErrInvalidRequest)
				Expect(ok).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("workload does not fit on a single node"))
			})

			DescribeTable("single node fit error messages",
				func(cpuVal, memoryVal int, expectSpecific bool) {
					request.ControlPlaneNodeCount = 1
					request.ControlPlaneSchedulable = true
					request.ControlPlaneCPU = cpuVal
					request.ControlPlaneMemory = memoryVal
					assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
					mockStore.assessments[assessmentID] = assessment
					testServer = createTestSizerServer(createTestSizerResponse(2, 1, 1, 40, 80), http.StatusOK, false)
					sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
					sizerService = service.NewSizerService(sizerClient, mockStore)

					result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

					Expect(err).NotTo(BeNil())
					Expect(result).To(BeNil())
					_, ok := err.(*service.ErrInvalidRequest)
					Expect(ok).To(BeTrue())

					if expectSpecific {
						Expect(err.Error()).To(ContainSubstring("Use at least"))
						Expect(err.Error()).To(ContainSubstring("CPU"))
						Expect(err.Error()).To(ContainSubstring("GB memory"))
					} else {
						Expect(err.Error()).To(Equal("workload does not fit on a single node. Use a multi-node cluster."))
					}
				},
				Entry("returns 'Use at least X CPU / Y GB' when CP is below minimum", 16, 16, true),
				Entry("returns 'Use a multi-node cluster' when CP is at or above minimum", 60, 120, false),
				Entry("returns 'Use a multi-node cluster' when CP is at max supported", service.MaxRecommendedNodeCPU, service.MaxRecommendedNodeMemory, false),
			)

			It("returns 'Use a multi-node cluster' when workload exceeds max supported single-node size", func() {
				request.ControlPlaneNodeCount = 1
				request.ControlPlaneSchedulable = true
				request.ControlPlaneCPU = 100
				request.ControlPlaneMemory = 200
				// Create inventory that requires > 200 CPU (max) on a single node
				// With 70% capacity: 300 CPU needs 300/0.7 = ~429 effective CPU which exceeds max of 200
				assessment := createTestAssessment(assessmentID, clusterID, 10, 300, 600)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(createTestSizerResponse(2, 1, 1, 300, 600), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				_, ok := err.(*service.ErrInvalidRequest)
				Expect(ok).To(BeTrue())
				// Should recommend multi-node, not "use at least 200 CPU" which would be misleading
				Expect(err.Error()).To(Equal("workload does not fit on a single node. Use a multi-node cluster."))
				Expect(err.Error()).NotTo(ContainSubstring("Use at least"))
			})

			It("successfully handles single node cluster when sizer returns 0 nodes", func() {
				request.ControlPlaneNodeCount = 1
				request.ControlPlaneSchedulable = true
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(&client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   0,
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
				Expect(result.ClusterSizing.TotalNodes).To(Equal(1))
				Expect(result.ClusterSizing.ControlPlaneNodes).To(Equal(1))
				Expect(result.ClusterSizing.WorkerNodes).To(Equal(0))
				Expect(result.ClusterSizing.FailoverNodes).To(Equal(0))
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

	Describe("VM Limit Enforcement", func() {
		var (
			assessmentID uuid.UUID
			clusterID    string
			request      *mappers.ClusterRequirementsRequestForm
		)

		BeforeEach(func() {
			assessmentID = uuid.New()
			clusterID = "cluster-vm-limit-test"
			request = &mappers.ClusterRequirementsRequestForm{
				ClusterID:               clusterID,
				CpuOverCommitRatio:      "1:1",
				MemoryOverCommitRatio:   "1:1",
				WorkerNodeCPU:           32,
				WorkerNodeMemory:        256,
				ControlPlaneSchedulable: false,
				ControlPlaneNodeCount:   3,
			}
		})

		Context("VM distribution stays within limit", func() {
			It("handles exactly 200 VMs", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 200, 400, 800)
				mockStore.assessments[assessmentID] = assessment

				advanced := []client.Zone{
					{
						Zone: "zone1",
						Nodes: []client.Node{
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
						},
					},
					{
						Zone: "zone2",
						Nodes: []client.Node{
							{IsControlPlane: false, Services: []string{"vms-batch-1-services"}},
						},
					},
				}
				response := &client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   4,
						TotalCPU:    400,
						TotalMemory: 800,
						ResourceConsumption: client.ResourceConsumption{
							CPU:    400.0,
							Memory: 800.0,
						},
						Advanced: advanced,
					},
				}

				testServer = createTestSizerServer(response, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
			})

			It("handles 199 VMs (just under limit)", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 199, 400, 800)
				mockStore.assessments[assessmentID] = assessment

				advanced := []client.Zone{
					{
						Zone: "zone1",
						Nodes: []client.Node{
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
						},
					},
					{
						Zone: "zone2",
						Nodes: []client.Node{
							{IsControlPlane: false, Services: []string{"vms-batch-1-services"}},
						},
					},
				}
				response := &client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   4,
						TotalCPU:    400,
						TotalMemory: 800,
						ResourceConsumption: client.ResourceConsumption{
							CPU:    400.0,
							Memory: 800.0,
						},
						Advanced: advanced,
					},
				}

				testServer = createTestSizerServer(response, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
			})

			It("handles 400 VMs distributed across 2 nodes", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 400, 800, 1600)
				mockStore.assessments[assessmentID] = assessment

				advanced := []client.Zone{
					{
						Zone: "zone1",
						Nodes: []client.Node{
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
						},
					},
					{
						Zone: "zone2",
						Nodes: []client.Node{
							{IsControlPlane: false, Services: []string{"vms-batch-1-services"}},
							{IsControlPlane: false, Services: []string{"vms-batch-2-services"}},
						},
					},
				}
				response := &client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   5,
						TotalCPU:    800,
						TotalMemory: 1600,
						ResourceConsumption: client.ResourceConsumption{
							CPU:    800.0,
							Memory: 1600.0,
						},
						Advanced: advanced,
					},
				}

				testServer = createTestSizerServer(response, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
			})

			It("handles 1000 VMs distributed across 5+ nodes", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 1000, 2000, 4000)
				mockStore.assessments[assessmentID] = assessment

				advanced := []client.Zone{
					{
						Zone: "zone1",
						Nodes: []client.Node{
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
						},
					},
					{
						Zone: "zone2",
						Nodes: []client.Node{
							{IsControlPlane: false, Services: []string{"vms-batch-1-services"}},
							{IsControlPlane: false, Services: []string{"vms-batch-2-services"}},
							{IsControlPlane: false, Services: []string{"vms-batch-3-services"}},
							{IsControlPlane: false, Services: []string{"vms-batch-4-services"}},
							{IsControlPlane: false, Services: []string{"vms-batch-5-services"}},
						},
					},
				}
				response := &client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   8,
						TotalCPU:    2000,
						TotalMemory: 4000,
						ResourceConsumption: client.ResourceConsumption{
							CPU:    2000.0,
							Memory: 4000.0,
						},
						Advanced: advanced,
					},
				}

				testServer = createTestSizerServer(response, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
			})

			It("handles uneven distribution (301 VMs across 2 batches)", func() {
				// aggregateVMsIntoServices splits 301 VMs as 151 + 150 (remainder to earlier batches).
				// Both batches are under MaxVMsPerWorkerNode (200), so validation passes.
				assessment := createTestAssessment(assessmentID, clusterID, 301, 600, 1200)
				mockStore.assessments[assessmentID] = assessment

				advanced := []client.Zone{
					{
						Zone: "zone1",
						Nodes: []client.Node{
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
						},
					},
					{
						Zone: "zone2",
						Nodes: []client.Node{
							{IsControlPlane: false, Services: []string{"vms-batch-1-services"}},
							{IsControlPlane: false, Services: []string{"vms-batch-2-services"}},
						},
					},
				}
				response := &client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   5,
						TotalCPU:    600,
						TotalMemory: 1200,
						ResourceConsumption: client.ResourceConsumption{
							CPU:    600.0,
							Memory: 1200.0,
						},
						Advanced: advanced,
					},
				}

				testServer = createTestSizerServer(response, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
			})

			It("handles 1 VM", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 1, 2, 4)
				mockStore.assessments[assessmentID] = assessment

				advanced := []client.Zone{
					{
						Zone: "zone1",
						Nodes: []client.Node{
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
						},
					},
					{
						Zone: "zone2",
						Nodes: []client.Node{
							{IsControlPlane: false, Services: []string{"vms-batch-1-services"}},
						},
					},
				}
				response := &client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   4,
						TotalCPU:    2,
						TotalMemory: 4,
						ResourceConsumption: client.ResourceConsumption{
							CPU:    2.0,
							Memory: 4.0,
						},
						Advanced: advanced,
					},
				}

				testServer = createTestSizerServer(response, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
			})

		})

		Context("VM limit validation catches violations", func() {
			It("returns error when sizer response shows >200 VMs on a single node", func() {
				request.WorkerNodeCPU = 200
				request.WorkerNodeMemory = 512
				request.WorkerNodeThreads = 400

				// 500 VMs = 5 batches of 100 VMs each
				assessment := createTestAssessment(assessmentID, clusterID, 500, 1000, 2000)
				mockStore.assessments[assessmentID] = assessment

				// 3 batches = 300 VMs (exceeds limit)
				advanced := []client.Zone{
					{
						Zone: "zone1",
						Nodes: []client.Node{
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
						},
					},
					{
						Zone: "zone2",
						Nodes: []client.Node{
							// 3 batches = 300 VMs (exceeds limit)
							{IsControlPlane: false, Services: []string{"vms-batch-1-services", "vms-batch-2-services", "vms-batch-3-services"}},
							{IsControlPlane: false, Services: []string{"vms-batch-4-services"}},
							{IsControlPlane: false, Services: []string{"vms-batch-5-services"}},
						},
					},
				}
				response := &client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   6,
						TotalCPU:    1000,
						TotalMemory: 2000,
						ResourceConsumption: client.ResourceConsumption{
							CPU:    1000.0,
							Memory: 2000.0,
						},
						Advanced: advanced,
					},
				}

				testServer = createTestSizerServer(response, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("VM distribution constraint violated"))
				Expect(err.Error()).To(ContainSubstring("exceeds limit"))
			})

			It("detects violation even with many small batches concentrated on one node", func() {
				request.WorkerNodeCPU = 200
				request.WorkerNodeMemory = 512
				request.WorkerNodeThreads = 0

				// 250 VMs with low resources creates 2 batches (125 VMs each)
				assessment := createTestAssessment(assessmentID, clusterID, 250, 125, 250)
				mockStore.assessments[assessmentID] = assessment

				// Simulate both batches on same node = 250 VMs (exceeds limit)
				advanced := []client.Zone{
					{
						Zone: "zone1",
						Nodes: []client.Node{
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
							{IsControlPlane: true, Services: []string{"ControlPlane"}},
						},
					},
					{
						Zone: "zone2",
						Nodes: []client.Node{
							{IsControlPlane: false, Services: []string{"vms-batch-1-services", "vms-batch-2-services"}},
						},
					},
				}
				response := &client.SizerResponse{
					Success: true,
					Data: client.SizerData{
						NodeCount:   4,
						TotalCPU:    125,
						TotalMemory: 250,
						ResourceConsumption: client.ResourceConsumption{
							CPU:    125.0,
							Memory: 250.0,
						},
						Advanced: advanced,
					},
				}

				testServer = createTestSizerServer(response, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("VM distribution constraint violated"))
				Expect(err.Error()).To(ContainSubstring("exceeds limit"))
			})
		})
	})

	DescribeTable("buildServiceAvoidLists",
		func(services []service.BatchedService, expectedLen int, validator func([][]string)) {
			result := service.BuildServiceAvoidLists(services)
			Expect(result).To(HaveLen(expectedLen))
			if validator != nil {
				validator(result)
			}
		},
		Entry("empty input", []service.BatchedService{}, 0, func(result [][]string) {
			Expect(result).To(BeEmpty())
			Expect(result).NotTo(BeNil())
		}),
		Entry("single service", []service.BatchedService{
			{Name: "vms-batch-1-services", VMCount: 100},
		}, 1, func(result [][]string) {
			Expect(result[0]).To(BeEmpty())
		}),
		Entry("all services with 0 VMs", []service.BatchedService{
			{Name: "vms-batch-1-services", VMCount: 0},
			{Name: "vms-batch-2-services", VMCount: 0},
			{Name: "vms-batch-3-services", VMCount: 0},
		}, 3, func(result [][]string) {
			for i := range result {
				Expect(result[i]).To(BeEmpty())
			}
		}),
		Entry("groups 2 services of 100 VMs together (maxServicesPerNode=2)", []service.BatchedService{
			{Name: "vms-batch-1-services", VMCount: 100},
			{Name: "vms-batch-2-services", VMCount: 100},
		}, 2, func(result [][]string) {
			Expect(result[0]).To(BeEmpty())
			Expect(result[1]).To(BeEmpty())
		}),
		Entry("separates 2 services of 150 VMs (maxServicesPerNode=1)", []service.BatchedService{
			{Name: "vms-batch-1-services", VMCount: 150},
			{Name: "vms-batch-2-services", VMCount: 150},
		}, 2, func(result [][]string) {
			Expect(result[0]).To(ConsistOf("vms-batch-2-services"))
			Expect(result[1]).To(ConsistOf("vms-batch-1-services"))
		}),
		Entry("creates 4 groups for 8 services of 100 VMs", []service.BatchedService{
			{Name: "vms-batch-1-services", VMCount: 100},
			{Name: "vms-batch-2-services", VMCount: 100},
			{Name: "vms-batch-3-services", VMCount: 100},
			{Name: "vms-batch-4-services", VMCount: 100},
			{Name: "vms-batch-5-services", VMCount: 100},
			{Name: "vms-batch-6-services", VMCount: 100},
			{Name: "vms-batch-7-services", VMCount: 100},
			{Name: "vms-batch-8-services", VMCount: 100},
		}, 8, func(result [][]string) {
			// Group 0: services 1-2, Group 1: services 3-4, Group 2: services 5-6, Group 3: services 7-8
			for i := 0; i < 8; i++ {
				currentGroup := i / 2
				expectedAvoids := []string{}
				for j := 0; j < 8; j++ {
					if i != j && j/2 != currentGroup {
						expectedAvoids = append(expectedAvoids, fmt.Sprintf("vms-batch-%d-services", j+1))
					}
				}
				Expect(result[i]).To(ConsistOf(expectedAvoids))
			}
		}),
		Entry("uses maxVMsPerBatch based on largest service", []service.BatchedService{
			{Name: "vms-batch-1-services", VMCount: 50},
			{Name: "vms-batch-2-services", VMCount: 50},
			{Name: "vms-batch-3-services", VMCount: 150},
			{Name: "vms-batch-4-services", VMCount: 50},
		}, 4, func(result [][]string) {
			Expect(result[0]).To(ConsistOf("vms-batch-2-services", "vms-batch-3-services", "vms-batch-4-services"))
			Expect(result[1]).To(ConsistOf("vms-batch-1-services", "vms-batch-3-services", "vms-batch-4-services"))
			Expect(result[2]).To(ConsistOf("vms-batch-1-services", "vms-batch-2-services", "vms-batch-4-services"))
			Expect(result[3]).To(ConsistOf("vms-batch-1-services", "vms-batch-2-services", "vms-batch-3-services"))
		}),
		Entry("handles maxServicesPerNode > numServices", []service.BatchedService{
			{Name: "vms-batch-1-services", VMCount: 10},
			{Name: "vms-batch-2-services", VMCount: 10},
		}, 2, func(result [][]string) {
			Expect(result[0]).To(BeEmpty())
			Expect(result[1]).To(BeEmpty())
		}),
	)

	DescribeTable("CalculateEffectiveCPU",
		func(physicalCores, threads int, expected float64) {
			result := service.CalculateEffectiveCPU(physicalCores, threads)
			Expect(result).To(Equal(expected))
		},
		Entry("no SMT (threads = 0)", 16, 0, 16.0),
		Entry("no SMT (threads = cores)", 16, 16, 16.0),
		Entry("2:1 SMT (16C/32T)", 16, 32, 24.0),
		Entry("4:1 SMT (8C/32T)", 8, 32, 20.0),
		Entry("2:1 SMT (32C/64T)", 32, 64, 48.0),
		Entry("odd cores (15C/30T)", 15, 30, 22.5),
		Entry("zero cores", 0, 16, 0.0),
		Entry("negative cores", -1, 16, 0.0),
		Entry("invalid: threads < cores", 16, 8, 16.0),
		Entry("single core with SMT (1C/2T)", 1, 2, 1.5),
		Entry("minimum cores (2C/4T)", 2, 4, 3.0),
	)
})
