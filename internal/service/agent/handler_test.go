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
	insertSourceWithUsernameStm = "INSERT INTO sources (id, name, username, org_id) VALUES ('%s', 'source_name', '%s', '%s');"
	insertAgentStm              = "INSERT INTO agents (id, status, status_info, cred_url, source_id, version) VALUES ('%s', '%s', '%s', '%s', '%s', 'version_1');"
	insertAgentWithUpdateAtStm  = "INSERT INTO agents (id, status, status_info, cred_url, updated_at, version) VALUES ('%s', '%s', '%s', '%s', '%s', 'version_1');"
)

var _ = Describe("agent service", Ordered, func() {
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
		_ = s.InitialMigration()
	})

	AfterAll(func() {
		s.Close()
	})

	Context("Update agent status", func() {
		It("successfully creates the agent", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			agentID := uuid.New()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandlerLogger(service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter)))
			resp, err := srv.UpdateAgentStatus(ctx, server.UpdateAgentStatusRequestObject{
				Id: agentID,
				Body: &apiAgent.UpdateAgentStatusJSONRequestBody{
					Status:        string(v1alpha1.AgentStatusWaitingForCredentials),
					StatusInfo:    "waiting-for-credentials",
					CredentialUrl: "creds-url",
					Version:       "version-1",
					SourceId:      sourceID,
				},
			})
			Expect(err).To(BeNil())
			Expect(resp).To(Equal(server.UpdateAgentStatus201Response{}))

			count := -1
			tx = gormdb.Raw("SELECT COUNT(*) FROM agents;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			status := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT status from agents WHERE id = '%s';", agentID)).Scan(&status)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("waiting-for-credentials"))

			status_info := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT status_info from agents WHERE id = '%s';", agentID)).Scan(&status_info)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("waiting-for-credentials"))

			credsUrl := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT cred_url from agents WHERE id = '%s';", agentID)).Scan(&credsUrl)
			Expect(tx.Error).To(BeNil())
			Expect(credsUrl).To(Equal("creds-url"))

			// should find one event
			<-time.After(500 * time.Millisecond)
			Expect(eventWriter.Messages).To(HaveLen(1))
		})
		It("successfully updates the agent", func() {
			sourceID := uuid.NewString()
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandlerLogger(service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter)))
			resp, err := srv.UpdateAgentStatus(ctx, server.UpdateAgentStatusRequestObject{
				Id: agentID,
				Body: &apiAgent.UpdateAgentStatusJSONRequestBody{
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

			status_info := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT status_info from agents WHERE id = '%s';", agentID)).Scan(&status_info)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("waiting-for-credentials"))

			credsUrl := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT cred_url from agents WHERE id = '%s';", agentID)).Scan(&credsUrl)
			Expect(tx.Error).To(BeNil())
			Expect(credsUrl).To(Equal("creds-url"))

			// should find one event
			<-time.After(500 * time.Millisecond)
			Expect(eventWriter.Messages).To(HaveLen(1))
		})

		It("failed to update agent -- source id is missing", func() {
			sourceID := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "batman",
				Organization: "wayne_enterprises",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandlerLogger(service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter)))
			resp, err := srv.UpdateAgentStatus(ctx, server.UpdateAgentStatusRequestObject{
				Id: uuid.New(),
				Body: &apiAgent.UpdateAgentStatusJSONRequestBody{
					Status:        string(v1alpha1.AgentStatusWaitingForCredentials),
					StatusInfo:    "waiting-for-credentials",
					CredentialUrl: "creds-url",
					Version:       "version-1",
					SourceId:      uuid.New(),
				},
			})
			Expect(err).To(BeNil())
			Expect(resp).To(Equal(server.UpdateAgentStatus400JSONResponse{}))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("Update source", func() {
		It("successfully updates the source", func() {
			sourceID := uuid.New()
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandlerLogger(service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter)))
			resp, err := srv.UpdateSourceInventory(ctx, server.UpdateSourceInventoryRequestObject{
				Id: sourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId: agentID,
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSourceInventory200JSONResponse{}).String()))

			// the source should have the agent associated
			source, err := s.Source().Get(ctx, sourceID)
			Expect(err).To(BeNil())
			Expect(source.Inventory.Data.Vcenter.Id).To(Equal("vcenter"))

			// should have one 1 event only
			<-time.After(500 * time.Millisecond)
			Expect(eventWriter.Messages).To(HaveLen(1))
		})

		It("successfully updates the source - two agents", func() {
			sourceID := uuid.New()
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			secondAgentID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, secondAgentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			// first agent request
			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandlerLogger(service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter)))
			resp, err := srv.UpdateSourceInventory(ctx, server.UpdateSourceInventoryRequestObject{
				Id: sourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId: agentID,
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSourceInventory200JSONResponse{}).String()))

			// the source should have the agent associated
			source, err := s.Source().Get(ctx, sourceID)
			Expect(err).To(BeNil())
			Expect(source.Inventory.Data.Vcenter.Id).To(Equal("vcenter"))

			// second agent request
			resp, err = srv.UpdateSourceInventory(ctx, server.UpdateSourceInventoryRequestObject{
				Id: sourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId: secondAgentID,
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSourceInventory200JSONResponse{}).String()))

			// should have one 1 event only
			<-time.After(500 * time.Millisecond)
			Expect(eventWriter.Messages).To(HaveLen(2))
		})

		It("agents not associated with the source are not allowed to update inventory", func() {
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

			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandlerLogger(service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter)))
			resp, err := srv.UpdateSourceInventory(ctx, server.UpdateSourceInventoryRequestObject{
				Id: firstSourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId:   secondSourceID,
					Inventory: v1alpha1.Inventory{},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSourceInventory400JSONResponse{}).String()))
		})

		It("updates with a different vCenter are not allowed", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			eventWriter := newTestWriter()
			srv := service.NewAgentServiceHandlerLogger(service.NewAgentServiceHandler(s, events.NewEventProducer(eventWriter)))
			resp, err := srv.UpdateSourceInventory(ctx, server.UpdateSourceInventoryRequestObject{
				Id: firstSourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId: firstAgentID,
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSourceInventory200JSONResponse{}).String()))

			resp, err = srv.UpdateSourceInventory(ctx, server.UpdateSourceInventoryRequestObject{
				Id: firstSourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId: firstSourceID,
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "anotherVCenterID",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSourceInventory400JSONResponse{}).String()))

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
