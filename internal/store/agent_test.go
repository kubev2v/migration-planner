package store_test

import (
	"context"
	"fmt"
	"time"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertAgentStm              = "INSERT INTO agents (id, status, status_info, cred_url, version) VALUES ('%s', '%s', '%s', '%s', 'version_1');"
	insertAgentWithUsernameStm  = "INSERT INTO agents (id, status, status_info, cred_url,username, org_id,  version) VALUES ('%s', '%s', '%s', '%s','%s', '%s', 'version_1');"
	insertAgentWithSourceStm    = "INSERT INTO agents (id, status, status_info, cred_url, source_id, version) VALUES ('%s', '%s', '%s', '%s', '%s', 'version_1');"
	insertAgentWithUpdateAtStm  = "INSERT INTO agents (id, status, status_info, cred_url, updated_at, version) VALUES ('%s', '%s', '%s', '%s', '%s', 'version_1');"
	insertAgentWithDeletedAtStm = "INSERT INTO agents (id, status, status_info, cred_url, deleted_at, version) VALUES ('%s', '%s', '%s', '%s', '%s', 'version_1');"
)

var _ = Describe("agent store", Ordered, func() {
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
		It("successfuly list all the agents -- without filter and options", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-1", "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-2", "not-connected", "status-info-2", "cred_url-2"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-3", "not-connected", "status-info-3", "cred_url-3"))
			Expect(tx.Error).To(BeNil())

			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions())
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(3))

			Expect(agents[0].ID).Should(BeElementOf("agent-1", "agent-2", "agent-3"))
			Expect(string(agents[0].Status)).To(Equal("not-connected"))
			Expect(agents[0].StatusInfo).Should(BeElementOf("status-info-1", "status-info-2", "status-info-3"))
			Expect(agents[0].CredUrl).Should(BeElementOf("cred_url-1", "cred_url-2", "cred_url-3"))
			Expect(agents[1].ID).Should(BeElementOf("agent-1", "agent-2", "agent-3"))
			Expect(agents[2].ID).Should(BeElementOf("agent-1", "agent-2", "agent-3"))
		})

		It("list all the agents -- no agents to be found in the db", func() {
			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions())
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(0))
		})

		It("successfuly list all the agents -- with options order by id", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-1", "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-2", "not-connected", "status-info-2", "cred_url-2"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-3", "not-connected", "status-info-3", "cred_url-3"))
			Expect(tx.Error).To(BeNil())

			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions().WithSortOrder(store.SortByID))
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(3))

			Expect(agents[0].ID).To(Equal("agent-1"))
		})

		It("successfuly list all the agents -- with options order by update_id", func() {
			// 24h from now
			tx := gormdb.Exec(fmt.Sprintf(insertAgentWithUpdateAtStm, "agent-1", "not-connected", "status-info-1", "cred_url-1", time.Now().Add(24*time.Hour).Format(time.RFC3339)))
			Expect(tx.Error).To(BeNil())

			// this one should be the first
			tx = gormdb.Exec(fmt.Sprintf(insertAgentWithUpdateAtStm, "agent-2", "not-connected", "status-info-2", "cred_url-2", time.Now().Format(time.RFC3339)))
			Expect(tx.Error).To(BeNil())

			// 36h from now
			tx = gormdb.Exec(fmt.Sprintf(insertAgentWithUpdateAtStm, "agent-3", "not-connected", "status-info-3", "cred_url-3", time.Now().Add(36*time.Hour).Format(time.RFC3339)))
			Expect(tx.Error).To(BeNil())

			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions().WithSortOrder(store.SortByUpdatedTime))
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(3))

			Expect(agents[0].ID).To(Equal("agent-2"))
		})

		It("successfuly list the agents -- with filter by source-id", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, "source-1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, "source-2"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentWithSourceStm, "agent-1", "not-connected", "status-info-1", "cred_url-1", "source-1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentWithSourceStm, "agent-2", "not-connected", "status-info-2", "cred_url-2", "source-2"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-3", "not-connected", "status-info-3", "cred_url-3"))
			Expect(tx.Error).To(BeNil())

			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter().BySourceID("source-1"), store.NewAgentQueryOptions())
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(1))

			Expect(agents[0].ID).To(Equal("agent-1"))
		})

		It("successfuly list the agents -- with filter by soft deletion", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentWithDeletedAtStm, "agent-1", "not-connected", "status-info-1", "cred_url-1", time.Now().Format(time.RFC3339)))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-2", "not-connected", "status-info-2", "cred_url-2"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-3", "not-connected", "status-info-3", "cred_url-3"))
			Expect(tx.Error).To(BeNil())

			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter().BySoftDeleted(true), store.NewAgentQueryOptions())
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(1))

			Expect(agents[0].ID).To(Equal("agent-1"))
		})

		It("successfuly list the agents -- without filter by soft deletion", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentWithDeletedAtStm, "agent-1", "not-connected", "status-info-1", "cred_url-1", time.Now().Format(time.RFC3339)))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-2", "not-connected", "status-info-2", "cred_url-2"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-3", "not-connected", "status-info-3", "cred_url-3"))
			Expect(tx.Error).To(BeNil())

			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions())
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(2))
		})

		It("successfuly list all the agents -- filtered by user", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentWithUsernameStm, "agent-1", "not-connected", "status-info-1", "cred_url-1", "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentWithUsernameStm, "agent-2", "not-connected", "status-info-1", "cred_url-1", "user", "user"))
			Expect(tx.Error).To(BeNil())

			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter().ByUsername("admin"), store.NewAgentQueryOptions())
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(1))

			Expect(agents[0].Username).To(Equal("admin"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("create", func() {
		It("successfuly creates an agent", func() {
			agentData := model.Agent{
				ID:         "agent-1",
				CredUrl:    "creds-url-1",
				Username:   "admin",
				OrgID:      "whatever",
				Status:     "waiting-for-credentials",
				StatusInfo: "status-info-1",
			}

			agent, err := s.Agent().Create(context.TODO(), agentData)
			Expect(err).To(BeNil())

			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) from agents;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			Expect(agent).ToNot(BeNil())
			Expect(agent.ID).To(Equal("agent-1"))
			Expect(agent.Username).To(Equal("admin"))
			Expect(string(agent.Status)).To(Equal("waiting-for-credentials"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
		})
	})

	Context("get", func() {
		It("successfuly return ErrRecordNotFound when agent is not found", func() {
			agent, err := s.Agent().Get(context.TODO(), "id")
			Expect(err).ToNot(BeNil())
			Expect(err).To(Equal(store.ErrRecordNotFound))
			Expect(agent).To(BeNil())
		})

		It("successfuly return the agent", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-1", "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())

			agent, err := s.Agent().Get(context.TODO(), "agent-1")
			Expect(err).To(BeNil())

			Expect(agent.ID).To(Equal("agent-1"))
			Expect(string(agent.Status)).To(Equal("not-connected"))
			Expect(agent.StatusInfo).To(Equal("status-info-1"))
			Expect(agent.CredUrl).To(Equal("cred_url-1"))
		})

		It("successfuly return the agent connected to a source", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, "source-1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentWithSourceStm, "agent-1", "not-connected", "status-info-1", "cred_url-1", "source-1"))
			Expect(tx.Error).To(BeNil())

			agent, err := s.Agent().Get(context.TODO(), "agent-1")
			Expect(err).To(BeNil())

			Expect(agent.ID).To(Equal("agent-1"))
			Expect(string(agent.Status)).To(Equal("not-connected"))
			Expect(agent.StatusInfo).To(Equal("status-info-1"))
			Expect(agent.CredUrl).To(Equal("cred_url-1"))
			Expect(agent.SourceID).ToNot(BeNil())
			Expect(*agent.SourceID).To(Equal("source-1"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("update", func() {
		It("successfuly updates an agent -- status field", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-1", "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())

			agentData := model.Agent{
				ID:         "agent-1",
				CredUrl:    "creds-url-1",
				Status:     "waiting-for-credentials",
				StatusInfo: "status-info-1",
			}

			agent, err := s.Agent().Update(context.TODO(), agentData)
			Expect(err).To(BeNil())
			Expect(agent).NotTo(BeNil())
			Expect(string(agent.Status)).To(Equal("waiting-for-credentials"))

			rawStatus := ""
			gormdb.Raw("select status from agents where id = 'agent-1';").Scan(&rawStatus)
			Expect(rawStatus).To(Equal("waiting-for-credentials"))
		})

		It("successfuly updates an agent -- status-info field", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-1", "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())

			agentData := model.Agent{
				ID:         "agent-1",
				CredUrl:    "creds-url-1",
				Status:     "not-connected",
				StatusInfo: "another-status-info-1",
			}

			agent, err := s.Agent().Update(context.TODO(), agentData)
			Expect(err).To(BeNil())
			Expect(agent).NotTo(BeNil())
			Expect(agent.StatusInfo).To(Equal("another-status-info-1"))

			rawStatusInfo := ""
			gormdb.Raw("select status_info from agents where id = 'agent-1';").Scan(&rawStatusInfo)
			Expect(rawStatusInfo).To(Equal("another-status-info-1"))
		})

		It("successfuly updates an agent -- source_id field", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, "source-1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-1", "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())

			associated := true
			agent, err := s.Agent().UpdateSourceID(context.TODO(), "agent-1", "source-1", associated)
			Expect(err).To(BeNil())
			Expect(agent).NotTo(BeNil())
			Expect(*agent.SourceID).To(Equal("source-1"))
			Expect(agent.Associated).To(BeTrue())

			rawSourceID := ""
			gormdb.Raw("select source_id from agents where id = 'agent-1';").Scan(&rawSourceID)
			Expect(rawSourceID).To(Equal("source-1"))
		})

		It("fails updates an agent -- agent is missing", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-1", "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())

			agent, err := s.Agent().UpdateSourceID(context.TODO(), "agent-1", "source-1", true)
			Expect(err).ToNot(BeNil())
			Expect(agent).To(BeNil())
		})

		It("fails updates an agent -- source is missing", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-1", "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())

			agentData := model.Agent{
				ID: "unknown-id",
			}

			agent, err := s.Agent().Update(context.TODO(), agentData)
			Expect(err).ToNot(BeNil())
			Expect(agent).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("delete", func() {
		It("successfuly delete an agents -- soft deletion", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-1", "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-2", "not-connected", "status-info-2", "cred_url-2"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-3", "not-connected", "status-info-3", "cred_url-3"))
			Expect(tx.Error).To(BeNil())

			softDeletion := true
			err := s.Agent().Delete(context.TODO(), "agent-1", softDeletion)
			Expect(err).To(BeNil())

			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions().WithIncludeSoftDeleted(true))
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(3))
		})

		It("successfuly delete an agent -- hard deletion", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-1", "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-2", "not-connected", "status-info-2", "cred_url-2"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-3", "not-connected", "status-info-3", "cred_url-3"))
			Expect(tx.Error).To(BeNil())

			softDeletion := false
			err := s.Agent().Delete(context.TODO(), "agent-1", softDeletion)
			Expect(err).To(BeNil())

			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions())
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(2))
		})

		It("successfuly delete an agent -- soft and hard deletion", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-1", "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-2", "not-connected", "status-info-2", "cred_url-2"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-3", "not-connected", "status-info-3", "cred_url-3"))
			Expect(tx.Error).To(BeNil())

			softDeletion := true
			err := s.Agent().Delete(context.TODO(), "agent-1", softDeletion)
			Expect(err).To(BeNil())

			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions().WithIncludeSoftDeleted(true))
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(3))

			softDeletion = false
			err = s.Agent().Delete(context.TODO(), "agent-1", softDeletion)
			Expect(err).To(BeNil())

			agents, err = s.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions())
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(2))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
		})
	})
})
