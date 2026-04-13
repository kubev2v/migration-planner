package v1alpha1_test

import (
	"context"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	openapi_types "github.com/oapi-codegen/runtime/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertAccountsHandlerGroupStm  = "INSERT INTO groups (id, name, description, kind, icon, company, parent_id) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', %s);"
	insertAccountsHandlerMemberStm = "INSERT INTO members (id, username, email, group_id) VALUES ('%s', '%s', '%s', '%s');"
)

var _ = Describe("accounts handler", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
		srv    *handlers.ServiceHandler
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
		accountsSvc := service.NewAccountsService(s)
		srv = handlers.NewServiceHandler(nil, nil, nil, nil, nil, service.NewPartnerService(s, accountsSvc), accountsSvc)
	})

	AfterAll(func() {
		_ = s.Close()
	})

	Context("GetIdentity", func() {
		It("returns identity from JWT when member not in DB", func() {
			authUser := auth.User{
				Username:     "jwtonly",
				Organization: "jwt-org",
			}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.GetIdentity(ctx, server.GetIdentityRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.GetIdentity200JSONResponse{})))

			body := resp.(server.GetIdentity200JSONResponse)
			Expect(body.Username).To(Equal("jwtonly"))
			Expect(string(body.Kind)).To(Equal("regular"))
			Expect(body.GroupId).To(BeNil())
			Expect(body.PartnerId).To(BeNil())
		})

		It("returns admin identity when member belongs to admin group", func() {
			orgID := uuid.New()
			userID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Admin Org", "desc", "admin", "icon", "Red Hat", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerMemberStm, userID, "adminuser", "admin@rh.com", orgID))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "adminuser", Organization: "jwt-org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.GetIdentity(ctx, server.GetIdentityRequestObject{})
			Expect(err).To(BeNil())

			body := resp.(server.GetIdentity200JSONResponse)
			Expect(body.Username).To(Equal("adminuser"))
			Expect(string(body.Kind)).To(Equal("admin"))
			Expect(*body.GroupId).To(Equal(orgID.String()))
			Expect(body.PartnerId).To(BeNil())
		})

		It("returns partner identity when member belongs to partner group", func() {
			orgID := uuid.New()
			userID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerMemberStm, userID, "partneruser", "partner@acme.com", orgID))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "partneruser", Organization: "jwt-org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.GetIdentity(ctx, server.GetIdentityRequestObject{})
			Expect(err).To(BeNil())

			body := resp.(server.GetIdentity200JSONResponse)
			Expect(body.Username).To(Equal("partneruser"))
			Expect(string(body.Kind)).To(Equal("partner"))
			Expect(*body.GroupId).To(Equal(orgID.String()))
			Expect(body.PartnerId).To(BeNil())
		})

		It("returns customer identity when user has accepted partner request", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, partnerGroupID, "Customer Partner", "desc", "partner", "icon", "CustPartnerCo", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO partners_customers (id, username, partner_id, request_status, name, contact_name, contact_phone, email, location) VALUES ('%s', 'customeruser', '%s', 'accepted', 'Name', 'Contact', '555-0001', 'c@example.com', 'Location')", uuid.New(), partnerGroupID))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "customeruser", Organization: "jwt-org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.GetIdentity(ctx, server.GetIdentityRequestObject{})
			Expect(err).To(BeNil())

			body := resp.(server.GetIdentity200JSONResponse)
			Expect(body.Username).To(Equal("customeruser"))
			Expect(string(body.Kind)).To(Equal("customer"))
			Expect(body.GroupId).To(BeNil())
			Expect(body.PartnerId).ToNot(BeNil())
			Expect(*body.PartnerId).To(Equal(partnerGroupID.String()))
		})

		It("returns regular when partner request is pending", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, partnerGroupID, "Pending Partner", "desc", "partner", "icon", "PendPartnerCo", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO partners_customers (id, username, partner_id, request_status, name, contact_name, contact_phone, email, location) VALUES ('%s', 'pendinguser', '%s', 'pending', 'Name', 'Contact', '555-0001', 'p@example.com', 'Location')", uuid.New(), partnerGroupID))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "pendinguser", Organization: "jwt-org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.GetIdentity(ctx, server.GetIdentityRequestObject{})
			Expect(err).To(BeNil())

			body := resp.(server.GetIdentity200JSONResponse)
			Expect(body.Username).To(Equal("pendinguser"))
			Expect(string(body.Kind)).To(Equal("regular"))
			Expect(body.GroupId).To(BeNil())
			Expect(body.PartnerId).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM members;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("Groups", func() {
		Context("ListGroups", func() {
			It("returns all groups", func() {
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, uuid.New(), "Org A", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, uuid.New(), "Org B", "desc", "partner", "icon", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.ListGroups(context.TODO(), server.ListGroupsRequestObject{})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ListGroups200JSONResponse{})))
				Expect(resp.(server.ListGroups200JSONResponse)).To(HaveLen(2))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("CreateGroup", func() {
			It("creates a group", func() {
				body := v1alpha1.GroupCreate{
					Name:        "New Org",
					Description: "desc",
					Kind:        v1alpha1.GroupCreateKindPartner,
					Icon:        "icon",
					Company:     "Acme",
				}
				resp, err := srv.CreateGroup(context.TODO(), server.CreateGroupRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateGroup201JSONResponse{})))

				created := resp.(server.CreateGroup201JSONResponse)
				Expect(created.Name).To(Equal("New Org"))
				Expect(created.Company).To(Equal("Acme"))
			})

			It("returns 400 for duplicate company+name", func() {
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, uuid.New(), "Sales", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				body := v1alpha1.GroupCreate{
					Name:        "Sales",
					Description: "desc",
					Kind:        v1alpha1.GroupCreateKindPartner,
					Icon:        "icon",
					Company:     "Acme",
				}
				resp, err := srv.CreateGroup(context.TODO(), server.CreateGroupRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateGroup400JSONResponse{})))
			})

			It("allows same name in different company", func() {
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, uuid.New(), "Sales", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				body := v1alpha1.GroupCreate{
					Name:        "Sales",
					Description: "desc",
					Kind:        v1alpha1.GroupCreateKindPartner,
					Icon:        "icon",
					Company:     "Globex",
				}
				resp, err := srv.CreateGroup(context.TODO(), server.CreateGroupRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateGroup201JSONResponse{})))
			})

			It("allows different names in same company", func() {
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, uuid.New(), "Sales", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				body := v1alpha1.GroupCreate{
					Name:        "Engineering",
					Description: "desc",
					Kind:        v1alpha1.GroupCreateKindPartner,
					Icon:        "icon",
					Company:     "Acme",
				}
				resp, err := srv.CreateGroup(context.TODO(), server.CreateGroupRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateGroup201JSONResponse{})))
			})

			It("returns 400 for nil body", func() {
				resp, err := srv.CreateGroup(context.TODO(), server.CreateGroupRequestObject{Body: nil})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateGroup400JSONResponse{})))
			})

			It("returns 400 for empty name", func() {
				body := v1alpha1.GroupCreate{
					Name: "", Description: "desc", Kind: v1alpha1.GroupCreateKindPartner, Icon: "icon", Company: "Acme",
				}
				resp, err := srv.CreateGroup(context.TODO(), server.CreateGroupRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateGroup400JSONResponse{})))
			})

			It("returns 400 for whitespace-only name", func() {
				body := v1alpha1.GroupCreate{
					Name: "   ", Description: "desc", Kind: v1alpha1.GroupCreateKindPartner, Icon: "icon", Company: "Acme",
				}
				resp, err := srv.CreateGroup(context.TODO(), server.CreateGroupRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateGroup400JSONResponse{})))
			})

			It("returns 400 for empty company", func() {
				body := v1alpha1.GroupCreate{
					Name: "Org", Description: "desc", Kind: v1alpha1.GroupCreateKindPartner, Icon: "icon", Company: "",
				}
				resp, err := srv.CreateGroup(context.TODO(), server.CreateGroupRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateGroup400JSONResponse{})))
			})

			It("returns 400 for whitespace-only company", func() {
				body := v1alpha1.GroupCreate{
					Name: "Org", Description: "desc", Kind: v1alpha1.GroupCreateKindPartner, Icon: "icon", Company: "   ",
				}
				resp, err := srv.CreateGroup(context.TODO(), server.CreateGroupRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateGroup400JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("GetGroup", func() {
			It("returns the group", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.GetGroup(context.TODO(), server.GetGroupRequestObject{Id: orgID})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.GetGroup200JSONResponse{})))

				body := resp.(server.GetGroup200JSONResponse)
				Expect(body.Name).To(Equal("Test Org"))
			})

			It("returns 404 for missing group", func() {
				resp, err := srv.GetGroup(context.TODO(), server.GetGroupRequestObject{Id: uuid.New()})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.GetGroup404JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("UpdateGroup", func() {
			It("updates the group", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Old Name", "old desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				newName := "New Name"
				body := v1alpha1.GroupUpdate{Name: &newName}
				resp, err := srv.UpdateGroup(context.TODO(), server.UpdateGroupRequestObject{Id: orgID, Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateGroup200JSONResponse{})))

				updated := resp.(server.UpdateGroup200JSONResponse)
				Expect(updated.Name).To(Equal("New Name"))
			})

			It("returns 404 for missing group", func() {
				newName := "New Name"
				body := v1alpha1.GroupUpdate{Name: &newName}
				resp, err := srv.UpdateGroup(context.TODO(), server.UpdateGroupRequestObject{Id: uuid.New(), Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateGroup404JSONResponse{})))
			})

			It("updates without company field preserves existing company", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				newDesc := "updated desc"
				body := v1alpha1.GroupUpdate{Description: &newDesc}
				resp, err := srv.UpdateGroup(context.TODO(), server.UpdateGroupRequestObject{Id: orgID, Body: &body})
				Expect(err).To(BeNil())

				updated := resp.(server.UpdateGroup200JSONResponse)
				Expect(updated.Company).To(Equal("Acme"))
			})

			It("returns 400 for empty company", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				empty := ""
				body := v1alpha1.GroupUpdate{Company: &empty}
				resp, err := srv.UpdateGroup(context.TODO(), server.UpdateGroupRequestObject{Id: orgID, Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateGroup400JSONResponse{})))
			})

			It("returns 400 for whitespace-only company", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				spaces := "   "
				body := v1alpha1.GroupUpdate{Company: &spaces}
				resp, err := srv.UpdateGroup(context.TODO(), server.UpdateGroupRequestObject{Id: orgID, Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateGroup400JSONResponse{})))
			})

			It("returns 400 for empty name", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				empty := ""
				body := v1alpha1.GroupUpdate{Name: &empty}
				resp, err := srv.UpdateGroup(context.TODO(), server.UpdateGroupRequestObject{Id: orgID, Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateGroup400JSONResponse{})))
			})

			It("returns 400 for nil body", func() {
				resp, err := srv.UpdateGroup(context.TODO(), server.UpdateGroupRequestObject{Id: uuid.New(), Body: nil})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateGroup400JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("DeleteGroup", func() {
			It("deletes the group", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "To Delete", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.DeleteGroup(context.TODO(), server.DeleteGroupRequestObject{Id: orgID})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.DeleteGroup200JSONResponse{})))

				deleted := resp.(server.DeleteGroup200JSONResponse)
				Expect(deleted.Name).To(Equal("To Delete"))
			})

			It("returns 404 for missing group", func() {
				resp, err := srv.DeleteGroup(context.TODO(), server.DeleteGroupRequestObject{Id: uuid.New()})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.DeleteGroup404JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM groups;")
			})
		})
	})

	Context("CreateGroupMember", func() {
		It("creates a member under the group", func() {
			orgID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())

			body := v1alpha1.MemberCreate{
				Username: "newuser",
				Email:    openapi_types.Email("new@acme.com"),
			}
			resp, err := srv.CreateGroupMember(context.TODO(), server.CreateGroupMemberRequestObject{Id: orgID, Body: &body})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateGroupMember201JSONResponse{})))

			created := resp.(server.CreateGroupMember201JSONResponse)
			Expect(created.Username).To(Equal("newuser"))
			Expect(created.GroupId).To(Equal(orgID))
		})

		It("returns 409 for duplicate username", func() {
			orgID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerMemberStm, uuid.New(), "dupuser", "dup@acme.com", orgID))
			Expect(tx.Error).To(BeNil())

			body := v1alpha1.MemberCreate{
				Username: "dupuser",
				Email:    openapi_types.Email("dup2@acme.com"),
			}
			resp, err := srv.CreateGroupMember(context.TODO(), server.CreateGroupMemberRequestObject{Id: orgID, Body: &body})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateGroupMember409JSONResponse{})))
		})

		It("returns 404 for missing group", func() {
			body := v1alpha1.MemberCreate{
				Username: "newuser",
				Email:    openapi_types.Email("new@acme.com"),
			}
			resp, err := srv.CreateGroupMember(context.TODO(), server.CreateGroupMemberRequestObject{Id: uuid.New(), Body: &body})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateGroupMember404JSONResponse{})))
		})

		It("returns 400 for nil body", func() {
			orgID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())

			resp, err := srv.CreateGroupMember(context.TODO(), server.CreateGroupMemberRequestObject{Id: orgID, Body: nil})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateGroupMember400JSONResponse{})))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM members;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("Membership", func() {
		Context("ListGroupMembers", func() {
			It("returns members belonging to the group", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerMemberStm, uuid.New(), "member1", "m1@acme.com", orgID))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerMemberStm, uuid.New(), "member2", "m2@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.ListGroupMembers(context.TODO(), server.ListGroupMembersRequestObject{Id: orgID})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ListGroupMembers200JSONResponse{})))
				Expect(resp.(server.ListGroupMembers200JSONResponse)).To(HaveLen(2))
			})

			It("returns 404 for missing group", func() {
				resp, err := srv.ListGroupMembers(context.TODO(), server.ListGroupMembersRequestObject{Id: uuid.New()})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ListGroupMembers404JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("UpdateGroupMember", func() {
			It("updates member email", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerMemberStm, uuid.New(), "theuser", "old@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				newEmail := openapi_types.Email("new@acme.com")
				resp, err := srv.UpdateGroupMember(context.TODO(), server.UpdateGroupMemberRequestObject{
					Id:       orgID,
					Username: "theuser",
					Body:     &v1alpha1.MemberUpdate{Email: &newEmail},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateGroupMember200JSONResponse{})))

				updated := resp.(server.UpdateGroupMember200JSONResponse)
				Expect(string(updated.Email)).To(Equal("new@acme.com"))
			})

			It("returns 400 for member in different group", func() {
				orgA := uuid.New()
				orgB := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgA, "Org A", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgB, "Org B", "desc", "partner", "icon", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerMemberStm, uuid.New(), "theuser", "u@acme.com", orgA))
				Expect(tx.Error).To(BeNil())

				newEmail := openapi_types.Email("new@acme.com")
				resp, err := srv.UpdateGroupMember(context.TODO(), server.UpdateGroupMemberRequestObject{
					Id:       orgB,
					Username: "theuser",
					Body:     &v1alpha1.MemberUpdate{Email: &newEmail},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateGroupMember400JSONResponse{})))
			})

			It("returns 404 for non-existent member", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				newEmail := openapi_types.Email("new@acme.com")
				resp, err := srv.UpdateGroupMember(context.TODO(), server.UpdateGroupMemberRequestObject{
					Id:       orgID,
					Username: "ghost",
					Body:     &v1alpha1.MemberUpdate{Email: &newEmail},
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateGroupMember404JSONResponse{})))
			})

			It("returns 400 for nil body", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerMemberStm, uuid.New(), "theuser", "u@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.UpdateGroupMember(context.TODO(), server.UpdateGroupMemberRequestObject{
					Id:       orgID,
					Username: "theuser",
					Body:     nil,
				})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateGroupMember400JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("RemoveGroupMember", func() {
			It("deletes member from the group", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerMemberStm, uuid.New(), "removeme", "rm@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.RemoveGroupMember(context.TODO(), server.RemoveGroupMemberRequestObject{Id: orgID, Username: "removeme"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.RemoveGroupMember200Response{})))

				// Verify member is gone
				listResp, err := srv.ListGroupMembers(context.TODO(), server.ListGroupMembersRequestObject{Id: orgID})
				Expect(err).To(BeNil())
				Expect(listResp.(server.ListGroupMembers200JSONResponse)).To(HaveLen(0))
			})

			It("returns 400 when member belongs to different group", func() {
				orgA := uuid.New()
				orgB := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgA, "Org A", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgB, "Org B", "desc", "partner", "icon", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerMemberStm, uuid.New(), "wrongorg", "w@acme.com", orgB))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.RemoveGroupMember(context.TODO(), server.RemoveGroupMemberRequestObject{Id: orgA, Username: "wrongorg"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.RemoveGroupMember400JSONResponse{})))
			})

			It("returns 404 for missing group", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerMemberStm, uuid.New(), "someuser", "u@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.RemoveGroupMember(context.TODO(), server.RemoveGroupMemberRequestObject{Id: uuid.New(), Username: "someuser"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.RemoveGroupMember404JSONResponse{})))
			})

			It("returns 404 for missing member", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerGroupStm, orgID, "Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.RemoveGroupMember(context.TODO(), server.RemoveGroupMemberRequestObject{Id: orgID, Username: "ghost"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.RemoveGroupMember404JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})
	})

	Context("Authz 403 responses", func() {
		var authzSrv *handlers.ServiceHandler

		BeforeAll(func() {
			accountsSvc := service.NewAccountsService(s)
			authzAccountsSvc := service.NewAuthzAccountsService(accountsSvc)
			authzSrv = handlers.NewServiceHandler(nil, nil, nil, nil, nil, service.NewPartnerService(s, accountsSvc), authzAccountsSvc)
		})

		nonAdminCtx := func() context.Context {
			return auth.NewTokenContext(context.TODO(), auth.User{Username: "regularuser", Organization: "org"})
		}

		It("ListGroups returns 403 for non-admin", func() {
			resp, err := authzSrv.ListGroups(nonAdminCtx(), server.ListGroupsRequestObject{})
			Expect(err).To(BeNil())
			Expect(resp).To(BeAssignableToTypeOf(server.ListGroups403JSONResponse{}))
		})

		It("CreateGroup returns 403 for non-admin", func() {
			body := v1alpha1.GroupCreate{Name: "X", Kind: v1alpha1.GroupCreateKindPartner, Icon: "i", Company: "C"}
			resp, err := authzSrv.CreateGroup(nonAdminCtx(), server.CreateGroupRequestObject{Body: &body})
			Expect(err).To(BeNil())
			Expect(resp).To(BeAssignableToTypeOf(server.CreateGroup403JSONResponse{}))
		})

		It("GetGroup returns 403 for non-admin", func() {
			resp, err := authzSrv.GetGroup(nonAdminCtx(), server.GetGroupRequestObject{Id: uuid.New()})
			Expect(err).To(BeNil())
			Expect(resp).To(BeAssignableToTypeOf(server.GetGroup403JSONResponse{}))
		})

		It("UpdateGroup returns 403 for non-admin", func() {
			name := "X"
			body := v1alpha1.GroupUpdate{Name: &name}
			resp, err := authzSrv.UpdateGroup(nonAdminCtx(), server.UpdateGroupRequestObject{Id: uuid.New(), Body: &body})
			Expect(err).To(BeNil())
			Expect(resp).To(BeAssignableToTypeOf(server.UpdateGroup403JSONResponse{}))
		})

		It("DeleteGroup returns 403 for non-admin", func() {
			resp, err := authzSrv.DeleteGroup(nonAdminCtx(), server.DeleteGroupRequestObject{Id: uuid.New()})
			Expect(err).To(BeNil())
			Expect(resp).To(BeAssignableToTypeOf(server.DeleteGroup403JSONResponse{}))
		})

		It("ListGroupMembers returns 403 for non-admin", func() {
			resp, err := authzSrv.ListGroupMembers(nonAdminCtx(), server.ListGroupMembersRequestObject{Id: uuid.New()})
			Expect(err).To(BeNil())
			Expect(resp).To(BeAssignableToTypeOf(server.ListGroupMembers403JSONResponse{}))
		})

		It("CreateGroupMember returns 403 for non-admin", func() {
			body := v1alpha1.MemberCreate{Username: "u", Email: "u@x.com"}
			resp, err := authzSrv.CreateGroupMember(nonAdminCtx(), server.CreateGroupMemberRequestObject{Id: uuid.New(), Body: &body})
			Expect(err).To(BeNil())
			Expect(resp).To(BeAssignableToTypeOf(server.CreateGroupMember403JSONResponse{}))
		})

		It("UpdateGroupMember returns 403 for non-admin", func() {
			email := openapi_types.Email("u@x.com")
			body := v1alpha1.MemberUpdate{Email: &email}
			resp, err := authzSrv.UpdateGroupMember(nonAdminCtx(), server.UpdateGroupMemberRequestObject{Id: uuid.New(), Username: "u", Body: &body})
			Expect(err).To(BeNil())
			Expect(resp).To(BeAssignableToTypeOf(server.UpdateGroupMember403JSONResponse{}))
		})

		It("RemoveGroupMember returns 403 for non-admin", func() {
			resp, err := authzSrv.RemoveGroupMember(nonAdminCtx(), server.RemoveGroupMemberRequestObject{Id: uuid.New(), Username: "u"})
			Expect(err).To(BeNil())
			Expect(resp).To(BeAssignableToTypeOf(server.RemoveGroupMember403JSONResponse{}))
		})
	})
})
