package store_test

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertAgentStm             = "INSERT INTO agents (id, status, status_info, cred_url,source_id, version) VALUES ('%s', '%s', '%s', '%s', '%s', 'version_1');"
	insertAgentWithUpdateAtStm = "INSERT INTO agents (id, status, status_info, cred_url, updated_at, source_id, version) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', 'version_1');"
)

var _ = Describe("agent store", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
	)

	BeforeAll(func() {
		cfg, err := config.NewDefault()
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
		It("successfuly list all the agents -- without filter and options", func() {
			source1 := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, source1, "source-1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())
			agentID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", source1))
			Expect(tx.Error).To(BeNil())

			source2 := uuid.NewString()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, source2, "source-2", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())
			agent2ID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agent2ID, "not-connected", "status-info-2", "cred_url-2", source2))
			Expect(tx.Error).To(BeNil())

			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions())
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(2))

			Expect(agents[0].ID.String()).Should(BeElementOf(agentID.String(), agent2ID.String()))
			Expect(string(agents[0].Status)).To(Equal("not-connected"))
			Expect(agents[0].StatusInfo).Should(BeElementOf("status-info-1", "status-info-2"))
			Expect(agents[0].CredUrl).Should(BeElementOf("cred_url-1", "cred_url-2"))
			Expect(agents[1].ID.String()).Should(BeElementOf(agentID.String(), agent2ID.String()))
		})

		It("list all the agents -- no agents to be found in the db", func() {
			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions())
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(0))
		})

		It("successfuly list all the agents -- with options order by id", func() {
			source1 := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, source1, "source-1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())
			agentID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", source1))
			Expect(tx.Error).To(BeNil())

			source2 := uuid.NewString()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, source2, "source-2", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())
			agent2ID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agent2ID, "not-connected", "status-info-2", "cred_url-2", source2))
			Expect(tx.Error).To(BeNil())

			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions().WithSortOrder(store.SortByID))
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(2))
		})

		It("successfuly list all the agents -- with options order by update_id", func() {
			source1 := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, source1, "source-1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())
			// 24h from now
			agentID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentWithUpdateAtStm, agentID, "not-connected", "status-info-1", "cred_url-1", time.Now().Add(24*time.Hour).Format(time.RFC3339), source1))
			Expect(tx.Error).To(BeNil())

			// this one should be the first
			source2 := uuid.NewString()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, source2, "source-2", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())
			agent1ID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentWithUpdateAtStm, agent1ID, "not-connected", "status-info-2", "cred_url-2", time.Now().Format(time.RFC3339), source2))
			Expect(tx.Error).To(BeNil())

			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions().WithSortOrder(store.SortByUpdatedTime))
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(2))

			Expect(agents[0].ID.String()).To(Equal(agent1ID.String()))
		})

		It("successfuly list the agents -- with filter by source-id", func() {
			source1 := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, source1, "source-1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())
			source2 := uuid.NewString()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, source2, "source-2", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())

			agent1ID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agent1ID, "not-connected", "status-info-1", "cred_url-1", source1))
			Expect(tx.Error).To(BeNil())
			agent2ID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agent2ID, "not-connected", "status-info-2", "cred_url-2", source2))
			Expect(tx.Error).To(BeNil())

			agents, err := s.Agent().List(context.TODO(), store.NewAgentQueryFilter().BySourceID(source1), store.NewAgentQueryOptions())
			Expect(err).To(BeNil())
			Expect(agents).To(HaveLen(1))

			Expect(agents[0].ID.String()).To(Equal(agent1ID.String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("create", func() {
		It("successfuly creates an agent", func() {
			sourceID, _ := uuid.NewUUID()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "source-1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())

			agentID, _ := uuid.NewUUID()
			agentData := model.Agent{
				ID:         agentID,
				CredUrl:    "creds-url-1",
				Status:     "waiting-for-credentials",
				StatusInfo: "status-info-1",
				SourceID:   sourceID,
			}
			agent, err := s.Agent().Create(context.TODO(), agentData)
			Expect(err).To(BeNil())

			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) from agents;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			Expect(agent).ToNot(BeNil())
			Expect(agent.ID.String()).To(Equal(agentID.String()))
			Expect(string(agent.Status)).To(Equal("waiting-for-credentials"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("get", func() {
		It("successfuly return ErrRecordNotFound when agent is not found", func() {
			agent, err := s.Agent().Get(context.TODO(), uuid.New())
			Expect(err).ToNot(BeNil())
			Expect(err).To(Equal(store.ErrRecordNotFound))
			Expect(agent).To(BeNil())
		})

		It("successfuly return the agent", func() {
			source1 := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, source1, "source-1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())

			agentID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", source1))
			Expect(tx.Error).To(BeNil())

			agent, err := s.Agent().Get(context.TODO(), agentID)
			Expect(err).To(BeNil())

			Expect(agent.ID.String()).To(Equal(agentID.String()))
			Expect(string(agent.Status)).To(Equal("not-connected"))
			Expect(agent.StatusInfo).To(Equal("status-info-1"))
			Expect(agent.CredUrl).To(Equal("cred_url-1"))
			Expect(agent.SourceID.String()).To(Equal(source1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("update", func() {
		It("successfuly updates an agent -- status field", func() {
			source1 := uuid.NewString()
			agentID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, source1, "source-1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", source1))
			Expect(tx.Error).To(BeNil())

			agentData := model.Agent{
				ID:         agentID,
				CredUrl:    "creds-url-1",
				Status:     "waiting-for-credentials",
				StatusInfo: "status-info-1",
			}

			agent, err := s.Agent().Update(context.TODO(), agentData)
			Expect(err).To(BeNil())
			Expect(agent).NotTo(BeNil())
			Expect(string(agent.Status)).To(Equal("waiting-for-credentials"))

			rawStatus := ""
			gormdb.Raw(fmt.Sprintf("select status from agents where id = '%s';", agentID)).Scan(&rawStatus)
			Expect(rawStatus).To(Equal("waiting-for-credentials"))
		})

		It("successfuly updates an agent -- status-info field", func() {
			source1 := uuid.NewString()
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, source1, "source-1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", source1))
			Expect(tx.Error).To(BeNil())

			agentData := model.Agent{
				ID:         agentID,
				CredUrl:    "creds-url-1",
				Status:     "not-connected",
				StatusInfo: "another-status-info-1",
			}

			agent, err := s.Agent().Update(context.TODO(), agentData)
			Expect(err).To(BeNil())
			Expect(agent).NotTo(BeNil())
			Expect(agent.StatusInfo).To(Equal("another-status-info-1"))

			rawStatusInfo := ""
			gormdb.Raw(fmt.Sprintf("select status_info from agents where id = '%s';", agentID)).Scan(&rawStatusInfo)
			Expect(rawStatusInfo).To(Equal("another-status-info-1"))
		})

		It("fails updates an agent -- agent is missing", func() {
			source1 := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, source1, "source1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-1", "not-connected", "status-info-1", "cred_url-1", source1))
			Expect(tx.Error).To(BeNil())

			agentData := model.Agent{
				ID: uuid.New(),
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
})
