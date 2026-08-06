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

var _ = Describe("assessment enhancement data store", Ordered, func() {
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
		gormdb.Exec("DELETE FROM assessment_enhancement_data;")
		gormdb.Exec("DELETE FROM assessments;")
	})

	It("upserts and retrieves enhancement data with all field types", func() {
		assessmentID := uuid.New()
		err := gormdb.Exec(
			"INSERT INTO assessments (id, name, org_id, username, source_type, source_id) VALUES (?, ?, ?, ?, ?, NULL);",
			assessmentID, "assessment-one", "org1", "user1", "inventory",
		).Error
		Expect(err).To(BeNil())

		env := "on_premises"
		count := 5
		locCount := 3
		_, err = s.AssessmentEnhancementData().Upsert(context.Background(), model.AssessmentEnhancementData{
			AssessmentID:                    assessmentID,
			DeployedEnvEnvironment:          &env,
			VMwareVerPerpetualLicensesCount: &count,
			NsxFeatures:                     model.StringArray{"microsegmentation", "multi_cloud"},
			CustomerPhysicalLocationsCount:  &locCount,
		})
		Expect(err).To(BeNil())

		stored, err := s.AssessmentEnhancementData().Get(context.Background(), assessmentID)
		Expect(err).To(BeNil())
		Expect(stored.DeployedEnvEnvironment).ToNot(BeNil())
		Expect(*stored.DeployedEnvEnvironment).To(Equal("on_premises"))
		Expect(stored.VMwareVerPerpetualLicensesCount).ToNot(BeNil())
		Expect(*stored.VMwareVerPerpetualLicensesCount).To(Equal(5))
		Expect(stored.NsxFeatures).To(Equal(model.StringArray{"microsegmentation", "multi_cloud"}))
		Expect(stored.CustomerPhysicalLocationsCount).ToNot(BeNil())
		Expect(*stored.CustomerPhysicalLocationsCount).To(Equal(3))
	})

	It("upserts and replaces existing data for the same assessment", func() {
		assessmentID := uuid.New()
		err := gormdb.Exec(
			"INSERT INTO assessments (id, name, org_id, username, source_type, source_id) VALUES (?, ?, ?, ?, ?, NULL);",
			assessmentID, "assessment-two", "org1", "user1", "inventory",
		).Error
		Expect(err).To(BeNil())

		env1 := "on_premises"
		hw := "Dell PowerEdge R750"
		_, err = s.AssessmentEnhancementData().Upsert(context.Background(), model.AssessmentEnhancementData{
			AssessmentID:           assessmentID,
			DeployedEnvEnvironment: &env1,
			CustomerTargetHardware: &hw,
		})
		Expect(err).To(BeNil())

		env2 := "on_cloud"
		_, err = s.AssessmentEnhancementData().Upsert(context.Background(), model.AssessmentEnhancementData{
			AssessmentID:           assessmentID,
			DeployedEnvEnvironment: &env2,
		})
		Expect(err).To(BeNil())

		stored, err := s.AssessmentEnhancementData().Get(context.Background(), assessmentID)
		Expect(err).To(BeNil())
		Expect(*stored.DeployedEnvEnvironment).To(Equal("on_cloud"))
		Expect(stored.CustomerTargetHardware).To(BeNil())
	})

	It("returns ErrRecordNotFound when record does not exist", func() {
		_, err := s.AssessmentEnhancementData().Get(context.Background(), uuid.New())
		Expect(err).To(Equal(store.ErrRecordNotFound))
	})
})
