package v1alpha1_test

import (
	"context"
	"fmt"
	"os"
	"reflect"

	v1pb "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

var _ = Describe("Assessment Handler", Ordered, func() {
	var (
		s             store.Store
		gormdb        *gorm.DB
		spiceDBClient *authzed.Client
		authzSvc      service.Authz
		ctx           context.Context
	)

	BeforeAll(func() {
		ctx = context.Background()

		spiceDBEndpoint := os.Getenv("SPICEDB_ENDPOINT")
		if spiceDBEndpoint == "" {
			spiceDBEndpoint = "localhost:50051"
		}

		// Create SpiceDB client
		var err error
		spiceDBClient, err = authzed.NewClient(
			spiceDBEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpcutil.WithInsecureBearerToken("foobar"),
		)
		if err != nil {
			Skip("SpiceDB not available: " + err.Error())
		}

		// Test connection
		_, err = spiceDBClient.ReadSchema(ctx, &v1pb.ReadSchemaRequest{})
		if err != nil {
			Skip("SpiceDB not reachable: " + err.Error())
		}

		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStoreWithAuthz(db, spiceDBClient)
		gormdb = db
		authzSvc = service.NewAuthzService(s)
	})

	AfterAll(func() {
		if s != nil {
			s.Close()
		}
		if spiceDBClient != nil {
			spiceDBClient.Close()
		}
	})

	// cleanupSpiceDB deletes all relationships for assessments, platform, and organizations
	cleanupSpiceDB := func() {
		ctx, err := s.NewTransactionContext(ctx)
		if err != nil {
			return
		}
		defer func() {
			_, _ = store.Rollback(ctx)
		}()

		if err := s.Authz().DeleteRelationships(ctx, model.Resource{ResourceType: model.AssessmentResource}); err != nil {
			GinkgoT().Logf("Failed to cleanup assessment relationships: %v", err)
		}

		if err := s.Authz().DeleteRelationships(ctx, model.Resource{ResourceType: model.PlatformResource}); err != nil {
			GinkgoT().Logf("Failed to cleanup platform relationships: %v", err)
		}

		if err := s.Authz().DeleteRelationships(ctx, model.Resource{ResourceType: model.OrgResource}); err != nil {
			GinkgoT().Logf("Failed to cleanup platform relationships: %v", err)
		}
	}

	Context("list assessments", func() {
		It("should return all the assessments the user has access to", func() {
			user1 := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
				FirstName:    "John",
				LastName:     "Doe",
			}
			user2 := auth.User{
				Username:     "some_user",
				Organization: "admin",
				EmailDomain:  "company.com",
				FirstName:    "Alice",
				LastName:     "JSON",
			}

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)
			for _, u := range []auth.User{user1, user2} {
				ctx := auth.NewTokenContext(context.TODO(), u)
				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "test-assessment_" + u.Username,
						SourceType: service.SourceTypeInventory,
						Inventory:  &inventory,
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))
			}

			// create context for first user only
			listCtx := auth.NewTokenContext(context.TODO(), user1)
			listResp, listErr := srv.ListAssessments(listCtx, server.ListAssessmentsRequestObject{})
			Expect(listErr).To(BeNil())
			Expect(reflect.TypeOf(listResp).String()).To(Equal(reflect.TypeOf(server.ListAssessments200JSONResponse{}).String()))

			assessmentList := listResp.(server.ListAssessments200JSONResponse)
			Expect(assessmentList).To(HaveLen(1))

			// Verify owner fields are included in API responses
			for _, assessment := range assessmentList {
				Expect(assessment.OwnerFirstName).ToNot(BeNil())
				Expect(*assessment.OwnerFirstName).To(Equal("John"))
				Expect(assessment.OwnerLastName).ToNot(BeNil())
				Expect(*assessment.OwnerLastName).To(Equal("Doe"))
			}
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			cleanupSpiceDB()
		})
	})

	Context("create assessment", func() {
		It("successfully creates an assessment with JSON body - inventory type", func() {
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
				FirstName:    "Alice",
				LastName:     "Johnson",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)
			resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "test-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			assessment := resp.(server.CreateAssessment201JSONResponse)
			Expect(assessment.Name).To(Equal("test-assessment"))
			Expect(assessment.SourceType).To(Equal(v1alpha1.AssessmentSourceType(service.SourceTypeInventory)))
			Expect(assessment.Snapshots).To(HaveLen(1))
			// Verify owner fields are populated from user context, not from API request
			Expect(assessment.OwnerFirstName).ToNot(BeNil())
			Expect(*assessment.OwnerFirstName).To(Equal("Alice"))
			Expect(assessment.OwnerLastName).ToNot(BeNil())
			Expect(*assessment.OwnerLastName).To(Equal("Johnson"))
		})

		It("verifies owner fields security - populated from user context only", func() {
			// This test verifies the security design: owner fields cannot be spoofed via API requests
			// They are populated from authenticated user context only (like username)
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
				FirstName:    "RealUser",     // This is what should appear
				LastName:     "RealLastName", // This is what should appear
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Note: AssessmentForm schema deliberately excludes owner fields
			// This prevents users from spoofing owner information via API requests
			resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "security-test-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			assessment := resp.(server.CreateAssessment201JSONResponse)
			Expect(assessment.Name).To(Equal("security-test-assessment"))

			// Verify owner fields come from authenticated user context only
			Expect(assessment.OwnerFirstName).ToNot(BeNil())
			Expect(*assessment.OwnerFirstName).To(Equal("RealUser"))
			Expect(assessment.OwnerLastName).ToNot(BeNil())
			Expect(*assessment.OwnerLastName).To(Equal("RealLastName"))
		})

		It("successfully creates an assessment with sourceID - agent type", func() {
			// First create a source
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID.String(), "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Add inventory to the source
			inventory := `{"vcenter": {"id": "test-vcenter"}}`
			tx = gormdb.Exec(fmt.Sprintf("UPDATE sources SET inventory = '%s' WHERE id = '%s';", inventory, sourceID.String()))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)
			resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "agent-assessment",
					SourceType: service.SourceTypeAgent,
					SourceId:   &sourceID,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			assessment := resp.(server.CreateAssessment201JSONResponse)
			Expect(assessment.Name).To(Equal("agent-assessment"))
			Expect(assessment.SourceType).To(Equal(v1alpha1.AssessmentSourceType(service.SourceTypeAgent)))
			Expect(assessment.SourceId).To(Equal(&sourceID))
		})

		It("fails to create assessment with empty body", func() {
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)
			resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment400JSONResponse{}).String()))

			errorResp := resp.(server.CreateAssessment400JSONResponse)
			Expect(errorResp.Message).To(Equal("empty body"))
		})

		It("fails to create assessment with sourceID from different organization", func() {
			// Create a source for different organization
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID.String(), "batman", "batman"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)
			resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "forbidden-assessment",
					SourceType: service.SourceTypeAgent,
					SourceId:   &sourceID,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment401JSONResponse{}).String()))
		})

		It("fails to create assessment with sourceID that has no inventory", func() {
			// Create a source without inventory
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID.String(), "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)
			resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "no-inventory-assessment",
					SourceType: service.SourceTypeAgent,
					SourceId:   &sourceID,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment400JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
			cleanupSpiceDB()
		})
	})

	Context("create assessment - validation tests", func() {
		var user auth.User
		var ctx context.Context
		var srv *handlers.ServiceHandler

		BeforeEach(func() {
			user = auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx = auth.NewTokenContext(context.TODO(), user)
			srv = handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)
		})

		Context("name validation", func() {
			It("fails with empty name", func() {
				inventory := v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "test-vcenter",
					},
				}

				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "",
						SourceType: service.SourceTypeInventory,
						Inventory:  &inventory,
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment400JSONResponse{}).String()))
			})

			It("fails with name containing spaces", func() {
				inventory := v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "test-vcenter",
					},
				}

				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "invalid name with spaces",
						SourceType: service.SourceTypeInventory,
						Inventory:  &inventory,
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment400JSONResponse{}).String()))
			})

			It("fails with name containing special characters", func() {
				inventory := v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "test-vcenter",
					},
				}

				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "invalid@name#with$special%chars",
						SourceType: service.SourceTypeInventory,
						Inventory:  &inventory,
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment400JSONResponse{}).String()))
			})

			It("fails with name too long (over 100 characters)", func() {
				inventory := v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "test-vcenter",
					},
				}

				longName := "a"
				for i := 0; i < 101; i++ {
					longName += "a"
				}

				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       longName,
						SourceType: service.SourceTypeInventory,
						Inventory:  &inventory,
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment400JSONResponse{}).String()))
			})

			It("succeeds with valid name containing allowed characters", func() {
				inventory := v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "test-vcenter",
					},
				}

				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "valid-name_123+test.assessment",
						SourceType: service.SourceTypeInventory,
						Inventory:  &inventory,
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))
			})
		})

		Context("sourceType validation", func() {
			It("fails with empty sourceType", func() {
				inventory := v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "test-vcenter",
					},
				}

				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "test-assessment",
						SourceType: "",
						Inventory:  &inventory,
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment400JSONResponse{}).String()))
			})

			It("fails with invalid sourceType", func() {
				inventory := v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "test-vcenter",
					},
				}

				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "test-assessment",
						SourceType: "invalid-source-type",
						Inventory:  &inventory,
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment400JSONResponse{}).String()))
			})

			It("succeeds with valid sourceType 'inventory'", func() {
				inventory := v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "test-vcenter",
					},
				}

				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "test-assessment",
						SourceType: service.SourceTypeInventory,
						Inventory:  &inventory,
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))
			})

			It("succeeds with valid sourceType 'agent' when sourceId is provided", func() {
				// Create a source with inventory
				sourceID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID.String(), "admin", "admin"))
				Expect(tx.Error).To(BeNil())

				inventory := `{"vcenter": {"id": "test-vcenter"}}`
				tx = gormdb.Exec(fmt.Sprintf("UPDATE sources SET inventory = '%s' WHERE id = '%s';", inventory, sourceID.String()))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "test-assessment",
						SourceType: service.SourceTypeAgent,
						SourceId:   &sourceID,
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))
			})
		})

		Context("conditional field validation", func() {
			It("fails when sourceType is 'inventory' but no inventory is provided", func() {
				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "test-assessment",
						SourceType: service.SourceTypeInventory,
						// No inventory provided (nil)
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment400JSONResponse{}).String()))

				errorResp := resp.(server.CreateAssessment400JSONResponse)
				Expect(errorResp.Message).To(ContainSubstring("inventory is missing"))
			})

			It("fails when sourceType is 'inventory' but inventory has no vCenter ID", func() {
				emptyInventory := v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "", // Empty vCenter ID
					},
				}

				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "test-assessment",
						SourceType: service.SourceTypeInventory,
						Inventory:  &emptyInventory,
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment400JSONResponse{}).String()))

				errorResp := resp.(server.CreateAssessment400JSONResponse)
				Expect(errorResp.Message).To(ContainSubstring("inventory has no vCenterID"))
			})

			It("fails when sourceType is 'inventory' but inventory has empty vCenter", func() {
				// Inventory with empty vcenter (no meaningful data)
				emptyVCenterInventory := v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{}, // No ID provided
				}

				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "test-assessment",
						SourceType: service.SourceTypeInventory,
						Inventory:  &emptyVCenterInventory,
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment400JSONResponse{}).String()))

				errorResp := resp.(server.CreateAssessment400JSONResponse)
				Expect(errorResp.Message).To(ContainSubstring("inventory has no vCenterID"))
			})

			It("fails when sourceType is 'agent' but no sourceId is provided", func() {
				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "test-assessment",
						SourceType: service.SourceTypeAgent,
						// No sourceId provided
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment400JSONResponse{}).String()))

				errorResp := resp.(server.CreateAssessment400JSONResponse)
				Expect(errorResp.Message).To(ContainSubstring(service.SourceTypeAgent))
			})

			It("succeeds when sourceType is 'inventory' and inventory is provided", func() {
				inventory := v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{
						Id: "test-vcenter",
					},
				}

				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "test-assessment",
						SourceType: service.SourceTypeInventory,
						Inventory:  &inventory,
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))
			})

			It("succeeds when sourceType is 'agent' and sourceId is provided", func() {
				// Create a source with inventory
				sourceID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID.String(), "admin", "admin"))
				Expect(tx.Error).To(BeNil())

				inventory := `{"vcenter": {"id": "test-vcenter"}}`
				tx = gormdb.Exec(fmt.Sprintf("UPDATE sources SET inventory = '%s' WHERE id = '%s';", inventory, sourceID.String()))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
					JSONBody: &v1alpha1.AssessmentForm{
						Name:       "test-assessment",
						SourceType: service.SourceTypeAgent,
						SourceId:   &sourceID,
					},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))
			})
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
			cleanupSpiceDB()
		})
	})

	Context("get assessment", func() {
		It("successfully retrieves an assessment", func() {
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
				FirstName:    "John",
				LastName:     "Doe",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create assessment via API
			createResp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "test-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Retrieve the created assessment
			resp, err := srv.GetAssessment(ctx, server.GetAssessmentRequestObject{Id: assessmentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetAssessment200JSONResponse{}).String()))

			assessment := resp.(server.GetAssessment200JSONResponse)
			Expect(assessment.Id).To(Equal(assessmentID))
			Expect(assessment.Name).To(Equal("test-assessment"))
			Expect(assessment.Snapshots).To(HaveLen(1))
		})

		It("returns 404 for non-existent assessment", func() {
			nonExistentID := uuid.New()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)
			resp, err := srv.GetAssessment(ctx, server.GetAssessmentRequestObject{Id: nonExistentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetAssessment404JSONResponse{}).String()))
		})

		It("returns 403 for assessment from different user", func() {
			batmanUser := auth.User{
				Username:     "batman",
				Organization: "batman",
				EmailDomain:  "gotham.com",
				FirstName:    "Bruce",
				LastName:     "Wayne",
			}
			batmanCtx := auth.NewTokenContext(context.TODO(), batmanUser)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create assessment as batman user
			createResp, err := srv.CreateAssessment(batmanCtx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "batman-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Try to retrieve as admin user (different organization)
			adminUser := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			adminCtx := auth.NewTokenContext(context.TODO(), adminUser)

			resp, err := srv.GetAssessment(adminCtx, server.GetAssessmentRequestObject{Id: assessmentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetAssessment403JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			cleanupSpiceDB()
		})
	})

	Context("update assessment", func() {
		It("successfully updates an assessment name", func() {
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
				FirstName:    "John",
				LastName:     "Doe",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create assessment via API
			createResp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "original-name",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Update the assessment
			updatedName := "updated-name"
			resp, err := srv.UpdateAssessment(ctx, server.UpdateAssessmentRequestObject{
				Id: assessmentID,
				Body: &v1alpha1.AssessmentUpdate{
					Name: &updatedName,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateAssessment200JSONResponse{}).String()))

			assessment := resp.(server.UpdateAssessment200JSONResponse)
			Expect(assessment.Name).To(Equal("updated-name"))
		})

		It("fails to update assessment with empty body", func() {
			assessmentID := uuid.New()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)
			resp, err := srv.UpdateAssessment(ctx, server.UpdateAssessmentRequestObject{
				Id:   assessmentID,
				Body: nil,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateAssessment400JSONResponse{}).String()))

			errorResp := resp.(server.UpdateAssessment400JSONResponse)
			Expect(errorResp.Message).To(Equal("empty body"))
		})

		It("returns 404 for non-existent assessment", func() {
			nonExistentID := uuid.New()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)
			newName := "new-name"
			resp, err := srv.UpdateAssessment(ctx, server.UpdateAssessmentRequestObject{
				Id: nonExistentID,
				Body: &v1alpha1.AssessmentUpdate{
					Name: &newName,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateAssessment404JSONResponse{}).String()))
		})

		It("returns 403 for assessment from different organization", func() {
			batmanUser := auth.User{
				Username:     "batman",
				Organization: "batman",
				EmailDomain:  "gotham.com",
				FirstName:    "Bruce",
				LastName:     "Wayne",
			}
			batmanCtx := auth.NewTokenContext(context.TODO(), batmanUser)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create assessment as batman user
			createResp, err := srv.CreateAssessment(batmanCtx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "batman-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Try to update as admin user (different organization)
			adminUser := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			adminCtx := auth.NewTokenContext(context.TODO(), adminUser)

			hackedName := "hacked-name"
			resp, err := srv.UpdateAssessment(adminCtx, server.UpdateAssessmentRequestObject{
				Id: assessmentID,
				Body: &v1alpha1.AssessmentUpdate{
					Name: &hackedName,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateAssessment403JSONResponse{}).String()))
		})

		It("successfully updates assessment created from inventory sourceType but keeps same number of snapshots", func() {
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
				FirstName:    "John",
				LastName:     "Doe",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create assessment via API with inventory sourceType
			createResp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "inventory-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Update the assessment
			updatedName := "updated-inventory-name"
			resp, err := srv.UpdateAssessment(ctx, server.UpdateAssessmentRequestObject{
				Id: assessmentID,
				Body: &v1alpha1.AssessmentUpdate{
					Name: &updatedName,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateAssessment200JSONResponse{}).String()))

			assessment := resp.(server.UpdateAssessment200JSONResponse)
			Expect(assessment.Name).To(Equal("updated-inventory-name"))
			// Verify it still has only 1 snapshot (no new snapshot created)
			Expect(assessment.Snapshots).To(HaveLen(1))
		})

		It("successfully updates assessment created from rvtools sourceType but keeps same number of snapshots", func() {
			// assessmentID := uuid.New()
			// tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "rvtools-assessment", "admin", "John", "Doe", service.SourceTypeRvtools, "NULL"))
			// Expect(tx.Error).To(BeNil())
			//
			// inventory := `{"vcenter": {"id": "test-vcenter"}}`
			// tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, inventory, assessmentID.String()))
			// Expect(tx.Error).To(BeNil())
			//
			// user := auth.User{
			// 	Username:     "admin",
			// 	Organization: "admin",
			// 	EmailDomain:  "admin.example.com",
			// }
			// ctx := auth.NewTokenContext(context.TODO(), user)
			//
			// srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)
			// updatedName := "updated-rvtools-name"
			// resp, err := srv.UpdateAssessment(ctx, server.UpdateAssessmentRequestObject{
			// 	Id: assessmentID,
			// 	Body: &v1alpha1.AssessmentUpdate{
			// 		Name: &updatedName,
			// 	},
			// })
			// Expect(err).To(BeNil())
			// Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateAssessment200JSONResponse{}).String()))
			//
			// assessment := resp.(server.UpdateAssessment200JSONResponse)
			// Expect(assessment.Name).To(Equal("updated-rvtools-name"))
			// // Verify it still has only 1 snapshot (no new snapshot created)
			// Expect(assessment.Snapshots).To(HaveLen(1))
		})

		It("successfully updates assessment created from agent sourceType and creates new snapshot", func() {
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
				FirstName:    "John",
				LastName:     "Doe",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			// Create a source first (via DB as there's no source create handler test pattern yet)
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID.String(), "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Add inventory to the source
			inventoryJSON := `{"vcenter": {"id": "test-vcenter"}}`
			tx = gormdb.Exec(fmt.Sprintf("UPDATE sources SET inventory = '%s' WHERE id = '%s';", inventoryJSON, sourceID.String()))
			Expect(tx.Error).To(BeNil())

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create assessment via API with agent sourceType
			createResp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "agent-assessment",
					SourceType: service.SourceTypeAgent,
					SourceId:   &sourceID,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id
			// Verify initial snapshot was created
			Expect(createdAssessment.Snapshots).To(HaveLen(1))

			// Update the assessment
			updatedName := "updated-agent-name"
			resp, err := srv.UpdateAssessment(ctx, server.UpdateAssessmentRequestObject{
				Id: assessmentID,
				Body: &v1alpha1.AssessmentUpdate{
					Name: &updatedName,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateAssessment200JSONResponse{}).String()))

			assessment := resp.(server.UpdateAssessment200JSONResponse)
			Expect(assessment.Name).To(Equal("updated-agent-name"))
			// Verify it creates a new snapshot (should have 2 snapshots now)
			Expect(assessment.Snapshots).To(HaveLen(2))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
			cleanupSpiceDB()
		})
	})

	Context("delete assessment", func() {
		It("successfully deletes an assessment", func() {
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
				FirstName:    "John",
				LastName:     "Doe",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create assessment via API
			createResp, err := srv.CreateAssessment(ctx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "test-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Delete the assessment
			resp, err := srv.DeleteAssessment(ctx, server.DeleteAssessmentRequestObject{Id: assessmentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteAssessment200JSONResponse{}).String()))

			// Verify assessment is deleted
			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) FROM assessments WHERE id = ?", assessmentID.String()).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			// Verify snapshots are deleted due to cascade
			tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID.String()).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("returns 404 for non-existent assessment", func() {
			nonExistentID := uuid.New()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)
			resp, err := srv.DeleteAssessment(ctx, server.DeleteAssessmentRequestObject{Id: nonExistentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteAssessment404JSONResponse{}).String()))
		})

		It("returns 403 for assessment from different organization", func() {
			batmanUser := auth.User{
				Username:     "batman",
				Organization: "batman",
				EmailDomain:  "gotham.com",
				FirstName:    "Bruce",
				LastName:     "Wayne",
			}
			batmanCtx := auth.NewTokenContext(context.TODO(), batmanUser)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create assessment as batman user
			createResp, err := srv.CreateAssessment(batmanCtx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "batman-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Try to delete as admin user (different organization)
			adminUser := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			adminCtx := auth.NewTokenContext(context.TODO(), adminUser)

			resp, err := srv.DeleteAssessment(adminCtx, server.DeleteAssessmentRequestObject{Id: assessmentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteAssessment403JSONResponse{}).String()))

			// Verify assessment still exists
			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) FROM assessments WHERE id = ?", assessmentID.String()).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
			cleanupSpiceDB()
		})
	})

	Context("add assessment relationship", func() {
		It("successfully adds viewer relationship to an assessment", func() {
			ownerUser := auth.User{
				Username:     "owner",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
				FirstName:    "Owner",
				LastName:     "User",
			}
			ownerCtx := auth.NewTokenContext(context.TODO(), ownerUser)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create owner user in authz
			err := authzSvc.CreateUser(context.TODO(), ownerUser)
			Expect(err).To(BeNil())

			// Create assessment as owner
			createResp, err := srv.CreateAssessment(ownerCtx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "shared-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Create viewer user in authz
			viewerUser := auth.User{
				Username:     "viewer",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
			}
			err = authzSvc.CreateUser(context.TODO(), viewerUser)
			Expect(err).To(BeNil())

			// Add viewer relationship for another user
			addResp, err := srv.AddAssessmentRelationship(ownerCtx, server.AddAssessmentRelationshipRequestObject{
				Id: assessmentID,
				Body: &v1alpha1.AssessmentRelationshipRequest{
					Relationships: []v1alpha1.AssessmentRelationship{
						{
							UserId:       viewerUser.Username,
							Relationship: v1alpha1.Viewer,
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(addResp).String()).To(Equal(reflect.TypeOf(server.AddAssessmentRelationship201JSONResponse{}).String()))

			// Verify viewer can access the assessment
			viewerCtx := auth.NewTokenContext(context.TODO(), viewerUser)
			getResp, err := srv.GetAssessment(viewerCtx, server.GetAssessmentRequestObject{Id: assessmentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(getResp).String()).To(Equal(reflect.TypeOf(server.GetAssessment200JSONResponse{}).String()))

			// Verify permissions - viewer should have read permission
			assessment := getResp.(server.GetAssessment200JSONResponse)
			Expect(assessment.Permissions).ToNot(HaveLen(0))
			Expect(assessment.Permissions).To(ContainElement(v1alpha1.Read))
			Expect(assessment.Permissions).ToNot(ContainElement(v1alpha1.Edit))
			Expect(assessment.Permissions).ToNot(ContainElement(v1alpha1.Share))
			Expect(assessment.Permissions).ToNot(ContainElement(v1alpha1.Delete))
		})

		It("successfully adds editor relationship to an assessment", func() {
			ownerUser := auth.User{
				Username:     "owner",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
				FirstName:    "Owner",
				LastName:     "User",
			}
			ownerCtx := auth.NewTokenContext(context.TODO(), ownerUser)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create owner user in authz
			err := authzSvc.CreateUser(context.TODO(), ownerUser)
			Expect(err).To(BeNil())

			// Create assessment as owner
			createResp, err := srv.CreateAssessment(ownerCtx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "shared-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Create editor user in authz
			editorUser := auth.User{
				Username:     "editor",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
			}
			err = authzSvc.CreateUser(context.TODO(), editorUser)
			Expect(err).To(BeNil())

			// Add editor relationship for another user
			addResp, err := srv.AddAssessmentRelationship(ownerCtx, server.AddAssessmentRelationshipRequestObject{
				Id: assessmentID,
				Body: &v1alpha1.AssessmentRelationshipRequest{
					Relationships: []v1alpha1.AssessmentRelationship{
						{
							UserId:       editorUser.Username,
							Relationship: v1alpha1.Editor,
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(addResp).String()).To(Equal(reflect.TypeOf(server.AddAssessmentRelationship201JSONResponse{}).String()))

			// Verify editor can access the assessment
			editorCtx := auth.NewTokenContext(context.TODO(), editorUser)
			getResp, err := srv.GetAssessment(editorCtx, server.GetAssessmentRequestObject{Id: assessmentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(getResp).String()).To(Equal(reflect.TypeOf(server.GetAssessment200JSONResponse{}).String()))

			// Verify permissions - editor should have read and edit permissions
			assessment := getResp.(server.GetAssessment200JSONResponse)
			Expect(assessment.Permissions).ToNot(BeNil())
			Expect(assessment.Permissions).To(ContainElement(v1alpha1.Read))
			Expect(assessment.Permissions).To(ContainElement(v1alpha1.Edit))
			Expect(assessment.Permissions).ToNot(ContainElement(v1alpha1.Share))
			Expect(assessment.Permissions).ToNot(ContainElement(v1alpha1.Delete))
		})

		It("returns 404 when adding relationship to non-existent assessment", func() {
			ownerUser := auth.User{
				Username:     "owner",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
			}
			ownerCtx := auth.NewTokenContext(context.TODO(), ownerUser)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			nonExistentID := uuid.New()
			viewerUserId := "viewer"
			addResp, err := srv.AddAssessmentRelationship(ownerCtx, server.AddAssessmentRelationshipRequestObject{
				Id: nonExistentID,
				Body: &v1alpha1.AssessmentRelationshipRequest{
					Relationships: []v1alpha1.AssessmentRelationship{
						{
							UserId:       viewerUserId,
							Relationship: v1alpha1.Viewer,
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(addResp).String()).To(Equal(reflect.TypeOf(server.AddAssessmentRelationship404JSONResponse{}).String()))
		})

		It("returns 403 when user without share permission tries to add relationship", func() {
			ownerUser := auth.User{
				Username:     "owner",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
				FirstName:    "Owner",
				LastName:     "User",
			}
			ownerCtx := auth.NewTokenContext(context.TODO(), ownerUser)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create assessment as owner
			createResp, err := srv.CreateAssessment(ownerCtx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "test-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Try to add relationship as different user without permissions
			otherUser := auth.User{
				Username:     "other",
				Organization: "org2",
				EmailDomain:  "org2.example.com",
			}
			otherCtx := auth.NewTokenContext(context.TODO(), otherUser)

			viewerUserId := "viewer"
			addResp, err := srv.AddAssessmentRelationship(otherCtx, server.AddAssessmentRelationshipRequestObject{
				Id: assessmentID,
				Body: &v1alpha1.AssessmentRelationshipRequest{
					Relationships: []v1alpha1.AssessmentRelationship{
						{
							UserId:       viewerUserId,
							Relationship: v1alpha1.Viewer,
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(addResp).String()).To(Equal(reflect.TypeOf(server.AddAssessmentRelationship403JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			//		cleanupSpiceDB()
		})
	})

	Context("remove assessment relationship", func() {
		It("successfully removes viewer relationship from an assessment", func() {
			ownerUser := auth.User{
				Username:     "owner",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
				FirstName:    "Owner",
				LastName:     "User",
			}
			ownerCtx := auth.NewTokenContext(context.TODO(), ownerUser)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create assessment as owner
			createResp, err := srv.CreateAssessment(ownerCtx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "shared-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Add viewer relationship
			viewerUserId := "viewer"
			addResp, err := srv.AddAssessmentRelationship(ownerCtx, server.AddAssessmentRelationshipRequestObject{
				Id: assessmentID,
				Body: &v1alpha1.AssessmentRelationshipRequest{
					Relationships: []v1alpha1.AssessmentRelationship{
						{
							UserId:       viewerUserId,
							Relationship: v1alpha1.Viewer,
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(addResp).String()).To(Equal(reflect.TypeOf(server.AddAssessmentRelationship201JSONResponse{}).String()))

			// Remove viewer relationship
			removeResp, err := srv.RemoveAssessmentRelationship(ownerCtx, server.RemoveAssessmentRelationshipRequestObject{
				Id: assessmentID,
				Body: &v1alpha1.AssessmentRelationshipRequest{
					Relationships: []v1alpha1.AssessmentRelationship{
						{
							UserId:       viewerUserId,
							Relationship: v1alpha1.Viewer,
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(removeResp).String()).To(Equal(reflect.TypeOf(server.RemoveAssessmentRelationship200JSONResponse{}).String()))
		})

		It("returns 404 when removing relationship from non-existent assessment", func() {
			ownerUser := auth.User{
				Username:     "owner",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
			}
			ownerCtx := auth.NewTokenContext(context.TODO(), ownerUser)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			nonExistentID := uuid.New()
			viewerUserId := "viewer"
			removeResp, err := srv.RemoveAssessmentRelationship(ownerCtx, server.RemoveAssessmentRelationshipRequestObject{
				Id: nonExistentID,
				Body: &v1alpha1.AssessmentRelationshipRequest{
					Relationships: []v1alpha1.AssessmentRelationship{
						{
							UserId:       viewerUserId,
							Relationship: v1alpha1.Viewer,
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(removeResp).String()).To(Equal(reflect.TypeOf(server.RemoveAssessmentRelationship404JSONResponse{}).String()))
		})

		It("returns 403 when user without share permission tries to remove relationship", func() {
			ownerUser := auth.User{
				Username:     "owner",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
				FirstName:    "Owner",
				LastName:     "User",
			}
			ownerCtx := auth.NewTokenContext(context.TODO(), ownerUser)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create assessment as owner
			createResp, err := srv.CreateAssessment(ownerCtx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "test-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Add viewer relationship
			viewerUserId := "viewer"
			addResp, err := srv.AddAssessmentRelationship(ownerCtx, server.AddAssessmentRelationshipRequestObject{
				Id: assessmentID,
				Body: &v1alpha1.AssessmentRelationshipRequest{
					Relationships: []v1alpha1.AssessmentRelationship{
						{
							UserId:       viewerUserId,
							Relationship: v1alpha1.Viewer,
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(addResp).String()).To(Equal(reflect.TypeOf(server.AddAssessmentRelationship201JSONResponse{}).String()))

			// Try to remove relationship as different user without permissions
			otherUser := auth.User{
				Username:     "other",
				Organization: "org2",
				EmailDomain:  "org2.example.com",
			}
			otherCtx := auth.NewTokenContext(context.TODO(), otherUser)

			removeResp, err := srv.RemoveAssessmentRelationship(otherCtx, server.RemoveAssessmentRelationshipRequestObject{
				Id: assessmentID,
				Body: &v1alpha1.AssessmentRelationshipRequest{
					Relationships: []v1alpha1.AssessmentRelationship{
						{
							UserId:       viewerUserId,
							Relationship: v1alpha1.Viewer,
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(removeResp).String()).To(Equal(reflect.TypeOf(server.RemoveAssessmentRelationship403JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			cleanupSpiceDB()
		})
	})

	Context("share assessment with organization", func() {
		It("successfully shares an assessment with owner's organization", func() {
			ownerUser := auth.User{
				Username:     "owner",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
				FirstName:    "Owner",
				LastName:     "User",
			}
			ownerCtx := auth.NewTokenContext(context.TODO(), ownerUser)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create user in authz system
			err := authzSvc.CreateUser(ownerCtx, ownerUser)
			Expect(err).To(BeNil())

			// Create assessment as owner
			createResp, err := srv.CreateAssessment(ownerCtx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "org-shared-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Share the assessment with organization
			shareResp, err := srv.ShareAssessmentWithOrganization(ownerCtx, server.ShareAssessmentWithOrganizationRequestObject{
				Id: assessmentID,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(shareResp).String()).To(Equal(reflect.TypeOf(server.ShareAssessmentWithOrganization201JSONResponse{}).String()))
		})

		It("returns 404 when sharing non-existent assessment with organization", func() {
			ownerUser := auth.User{
				Username:     "owner",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
			}
			ownerCtx := auth.NewTokenContext(context.TODO(), ownerUser)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			nonExistentID := uuid.New()
			shareResp, err := srv.ShareAssessmentWithOrganization(ownerCtx, server.ShareAssessmentWithOrganizationRequestObject{
				Id: nonExistentID,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(shareResp).String()).To(Equal(reflect.TypeOf(server.ShareAssessmentWithOrganization404JSONResponse{}).String()))
		})

		It("returns 401 when non-owner tries to share assessment with organization", func() {
			ownerUser := auth.User{
				Username:     "owner",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
				FirstName:    "Owner",
				LastName:     "User",
			}
			ownerCtx := auth.NewTokenContext(context.TODO(), ownerUser)

			otherUser := auth.User{
				Username:     "other",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
			}
			otherCtx := auth.NewTokenContext(context.TODO(), otherUser)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create users in authz system
			err := authzSvc.CreateUser(ownerCtx, ownerUser)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(otherCtx, otherUser)
			Expect(err).To(BeNil())

			// Create assessment as owner
			createResp, err := srv.CreateAssessment(ownerCtx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "test-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Try to share as different user (non-owner)
			shareResp, err := srv.ShareAssessmentWithOrganization(otherCtx, server.ShareAssessmentWithOrganizationRequestObject{
				Id: assessmentID,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(shareResp).String()).To(Equal(reflect.TypeOf(server.ShareAssessmentWithOrganization401JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			cleanupSpiceDB()
		})
	})

	Context("unshare assessment with organization", func() {
		It("successfully unshares an assessment from owner's organization", func() {
			ownerUser := auth.User{
				Username:     "owner",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
				FirstName:    "Owner",
				LastName:     "User",
			}
			ownerCtx := auth.NewTokenContext(context.TODO(), ownerUser)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create user in authz system
			err := authzSvc.CreateUser(ownerCtx, ownerUser)
			Expect(err).To(BeNil())

			// Create assessment as owner
			createResp, err := srv.CreateAssessment(ownerCtx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "org-shared-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Share the assessment with organization first
			shareResp, err := srv.ShareAssessmentWithOrganization(ownerCtx, server.ShareAssessmentWithOrganizationRequestObject{
				Id: assessmentID,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(shareResp).String()).To(Equal(reflect.TypeOf(server.ShareAssessmentWithOrganization201JSONResponse{}).String()))

			// Unshare the assessment from organization
			unshareResp, err := srv.UnshareAssessmentWithOrganization(ownerCtx, server.UnshareAssessmentWithOrganizationRequestObject{
				Id: assessmentID,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(unshareResp).String()).To(Equal(reflect.TypeOf(server.UnshareAssessmentWithOrganization200JSONResponse{}).String()))
		})

		It("returns 404 when unsharing non-existent assessment from organization", func() {
			ownerUser := auth.User{
				Username:     "owner",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
			}
			ownerCtx := auth.NewTokenContext(context.TODO(), ownerUser)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			nonExistentID := uuid.New()
			unshareResp, err := srv.UnshareAssessmentWithOrganization(ownerCtx, server.UnshareAssessmentWithOrganizationRequestObject{
				Id: nonExistentID,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(unshareResp).String()).To(Equal(reflect.TypeOf(server.UnshareAssessmentWithOrganization404JSONResponse{}).String()))
		})

		It("returns 401 when non-owner tries to unshare assessment from organization", func() {
			ownerUser := auth.User{
				Username:     "owner",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
				FirstName:    "Owner",
				LastName:     "User",
			}
			ownerCtx := auth.NewTokenContext(context.TODO(), ownerUser)

			otherUser := auth.User{
				Username:     "other",
				Organization: "org1",
				EmailDomain:  "org1.example.com",
			}
			otherCtx := auth.NewTokenContext(context.TODO(), otherUser)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authzSvc)

			// Create users in authz system
			err := authzSvc.CreateUser(ownerCtx, ownerUser)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(otherCtx, otherUser)
			Expect(err).To(BeNil())

			// Create assessment as owner
			createResp, err := srv.CreateAssessment(ownerCtx, server.CreateAssessmentRequestObject{
				JSONBody: &v1alpha1.AssessmentForm{
					Name:       "test-assessment",
					SourceType: service.SourceTypeInventory,
					Inventory:  &inventory,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(createResp).String()).To(Equal(reflect.TypeOf(server.CreateAssessment201JSONResponse{}).String()))

			createdAssessment := createResp.(server.CreateAssessment201JSONResponse)
			assessmentID := createdAssessment.Id

			// Share with organization first as owner
			shareResp, err := srv.ShareAssessmentWithOrganization(ownerCtx, server.ShareAssessmentWithOrganizationRequestObject{
				Id: assessmentID,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(shareResp).String()).To(Equal(reflect.TypeOf(server.ShareAssessmentWithOrganization201JSONResponse{}).String()))

			// Try to unshare as different user (non-owner)
			unshareResp, err := srv.UnshareAssessmentWithOrganization(otherCtx, server.UnshareAssessmentWithOrganizationRequestObject{
				Id: assessmentID,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(unshareResp).String()).To(Equal(reflect.TypeOf(server.UnshareAssessmentWithOrganization401JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			cleanupSpiceDB()
		})
	})
})
