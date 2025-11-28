package migrations_test

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/migrations"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

var _ = Describe("migrations", Ordered, func() {
	var (
		s       store.Store
		gormdb  *gorm.DB
		pgxPool *pgxpool.Pool
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db

		pgxDSN := fmt.Sprintf("host=%s user=%s password=%s port=%s dbname=%s",
			cfg.Database.Hostname,
			cfg.Database.User,
			cfg.Database.Password,
			cfg.Database.Port,
			cfg.Database.Name,
		)
		pgxPool, err = pgxpool.New(context.Background(), pgxDSN)
		Expect(err).To(BeNil())
	})

	AfterAll(func() {
		pgxPool.Close()
		s.Close()
	})

	Context("store migrations", Ordered, func() {
		It("fails to migration the db -- migration folder does not exists", func() {
			err := migrations.MigrateStore(gormdb, "some folder", pgxPool)
			Expect(err).NotTo(BeNil())
		})

		It("sucessfully migrate the db", func() {
			currentFolder, err := os.Getwd()
			Expect(err).To(BeNil())

			err = migrations.MigrateStore(gormdb, path.Join(currentFolder, "sql"), pgxPool)
			Expect(err).To(BeNil())

			tableExists := func(name string) bool {
				exists := false
				tx := gormdb.Raw(fmt.Sprintf("SELECT EXISTS (SELECT FROM pg_tables WHERE schemaname = 'public' and tablename = '%s');", name)).Scan(&exists)
				Expect(tx.Error).To(BeNil())

				return exists
			}

			for _, table := range []string{"agents", "sources", "keys", "image_infras", "assessments", "river_job"} {
				Expect(tableExists(table)).To(BeTrue())
			}
		})

		AfterEach(func() {
			gormdb.Exec("DROP TABLE IF EXISTS snapshots;")
			gormdb.Exec("DROP TABLE IF EXISTS assessments;")
			gormdb.Exec("DROP TABLE IF EXISTS agents;")
			gormdb.Exec("DROP TABLE IF EXISTS image_infras;")
			gormdb.Exec("DROP TABLE IF EXISTS keys;")
			gormdb.Exec("DROP TABLE IF EXISTS labels;")
			gormdb.Exec("DROP TABLE IF EXISTS sources;")
			gormdb.Exec("DROP TABLE IF EXISTS goose_db_version;")
			gormdb.Exec("DROP TABLE IF EXISTS river_job;")
			gormdb.Exec("DROP TABLE IF EXISTS river_queue;")
			gormdb.Exec("DROP TABLE IF EXISTS river_leader;")
			gormdb.Exec("DROP TABLE IF EXISTS river_client;")
			gormdb.Exec("DROP TABLE IF EXISTS river_migration;")
			gormdb.Exec("DROP TYPE IF EXISTS river_job_state;")
		})
	})
})
