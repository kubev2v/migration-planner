package store_test

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertLabelSourceStm = "INSERT INTO sources (id, name, username, org_id) VALUES ('%s', '%s', '%s', '%s');"
)

var _ = Describe("label store", Ordered, func() {
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
		s.Close()
	})

	Context("UpdateLabels", func() {
		It("creates labels when none exist", func() {
			sourceID := uuid.New()
			// create source record directly
			tx := gormdb.Exec(fmt.Sprintf(insertLabelSourceStm, sourceID.String(), "source1", "admin", "org"))
			Expect(tx.Error).To(BeNil())

			// verify no labels
			var count int
			tx = gormdb.Raw("SELECT COUNT(*) FROM labels WHERE source_id = ?", sourceID.String()).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			labels := []model.Label{{Key: "key1", Value: "value1"}, {Key: "key2", Value: "value2"}}
			err := s.Label().UpdateLabels(context.TODO(), sourceID, labels)
			Expect(err).To(BeNil())

			count = -1
			tx = gormdb.Raw("SELECT COUNT(*) FROM labels WHERE source_id = ?", sourceID.String()).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(2))
		})

		It("replaces existing labels with a new set", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertLabelSourceStm, sourceID.String(), "source1", "admin", "org"))
			Expect(tx.Error).To(BeNil())

			// create initial labels
			initialLabels := []model.Label{{Key: "key1", Value: "value1"}, {Key: "key2", Value: "value2"}}
			err := s.Label().UpdateLabels(context.TODO(), sourceID, initialLabels)
			Expect(err).To(BeNil())

			// replace with single different label
			updated := []model.Label{{Key: "new", Value: "val"}}
			err = s.Label().UpdateLabels(context.TODO(), sourceID, updated)
			Expect(err).To(BeNil())

			var labels []model.Label
			tx = gormdb.WithContext(context.TODO()).Where("source_id = ?", sourceID.String()).Find(&labels)
			Expect(tx.Error).To(BeNil())
			Expect(labels).To(HaveLen(1))
			Expect(labels[0].Key).To(Equal("new"))
			Expect(labels[0].Value).To(Equal("val"))
		})

		It("removes all labels when provided empty slice", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertLabelSourceStm, sourceID.String(), "source1", "admin", "org"))
			Expect(tx.Error).To(BeNil())

			// create initial labels
			initialLabels := []model.Label{{Key: "key1", Value: "value1"}}
			err := s.Label().UpdateLabels(context.TODO(), sourceID, initialLabels)
			Expect(err).To(BeNil())

			// now clear labels
			err = s.Label().UpdateLabels(context.TODO(), sourceID, []model.Label{})
			Expect(err).To(BeNil())

			var count int
			tx = gormdb.Raw("SELECT COUNT(*) FROM labels WHERE source_id = ?", sourceID.String()).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM labels;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("DeleteBySourceID", func() {
		It("deletes labels for the given source", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertLabelSourceStm, sourceID.String(), "source1", "admin", "org"))
			Expect(tx.Error).To(BeNil())

			// insert labels manually
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO labels (key, value, source_id) VALUES ('%s', '%s', '%s');", "k", "v", sourceID.String()))
			Expect(tx.Error).To(BeNil())

			err := s.Label().DeleteBySourceID(context.TODO(), sourceID.String())
			Expect(err).To(BeNil())

			var cnt int
			tx = gormdb.Raw("SELECT COUNT(*) FROM labels WHERE source_id = ?", sourceID.String()).Scan(&cnt)
			Expect(tx.Error).To(BeNil())
			Expect(cnt).To(Equal(0))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM labels;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})
})
