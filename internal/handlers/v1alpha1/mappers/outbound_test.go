package mappers_test

import (
	"testing"
	"time"

	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/service"
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
