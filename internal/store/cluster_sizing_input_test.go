package store_test

import (
	"context"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

var _ = Describe("cluster sizing input store", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
	})

	AfterAll(func() {
		_ = s.Close()
	})

	AfterEach(func() {
		gormdb.Exec("DELETE FROM assessment_cluster_sizing_inputs;")
		gormdb.Exec("DELETE FROM assessments;")
	})

	It("upserts and replaces an existing row for the same assessment and cluster", func() {
		assessmentID := uuid.New()
		err := gormdb.Exec(
			"INSERT INTO assessments (id, name, org_id, username, source_type, source_id) VALUES (?, ?, ?, ?, ?, NULL);",
			assessmentID, "assessment-one", "org1", "user1", "inventory",
		).Error
		Expect(err).To(BeNil())

		cpuRatio := "1:4"
		memRatio := "1:2"
		workerCPU := 16
		workerMemory := 128
		_, err = s.ClusterSizingInput().Upsert(context.Background(), model.AssessmentClusterSizingInput{
			AssessmentID:          assessmentID,
			ExternalClusterID:     "domain-c34",
			CpuOverCommitRatio:    &cpuRatio,
			MemoryOverCommitRatio: &memRatio,
			WorkerNodeCPU:         &workerCPU,
			WorkerNodeMemory:      &workerMemory,
		})
		Expect(err).To(BeNil())

		workerCPUUpdated := 32
		_, err = s.ClusterSizingInput().Upsert(context.Background(), model.AssessmentClusterSizingInput{
			AssessmentID:      assessmentID,
			ExternalClusterID: "domain-c34",
			WorkerNodeCPU:     &workerCPUUpdated,
		})
		Expect(err).To(BeNil())

		stored, err := s.ClusterSizingInput().Get(context.Background(), assessmentID, "domain-c34")
		Expect(err).To(BeNil())
		Expect(stored.WorkerNodeCPU).ToNot(BeNil())
		Expect(*stored.WorkerNodeCPU).To(Equal(32))
		Expect(stored.CpuOverCommitRatio).To(BeNil())
		Expect(stored.MemoryOverCommitRatio).To(BeNil())
	})

	It("stores separate records for different clusters in the same assessment", func() {
		assessmentID := uuid.New()
		err := gormdb.Exec(
			"INSERT INTO assessments (id, name, org_id, username, source_type, source_id) VALUES (?, ?, ?, ?, ?, NULL);",
			assessmentID, "assessment-two", "org1", "user1", "inventory",
		).Error
		Expect(err).To(BeNil())

		_, err = s.ClusterSizingInput().Upsert(context.Background(), model.AssessmentClusterSizingInput{
			AssessmentID:      assessmentID,
			ExternalClusterID: "domain-a",
		})
		Expect(err).To(BeNil())

		_, err = s.ClusterSizingInput().Upsert(context.Background(), model.AssessmentClusterSizingInput{
			AssessmentID:      assessmentID,
			ExternalClusterID: "domain-b",
		})
		Expect(err).To(BeNil())

		_, err = s.ClusterSizingInput().Get(context.Background(), assessmentID, "domain-a")
		Expect(err).To(BeNil())
		_, err = s.ClusterSizingInput().Get(context.Background(), assessmentID, "domain-b")
		Expect(err).To(BeNil())
	})

	It("returns ErrRecordNotFound when record does not exist", func() {
		nonExistentAssessmentID := uuid.New()

		_, err := s.ClusterSizingInput().Get(context.Background(), nonExistentAssessmentID, "non-existent-cluster")
		Expect(err).To(Equal(store.ErrRecordNotFound))
	})
})
