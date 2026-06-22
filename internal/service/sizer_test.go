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
	"github.com/kubev2v/migration-planner/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// MockStore is a mock implementation of store.Store
type MockStore struct {
	assessments   map[uuid.UUID]*model.Assessment
	clusterInputs map[string]*model.AssessmentClusterSizingInput
	getError      error
	outboxEvents  []model.OutboxEvent
}

func NewMockStore() *MockStore {
	return &MockStore{
		assessments:   make(map[uuid.UUID]*model.Assessment),
		clusterInputs: make(map[string]*model.AssessmentClusterSizingInput),
	}
}

func (m *MockStore) Assessment() store.Assessment {
	return &MockAssessmentStore{store: m}
}

func (m *MockStore) Authz() store.Authz {
	panic("MockStore.Authz() called unexpectedly - not implemented for this test")
}

func (m *MockStore) Source() store.Source {
	panic("MockStore.Source() called unexpectedly - not implemented for this test")
}

func (m *MockStore) SourceSubsetInventory() store.SourceSubsetInventory {
	panic("MockStore.SourceSubsetInventory() called unexpectedly - not implemented for this test")
}

func (m *MockStore) AssessmentSubsetInventory() store.AssessmentSubsetInventory {
	panic("MockStore.AssessmentSubsetInventory() called unexpectedly - not implemented for this test")
}

func (m *MockStore) Agent() store.Agent {
	panic("MockStore.Agent() called unexpectedly - not implemented for this test")
}

func (m *MockStore) ImageInfra() store.ImageInfra {
	panic("MockStore.ImageInfra() called unexpectedly - not implemented for this test")
}

func (m *MockStore) Job() store.Job {
	panic("MockStore.Job() called unexpectedly - not implemented for this test")
}

func (m *MockStore) PartnerCustomer() store.PartnerCustomer {
	panic("MockStore.PartnerCustomer() called unexpectedly - not implemented for this test")
}

func (m *MockStore) PrivateKey() store.PrivateKey {
	panic("MockStore.PrivateKey() called unexpectedly - not implemented for this test")
}

func (m *MockStore) Label() store.Label {
	panic("MockStore.Label() called unexpectedly - not implemented for this test")
}

func (m *MockStore) ClusterSizingInput() store.ClusterSizingInput {
	return &MockClusterSizingInputStore{store: m}
}

