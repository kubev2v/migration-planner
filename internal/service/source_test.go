package service_test

import (
	"context"
	"fmt"
	"reflect"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
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
	insertAgentStm              = "INSERT INTO agents (id, status, status_info, cred_url,source_id, version) VALUES ('%s', '%s', '%s', '%s', '%s', 'version_1');"
	insertSourceWithUsernameStm = "INSERT INTO sources (id, name, username, org_id) VALUES ('%s', 'source_name', '%s', '%s');"
	insertSourceOnPremisesStm   = "INSERT INTO sources (id, name, username, org_id, on_premises) VALUES ('%s', '%s', '%s', '%s', TRUE);"
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

			eventWriter := newTestWriter()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.ListSources(ctx, server.ListSourcesRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.ListSources200JSONResponse{}).String()))
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

			eventWriter := newTestWriter()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.ListSources(ctx, server.ListSourcesRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.ListSources200JSONResponse{}).String()))
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
			eventWriter := newTestWriter()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.CreateSource(ctx, server.CreateSourceRequestObject{
				Body: &v1alpha1.CreateSourceJSONRequestBody{
					Name: "test",
				},
			})
			Expect(err).To(BeNil())
			source, ok := resp.(server.CreateSource201JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(source.Name).To(Equal("test"))

			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("successfully creates a source -- with proxy paramters defined", func() {
			eventWriter := newTestWriter()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			toStrPtr := func(s string) *string {
				return &s
			}

			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.CreateSource(ctx, server.CreateSourceRequestObject{
				Body: &v1alpha1.CreateSourceJSONRequestBody{
					Name: "test",
					Proxy: &v1alpha1.AgentProxy{
						HttpUrl:  toStrPtr("http"),
						HttpsUrl: toStrPtr("https"),
						NoProxy:  toStrPtr("noproxy"),
					},
				},
			})
			Expect(err).To(BeNil())
			source, ok := resp.(server.CreateSource201JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(source.Name).To(Equal("test"))

			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("successfully creates a source -- with certificate chain defined", func() {
			eventWriter := newTestWriter()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			toStrPtr := func(s string) *string {
				return &s
			}

			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.CreateSource(ctx, server.CreateSourceRequestObject{
				Body: &v1alpha1.CreateSourceJSONRequestBody{
					Name:             "test",
					CertificateChain: toStrPtr("chain"),
				},
			})
			Expect(err).To(BeNil())
			source, ok := resp.(server.CreateSource201JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(source.Name).To(Equal("test"))

			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
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

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.GetSource(ctx, server.GetSourceRequestObject{Id: firstSourceID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSource200JSONResponse{}).String()))

			source := resp.(server.GetSource200JSONResponse)
			Expect(source.Id.String()).To(Equal(firstSourceID.String()))
			Expect(source.Agent).NotTo(BeNil())
			Expect(source.Agent.Id.String()).To(Equal(firstAgentID.String()))
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

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.GetSource(ctx, server.GetSourceRequestObject{Id: uuid.New()})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSource404JSONResponse{}).String()))
		})

		It("failed to get source - 403", func() {
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
				Username:     "joker",
				Organization: "joker",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.GetSource(ctx, server.GetSourceRequestObject{Id: firstSourceID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSource403JSONResponse{}).String()))
		})

		AfterEach(func() {
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
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			_, err := srv.DeleteSource(ctx, server.DeleteSourceRequestObject{Id: firstSourceID})
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

		It("fails to delete a source -- under user's scope", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			eventWriter := newTestWriter()

			user := auth.User{
				Username:     "user",
				Organization: "user",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.DeleteSource(ctx, server.DeleteSourceRequestObject{Id: firstSourceID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteSource403JSONResponse{}).String()))
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

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.SourceUpdateOnPrem{
					AgentId: uuid.New(),
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource200JSONResponse{}).String()))

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

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.SourceUpdateOnPrem{
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource200JSONResponse{}).String()))

			vCenterID := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT v_center_id FROM SOURCES where id = '%s';", firstSourceID)).Scan(&vCenterID)
			Expect(tx.Error).To(BeNil())
			Expect(vCenterID).To(Equal("vcenter"))

			onPrem := false
			tx = gormdb.Raw(fmt.Sprintf("SELECT on_premises FROM SOURCES where id = '%s';", firstSourceID)).Scan(&onPrem)
			Expect(tx.Error).To(BeNil())
			Expect(onPrem).To(BeTrue())

			resp, err = srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.SourceUpdateOnPrem{
					AgentId: uuid.New(),
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource200JSONResponse{}).String()))
		})

		It("fails to update source on prem -- different vcenter", func() {
			firstSourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			eventWriter := newTestWriter()
			srv := service.NewServiceHandler(s, events.NewEventProducer(eventWriter))
			resp, err := srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.SourceUpdateOnPrem{
					AgentId: uuid.New(),
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource200JSONResponse{}).String()))

			vCenterID := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT v_center_id FROM SOURCES where id = '%s';", firstSourceID)).Scan(&vCenterID)
			Expect(tx.Error).To(BeNil())
			Expect(vCenterID).To(Equal("vcenter"))

			onPrem := false
			tx = gormdb.Raw(fmt.Sprintf("SELECT on_premises FROM SOURCES where id = '%s';", firstSourceID)).Scan(&onPrem)
			Expect(tx.Error).To(BeNil())
			Expect(onPrem).To(BeTrue())

			resp, err = srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.SourceUpdateOnPrem{
					AgentId: uuid.New(),
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "another-vcenter-id",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource400JSONResponse{}).String()))
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
