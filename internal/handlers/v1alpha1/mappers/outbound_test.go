package mappers_test

import (
	"encoding/json"
	"testing"
	"time"

	model2 "github.com/kubev2v/migration-planner/internal/model"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/service"
	storemappers "github.com/kubev2v/migration-planner/internal/store/mappers"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/estimations/complexity"
	"github.com/kubev2v/migration-planner/pkg/estimations/engines"
	"github.com/kubev2v/migration-planner/pkg/estimations/estimation"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMappers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mappers Suite")
}

var _ = Describe("SourceToApi", func() {
	It("maps UpdateType when set", func() {
		source := model2.Source{
			ID:         uuid.New(),
			Name:       "test-source",
			UpdateType: "auto",
		}
		result, err := mappers.SourceToApi(source)
		Expect(err).To(BeNil())
		Expect(result.UpdateType).NotTo(BeNil())
		Expect(*result.UpdateType).To(Equal(api.SourceUpdateType("auto")))
	})

	It("omits UpdateType when empty", func() {
		source := model2.Source{
			ID:         uuid.New(),
			Name:       "test-source",
			UpdateType: "",
		}
		result, err := mappers.SourceToApi(source)
		Expect(err).To(BeNil())
		Expect(result.UpdateType).To(BeNil())
	})
})

var _ = Describe("MigrationComplexityResultToAPI", func() {
	It("maps complexityByDisk, complexityByOS, complexityByOSName, diskSizeRatings, osRatings", func() {
		result := service.MigrationComplexityResult{
			ComplexityByDisk: []complexity.DiskComplexityEntry{
				{Score: 1, VMCount: 10, TotalSizeTB: 5.0},
			},
			ComplexityByOS: []complexity.OSDifficultyEntry{
				{Score: 0, VMCount: 2},
			},
			ComplexityByOSName: []complexity.OSNameEntry{
				{Name: "Red Hat Enterprise Linux 9", Score: 1, VMCount: 10},
			},
			DiskSizeRatings: map[string]complexity.Score{"0-10TB": 1},
			OSRatings:       map[string]complexity.Score{"Red Hat Enterprise Linux 9": 1},
		}

		resp := mappers.MigrationComplexityResultToAPI(result)

		Expect(resp.ComplexityByDisk).To(HaveLen(1))
		Expect(resp.ComplexityByDisk[0].Score).To(Equal(1))
		Expect(resp.ComplexityByOS).To(HaveLen(1))
		Expect(resp.ComplexityByOSName).To(HaveLen(1))
		Expect(resp.DiskSizeRatings).To(HaveKey("0-10TB"))
		Expect(resp.OsRatings).To(HaveKey("Red Hat Enterprise Linux 9"))
	})
})

var _ = Describe("MigrationEstimationResultToAPI", func() {
	It("wraps results under estimation and populates estimationContext", func() {
		results := map[engines.Schema]*service.MigrationAssessmentResult{
			engines.SchemaNetworkBased: {
				MinTotalDuration: 2 * time.Hour,
				MaxTotalDuration: 4 * time.Hour,
			},
		}
		ctx := &service.EstimationContext{
			Schemas:    []engines.Schema{engines.SchemaNetworkBased},
			BaseParams: []estimation.Param{{Key: "work_hours_per_day", Value: 8.0}},
		}

		resp := mappers.MigrationEstimationResultToAPI(results, ctx)

		Expect(resp.Estimation).To(HaveKey("network-based"))
		Expect(resp.EstimationContext.Schemas).NotTo(BeNil())
		Expect(*resp.EstimationContext.Schemas).To(ContainElement("network-based"))
		Expect(resp.EstimationContext.Params).NotTo(BeNil())
		Expect(*resp.EstimationContext.Params).To(HaveKey("work_hours_per_day"))
	})

	It("preserves integer-typed param values in estimationContext", func() {
		results := map[engines.Schema]*service.MigrationAssessmentResult{
			engines.SchemaNetworkBased: {MinTotalDuration: time.Hour, MaxTotalDuration: time.Hour},
		}
		// post_migration_engineers default is int(10) — must not be silently dropped
		ctx := &service.EstimationContext{
			Schemas: []engines.Schema{engines.SchemaNetworkBased},
			BaseParams: []estimation.Param{
				{Key: "post_migration_engineers", Value: 10}, // int
				{Key: "work_hours_per_day", Value: 8.0},      // float64
			},
		}

		resp := mappers.MigrationEstimationResultToAPI(results, ctx)

		Expect(resp.EstimationContext.Params).NotTo(BeNil())
		Expect(*resp.EstimationContext.Params).To(HaveKey("post_migration_engineers"))
		Expect(*resp.EstimationContext.Params).To(HaveKey("work_hours_per_day"))
	})
})

