package v1alpha1_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1"
	apiAgent "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	server "github.com/kubev2v/migration-planner/internal/api/server/agent"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

var _ = Describe("agent service", Ordered, func() {
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

			srv := handlers.NewAgentHandler(service.NewAgentService(s))
			resp, err := srv.UpdateAgentStatus(ctx, server.UpdateAgentStatusRequestObject{
				Id: agentID,
				Body: &apiAgent.UpdateAgentStatusJSONRequestBody{
					Status:        string(v1alpha1.AgentStatusWaitingForCredentials),
					StatusInfo:    "waiting-for-credentials",
					CredentialUrl: "http://agent.com",
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
			Expect(credsUrl).To(Equal("http://agent.com"))
		})

		It("failed to create the agent -- form not valid", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			agentID := uuid.New()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewAgentHandler(service.NewAgentService(s))
			resp, err := srv.UpdateAgentStatus(ctx, server.UpdateAgentStatusRequestObject{
				Id: agentID,
				Body: &apiAgent.UpdateAgentStatusJSONRequestBody{
					Status:        string(v1alpha1.AgentStatusWaitingForCredentials),
					StatusInfo:    "waiting-for-credentials",
					CredentialUrl: "creds_url",
					Version:       "version-1",
					SourceId:      sourceID,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateAgentStatus400JSONResponse{}).String()))

			count := -1
			tx = gormdb.Raw("SELECT COUNT(*) FROM agents;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("successfully updates the agent", func() {
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

			srv := handlers.NewAgentHandler(service.NewAgentService(s))
			resp, err := srv.UpdateAgentStatus(ctx, server.UpdateAgentStatusRequestObject{
				Id: agentID,
				Body: &apiAgent.UpdateAgentStatusJSONRequestBody{
					SourceId:      sourceID,
					Status:        string(v1alpha1.AgentStatusWaitingForCredentials),
					StatusInfo:    "waiting-for-credentials",
					CredentialUrl: "http://agent.com",
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
			Expect(credsUrl).To(Equal("http://agent.com"))
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

			srv := handlers.NewAgentHandler(service.NewAgentService(s))
			resp, err := srv.UpdateAgentStatus(ctx, server.UpdateAgentStatusRequestObject{
				Id: uuid.New(),
				Body: &apiAgent.UpdateAgentStatusJSONRequestBody{
					Status:        string(v1alpha1.AgentStatusWaitingForCredentials),
					StatusInfo:    "waiting-for-credentials",
					CredentialUrl: "http://agent.com",
					Version:       "version-1",
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateAgentStatus400JSONResponse{}).String()))
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

			srv := handlers.NewAgentHandler(service.NewAgentService(s))
			resp, err := srv.UpdateSourceInventory(ctx, server.UpdateSourceInventoryRequestObject{
				Id: sourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId: agentID,
					Inventory: v1alpha1.Inventory{
						VcenterId: "vcenter",
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSourceInventory200JSONResponse{}).String()))

			// the source should have the agent associated
			source, err := s.Source().Get(ctx, sourceID)
			Expect(err).To(BeNil())

			var inventory v1alpha1.Inventory
			err = json.Unmarshal(source.Inventory, &inventory)
			Expect(err).To(BeNil())
			Expect(inventory.VcenterId).To(Equal("vcenter"))
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
			srv := handlers.NewAgentHandler(service.NewAgentService(s))
			resp, err := srv.UpdateSourceInventory(ctx, server.UpdateSourceInventoryRequestObject{
				Id: sourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId: agentID,
					Inventory: v1alpha1.Inventory{
						VcenterId: "vcenter",
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSourceInventory200JSONResponse{}).String()))

			// the source should have the agent associated
			source, err := s.Source().Get(ctx, sourceID)
			Expect(err).To(BeNil())

			var inventory v1alpha1.Inventory
			err = json.Unmarshal(source.Inventory, &inventory)
			Expect(err).To(BeNil())
			Expect(inventory.VcenterId).To(Equal("vcenter"))

			// second agent request
			resp, err = srv.UpdateSourceInventory(ctx, server.UpdateSourceInventoryRequestObject{
				Id: sourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId: secondAgentID,
					Inventory: v1alpha1.Inventory{
						VcenterId: "vcenter",
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSourceInventory200JSONResponse{}).String()))
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

			srv := handlers.NewAgentHandler(service.NewAgentService(s))
			resp, err := srv.UpdateSourceInventory(ctx, server.UpdateSourceInventoryRequestObject{
				Id: secondSourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId:   firstAgentID,
					Inventory: v1alpha1.Inventory{},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSourceInventory403JSONResponse{}).String()))
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

			srv := handlers.NewAgentHandler(service.NewAgentService(s))
			resp, err := srv.UpdateSourceInventory(ctx, server.UpdateSourceInventoryRequestObject{
				Id: firstSourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId: firstAgentID,
					Inventory: v1alpha1.Inventory{
						VcenterId: "vcenter",
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSourceInventory200JSONResponse{}).String()))

			resp, err = srv.UpdateSourceInventory(ctx, server.UpdateSourceInventoryRequestObject{
				Id: firstSourceID,
				Body: &apiAgent.SourceStatusUpdate{
					AgentId: firstAgentID,
					Inventory: v1alpha1.Inventory{
						VcenterId: "anotherVCenterID",
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
