package mappers_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/service"
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
		source := model.Source{
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
		source := model.Source{
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
	for _, tc := range []struct {
		id         int
		name       string
		version    uint
		raw        string
		created    time.Time
		apiVersion string
		collection *time.Time
	}{
		{
			id:         1,
			name:       "v2 preserves collection metadata",
			version:    model.InventoryVersionV2,
			raw:        `{"vcenter_id":"vc-id","vcenter_version":"8.0.3.0","created_at":"2026-10-13T09:00:00Z","collection_time":"2026-10-13T09:00:00Z","vcenter":{"vms":{"total":5}},"clusters":{"domain-c34":{"vms":{"total":2}}}}`,
			created:    collectedAt,
			apiVersion: "8.0.3.0",
			collection: &collectedAt,
		},
		{
			id:      2,
			name:    "v2 falls back to stored timestamp",
			version: model.InventoryVersionV2,
			raw:     `{"vcenter_id":"vc-id","vcenter":{"vms":{"total":5}},"clusters":{}}`,
			created: storedAt,
		},
		{
			id:      3,
			name:    "legacy inventory uses stored timestamp",
			version: model.InventoryVersionV1,
			raw:     `{"vcenter":{"id":"vc-id"},"vms":{"total":5}}`,
			created: storedAt,
		},
	} {
		It(tc.name, func() {
			data, clusters, err := mappers.AssessmentInventoryToAPI(model.AssessmentInventory{
				VCenterID: "vc-id", Version: tc.version, CreatedAt: storedAt, Inventory: []byte(tc.raw),
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

	DescribeTable("rejects malformed JSON", func(version uint) {
		_, _, err := mappers.AssessmentInventoryToAPI(model.AssessmentInventory{Version: version, Inventory: []byte(`{`)})
		Expect(err).To(HaveOccurred())
	},
		Entry("for legacy inventory", uint(model.InventoryVersionV1)),
		Entry("for v2 inventory", uint(model.InventoryVersionV2)),
	)
})

var _ = Describe("Inventory collection time", func() {
	It("preserves source, subset, and assessment collection times in API responses", func() {
		raw := []byte(`{"vcenter_id":"vc-1","clusters":{},"collection_time":"2026-09-20T10:30:00Z"}`)
		want := time.Date(2026, 9, 20, 10, 30, 0, 0, time.UTC)
		source, err := mappers.SourceToApi(model.Source{Inventory: raw})
		Expect(err).NotTo(HaveOccurred())
		Expect(source.Inventory.CollectionTime).To(Equal(&want))
		subset, err := mappers.SourceInventoryToApi(model.SourceSubsetInventory{Inventory: raw})
		Expect(err).NotTo(HaveOccurred())
		Expect(subset.Inventory.CollectionTime).To(Equal(&want))
		assessmentID := uuid.New()
		main, err := model.NewAssessmentInventory(uuid.New(), assessmentID, "main", false, raw)
		Expect(err).NotTo(HaveOccurred())
		group, err := model.NewAssessmentInventory(uuid.New(), assessmentID, "group", true, raw)
		Expect(err).NotTo(HaveOccurred())
		createdAt := want.Add(24 * time.Hour)
		assessment, err := mappers.AssessmentToApi(model.Assessment{
			CreatedAt:   createdAt,
			Inventories: []model.AssessmentInventory{main, group},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(assessment.CreatedAt).To(Equal(createdAt))
		Expect(assessment.Snapshots).To(HaveLen(1))
		snapshot := assessment.Snapshots[0]
		Expect(snapshot.Inventory.CollectionTime).To(Equal(&want))
		Expect(snapshot.SubsetInventories).NotTo(BeNil())
		Expect((*snapshot.SubsetInventories)[0].Inventory.CollectionTime).To(Equal(&want))
	})
})
