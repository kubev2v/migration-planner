package store_test

import (
	"context"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/config"
	st "github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

var _ = Describe("Store", Ordered, func() {
	var (
		store  st.Store
		gormDB *gorm.DB
	)

	BeforeAll(func() {
		cfg := config.NewDefault()
		db, err := st.InitDB(cfg)
		Expect(err).To(BeNil())
		gormDB = db

		store = st.NewStore(db)
		Expect(store).ToNot(BeNil())
	})

	AfterAll(func() {
		store.Close()
	})

	Context("transaction", func() {
		It("insert a source successfully", func() {
			ctx, err := store.NewTransactionContext(context.TODO())
			Expect(err).To(BeNil())

			sourceID := uuid.New()
			m := model.Source{
				ID:       sourceID,
				Username: "admin",
				OrgID:    "org",
			}
			source, err := store.Source().Create(ctx, m)
			Expect(source).ToNot(BeNil())
			Expect(err).To(BeNil())

			// commit
			_, cerr := st.Commit(ctx)
			Expect(cerr).To(BeNil())

			count := 0
			err = gormDB.Raw("SELECT COUNT(*) from sources;").Scan(&count).Error
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("rollback a source successfully", func() {
			ctx, err := store.NewTransactionContext(context.TODO())
			Expect(err).To(BeNil())

			m := model.Source{
				ID:       uuid.New(),
				Username: "admin",
				OrgID:    "org",
			}
			source, err := store.Source().Create(ctx, m)
			Expect(source).ToNot(BeNil())
			Expect(err).To(BeNil())

			// count in the same transaction
			sources, err := store.Source().List(ctx, st.NewSourceQueryFilter())
			Expect(err).To(BeNil())
			Expect(sources).NotTo(BeNil())
			Expect(sources).To(HaveLen(1))

			// rollback
			_, cerr := st.Rollback(ctx)
			Expect(cerr).To(BeNil())

			count := 0
			err = gormDB.Raw("SELECT COUNT(*) from sources;").Scan(&count).Error
			Expect(err).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("Seed the databsae", func() {
			err := store.Seed()
			Expect(err).To(BeNil())

			count := 0
			err = gormDB.Raw("SELECT COUNT(*) from sources;").Scan(&count).Error
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1))

			count = 0
			err = gormDB.Raw("SELECT COUNT(*) from agents;").Scan(&count).Error
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1))
		})

		AfterEach(func() {
			gormDB.Exec("DELETE from sources;")
			gormDB.Exec("DELETE from agents;")
		})
	})
})
