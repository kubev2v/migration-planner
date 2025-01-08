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
	insertSourceStm             = "INSERT INTO sources (id) VALUES ('%s');"
	insertSourceWithUsernameStm = "INSERT INTO sources (id, username, org_id) VALUES ('%s', '%s', '%s');"
)

var _ = Describe("source store", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
	)

	BeforeAll(func() {
		db, err := store.InitDB(config.NewDefault())
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
	})

	AfterAll(func() {
		s.Close()
	})

	Context("list", func() {
		It("successfully list all the sources", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString()))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString()))
			Expect(tx.Error).To(BeNil())

			sources, err := s.Source().List(context.TODO(), store.NewSourceQueryFilter())
			Expect(err).To(BeNil())
			Expect(sources).To(HaveLen(2))
		})

		It("successfully list all the sources -- with agents", func() {
			sourceID := uuid.NewString()
			agentID := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf("UPDATE agents set source_id = '%s';", sourceID))
			Expect(tx.Error).To(BeNil())

			sources, err := s.Source().List(context.TODO(), store.NewSourceQueryFilter())
			Expect(err).To(BeNil())
			Expect(sources).To(HaveLen(1))
			agents := sources[0].Agents
			Expect(agents).To(HaveLen(1))
			Expect(agents[0].ID).To(Equal(agentID))
		})

		It("list all sources -- no sources", func() {
			sources, err := s.Source().List(context.TODO(), store.NewSourceQueryFilter())
			Expect(err).To(BeNil())
			Expect(sources).To(HaveLen(0))
		})

		It("successfully list the user's sources", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, uuid.NewString(), "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, uuid.NewString(), "user", "user"))
			Expect(tx.Error).To(BeNil())

			sources, err := s.Source().List(context.TODO(), store.NewSourceQueryFilter().ByUsername("admin"))
			Expect(err).To(BeNil())
			Expect(sources).To(HaveLen(1))
			Expect(sources[0].Username).To(Equal("admin"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from agents;")
			gormdb.Exec("DELETE from sources;")
		})
	})

	Context("get", func() {
		It("successfully get a source", func() {
			id := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, id))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString()))
			Expect(tx.Error).To(BeNil())

			source, err := s.Source().Get(context.TODO(), id)
			Expect(err).To(BeNil())
			Expect(source).ToNot(BeNil())
		})

		It("failed get a source -- source does not exists", func() {
			id := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString()))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString()))
			Expect(tx.Error).To(BeNil())

			source, err := s.Source().Get(context.TODO(), id)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("record not found"))
			Expect(source).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from sources;")
		})

		Context("create", func() {
			It("successfully creates one source", func() {
				sourceID := uuid.New()
				m := model.Source{
					ID:       sourceID,
					Username: "admin",
					OrgID:    "org",
				}
				source, err := s.Source().Create(context.TODO(), m)
				Expect(err).To(BeNil())
				Expect(source).NotTo(BeNil())

				var count int
				tx := gormdb.Raw("SELECT COUNT(*) FROM sources;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("successfully creates one source without sshkey", func() {
				sourceID := uuid.New()
				m := model.Source{
					ID:       sourceID,
					Username: "admin",
					OrgID:    "org",
				}
				source, err := s.Source().Create(context.TODO(), m)
				Expect(err).To(BeNil())
				Expect(source).NotTo(BeNil())

				var count int
				tx := gormdb.Raw("SELECT COUNT(*) FROM sources;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE from sources;")
			})
		})

		Context("delete", func() {
			It("successfully delete a source", func() {
				id := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, id))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString()))
				Expect(tx.Error).To(BeNil())

				err := s.Source().Delete(context.TODO(), id)
				Expect(err).To(BeNil())

				count := 2
				tx = gormdb.Raw("SELECT COUNT(*) FROM sources;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("successfully delete all sources", func() {
				id := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, id))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString()))
				Expect(tx.Error).To(BeNil())

				err := s.Source().DeleteAll(context.TODO())
				Expect(err).To(BeNil())

				count := 2
				tx = gormdb.Raw("SELECT COUNT(*) FROM sources;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(0))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE from sources;")
			})
		})
	})
})
