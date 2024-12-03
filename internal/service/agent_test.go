package service_test

import (
	"context"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/events"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertAgentStm           = "INSERT INTO agents (id, status, status_info, cred_url, version) VALUES ('%s', '%s', '%s', '%s', 'version_1');"
	insertAssociatedAgentStm = "INSERT INTO agents (id, associated) VALUES ('%s',  TRUE);"
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

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
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

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
		})
	})
})