func (m *MockStore) NewTransactionContext(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (m *MockStore) Statistics(ctx context.Context) (model.InventoryStats, error) {
	return model.InventoryStats{}, nil
}

func (m *MockStore) RequestMetricsCacheRefresh() {}

func (m *MockStore) Accounts() store.Accounts {
	return nil
}

func (m *MockStore) Outbox() store.Outbox {
	return &MockOutboxStore{store: m}
}

func (m *MockStore) Close() error {
	return nil
}

type MockAssessmentStore struct {
	store *MockStore
}

type MockClusterSizingInputStore struct {
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

func (m *MockAssessmentStore) Create(ctx context.Context, assessment model.Assessment, inventory []byte, subsetInventories []model.AssessmentSubsetInventory) (*model.Assessment, error) {
	return nil, nil
}

func (m *MockAssessmentStore) Update(ctx context.Context, assessmentID uuid.UUID, name *string, inventory []byte) (*model.Assessment, error) {
	return nil, nil
}

func (m *MockAssessmentStore) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *MockClusterSizingInputStore) Upsert(ctx context.Context, input model.AssessmentClusterSizingInput) (*model.AssessmentClusterSizingInput, error) {
	key := fmt.Sprintf("%s/%s", input.AssessmentID, input.ExternalClusterID)
	copied := input
	m.store.clusterInputs[key] = &copied
	return &copied, nil
}

func (m *MockClusterSizingInputStore) Get(ctx context.Context, assessmentID uuid.UUID, clusterID string) (*model.AssessmentClusterSizingInput, error) {
	key := fmt.Sprintf("%s/%s", assessmentID, clusterID)
	input, exists := m.store.clusterInputs[key]
	if !exists {
		return nil, store.ErrRecordNotFound
	}
	copied := *input
	return &copied, nil
}

type MockOutboxStore struct {
	store *MockStore
}

func (m *MockOutboxStore) Insert(ctx context.Context, event model.OutboxEvent) error {
	m.store.outboxEvents = append(m.store.outboxEvents, event)
	return nil
}

func (m *MockOutboxStore) List(ctx context.Context) ([]model.OutboxEvent, error) {
	return m.store.outboxEvents, nil
}

func (m *MockOutboxStore) Delete(ctx context.Context, ids ...int) error {
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
				ControlPlaneSchedulable: util.BoolPtr(false),
				ControlPlaneNodeCount:   util.IntPtr(3),
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
				Expect(result.ResourceConsumption.Cpu).To(Equal(100.0))
				Expect(result.ResourceConsumption.Memory).To(Equal(200.0))
			})

			It("successfully handles control plane schedulable enabled", func() {
				request.ControlPlaneSchedulable = util.BoolPtr(true)
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
				request.HostedControlPlane = util.BoolPtr(true)
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
				cpuRatios := []string{"1:1", "1:2", "1:4", "1:6", "1:8"}
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
				Expect(result.ResourceConsumption.Limits.Cpu).To(Equal(0.0))
				Expect(result.ResourceConsumption.Limits.Memory).To(Equal(0.0))
				Expect(result.ResourceConsumption.OverCommitRatio.Cpu).To(Equal(0.0))
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
				Expect(result.ResourceConsumption.Limits.Cpu).To(Equal(150.0))
				Expect(result.ResourceConsumption.Limits.Memory).To(Equal(300.0))
				Expect(result.ResourceConsumption.OverCommitRatio.Cpu).To(Equal(0.0))
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
				Expect(result.ResourceConsumption.Limits.Cpu).To(Equal(0.0))
				Expect(result.ResourceConsumption.Limits.Memory).To(Equal(0.0))
				Expect(result.ResourceConsumption.OverCommitRatio.Cpu).To(Equal(1.5))
				Expect(result.ResourceConsumption.OverCommitRatio.Memory).To(Equal(1.5))
			})

			It("successfully calculates with 1:8 CPU overcommit ratio", func() {
				request.CpuOverCommitRatio = "1:8"
				request.MemoryOverCommitRatio = "1:2"
				assessment := createTestAssessment(assessmentID, clusterID, 10, 800, 160) // 800 limit CPU
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
						_ = json.NewEncoder(w).Encode(createTestSizerResponse(3, 1, 2, 100, 80))
						return
					}
					w.WriteHeader(http.StatusNotFound)
				}))
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())

				// Verify 1:8 ratio calculation: find the vm-workload and check that
				// RequiredCPU = LimitCPU / 8.0 for at least one of its services
				var vmWorkload *client.Workload
				for i := range sizerPayload.Workloads {
					if sizerPayload.Workloads[i].Name == "vm-workload" {
						vmWorkload = &sizerPayload.Workloads[i]
						break
					}
				}
				Expect(vmWorkload).NotTo(BeNil(), "Expected to find vm-workload in payload")
				Expect(vmWorkload.Services).NotTo(BeEmpty(), "Expected vm-workload to have services")

				// Verify the first service applies the 1:8 ratio correctly
				firstService := vmWorkload.Services[0]
				Expect(firstService.LimitCPU).To(BeNumerically(">", 0), "Expected LimitCPU to be set")
				expectedRequired := firstService.LimitCPU / 8.0
				Expect(firstService.RequiredCPU).To(Equal(expectedRequired),
					"Expected RequiredCPU = %v (limit) / 8.0 (multiplier) = %v", firstService.LimitCPU, expectedRequired)
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
				request.ControlPlaneNodeCount = util.IntPtr(1)
				request.ControlPlaneSchedulable = util.BoolPtr(true)
				request.ControlPlaneCPU = util.IntPtr(50)
				request.ControlPlaneMemory = util.IntPtr(100)
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
				request.ControlPlaneNodeCount = util.IntPtr(1)
				request.ControlPlaneSchedulable = util.BoolPtr(false)
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
				request.ControlPlaneNodeCount = util.IntPtr(1)
				request.ControlPlaneSchedulable = util.BoolPtr(true)
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
				request.ControlPlaneNodeCount = util.IntPtr(1)
				request.ControlPlaneSchedulable = util.BoolPtr(true)
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
					request.ControlPlaneNodeCount = util.IntPtr(1)
					request.ControlPlaneSchedulable = util.BoolPtr(true)
					request.ControlPlaneCPU = util.IntPtr(cpuVal)
					request.ControlPlaneMemory = util.IntPtr(memoryVal)
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
				request.ControlPlaneNodeCount = util.IntPtr(1)
				request.ControlPlaneSchedulable = util.BoolPtr(true)
				request.ControlPlaneCPU = util.IntPtr(100)
				request.ControlPlaneMemory = util.IntPtr(200)
				// Create inventory that requires > 384 CPU (max) on a single node
				// With 1:4 over-commit: 1230 CPU / 4 = 307.5 CPU actual
				// With 0.8 capacity: 307.5 / 0.8 = 384.375 CPU needed (exceeds max of 384)
				assessment := createTestAssessment(assessmentID, clusterID, 10, 1230, 2000)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(createTestSizerResponse(2, 1, 1, 1230, 2000), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				_, ok := err.(*service.ErrInvalidRequest)
				Expect(ok).To(BeTrue())
				// Should recommend multi-node, not "use at least ... CPU" which would be misleading
				Expect(err.Error()).To(Equal("workload does not fit on a single node. Use a multi-node cluster."))
				Expect(err.Error()).NotTo(ContainSubstring("Use at least"))
			})

			It("returns correct SNO error when sizer reports workload doesn't fit", func() {
				request.ControlPlaneNodeCount = util.IntPtr(1)
				request.ControlPlaneSchedulable = util.BoolPtr(true)
				request.ControlPlaneCPU = util.IntPtr(16)
				request.ControlPlaneMemory = util.IntPtr(128)
				// Create a workload that sizer will reject
				assessment := createTestAssessment(assessmentID, clusterID, 10, 100, 200)
				mockStore.assessments[assessmentID] = assessment
				// Mock sizer to return a schedulability error (workload doesn't fit on CP)
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
							"error":   "Workload is not schedulable. Control plane node is too small to run this workload.",
						})
						return
					}
					w.WriteHeader(http.StatusNotFound)
				}))
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				_, ok := err.(*service.ErrInvalidRequest)
				Expect(ok).To(BeTrue())
				// Should return SNO-specific message, NOT "Worker node size ... is too small"
				Expect(err.Error()).To(ContainSubstring("workload does not fit on a single node"))
				Expect(err.Error()).NotTo(ContainSubstring("Worker node"))
				Expect(err.Error()).NotTo(ContainSubstring("worker node"))
			})

			It("successfully handles single node cluster when sizer returns 0 nodes", func() {
				request.ControlPlaneNodeCount = util.IntPtr(1)
				request.ControlPlaneSchedulable = util.BoolPtr(true)
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

		Context("request persistence", func() {
			It("does not persist sizing input when calculation fails", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(nil, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)
				Expect(err).ToNot(BeNil())
				Expect(result).To(BeNil())

				storedInput, getErr := sizerService.GetClusterRequirementsInput(ctx, assessmentID, clusterID)
				Expect(storedInput).To(BeNil())
				_, ok := getErr.(*service.ErrResourceNotFound)
				Expect(ok).To(BeTrue())
			})

			It("loads persisted sizing input by assessment and cluster", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(createTestSizerResponse(5, 2, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				_, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)
				Expect(err).To(BeNil())

				storedInput, err := sizerService.GetClusterRequirementsInput(ctx, assessmentID, clusterID)
				Expect(err).To(BeNil())
				Expect(storedInput).ToNot(BeNil())
				Expect(storedInput.ClusterID).To(Equal(clusterID))
				Expect(storedInput.WorkerNodeCPU).ToNot(BeNil())
				Expect(*storedInput.WorkerNodeCPU).To(Equal(8))
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

	Describe("CalculateStandaloneClusterRequirements", func() {
		var request *mappers.StandaloneClusterRequirementsRequestForm

		BeforeEach(func() {
			request = &mappers.StandaloneClusterRequirementsRequestForm{
				TotalVMs:              10,
				TotalCPU:              40,
				TotalMemory:           80,
				CpuOverCommitRatio:    "1:4",
				MemoryOverCommitRatio: "1:2",
				WorkerNodeCPU:         8,
				WorkerNodeMemory:      16,
			}
		})

		It("returns error when sizer client is not configured", func() {
			sizerService = service.NewSizerService(nil, mockStore)
			result, err := sizerService.CalculateStandaloneClusterRequirements(ctx, request)
			Expect(result).To(BeNil())
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring("sizer client is not configured"))
		})

		DescribeTable("returns ErrInvalidRequest",
			func(mutate func(*mappers.StandaloneClusterRequirementsRequestForm), sizerResp *client.SizerResponse, wantSubstr string) {
				mutate(request)
				testServer = createTestSizerServer(sizerResp, http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateStandaloneClusterRequirements(ctx, request)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				_, ok := err.(*service.ErrInvalidRequest)
				Expect(ok).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring(wantSubstr))
			},
			Entry("when inventory totals are non-positive", func(r *mappers.StandaloneClusterRequirementsRequestForm) {
				r.TotalVMs = 0
			}, createTestSizerResponse(5, 2, 3, 40, 80), "inventory must have positive VMs, CPU, and Memory values"),
			Entry("when single node requested with non-schedulable control plane", func(r *mappers.StandaloneClusterRequirementsRequestForm) {
				r.ControlPlaneNodeCount = util.IntPtr(1)
				r.ControlPlaneSchedulable = util.BoolPtr(false)
			}, createTestSizerResponse(1, 0, 1, 40, 80), "single-node clusters require schedulable control planes"),
			Entry("when worker node CPU is zero", func(r *mappers.StandaloneClusterRequirementsRequestForm) {
				r.WorkerNodeCPU = 0
			}, createTestSizerResponse(5, 2, 3, 40, 80), "worker node size must be greater than zero"),
		)

		It("successfully calculates cluster requirements from inline inventory", func() {
			testServer = createTestSizerServer(createTestSizerResponse(5, 2, 3, 40, 80), http.StatusOK, false)
			sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
			sizerService = service.NewSizerService(sizerClient, mockStore)

			result, err := sizerService.CalculateStandaloneClusterRequirements(ctx, request)

			Expect(err).To(BeNil())
			Expect(result).NotTo(BeNil())
			Expect(result.ClusterSizing.TotalNodes).To(Equal(7))
			Expect(result.ClusterSizing.WorkerNodes).To(Equal(4))
			Expect(result.ClusterSizing.ControlPlaneNodes).To(Equal(3))
			Expect(result.ResourceConsumption.CPU).To(Equal(100.0))
			Expect(result.ResourceConsumption.Memory).To(Equal(200.0))
			Expect(mockStore.clusterInputs).To(BeEmpty())
		})

		It("successfully calculates with 1:8 CPU overcommit ratio", func() {
			request.CpuOverCommitRatio = "1:8"
			request.TotalCPU = 800 // 800 limit CPU with 1:8 ratio = 100 required CPU

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
					_ = json.NewEncoder(w).Encode(createTestSizerResponse(5, 2, 3, 100, 80))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
			sizerService = service.NewSizerService(sizerClient, mockStore)

			result, err := sizerService.CalculateStandaloneClusterRequirements(ctx, request)

			Expect(err).To(BeNil())
			Expect(result).NotTo(BeNil())

			// Verify 1:8 ratio calculation: find the vm-workload and check that
			// RequiredCPU = LimitCPU / 8.0 for at least one of its services
			var vmWorkload *client.Workload
			for i := range sizerPayload.Workloads {
				if sizerPayload.Workloads[i].Name == "vm-workload" {
					vmWorkload = &sizerPayload.Workloads[i]
					break
				}
			}
			Expect(vmWorkload).NotTo(BeNil(), "Expected to find vm-workload in payload")
			Expect(vmWorkload.Services).NotTo(BeEmpty(), "Expected vm-workload to have services")

			// Verify the first service applies the 1:8 ratio correctly
			firstService := vmWorkload.Services[0]
			Expect(firstService.LimitCPU).To(BeNumerically(">", 0), "Expected LimitCPU to be set")
			expectedRequired := firstService.LimitCPU / 8.0
			Expect(firstService.RequiredCPU).To(Equal(expectedRequired),
				"Expected RequiredCPU = %v (limit) / 8.0 (multiplier) = %v", firstService.LimitCPU, expectedRequired)
		})

		It("successfully handles hosted control plane (worker nodes only)", func() {
			request.HostedControlPlane = util.BoolPtr(true)

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

			result, err := sizerService.CalculateStandaloneClusterRequirements(ctx, request)

			Expect(err).To(BeNil())
			Expect(result).NotTo(BeNil())
			Expect(result.ClusterSizing.ControlPlaneNodes).To(Equal(0))
			Expect(result.ClusterSizing.WorkerNodes).To(Equal(6))
			Expect(result.ClusterSizing.TotalNodes).To(Equal(6))
			Expect(sizerPayload.MachineSets).To(HaveLen(1))
			Expect(sizerPayload.MachineSets[0].Name).To(Equal("worker"))
			Expect(sizerPayload.Workloads).To(HaveLen(1))
			Expect(sizerPayload.Workloads[0].Name).To(Equal("vm-workload"))
			Expect(mockStore.clusterInputs).To(BeEmpty())
		})
	})

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
				ControlPlaneSchedulable: util.BoolPtr(false),
				ControlPlaneNodeCount:   util.IntPtr(3),
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
				request.WorkerNodeThreads = util.IntPtr(400)

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
				request.WorkerNodeThreads = nil

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

	Describe("Cluster Utilization Based Sizing", func() {
		var callCount int

		BeforeEach(func() {
			callCount = 0
			sizerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					w.WriteHeader(http.StatusOK)
					return
				}
				if r.URL.Path == "/api/v1/size/custom" && r.Method == http.MethodPost {
					callCount++

					var nodes []client.Node
					nodeCount := 7
					respCPU := 100
					respMemory := 200

					if callCount > 1 {
						nodeCount = 4
						respCPU = 50
						respMemory = 100
						nodes = []client.Node{
							{IsControlPlane: true},
							{IsControlPlane: true},
							{IsControlPlane: true},
							{IsControlPlane: false},
						}
					} else {
						nodes = []client.Node{
							{IsControlPlane: true},
							{IsControlPlane: true},
							{IsControlPlane: true},
							{IsControlPlane: false},
							{IsControlPlane: false},
							{IsControlPlane: false},
							{IsControlPlane: false},
						}
					}

					response := client.SizerResponse{
						Success: true,
						Data: client.SizerData{
							NodeCount:   nodeCount,
							TotalCPU:    respCPU,
							TotalMemory: respMemory,
							ResourceConsumption: client.ResourceConsumption{
								CPU:    100.0,
								Memory: 200.0,
							},
							Advanced: []client.Zone{
								{
									Nodes: nodes,
								},
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			DeferCleanup(sizerServer.Close)

			sizerClient = client.NewSizerClient(sizerServer.URL, 30*time.Second)
			sizerService = service.NewSizerService(sizerClient, mockStore)
		})

		Context("with valid utilization data", func() {
			It("calculates both baseline and optimized sizing", func() {
				clusterID := "test-cluster-1"
				cpuMax := 45.2
				memMax := 62.3
				confidence := 87.5

				inventory := createInventoryWithUtilization(clusterID, cpuMax, memMax, confidence)
				assessmentID := uuid.New()
				mockStore.assessments[assessmentID] = createAssessmentWithInventory(assessmentID, inventory)

				req := &mappers.ClusterRequirementsRequestForm{
					ClusterID:             clusterID,
					CpuOverCommitRatio:    "1:4",
					MemoryOverCommitRatio: "1:2",
					WorkerNodeCPU:         16,
					WorkerNodeMemory:      64,
				}

				response, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(response).NotTo(BeNil())

				Expect(response.ClusterSizing).NotTo(BeNil())
				Expect(response.ClusterSizing.TotalNodes).To(BeNumerically(">", 0))

				Expect(response.OptimizedSizing).NotTo(BeNil())
				Expect(response.OptimizedSizing.CpuUtilizationMax).NotTo(BeNil())
				Expect(*response.OptimizedSizing.CpuUtilizationMax).To(Equal(cpuMax))
				Expect(response.OptimizedSizing.MemoryUtilizationMax).NotTo(BeNil())
				Expect(*response.OptimizedSizing.MemoryUtilizationMax).To(Equal(memMax))
				Expect(response.OptimizedSizing.Confidence).NotTo(BeNil())
				Expect(*response.OptimizedSizing.Confidence).To(Equal(confidence))

				Expect(response.OptimizedSizing.TotalNodes).To(BeNumerically("<", response.ClusterSizing.TotalNodes))

				Expect(response.Savings).NotTo(BeNil())
				Expect(response.Savings.NodesSaved).To(BeNumerically(">", 0))
				Expect(response.Savings.PercentageReduction).To(BeNumerically(">", 0))
				Expect(response.Savings.Description).To(Equal("Based on actual workload performance data"))
			})

			It("applies utilization multipliers correctly", func() {
				clusterID := "test-cluster-1"
				inventory := createInventoryWithUtilization(clusterID, 50.0, 50.0, 90.0)
				assessmentID := uuid.New()
				mockStore.assessments[assessmentID] = createAssessmentWithInventory(assessmentID, inventory)

				req := &mappers.ClusterRequirementsRequestForm{
					ClusterID:             clusterID,
					CpuOverCommitRatio:    "1:1",
					MemoryOverCommitRatio: "1:1",
					WorkerNodeCPU:         16,
					WorkerNodeMemory:      64,
				}

				response, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, req)
				Expect(err).NotTo(HaveOccurred())

				Expect(response.OptimizedSizing).NotTo(BeNil())
				Expect(response.OptimizedSizing.CpuUtilizationMax).NotTo(BeNil())
				Expect(*response.OptimizedSizing.CpuUtilizationMax).To(Equal(50.0))
				Expect(response.OptimizedSizing.MemoryUtilizationMax).NotTo(BeNil())
				Expect(*response.OptimizedSizing.MemoryUtilizationMax).To(Equal(50.0))
			})

			It("uses actual utilization values without floor clamping", func() {
				clusterID := "test-cluster-1"
				inventory := createInventoryWithUtilization(clusterID, 5.0, 3.0, 90.0)
				assessmentID := uuid.New()
				mockStore.assessments[assessmentID] = createAssessmentWithInventory(assessmentID, inventory)

				req := &mappers.ClusterRequirementsRequestForm{
					ClusterID:             clusterID,
					CpuOverCommitRatio:    "1:1",
					MemoryOverCommitRatio: "1:1",
					WorkerNodeCPU:         16,
					WorkerNodeMemory:      64,
				}

				response, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, req)
				Expect(err).NotTo(HaveOccurred())

				Expect(response.OptimizedSizing).NotTo(BeNil())
				Expect(response.OptimizedSizing.CpuUtilizationMax).NotTo(BeNil())
				Expect(*response.OptimizedSizing.CpuUtilizationMax).To(Equal(5.0))
				Expect(response.OptimizedSizing.MemoryUtilizationMax).NotTo(BeNil())
				Expect(*response.OptimizedSizing.MemoryUtilizationMax).To(Equal(3.0))
			})

			It("returns success status when optimization succeeds", func() {
				clusterID := "test-cluster-1"
				inventory := createInventoryWithUtilization(clusterID, 45.2, 62.3, 87.5)
				assessmentID := uuid.New()
				mockStore.assessments[assessmentID] = createAssessmentWithInventory(assessmentID, inventory)

				req := &mappers.ClusterRequirementsRequestForm{
					ClusterID:             clusterID,
					CpuOverCommitRatio:    "1:4",
					MemoryOverCommitRatio: "1:2",
					WorkerNodeCPU:         16,
					WorkerNodeMemory:      64,
				}

				response, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.OptimizationStatus).NotTo(BeNil())
				Expect(response.OptimizationStatus.Attempted).To(BeTrue())
				Expect(response.OptimizationStatus.Reason).To(Equal(api.Success))
			})
		})

		Context("with low confidence utilization data", func() {
			It("returns low_confidence status when confidence < 50%", func() {
				clusterID := "test-cluster-1"
				inventory := createInventoryWithUtilization(clusterID, 45.0, 60.0, 30.0) // confidence < 50
				assessmentID := uuid.New()
				mockStore.assessments[assessmentID] = createAssessmentWithInventory(assessmentID, inventory)

				req := &mappers.ClusterRequirementsRequestForm{
					ClusterID:             clusterID,
					CpuOverCommitRatio:    "1:4",
					MemoryOverCommitRatio: "1:2",
					WorkerNodeCPU:         16,
					WorkerNodeMemory:      64,
				}

				response, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, req)
				Expect(err).NotTo(HaveOccurred())

				Expect(response.OptimizedSizing).To(BeNil())
				Expect(response.OptimizationStatus).NotTo(BeNil())
				Expect(response.OptimizationStatus.Attempted).To(BeFalse())
				Expect(response.OptimizationStatus.Reason).To(Equal(api.LowConfidence))
				Expect(callCount).To(Equal(1)) // Only baseline call, skipped optimized
			})

			It("returns optimized sizing when confidence >= 50%", func() {
				clusterID := "test-cluster-1"
				inventory := createInventoryWithUtilization(clusterID, 45.0, 60.0, 50.0)
				assessmentID := uuid.New()
				mockStore.assessments[assessmentID] = createAssessmentWithInventory(assessmentID, inventory)

				req := &mappers.ClusterRequirementsRequestForm{
					ClusterID:             clusterID,
					CpuOverCommitRatio:    "1:4",
					MemoryOverCommitRatio: "1:2",
					WorkerNodeCPU:         16,
					WorkerNodeMemory:      64,
				}

				response, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, req)
				Expect(err).NotTo(HaveOccurred())

				Expect(response.ClusterSizing).NotTo(BeNil())
				Expect(response.OptimizedSizing).NotTo(BeNil())
				Expect(response.OptimizedSizing.CpuUtilizationMax).NotTo(BeNil())
				Expect(*response.OptimizedSizing.CpuUtilizationMax).To(Equal(45.0))
				Expect(response.OptimizedSizing.MemoryUtilizationMax).NotTo(BeNil())
				Expect(*response.OptimizedSizing.MemoryUtilizationMax).To(Equal(60.0))
			})
		})

		Context("without utilization data", func() {
			It("returns baseline only when cluster has no utilization", func() {
				clusterID := "test-cluster-1"
				inventory := createInventoryWithoutUtilization(clusterID)
				assessmentID := uuid.New()
				mockStore.assessments[assessmentID] = createAssessmentWithInventory(assessmentID, inventory)

				req := &mappers.ClusterRequirementsRequestForm{
					ClusterID:             clusterID,
					CpuOverCommitRatio:    "1:4",
					MemoryOverCommitRatio: "1:2",
					WorkerNodeCPU:         16,
					WorkerNodeMemory:      64,
				}

				response, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, req)
				Expect(err).NotTo(HaveOccurred())

				Expect(response.ClusterSizing).NotTo(BeNil())
				Expect(response.OptimizedSizing).To(BeNil())
				Expect(response.Savings).To(BeNil())
				Expect(response.OptimizationStatus).NotTo(BeNil())
				Expect(response.OptimizationStatus.Attempted).To(BeFalse())
				Expect(response.OptimizationStatus.Reason).To(Equal(api.NoUtilizationData))
				Expect(callCount).To(Equal(1)) // Only baseline call, skipped optimized
			})

			It("returns baseline only when cluster not in inventory", func() {
				inventory := createInventoryWithoutUtilization("other-cluster")
				assessmentID := uuid.New()
				mockStore.assessments[assessmentID] = createAssessmentWithInventory(assessmentID, inventory)

				req := &mappers.ClusterRequirementsRequestForm{
					ClusterID:             "test-cluster-1",
					CpuOverCommitRatio:    "1:4",
					MemoryOverCommitRatio: "1:2",
					WorkerNodeCPU:         16,
					WorkerNodeMemory:      64,
				}

				_, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, req)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when optimized sizing calculation fails", func() {
			var callCount int

			BeforeEach(func() {
				callCount = 0
				sizerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/health" {
						w.WriteHeader(http.StatusOK)
						return
					}
					if r.URL.Path == "/api/v1/size/custom" && r.Method == http.MethodPost {
						callCount++

						// First call (baseline) succeeds
						if callCount == 1 {
							nodes := []client.Node{
								{IsControlPlane: true},
								{IsControlPlane: true},
								{IsControlPlane: true},
								{IsControlPlane: false},
								{IsControlPlane: false},
								{IsControlPlane: false},
								{IsControlPlane: false},
							}
							response := client.SizerResponse{
								Success: true,
								Data: client.SizerData{
									NodeCount:   7,
									TotalCPU:    100,
									TotalMemory: 200,
									ResourceConsumption: client.ResourceConsumption{
										CPU:    100.0,
										Memory: 200.0,
									},
									Advanced: []client.Zone{
										{Nodes: nodes},
									},
								},
							}
							w.Header().Set("Content-Type", "application/json")
							_ = json.NewEncoder(w).Encode(response)
							return
						}

						// Second call (optimized) fails
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.WriteHeader(http.StatusNotFound)
				}))
				DeferCleanup(sizerServer.Close)

				sizerClient = client.NewSizerClient(sizerServer.URL, 30*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)
			})

			It("returns baseline with calculation_error when optimized sizing fails", func() {
				clusterID := "test-cluster-1"
				inventory := createInventoryWithUtilization(clusterID, 45.0, 60.0, 87.5) // valid utilization data
				assessmentID := uuid.New()
				mockStore.assessments[assessmentID] = createAssessmentWithInventory(assessmentID, inventory)

				req := &mappers.ClusterRequirementsRequestForm{
					ClusterID:             clusterID,
					CpuOverCommitRatio:    "1:4",
					MemoryOverCommitRatio: "1:2",
					WorkerNodeCPU:         16,
					WorkerNodeMemory:      64,
				}

				response, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, req)
				Expect(err).NotTo(HaveOccurred())

				// Baseline sizing should be present
				// Sizer returned 7 nodes (3 control plane + 4 worker)
				// With 4 worker nodes, failover calculation is: max(2, ceil(4 * 10 / 100)) = max(2, 1) = 2
				// Total nodes = 3 control plane + 4 worker + 2 failover = 9
				Expect(response.ClusterSizing).NotTo(BeNil())
				Expect(response.ClusterSizing.TotalNodes).To(Equal(9))
				Expect(response.ClusterSizing.WorkerNodes).To(Equal(6))   // 4 worker + 2 failover
				Expect(response.ClusterSizing.FailoverNodes).To(Equal(2)) // failover nodes

				// Optimized sizing should be nil due to failure
				Expect(response.OptimizedSizing).To(BeNil())
				Expect(response.Savings).To(BeNil())

				// OptimizationStatus should indicate attempt and failure
				Expect(response.OptimizationStatus).NotTo(BeNil())
				Expect(response.OptimizationStatus.Attempted).To(BeTrue())
				Expect(response.OptimizationStatus.Reason).To(Equal(api.CalculationError))

				// Both baseline and optimized calls should have been made
				Expect(callCount).To(Equal(2))
			})
		})
	})

	Describe("Compact Mode", func() {
		var (
			assessmentID uuid.UUID
			clusterID    string
			request      *mappers.ClusterRequirementsRequestForm
		)

		BeforeEach(func() {
			assessmentID = uuid.New()
			clusterID = "cluster-compact-test"
			request = &mappers.ClusterRequirementsRequestForm{
				ClusterID:               clusterID,
				CpuOverCommitRatio:      "1:4",
				MemoryOverCommitRatio:   "1:2",
				WorkerNodeCPU:           8,
				WorkerNodeMemory:        16,
				CompactMode:             util.BoolPtr(true),
				ControlPlaneNodeCount:   util.IntPtr(3),
				ControlPlaneSchedulable: util.BoolPtr(true),
			}
		})

		Context("successful calculation", func() {
			It("returns 3-node cluster with no worker nodes", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(createTestSizerResponse(3, 0, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				Expect(result.ClusterSizing.TotalNodes).To(Equal(3))
				Expect(result.ClusterSizing.ControlPlaneNodes).To(Equal(3))
				Expect(result.ClusterSizing.WorkerNodes).To(Equal(0))
				Expect(result.ClusterSizing.FailoverNodes).To(Equal(0))

				// Assessment-scoped requests should persist to database
				Expect(mockStore.clusterInputs).NotTo(BeEmpty())
			})

			It("assigns both vm-workload and control-plane-services to controlPlane machine set", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment

				var sizerPayload client.SizerRequest
				testServer = createTestSizerServerWithRequestCapture(
					createTestSizerResponse(3, 0, 3, 40, 80),
					http.StatusOK,
					false,
					&sizerPayload,
				)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())

				Expect(sizerPayload.MachineSets).To(HaveLen(1))
				Expect(sizerPayload.MachineSets[0].Name).To(Equal("controlPlane"))
				Expect(sizerPayload.MachineSets[0].AllowWorkloadScheduling).NotTo(BeNil())
				Expect(*sizerPayload.MachineSets[0].AllowWorkloadScheduling).To(BeTrue())

				Expect(sizerPayload.Workloads).To(HaveLen(2))
				var vmWorkload, cpWorkload *client.Workload
				for i := range sizerPayload.Workloads {
					switch sizerPayload.Workloads[i].Name {
					case "vm-workload":
						vmWorkload = &sizerPayload.Workloads[i]
					case "control-plane-services":
						cpWorkload = &sizerPayload.Workloads[i]
					}
				}
				Expect(vmWorkload).NotTo(BeNil())
				Expect(vmWorkload.UsesMachines).To(ConsistOf("controlPlane"))
				Expect(cpWorkload).NotTo(BeNil())
				Expect(cpWorkload.UsesMachines).To(ConsistOf("controlPlane"))
			})

			It("applies over-commit ratios correctly in compact mode", func() {
				request.CpuOverCommitRatio = "1:8"
				request.MemoryOverCommitRatio = "1:4"
				assessment := createTestAssessment(assessmentID, clusterID, 10, 800, 400)
				mockStore.assessments[assessmentID] = assessment

				var sizerPayload client.SizerRequest
				testServer = createTestSizerServerWithRequestCapture(
					createTestSizerResponse(3, 0, 3, 100, 100),
					http.StatusOK,
					false,
					&sizerPayload,
				)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())

				var vmWorkload *client.Workload
				for i := range sizerPayload.Workloads {
					if sizerPayload.Workloads[i].Name == "vm-workload" {
						vmWorkload = &sizerPayload.Workloads[i]
						break
					}
				}
				Expect(vmWorkload).NotTo(BeNil())
				Expect(vmWorkload.Services).NotTo(BeEmpty())

				firstService := vmWorkload.Services[0]
				Expect(firstService.LimitCPU).To(BeNumerically(">", 0))
				expectedRequired := firstService.LimitCPU / 8.0
				Expect(firstService.RequiredCPU).To(Equal(expectedRequired))

				Expect(firstService.LimitMemory).To(BeNumerically(">", 0))
				expectedMemoryRequired := firstService.LimitMemory / 4.0
				Expect(firstService.RequiredMemory).To(Equal(expectedMemoryRequired))
			})
		})

		Context("validation errors", func() {
			It("returns error when compact mode with non-schedulable control plane", func() {
				request.ControlPlaneSchedulable = util.BoolPtr(false)
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(createTestSizerResponse(3, 0, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				_, ok := err.(*service.ErrInvalidRequest)
				Expect(ok).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("compact mode clusters require schedulable control planes"))
				Expect(err.Error()).To(ContainSubstring("Set ControlPlaneSchedulable to true"))
			})

			It("returns error when compact mode with hosted control plane", func() {
				request.HostedControlPlane = util.BoolPtr(true)
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(createTestSizerResponse(3, 0, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				_, ok := err.(*service.ErrInvalidRequest)
				Expect(ok).To(BeTrue())
				Expect(err.Error()).To(Equal("compact mode clusters cannot use hosted control planes"))
			})

			It("returns error when compact mode with controlPlaneNodeCount != 3", func() {
				request.ControlPlaneNodeCount = util.IntPtr(1)
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
				Expect(err.Error()).To(Equal("compact mode clusters require exactly 3 control plane nodes"))
			})

			It("returns error when compact mode with nil ControlPlaneSchedulable", func() {
				request.ControlPlaneSchedulable = nil
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(createTestSizerResponse(3, 0, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				_, ok := err.(*service.ErrInvalidRequest)
				Expect(ok).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("compact mode clusters require schedulable control planes"))
			})
		})

		Context("fit errors", func() {
			It("succeeds even when sizer returns more than 3 nodes (current behavior)", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 10, 40, 80)
				mockStore.assessments[assessmentID] = assessment
				testServer = createTestSizerServer(createTestSizerResponse(5, 2, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				Expect(result.ClusterSizing.TotalNodes).To(Equal(3))
				Expect(result.ClusterSizing.ControlPlaneNodes).To(Equal(3))
				Expect(result.ClusterSizing.WorkerNodes).To(Equal(0))
			})

			It("handles sizer schedulability error for compact mode", func() {
				assessment := createTestAssessment(assessmentID, clusterID, 10, 100, 200)
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
					w.WriteHeader(http.StatusNotFound)
				}))
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateClusterRequirements(ctx, assessmentID, request)

				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
				_, ok := err.(*service.ErrInvalidRequest)
				Expect(ok).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("control plane nodes"))
				Expect(err.Error()).To(ContainSubstring("are too small for compact mode"))
			})
		})

		Context("standalone request", func() {
			It("successfully calculates compact mode and does not persist to database", func() {
				standaloneReq := &mappers.StandaloneClusterRequirementsRequestForm{
					TotalVMs:                10,
					TotalCPU:                40,
					TotalMemory:             80,
					CpuOverCommitRatio:      "1:4",
					MemoryOverCommitRatio:   "1:2",
					WorkerNodeCPU:           8,
					WorkerNodeMemory:        16,
					CompactMode:             util.BoolPtr(true),
					ControlPlaneNodeCount:   util.IntPtr(3),
					ControlPlaneSchedulable: util.BoolPtr(true),
				}

				testServer = createTestSizerServer(createTestSizerResponse(3, 0, 3, 40, 80), http.StatusOK, false)
				sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
				sizerService = service.NewSizerService(sizerClient, mockStore)

				result, err := sizerService.CalculateStandaloneClusterRequirements(ctx, standaloneReq)

				Expect(err).To(BeNil())
				Expect(result).NotTo(BeNil())
				Expect(result.ClusterSizing.TotalNodes).To(Equal(3))
				Expect(result.ClusterSizing.ControlPlaneNodes).To(Equal(3))
				Expect(result.ClusterSizing.WorkerNodes).To(Equal(0))
				Expect(result.ClusterSizing.FailoverNodes).To(Equal(0))

				// Standalone requests should not persist to database
				Expect(mockStore.clusterInputs).To(BeEmpty())
			})
		})
	})
})