var _ = Describe("OsDiskEstimationResultToAPI", func() {
	It("maps complexityByOsDisk entries with estimation and estimationContext", func() {
		buckets := []complexity.OSDiskEntry{
			{Score: 1, VMCount: 20, TotalSizeTB: 4.0},
			{Score: 2, VMCount: 5, TotalSizeTB: 1.5},
		}
		estimations := map[int]map[string]*service.MigrationAssessmentResult{
			1: {"network-based": {MinTotalDuration: 24 * time.Hour, MaxTotalDuration: 48 * time.Hour}},
		}
		ctx := &service.EstimationContext{
			Schemas:    []engines.Schema{engines.SchemaNetworkBased},
			BaseParams: []estimation.Param{{Key: "work_hours_per_day", Value: 8.0}},
		}

		resp := mappers.OsDiskEstimationResultToAPI(buckets, estimations, ctx, complexity.ComplexityMatrix)

		Expect(resp.ComplexityByOsDisk).To(HaveLen(2))
		entry1 := resp.ComplexityByOsDisk[0]
		Expect(entry1.Score).To(Equal(1))
		Expect(entry1.VmCount).To(Equal(20))
		Expect(entry1.TotalDiskSizeTB).To(BeNumerically("==", float32(4.0)))
		Expect(entry1.Estimation).NotTo(BeNil())
		Expect(*entry1.Estimation).To(HaveKey("network-based"))

		entry2 := resp.ComplexityByOsDisk[1]
		Expect(entry2.Estimation).To(BeNil())

		Expect(resp.EstimationContext).NotTo(BeNil())
		Expect(resp.EstimationContext.Schemas).NotTo(BeNil())
		Expect(*resp.EstimationContext.Schemas).To(ContainElement("network-based"))
		Expect(resp.ComplexityMatrix).NotTo(BeEmpty())

	})
})

var _ = Describe("AssessmentInventoryToAPI", func() {
	storedAt := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	collectedAt := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	vcenterVersion := "8.0.3.0"
	for _, tc := range []struct {
		id         int
		name       string
		version    uint
		raw        model2.Inventory
		created    time.Time
		apiVersion string
		collection *time.Time
	}{
		{
			id:      1,
			name:    "v2 preserves collection metadata",
			version: model.InventoryVersionV2,
			raw: model2.Inventory{
				VcenterId:      "vcenter-id",
				VcenterVersion: &vcenterVersion,
				CreatedAt:      &collectedAt,
				CollectionTime: &collectedAt,
				Vcenter: &model2.InventoryData{
					Vms: model2.VMs{
						Total: 5,
					},
				},
				Clusters: map[string]model2.InventoryData{
					"domain-c34": {
						Vms: model2.VMs{
							Total: 2,
						},
					},
				},
			},
			created:    collectedAt,
			apiVersion: "8.0.3.0",
			collection: &collectedAt,
		},
		{
			id:      2,
			name:    "v2 falls back to stored timestamp",
			version: model.InventoryVersionV2,
			raw: model2.Inventory{
				VcenterId: "vc-id",
				Vcenter: &model2.InventoryData{
					Vms: model2.VMs{
						Total: 5,
					},
				},
			},
			created: storedAt,
		},
		{
			id:      3,
			name:    "legacy inventory uses stored timestamp",
			version: model.InventoryVersionV1,
			raw: model2.Inventory{
				VcenterId: "vc-id",
				Vcenter: &model2.InventoryData{
					Vms: model2.VMs{
						Total: 5,
					},
				},
			},
			created: storedAt,
		},
	} {
		It(tc.name, func() {

			data, clusters, err := mappers.AssessmentInventoryToAPI(model2.AssessmentInventory{
				VCenterID: "vc-id", Version: tc.version, CreatedAt: storedAt, Inventory: tc.raw,
			})
			Expect(err).NotTo(HaveOccurred())
			raw, err := json.Marshal(data)
			Expect(err).NotTo(HaveOccurred())
			var result api.InventoryV2
			Expect(json.Unmarshal(raw, &result)).To(Succeed())
			Expect(result.CreatedAt).To(HaveValue(BeTemporally("==", tc.created)))
			Expect(result.CollectionTime).To(Equal(tc.collection))
			Expect(result.Data.Vms.Total).To(Equal(5))
			if tc.apiVersion != "" {
				Expect(result.VcenterVersion).To(HaveValue(Equal(tc.apiVersion)))
			} else {
				Expect(result.VcenterVersion).To(BeNil())
			}
			Expect(result.Data.Infra.Datastores).NotTo(BeNil(), "inventory collections must be normalized")
			Expect(result.Data.Vms.PowerStates).NotTo(BeNil(), "inventory collections must be normalized")

			if tc.id == 1 {
				Expect(clusters).To(HaveKey("domain-c34"))
				Expect(clusters["domain-c34"].Vms.Total).To(Equal(2))
			}
		})
	}
})

