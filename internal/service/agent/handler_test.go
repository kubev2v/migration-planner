package service_test

import (
	"context"
	"fmt"
	"reflect"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1"
	apiAgent "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	server "github.com/kubev2v/migration-planner/internal/api/server/agent"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/events"
	service "github.com/kubev2v/migration-planner/internal/service/agent"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertAgentStm              = "INSERT INTO agents (id, status, status_info, cred_url, version) VALUES ('%s', '%s', '%s', '%s', 'version_1');"
	insertAgentWithUsernameStm  = "INSERT INTO agents (id, status, status_info, cred_url,username, org_id, version) VALUES ('%s', '%s', '%s', '%s','%s','%s', 'version_1');"
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
		db, err := store.InitDB(config.NewDefault())
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
		_ = s.InitialMigration()
	})

	AfterAll(func() {
		s.Close()
	})

	Context("Update agent status", func() {
		It("successfully creates the agent", func() {
			agentID := uuid.New()

			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter))
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
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			status := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT status from agents WHERE id = '%s';", agentID)).Scan(&status)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("waiting-for-credentials"))

			// should find one event
			<-time.After(500 * time.Millisecond)
			Expect(eventWriter.Messages).To(HaveLen(1))
		})

		It("successfully creates the agent -- under username scope", func() {
			agentID := uuid.New()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewUserContext(context.TODO(), user)

			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.UpdateAgentStatus(ctx, server.UpdateAgentStatusRequestObject{
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
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			d := struct {
				Username string
				Status   string
			}{}
			tx = gormdb.Raw(fmt.Sprintf("SELECT username,status from agents WHERE id = '%s';", agentID)).Scan(&d)
			Expect(tx.Error).To(BeNil())
			Expect(d.Status).To(Equal("waiting-for-credentials"))
			Expect(d.Username).To(Equal("admin"))

			// should find one event
			<-time.After(500 * time.Millisecond)
			Expect(eventWriter.Messages).To(HaveLen(1))
		})

		It("successfully updates the agent", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter))
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
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			status := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT status from agents WHERE id = '%s';", agentID)).Scan(&status)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("waiting-for-credentials"))

			// should find one event
			<-time.After(500 * time.Millisecond)
			Expect(eventWriter.Messages).To(HaveLen(1))
		})

		It("successfully updates the agent -- under agent scope", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentWithUsernameStm, agentID, "not-connected", "status-info-1", "cred_url-1", "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewUserContext(context.TODO(), user)

			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.UpdateAgentStatus(ctx, server.UpdateAgentStatusRequestObject{
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
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			d := struct {
				Username string
				Status   string
			}{}
			tx = gormdb.Raw(fmt.Sprintf("SELECT username,status from agents WHERE id = '%s';", agentID)).Scan(&d)
			Expect(tx.Error).To(BeNil())
			Expect(d.Status).To(Equal("waiting-for-credentials"))
			Expect(d.Username).To(Equal("admin"))

			// should find one event
			<-time.After(500 * time.Millisecond)
			Expect(eventWriter.Messages).To(HaveLen(1))
		})

		It("failed to update the agent -- username is different", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentWithUsernameStm, agentID, "not-connected", "status-info-1", "cred_url-1", "user", "user"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewUserContext(context.TODO(), user)

			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.UpdateAgentStatus(ctx, server.UpdateAgentStatusRequestObject{
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
			Expect(resp).To(Equal(server.UpdateAgentStatus403JSONResponse{}))
		})

		It("should receive 410 when agent is soft deleted", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentWithDeletedAtStm, agentID, "not-connected", "status-info-1", "cred_url-1", time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339)))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter))
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
			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter))
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
			Expect(source.Agents).To(HaveLen(1))
			Expect(source.Agents[0].ID).To(Equal(agentID.String()))

			// should have one 1 event only
			<-time.After(500 * time.Millisecond)
			Expect(eventWriter.Messages).To(HaveLen(1))
		})

		It("successfully creates the source -- under user's scope", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentWithUsernameStm, agentID, "not-connected", "status-info-1", "cred_url-1", "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			sourceID := uuid.New()
			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter))

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewUserContext(context.TODO(), user)

			resp, err := srv.ReplaceSourceStatus(ctx, server.ReplaceSourceStatusRequestObject{
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
			Expect(source.Agents).To(HaveLen(1))
			Expect(source.Agents[0].ID).To(Equal(agentID.String()))

			// should have one 1 event only
			<-time.After(500 * time.Millisecond)
			Expect(eventWriter.Messages).To(HaveLen(1))
		})

		It("fail to creates the source -- under user's scope", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentWithUsernameStm, agentID, "not-connected", "status-info-1", "cred_url-1", "user", "user"))
			Expect(tx.Error).To(BeNil())

			sourceID := uuid.New()
			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter))

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewUserContext(context.TODO(), user)

			resp, err := srv.ReplaceSourceStatus(ctx, server.ReplaceSourceStatusRequestObject{
				Id: sourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId:   agentID,
					Inventory: v1alpha1.Inventory{},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ReplaceSourceStatus403JSONResponse{})))
		})

		It("agents not associated with the source are not allowed to update inventory", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())

			sourceID := uuid.New()
			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.ReplaceSourceStatus(context.TODO(), server.ReplaceSourceStatusRequestObject{
				Id: sourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId:   agentID,
					Inventory: v1alpha1.Inventory{},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ReplaceSourceStatus200JSONResponse{})))

			// should have 1 event (inventory)
			<-time.After(500 * time.Millisecond)
			Expect(eventWriter.Messages).To(HaveLen(1))

			// according to the multi source model the agent should be associated with the source
			agent, err := s.Agent().Get(context.TODO(), agentID.String())
			Expect(err).To(BeNil())
			Expect(agent.Associated).To(BeTrue())

			// the source should have the agent associated
			source, err := s.Source().Get(context.TODO(), sourceID)
			Expect(err).To(BeNil())
			Expect(source.Agents).ToNot(BeNil())
			Expect(source.Agents).To(HaveLen(1))
			Expect(source.Agents[0].ID).To(Equal(agentID.String()))

			//			 make another request from another agent
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

			// should have one 1 event only
			<-time.After(500 * time.Millisecond)
			Expect(eventWriter.Messages).To(HaveLen(1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})
})

type testwriter struct {
	Messages []cloudevents.Event
}

func newTestWriter() *testwriter {
	return &testwriter{Messages: []cloudevents.Event{}}
}

func (t *testwriter) Write(ctx context.Context, topic string, e cloudevents.Event) error {
	t.Messages = append(t.Messages, e)
	return nil
}

func (t *testwriter) Close(_ context.Context) error {
	return nil
}
