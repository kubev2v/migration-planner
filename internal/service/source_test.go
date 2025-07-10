package service_test

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertAgentStm                 = "INSERT INTO agents (id, status, status_info, cred_url,source_id, version) VALUES ('%s', '%s', '%s', '%s', '%s', 'version_1');"
	insertSourceWithUsernameStm    = "INSERT INTO sources (id, name, username, org_id) VALUES ('%s', 'source_name', '%s', '%s');"
	insertSourceWithEmailDomainStm = "INSERT INTO sources (id, name, username, org_id, email_domain) VALUES ('%s', 'source_name', '%s', '%s', '%s');"
	insertSourceOnPremisesStm      = "INSERT INTO sources (id, name, username, org_id, on_premises) VALUES ('%s', '%s', '%s', '%s', TRUE);"
	insertLabelStm                 = "INSERT INTO labels (key, value, source_id) VALUES ('%s', '%s', '%s');"
)

var _ = Describe("source handler", Ordered, func() {
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

	Context("list", func() {
		It("successfully list all the sources", func() {
			sourceID := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			sourceID = uuid.NewString()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "batman", "batman"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			srv := service.NewSourceService(s)
			resp, err := srv.ListSources(context.TODO(), service.NewSourceFilter(service.WithOrgID("batman")))
			Expect(err).To(BeNil())
			Expect(resp).To(HaveLen(1))
		})

		It("successfully list all the sources -- on premises", func() {
			sourceID := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceOnPremisesStm, sourceID, "source1", "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			sourceID = uuid.NewString()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceOnPremisesStm, sourceID, "source2", "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			srv := service.NewSourceService(s)
			resp, err := srv.ListSources(context.TODO(), service.NewSourceFilter(service.WithOrgID("admin")))
			Expect(err).To(BeNil())
			Expect(resp).To(HaveLen(2))

			count := 0
			tx = gormdb.Raw("SELECT count(*) from sources where on_premises IS TRUE;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(2))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("create", func() {
		It("successfully creates a source", func() {
			srv := service.NewSourceService(s)
			source, err := srv.CreateSource(context.TODO(), mappers.SourceCreateForm{Name: "test", OrgID: "admin", Username: "admin"})
			Expect(err).To(BeNil())
			Expect(source.Name).To(Equal("test"))

			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("successfully creates a source -- with proxy paramters defined", func() {
			srv := service.NewSourceService(s)
			source, err := srv.CreateSource(context.TODO(), mappers.SourceCreateForm{
				Username: "admin",
				OrgID:    "admin",
				Name:     "test",
				HttpUrl:  "http",
				HttpsUrl: "https",
				NoProxy:  "noproxy",
			})
			Expect(err).To(BeNil())
			Expect(source.Name).To(Equal("test"))

			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("successfully creates a source -- with certificate chain defined", func() {
			srv := service.NewSourceService(s)
			source, err := srv.CreateSource(context.TODO(), mappers.SourceCreateForm{
				Username:         "admin",
				OrgID:            "admin",
				Name:             "test",
				CertificateChain: "chain",
			})
			Expect(err).To(BeNil())
			Expect(source.Name).To(Equal("test"))

			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("successfully creates a source -- email domain defined", func() {
			srv := service.NewSourceService(s)
			source, err := srv.CreateSource(context.TODO(), mappers.SourceCreateForm{Name: "test", OrgID: "admin", Username: "admin", EmailDomain: "domain.com"})
			Expect(err).To(BeNil())
			Expect(source.Name).To(Equal("test"))
			Expect(source.EmailDomain).ToNot(BeNil())
			Expect(*source.EmailDomain).To(Equal("domain.com"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("get", func() {
		It("successfully retrieve the source", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			secondSourceID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, secondSourceID, "batman", "batman"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", secondSourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := service.NewSourceService(s)
			source, err := srv.GetSource(ctx, firstSourceID)
			Expect(err).To(BeNil())
			Expect(source.ID.String()).To(Equal(firstSourceID.String()))
			Expect(source.Agents).To(HaveLen(1))
		})

		It("successfully retrieve the source -- with email domain", func() {
			firstSourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithEmailDomainStm, firstSourceID, "admin", "admin", "example.com"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := service.NewSourceService(s)
			source, err := srv.GetSource(ctx, firstSourceID)
			Expect(err).To(BeNil())
			Expect(source.ID.String()).To(Equal(firstSourceID.String()))
			Expect(source.EmailDomain).ToNot(BeNil())
			Expect(*source.EmailDomain).To(Equal("example.com"))
		})

		It("failed to get source - 404", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			secondSourceID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, secondSourceID, "batman", "batman"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", secondSourceID))
			Expect(tx.Error).To(BeNil())

			srv := service.NewSourceService(s)
			_, err := srv.GetSource(context.TODO(), uuid.New())
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from labels;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("delete", func() {
		It("successfully deletes all the sources", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			secondSourceID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, secondSourceID, "batman", "batman"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", secondSourceID))
			Expect(tx.Error).To(BeNil())

			srv := service.NewSourceService(s)
			err := srv.DeleteSources(context.TODO())
			Expect(err).To(BeNil())

			count := 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM SOURCES;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

		})

		It("successfully deletes a source", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			srv := service.NewSourceService(s)
			err := srv.DeleteSource(context.TODO(), firstSourceID)
			Expect(err).To(BeNil())

			count := 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM SOURCES;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			count = 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM AGENTS;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("update on prem", func() {
		It("successfully update source on prem", func() {
			firstSourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			srv := service.NewSourceService(s)
			_, err := srv.UpdateInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID: firstSourceID,
				AgentId:  uuid.New(),
				Inventory: v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "vcenter",
					},
				},
			})
			Expect(err).To(BeNil())

			// agent must be created
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM agents;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			vCenterID := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT v_center_id FROM SOURCES where id = '%s';", firstSourceID)).Scan(&vCenterID)
			Expect(tx.Error).To(BeNil())
			Expect(vCenterID).To(Equal("vcenter"))

			onPrem := false
			tx = gormdb.Raw(fmt.Sprintf("SELECT on_premises FROM SOURCES where id = '%s';", firstSourceID)).Scan(&onPrem)
			Expect(tx.Error).To(BeNil())
			Expect(onPrem).To(BeTrue())
		})

		It("successfully update source on prem -- same vcenter", func() {
			firstSourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			srv := service.NewSourceService(s)
			_, err := srv.UpdateInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID: firstSourceID,
				AgentId:  uuid.New(),
				Inventory: v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "vcenter",
					},
				},
			})
			Expect(err).To(BeNil())

			vCenterID := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT v_center_id FROM SOURCES where id = '%s';", firstSourceID)).Scan(&vCenterID)
			Expect(tx.Error).To(BeNil())
			Expect(vCenterID).To(Equal("vcenter"))

			onPrem := false
			tx = gormdb.Raw(fmt.Sprintf("SELECT on_premises FROM SOURCES where id = '%s';", firstSourceID)).Scan(&onPrem)
			Expect(tx.Error).To(BeNil())
			Expect(onPrem).To(BeTrue())

			_, err = srv.UpdateInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID: firstSourceID,
				AgentId:  uuid.New(),
				Inventory: v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "vcenter",
					},
				},
			})
			Expect(err).To(BeNil())
		})

		It("fails to update source on prem -- different vcenter", func() {
			firstSourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			srv := service.NewSourceService(s)
			_, err := srv.UpdateInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID: firstSourceID,
				AgentId:  uuid.New(),
				Inventory: v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "vcenter",
					},
				},
			})
			Expect(err).To(BeNil())

			vCenterID := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT v_center_id FROM SOURCES where id = '%s';", firstSourceID)).Scan(&vCenterID)
			Expect(tx.Error).To(BeNil())
			Expect(vCenterID).To(Equal("vcenter"))

			onPrem := false
			tx = gormdb.Raw(fmt.Sprintf("SELECT on_premises FROM SOURCES where id = '%s';", firstSourceID)).Scan(&onPrem)
			Expect(tx.Error).To(BeNil())
			Expect(onPrem).To(BeTrue())

			_, err = srv.UpdateInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID: firstSourceID,
				AgentId:  uuid.New(),
				Inventory: v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "another-vcenter-id",
					},
				},
			})
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrInvalidVCenterID)
			Expect(ok).To(BeTrue())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("source filter", func() {
		It("filter does not have default inventory", func() {
			f := service.NewSourceFilter()
			Expect(f.IncludeDefault).To(BeFalse())
		})
		It("filter does have default inventory", func() {
			f := service.NewSourceFilter(service.WithDefaultInventory())
			Expect(f.IncludeDefault).To(BeTrue())
		})
		It("filter has orgID set properlly", func() {
			f := service.NewSourceFilter(service.WithOrgID("test"))
			Expect(f.OrgID).To(Equal("test"))
		})
		It("filter has id set properlly", func() {
			id := uuid.New()
			f := service.NewSourceFilter(service.WithSourceID(id))
			Expect(f.ID).To(Equal(id))
		})
	})

	Context("delete source with share token", func() {
		It("deletes share token when source is deleted via service", func() {
			sourceID := uuid.New()

			// Create source
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Create share token for the source
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO share_tokens (token, source_id) VALUES ('%s', '%s');", "test-token", sourceID))
			Expect(tx.Error).To(BeNil())

			// Verify share token exists
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM share_tokens WHERE source_id = ?", sourceID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			// Delete the source via service
			srv := service.NewSourceService(s)
			err := srv.DeleteSource(context.TODO(), sourceID)
			Expect(err).To(BeNil())

			// Verify source is deleted
			count = 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM sources WHERE id = ?", sourceID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			// Verify share token is automatically deleted due to CASCADE
			count = 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM share_tokens WHERE source_id = ?", sourceID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("deletes all share tokens when all sources are deleted via service", func() {
			sourceID1 := uuid.New()
			sourceID2 := uuid.New()

			// Create two sources
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID1, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID2, "user", "user"))
			Expect(tx.Error).To(BeNil())

			// Create share tokens for both sources
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO share_tokens (token, source_id) VALUES ('%s', '%s');", "test-token-1", sourceID1))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO share_tokens (token, source_id) VALUES ('%s', '%s');", "test-token-2", sourceID2))
			Expect(tx.Error).To(BeNil())

			// Verify both share tokens exist
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM share_tokens").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(2))

			// Delete all sources via service
			srv := service.NewSourceService(s)
			err := srv.DeleteSources(context.TODO())
			Expect(err).To(BeNil())

			// Verify all sources are deleted
			count = 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM sources").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			// Verify all share tokens are deleted due to CASCADE
			count = 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM share_tokens").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("successfully deletes source without share token", func() {
			sourceID := uuid.New()

			// Create source without share token
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Verify source exists but no share token
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM sources WHERE id = ?", sourceID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			count = 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM share_tokens WHERE source_id = ?", sourceID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			// Delete the source via service
			srv := service.NewSourceService(s)
			err := srv.DeleteSource(context.TODO(), sourceID)
			Expect(err).To(BeNil())

			// Verify source is deleted
			count = 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM sources WHERE id = ?", sourceID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})
})
