package service_test

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/events"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertAgentStm             = "INSERT INTO agents (id, status, status_info, cred_url, version) VALUES ('%s', '%s', '%s', '%s', 'version_1');"
	insertAgentWithUsernameStm = "INSERT INTO agents (id, status, status_info, cred_url,username, org_id, version) VALUES ('%s', '%s', '%s', '%s','%s','%s', 'version_1');"
	insertSoftDeletedAgentStm  = "INSERT INTO agents (id, deleted_at) VALUES ('%s', '%s');"
	insertAssociatedAgentStm   = "INSERT INTO agents (id, associated) VALUES ('%s',  TRUE);"
)

var _ = Describe("agent handler", Ordered, func() {
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

	Context("list", func() {
		It("successfully list all the agents", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-1", "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-2", "not-connected", "status-info-2", "cred_url-2"))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))

			resp, err := srv.ListAgents(context.TODO(), server.ListAgentsRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ListAgents200JSONResponse{})))
			Expect(resp).To(HaveLen(2))
		})

		It("successfully list agents -- except soft-deleted agents", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-1", "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, "agent-2", "not-connected", "status-info-2", "cred_url-2"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSoftDeletedAgentStm, "agent-3", time.Now().Format(time.RFC3339)))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))

			resp, err := srv.ListAgents(context.TODO(), server.ListAgentsRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ListAgents200JSONResponse{})))
			Expect(resp).To(HaveLen(2))
		})

		It("successfully list all the agents -- filtered by user", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertAgentWithUsernameStm, "agent-1", "not-connected", "status-info-1", "cred_url-1", "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentWithUsernameStm, "agent-2", "not-connected", "status-info-2", "cred_url-2", "user", "user"))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewUserContext(context.TODO(), user)
			resp, err := srv.ListAgents(ctx, server.ListAgentsRequestObject{})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ListAgents200JSONResponse{})))
			Expect(resp).To(HaveLen(1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("delete", func() {
		It("successfully deletes an unassociated agents", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.DeleteAgent(context.TODO(), server.DeleteAgentRequestObject{Id: agentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.DeleteAgent200JSONResponse{})))

			myAgent, err := s.Agent().Get(context.TODO(), agentID.String())
			Expect(err).To(BeNil())
			Expect(myAgent.DeletedAt).NotTo(BeNil())
		})

		It("fails to delete agent -- 404", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1"))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.DeleteAgent(context.TODO(), server.DeleteAgentRequestObject{Id: uuid.New()})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.DeleteAgent404JSONResponse{})))
		})

		It("fails to delete an associated agent", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssociatedAgentStm, agentID))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.DeleteAgent(context.TODO(), server.DeleteAgentRequestObject{Id: agentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.DeleteAgent400JSONResponse{})))
		})

		It("successfully delete user's agent", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentWithUsernameStm, agentID, "not-connected", "status-info-1", "cred_url-1", "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewUserContext(context.TODO(), user)
			resp, err := srv.DeleteAgent(ctx, server.DeleteAgentRequestObject{Id: agentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.DeleteAgent200JSONResponse{})))
		})

		It("fails to delete other user's agent", func() {
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAgentWithUsernameStm, agentID, "not-connected", "status-info-1", "cred_url-1", "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))

			user := auth.User{
				Username:     "user",
				Organization: "user",
			}
			ctx := auth.NewUserContext(context.TODO(), user)
			resp, err := srv.DeleteAgent(ctx, server.DeleteAgentRequestObject{Id: agentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.DeleteAgent403JSONResponse{})))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})
})
