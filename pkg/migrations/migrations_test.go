package migrations_test

import (
	"fmt"
	"os"
	"path"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/migrations"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

var _ = Describe("migrations", Ordered, func() {
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

	Context("store migrations", Ordered, func() {
		It("fails to migration the db -- migration folder does not exists", func() {
			err := migrations.MigrateStore(gormdb, "some folder")
			Expect(err).NotTo(BeNil())
		})

		It("sucessfully migrate the db", func() {
			currentFolder, err := os.Getwd()
			Expect(err).To(BeNil())

			err = migrations.MigrateStore(gormdb, path.Join(currentFolder, "sql"))
			Expect(err).To(BeNil())

			tableExists := func(name string) bool {
				exists := false
				tx := gormdb.Raw(fmt.Sprintf("SELECT EXISTS (SELECT FROM pg_tables WHERE schemaname = 'public' and tablename = '%s');", name)).Scan(&exists)
				Expect(tx.Error).To(BeNil())

				return exists
			}

			for _, table := range []string{"relations", "agents", "sources", "keys", "image_infras", "assessments", "assessment_subset_inventories", "assessment_cluster_sizing_inputs", "groups", "members"} {
				Expect(tableExists(table)).To(BeTrue())
			}

			// Verify foreign key constraint from assessment_subset_inventories.snapshot_id to snapshots.id with CASCADE DELETE
			type FKInfo struct {
				ConstraintName string
				ChildColumn    string
				ParentColumn   string
				DeleteAction   string
			}
			var fkInfo FKInfo
			tx := gormdb.Raw(`
				SELECT
					c.conname AS constraint_name,
					a_child.attname AS child_column,
					a_parent.attname AS parent_column,
					c.confdeltype AS delete_action
				FROM pg_constraint c
				JOIN pg_class child ON child.oid = c.conrelid
				JOIN pg_class parent ON parent.oid = c.confrelid
				JOIN pg_attribute a_child ON a_child.attrelid = c.conrelid AND a_child.attnum = ANY(c.conkey)
				JOIN pg_attribute a_parent ON a_parent.attrelid = c.confrelid AND a_parent.attnum = ANY(c.confkey)
				WHERE c.contype = 'f'
					AND child.relname = 'assessment_subset_inventories'
					AND parent.relname = 'snapshots'
				LIMIT 1;
			`).Scan(&fkInfo)
			Expect(tx.Error).To(BeNil())
			Expect(fkInfo.ConstraintName).NotTo(BeEmpty(), "FK constraint should exist")
			Expect(fkInfo.ChildColumn).To(Equal("snapshot_id"), "FK should reference snapshot_id column")
			Expect(fkInfo.ParentColumn).To(Equal("id"), "FK should reference snapshots.id column")
			Expect(fkInfo.DeleteAction).To(Equal("c"), "FK should have ON DELETE CASCADE (confdeltype='c')")
		})

		AfterEach(func() {
			gormdb.Exec("DROP TABLE IF EXISTS source_subset_inventories;")
			gormdb.Exec("DROP TABLE IF EXISTS assessment_subset_inventories;")
			gormdb.Exec("DROP TABLE IF EXISTS relations;")
			gormdb.Exec("DROP TABLE IF EXISTS snapshots;")
			gormdb.Exec("DROP TABLE IF EXISTS assessment_cluster_sizing_inputs;")
			gormdb.Exec("DROP TABLE IF EXISTS assessments;")
			gormdb.Exec("DROP TABLE IF EXISTS agents;")
			gormdb.Exec("DROP TABLE IF EXISTS image_infras;")
			gormdb.Exec("DROP TABLE IF EXISTS keys;")
			gormdb.Exec("DROP TABLE IF EXISTS labels;")
			gormdb.Exec("DROP TABLE IF EXISTS sources;")
			gormdb.Exec("DROP TABLE IF EXISTS members;")
			gormdb.Exec("DROP INDEX IF EXISTS idx_partners_customers_partner_id;")
			gormdb.Exec("DROP INDEX IF EXISTS uq_partner_customer_active_username;")
			gormdb.Exec("DROP TABLE IF EXISTS partners_customers;")
			gormdb.Exec("DROP TYPE IF EXISTS request_status;")
			gormdb.Exec("DROP TABLE IF EXISTS groups;")
			gormdb.Exec("DROP TABLE IF EXISTS rvtools_files;")
			gormdb.Exec("DROP TABLE IF EXISTS goose_db_version;")
		})
	})
})
