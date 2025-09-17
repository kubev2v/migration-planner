package v1alpha1_test

import (
	"context"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertAssessmentStm = "INSERT INTO assessments (id, created_at, name, org_id, source_type, source_id) VALUES ('%s', now(), '%s', '%s', '%s', %s);"
	insertSnapshotStm   = "INSERT INTO snapshots (created_at, inventory, assessment_id) VALUES (now(), '%s', '%s');"
)

var _ = Describe("assessment handler", Ordered, func() {
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

	Context("list assessments", func() {
		It("successfully lists all assessments for the user's organization", func() {
			assessmentID1 := uuid.New()
			assessmentID2 := uuid.New()
			assessmentID3 := uuid.New()

			// Create assessments for different organizations
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID1.String(), "assessment1", "admin", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID2.String(), "assessment2", "admin", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID3.String(), "assessment3", "batman", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())

			// Create snapshots for assessments
			inventory := `{"vcenter": {"id": "test-vcenter"}}`
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, inventory, assessmentID1.String()))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, inventory, assessmentID2.String()))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, inventory, assessmentID3.String()))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
			resp, err := srv.ListAssessments(ctx, server.ListAssessmentsRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.ListAssessments200JSONResponse{}).String()))

			assessmentList := resp.(server.ListAssessments200JSONResponse)
			Expect(assessmentList).To(HaveLen(2)) // Only admin assessments
		})

		It("returns empty list when no assessments exist for the organization", func() {
			user := auth.User{
				Username:     "empty",
				Organization: "empty",
				EmailDomain:  "empty.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
			resp, err := srv.ListAssessments(ctx, server.ListAssessmentsRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.ListAssessments200JSONResponse{}).String()))

			assessmentList := resp.(server.ListAssessments200JSONResponse)
			Expect(assessmentList).To(HaveLen(0))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
		})
	})

	Context("create assessment", func() {
		It("successfully creates an assessment with JSON body - inventory type", func() {
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			inventory := v1alpha1.Inventory{
				Vcenter: v1alpha1.VCenter{
					Id: "test-vcenter",
				},
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
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
			srv = handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
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
		})
	})

	Context("get assessment", func() {
		It("successfully retrieves an assessment", func() {
			assessmentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "test-assessment", "admin", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())

			inventory := `{"vcenter": {"id": "test-vcenter"}}`
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, inventory, assessmentID.String()))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
			resp, err := srv.GetAssessment(ctx, server.GetAssessmentRequestObject{Id: nonExistentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetAssessment404JSONResponse{}).String()))
		})

		It("returns 403 for assessment from different organization", func() {
			assessmentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "batman-assessment", "batman", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())

			inventory := `{"vcenter": {"id": "test-vcenter"}}`
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, inventory, assessmentID.String()))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
			resp, err := srv.GetAssessment(ctx, server.GetAssessmentRequestObject{Id: assessmentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetAssessment403JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
		})
	})

	Context("update assessment", func() {
		It("successfully updates an assessment name", func() {
			assessmentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "original-name", "admin", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())

			inventory := `{"vcenter": {"id": "test-vcenter"}}`
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, inventory, assessmentID.String()))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
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
			assessmentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "batman-assessment", "batman", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())

			inventory := `{"vcenter": {"id": "test-vcenter"}}`
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, inventory, assessmentID.String()))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
			hackedName := "hacked-name"
			resp, err := srv.UpdateAssessment(ctx, server.UpdateAssessmentRequestObject{
				Id: assessmentID,
				Body: &v1alpha1.AssessmentUpdate{
					Name: &hackedName,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateAssessment403JSONResponse{}).String()))
		})

		It("successfully updates assessment created from inventory sourceType but keeps same number of snapshots", func() {
			assessmentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "inventory-assessment", "admin", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())

			inventory := `{"vcenter": {"id": "test-vcenter"}}`
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, inventory, assessmentID.String()))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
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
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("successfully updates assessment created from rvtools sourceType but keeps same number of snapshots", func() {
			assessmentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "rvtools-assessment", "admin", service.SourceTypeRvtools, "NULL"))
			Expect(tx.Error).To(BeNil())

			inventory := `{"vcenter": {"id": "test-vcenter"}}`
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, inventory, assessmentID.String()))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
			updatedName := "updated-rvtools-name"
			resp, err := srv.UpdateAssessment(ctx, server.UpdateAssessmentRequestObject{
				Id: assessmentID,
				Body: &v1alpha1.AssessmentUpdate{
					Name: &updatedName,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateAssessment200JSONResponse{}).String()))

			assessment := resp.(server.UpdateAssessment200JSONResponse)
			Expect(assessment.Name).To(Equal("updated-rvtools-name"))
			// Verify it still has only 1 snapshot (no new snapshot created)
			Expect(assessment.Snapshots).To(HaveLen(1))
		})

		It("successfully updates assessment created from agent sourceType and creates new snapshot", func() {
			// Create a source first
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID.String(), "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Add inventory to the source
			inventory := `{"vcenter": {"id": "test-vcenter"}}`
			tx = gormdb.Exec(fmt.Sprintf("UPDATE sources SET inventory = '%s' WHERE id = '%s';", inventory, sourceID.String()))
			Expect(tx.Error).To(BeNil())

			// Create assessment with agent sourceType
			assessmentID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO assessments (id, created_at, name, org_id, source_type, source_id) VALUES ('%s', now(), '%s', '%s', '%s', '%s');",
				assessmentID.String(), "agent-assessment", "admin", service.SourceTypeAgent, sourceID.String()))
			Expect(tx.Error).To(BeNil())

			// Create initial snapshot
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, inventory, assessmentID.String()))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
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
		})
	})

	Context("delete assessment", func() {
		It("successfully deletes an assessment", func() {
			assessmentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "test-assessment", "admin", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())

			inventory := `{"vcenter": {"id": "test-vcenter"}}`
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, inventory, assessmentID.String()))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
			resp, err := srv.DeleteAssessment(ctx, server.DeleteAssessmentRequestObject{Id: assessmentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteAssessment200JSONResponse{}).String()))

			// Verify assessment is deleted
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM assessments WHERE id = ?", assessmentID.String()).Scan(&count)
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
			resp, err := srv.DeleteAssessment(ctx, server.DeleteAssessmentRequestObject{Id: nonExistentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteAssessment404JSONResponse{}).String()))
		})

		It("returns 403 for assessment from different organization", func() {
			assessmentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "batman-assessment", "batman", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())

			inventory := `{"vcenter": {"id": "test-vcenter"}}`
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, inventory, assessmentID.String()))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil))
			resp, err := srv.DeleteAssessment(ctx, server.DeleteAssessmentRequestObject{Id: assessmentID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteAssessment403JSONResponse{}).String()))

			// Verify assessment still exists
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM assessments WHERE id = ?", assessmentID.String()).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})
})