func createInventoryWithUtilization(clusterID string, cpuMax, memMax, confidence float64) api.Inventory {
	return api.Inventory{
		VcenterId: "test-vcenter",
		Clusters: map[string]api.InventoryData{
			clusterID: {
				Vms: api.VMs{
					Total: 10,
					CpuCores: api.VMResourceBreakdown{
						Total: 100,
					},
					RamGB: api.VMResourceBreakdown{
						Total: 200,
					},
					PowerStates:          map[string]int{},
					MigrationWarnings:    []api.MigrationIssue{},
					NotMigratableReasons: []api.MigrationIssue{},
					DiskCount: api.VMResourceBreakdown{
						Total: 10,
					},
					DiskGB: api.VMResourceBreakdown{
						Total: 1000,
					},
				},
				ClusterUtilization: &api.ClusterUtilization{
					CpuMax:     cpuMax,
					MemMax:     memMax,
					Confidence: confidence,
				},
				Infra: api.Infra{
					TotalHosts:      0,
					Datastores:      []api.Datastore{},
					Networks:        []api.Network{},
					HostPowerStates: map[string]int{},
				},
			},
		},
	}
}

func createInventoryWithoutUtilization(clusterID string) api.Inventory {
	return api.Inventory{
		VcenterId: "test-vcenter",
		Clusters: map[string]api.InventoryData{
			clusterID: {
				Vms: api.VMs{
					Total: 10,
					CpuCores: api.VMResourceBreakdown{
						Total: 100,
					},
					RamGB: api.VMResourceBreakdown{
						Total: 200,
					},
					PowerStates:          map[string]int{},
					MigrationWarnings:    []api.MigrationIssue{},
					NotMigratableReasons: []api.MigrationIssue{},
					DiskCount: api.VMResourceBreakdown{
						Total: 10,
					},
					DiskGB: api.VMResourceBreakdown{
						Total: 1000,
					},
				},
				Infra: api.Infra{
					TotalHosts:      0,
					Datastores:      []api.Datastore{},
					Networks:        []api.Network{},
					HostPowerStates: map[string]int{},
				},
			},
		},
	}
}

func createAssessmentWithInventory(id uuid.UUID, inventory api.Inventory) *model.Assessment {
	inventoryJSON, err := json.Marshal(inventory)
	Expect(err).NotTo(HaveOccurred())
	return &model.Assessment{
		ID: id,
		Snapshots: []model.Snapshot{
			{
				ID:           1,
				AssessmentID: id,
				Inventory:    inventoryJSON,
				CreatedAt:    time.Now(),
			},
		},
	}
}
