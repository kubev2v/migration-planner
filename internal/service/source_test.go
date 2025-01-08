package service_test

import (
	"context"
	"fmt"
	"reflect"

	cloudevents "github.com/cloudevents/sdk-go/v2"
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
	insertAgentWithSourceStm    = "INSERT INTO agents (id, source_id,associated) VALUES ('%s', '%s',  TRUE);"
	insertSourceStm             = "INSERT INTO sources (id) VALUES ('%s');"
	insertSourceWithUsernameStm = "INSERT INTO sources (id,username, org_id) VALUES ('%s', '%s', '%s');"
)

var _ = Describe("source handler", Ordered, func() {
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
		It("successfully list all the sources", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString()))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString()))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.ListSources(context.TODO(), server.ListSourcesRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ListSources200JSONResponse{})))
			Expect(resp).To(HaveLen(2))
		})

		It("successfully list all the sources -- filtered by user", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, uuid.NewString(), "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, uuid.NewString(), "cosmin", "cosmin"))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewUserContext(context.TODO(), user)

			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.ListSources(ctx, server.ListSourcesRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ListSources200JSONResponse{})))
			Expect(resp).To(HaveLen(1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("get", func() {
		It("successfully retrieve the source", func() {
			firstSource := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, firstSource))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString()))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.ReadSource(context.TODO(), server.ReadSourceRequestObject{Id: firstSource})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ReadSource200JSONResponse{})))
		})
		It("failed to get source - 404", func() {
			firstSource := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, firstSource))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString()))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.ReadSource(context.TODO(), server.ReadSourceRequestObject{Id: uuid.New()})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ReadSource404JSONResponse{})))
		})
		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("delete", func() {
		It("successfully deletes all the sources", func() {
			firstSource := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, firstSource))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString()))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			_, err := srv.DeleteSources(context.TODO(), server.DeleteSourcesRequestObject{})
			Expect(err).To(BeNil())

			count := 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM SOURCES;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

		})

		It("successfully deletes a source", func() {
			source := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, source))
			Expect(tx.Error).To(BeNil())

			agent := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentWithSourceStm, agent.String(), source.String()))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			_, err := srv.DeleteSource(context.TODO(), server.DeleteSourceRequestObject{Id: source})
			Expect(err).To(BeNil())

			count := 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM SOURCES;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			// we still have an agent but it's soft deleted
			count = 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM AGENTS;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			myAgent, err := s.Agent().Get(context.TODO(), agent.String())
			Expect(err).To(BeNil())
			Expect(myAgent.DeletedAt).NotTo(BeNil())
		})

		It("successfully deletes a source -- under user's scope", func() {
			source := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, source, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			agent := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentWithSourceStm, agent.String(), source.String()))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewUserContext(context.TODO(), user)

			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			_, err := srv.DeleteSource(ctx, server.DeleteSourceRequestObject{Id: source})
			Expect(err).To(BeNil())

			count := 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM SOURCES;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			// we still have an agent but it's soft deleted
			count = 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM AGENTS;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			myAgent, err := s.Agent().Get(context.TODO(), agent.String())
			Expect(err).To(BeNil())
			Expect(myAgent.DeletedAt).NotTo(BeNil())
		})

		It("fails to delete a source -- under user's scope", func() {
			source := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, source, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			agent := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentWithSourceStm, agent.String(), source.String()))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()

			user := auth.User{
				Username:     "user",
				Organization: "user",
			}
			ctx := auth.NewUserContext(context.TODO(), user)

			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.DeleteSource(ctx, server.DeleteSourceRequestObject{Id: source})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.DeleteSource403JSONResponse{})))
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
