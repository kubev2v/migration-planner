package store_test

import (
	"context"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/config"
	st "github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/log"
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
		log := log.InitLogs()
		cfg := config.NewDefault()
		db, err := st.InitDB(cfg, log)
		Expect(err).To(BeNil())
		gormDB = db

		store = st.NewStore(db, log.WithField("test", "store"))
		Expect(store).ToNot(BeNil())

		// migrate
		err = store.InitialMigration()
		Expect(err).To(BeNil())
	})

	AfterAll(func() {
		gormDB.Exec("DROP TABLE sources;")
		store.Close()
	})

	Context("transaction", func() {
		It("insert a source successfully", func() {
			ctx, err := store.NewTransactionContext(context.TODO())
			Expect(err).To(BeNil())

			source, err := store.Source().Create(ctx, api.SourceCreate{Name: "test", SshKey: "some key"})
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

			source, err := store.Source().Create(ctx, api.SourceCreate{Name: "test", SshKey: "some key"})
			Expect(source).ToNot(BeNil())
			Expect(err).To(BeNil())

			// count in the same transaction
			sources, err := store.Source().List(ctx)
			Expect(err).To(BeNil())
			Expect(sources).NotTo(BeNil())
			Expect(*sources).To(HaveLen(1))

			// rollback
			_, cerr := st.Rollback(ctx)
			Expect(cerr).To(BeNil())

			count := 0
			err = gormDB.Raw("SELECT COUNT(*) from sources;").Scan(&count).Error
			Expect(err).To(BeNil())
			Expect(count).To(Equal(0))
		})

		AfterEach(func() {
			gormDB.Exec("DELETE from sources;")
		})
	})
})