var _ = Describe("Inventory collection time", func() {
	It("preserves source, subset, and assessment collection times in API responses", func() {
		raw := []byte(`{"vcenter_id":"vc-1","clusters":{},"collection_time":"2026-09-20T10:30:00Z"}`)
		want := time.Date(2026, 9, 20, 10, 30, 0, 0, time.UTC)
		var input api.Inventory
		Expect(json.Unmarshal(raw, &input)).To(Succeed())
		domain := mappers.InventoryFromAPI(input)
		Expect(domain.CollectionTime).To(Equal(&want))
		Expect(mappers.InventoryToAPI(domain).CollectionTime).To(Equal(&want))
		source, err := mappers.SourceToApi(model2.Source{Version: model2.InventoryVersionV2, Inventory: &domain})
		Expect(err).NotTo(HaveOccurred())
		Expect(source.Inventory.CollectionTime).To(Equal(&want))
		subset, err := mappers.SourceInventoryToApi(model2.SourceSubsetInventory{Version: model2.InventoryVersionV2, Inventory: domain})
		Expect(err).NotTo(HaveOccurred())
		Expect(subset.Inventory.CollectionTime).To(Equal(&want))
		assessmentID := uuid.New()
		main, err := model.NewAssessmentInventory(uuid.New(), assessmentID, "main", false, raw)
		Expect(err).NotTo(HaveOccurred())
		group, err := model.NewAssessmentInventory(uuid.New(), assessmentID, "group", true, raw)
		Expect(err).NotTo(HaveOccurred())
		createdAt := want.Add(24 * time.Hour)
		domainAssessment, err := storemappers.AssessmentFromStore(&model.Assessment{
			CreatedAt:   createdAt,
			Inventories: []model.AssessmentInventory{main, group},
		})
		Expect(err).NotTo(HaveOccurred())
		assessment := mappers.AssessmentToApi(domainAssessment)
		Expect(assessment.CreatedAt).To(Equal(createdAt))
		Expect(assessment.Snapshots).To(HaveLen(1))
		snapshot := assessment.Snapshots[0]
		Expect(snapshot.Inventory.CollectionTime).To(Equal(&want))
		Expect(snapshot.SubsetInventories).NotTo(BeNil())
		Expect((*snapshot.SubsetInventories)[0].Inventory.CollectionTime).To(Equal(&want))
		assessment = mappers.AssessmentToApi(model2.Assessment{
			MainInventory:     model2.AssessmentInventory{Inventory: domain},
			SubsetInventories: []model2.AssessmentInventory{{Inventory: domain}},
		})
		Expect(assessment.Snapshots).To(HaveLen(1))
		snapshot = assessment.Snapshots[0]
		Expect(snapshot.Inventory.CollectionTime).To(Equal(&want))
		Expect(snapshot.SubsetInventories).NotTo(BeNil())
		Expect((*snapshot.SubsetInventories)[0].Inventory.CollectionTime).To(Equal(&want))
	})
})

var _ = Describe("Inventory datastore storage metadata", func() {
	It("preserves metadata through the service model and persisted inventory", func() {
		backingIDs := []string{"naa.123", "naa.456"}
		filesystem := "VMFS"
		version := int32(6)
		datastore := api.Datastore{
			DiskId:                 "datastore-1",
			BackingDeviceIds:       &backingIDs,
			FilesystemType:         &filesystem,
			FilesystemMajorVersion: &version,
		}
		input := api.Inventory{
			Vcenter: &api.InventoryData{Infra: api.Infra{Datastores: []api.Datastore{datastore}}},
			Clusters: map[string]api.InventoryData{
				"cluster-1": {Infra: api.Infra{Datastores: []api.Datastore{datastore}}},
			},
		}
		domain := mappers.InventoryFromAPI(input)
		data, err := json.Marshal(domain)
		Expect(err).NotTo(HaveOccurred())
		var restored model2.Inventory
		Expect(json.Unmarshal(data, &restored)).To(Succeed())
		output := mappers.InventoryToAPI(restored)
		Expect(output.Vcenter.Infra.Datastores).To(Equal(input.Vcenter.Infra.Datastores))
		Expect(output.Clusters["cluster-1"].Infra.Datastores).To(Equal(input.Clusters["cluster-1"].Infra.Datastores))
	})
})
