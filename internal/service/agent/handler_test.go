package service_test

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1"
	apiAgent "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	server "github.com/kubev2v/migration-planner/internal/api/server/agent"
	"github.com/kubev2v/migration-planner/internal/config"
	service "github.com/kubev2v/migration-planner/internal/service/agent"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const (
	insertAgentStm              = "INSERT INTO agents (id, status, status_info, cred_url, version) VALUES ('%s', '%s', '%s', '%s', 'version_1');"
	insertAgentWithSourceStm    = "INSERT INTO agents (id, status, status_info, cred_url, source_id, version) VALUES ('%s', '%s', '%s', '%s', '%s', 'version_1');"
	insertAgentWithUpdateAtStm  = "INSERT INTO agents (id, status, status_info, cred_url, updated_at, version) VALUES ('%s', '%s', '%s', '%s', '%s', 'version_1');"
	insertAgentWithDeletedAtStm = "INSERT INTO agents (id, status, status_info, cred_url,created_at, updated_at, deleted_at, version) VALUES ('%s', '%s', '%s', '%s', '%s','%s','%s', 'version_1');"
)

var _ = Describe("agent store", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
	)

	BeforeAll(func() {
		log := logrus.New()
		db, err := store.InitDB(config.NewDefault(), log)
		Expect(err).To(BeNil())

		s = store.NewStore(db, log)
		gormdb = db
		s.InitialMigration()
	})

	AfterAll(func() {
		s.Close()
	})

	Context("Update agent status", func() {
		It("successfully creates the agent", func() {
			agentID := uuid.New()

			srv := service.NewAgentServiceHandler(s, logrus.New())
			resp, err := srv.UpdateAgentStatus(context.TODO(), server.UpdateAgentStatusRequestObject{
				Id: agentID,
				Body: &apiAgent.UpdateAgentStatusJSONRequestBody{
					Id:            agentID.String(),
					Status:        string(v1alpha1.AgentStatusWaitingForCredentials),
					StatusInfo:    "waiting-for-credentials",
					CredentialUrl: "creds-url",
					Version:       "version-1",
				},
			})
			Expect(err).To(BeNil())
			Expect(resp).To(Equal(server.UpdateAgentStatus201Response{}))

			count := -1
			tx := gormdb.Raw("SELECT COUNT(*) FROM agents;").Scan(&count)
			Expect(count).To(Equal(1))

			status := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT status from agents WHERE id = '%s';", agentID)).Scan(&status)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("waiting-for-credentials"))
		})

		It("successfully updates the agent", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())

			srv := service.NewAgentServiceHandler(s, logrus.New())
			resp, err := srv.UpdateAgentStatus(context.TODO(), server.UpdateAgentStatusRequestObject{
				Id: agentID,
				Body: &apiAgent.UpdateAgentStatusJSONRequestBody{
					Id:            agentID.String(),
					Status:        string(v1alpha1.AgentStatusWaitingForCredentials),
					StatusInfo:    "waiting-for-credentials",
					CredentialUrl: "creds-url",
					Version:       "version-1",
				},
			})
			Expect(err).To(BeNil())
			Expect(resp).To(Equal(server.UpdateAgentStatus200Response{}))

			count := -1
			tx = gormdb.Raw("SELECT COUNT(*) FROM agents;").Scan(&count)
			Expect(count).To(Equal(1))

			status := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT status from agents WHERE id = '%s';", agentID)).Scan(&status)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("waiting-for-credentials"))
		})

		It("should receive 410 when agent is soft deleted", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentWithDeletedAtStm, agentID, "not-connected", "status-info-1", "cred_url-1", time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339)))
			Expect(tx.Error).To(BeNil())

			srv := service.NewAgentServiceHandler(s, logrus.New())
			resp, err := srv.UpdateAgentStatus(context.TODO(), server.UpdateAgentStatusRequestObject{
				Id: agentID,
				Body: &apiAgent.UpdateAgentStatusJSONRequestBody{
					Id:            agentID.String(),
					Status:        string(v1alpha1.AgentStatusWaitingForCredentials),
					StatusInfo:    "waiting-for-credentials",
					CredentialUrl: "creds-url",
					Version:       "version-1",
				},
			})
			Expect(err).To(BeNil())
			Expect(resp).To(Equal(server.UpdateAgentStatus410JSONResponse{}))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
		})
	})

	Context("Update source", func() {
		It("successfully creates the source", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())

			sourceID := uuid.New()
			srv := service.NewAgentServiceHandler(s, logrus.New())
			resp, err := srv.ReplaceSourceStatus(context.TODO(), server.ReplaceSourceStatusRequestObject{
				Id: sourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId:   agentID,
					Inventory: v1alpha1.Inventory{},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ReplaceSourceStatus200JSONResponse{})))

			// according to the multi source model the agent should be associated with the source
			agent, err := s.Agent().Get(context.TODO(), agentID.String())
			Expect(err).To(BeNil())
			Expect(agent.Associated).To(BeTrue())

			// the source should have the agent associated
			source, err := s.Source().Get(context.TODO(), sourceID)
			Expect(err).To(BeNil())
			Expect(source.Agents).ToNot(BeNil())
			Expect(*source.Agents).To(HaveLen(1))
			Expect((*source.Agents)[0].Id).To(Equal(agentID))
		})

		It("agents not associated with the source are not allowed to update inventory", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())

			sourceID := uuid.New()
			srv := service.NewAgentServiceHandler(s, logrus.New())
			resp, err := srv.ReplaceSourceStatus(context.TODO(), server.ReplaceSourceStatusRequestObject{
				Id: sourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId:   agentID,
					Inventory: v1alpha1.Inventory{},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ReplaceSourceStatus200JSONResponse{})))

			// according to the multi source model the agent should be associated with the source
			agent, err := s.Agent().Get(context.TODO(), agentID.String())
			Expect(err).To(BeNil())
			Expect(agent.Associated).To(BeTrue())

			// the source should have the agent associated
			source, err := s.Source().Get(context.TODO(), sourceID)
			Expect(err).To(BeNil())
			Expect(source.Agents).ToNot(BeNil())
			Expect(*source.Agents).To(HaveLen(1))
			Expect((*source.Agents)[0].Id).To(Equal(agentID))

			// make another request from another agent
			secondAgentID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, secondAgentID, "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())

			resp, err = srv.ReplaceSourceStatus(context.TODO(), server.ReplaceSourceStatusRequestObject{
				Id: sourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId:   secondAgentID,
					Inventory: v1alpha1.Inventory{},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ReplaceSourceStatus400JSONResponse{})))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

})
