package v1alpha1_test

import (
	"context"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
)

const (
	insertPartnerHandlerGroupStm    = "INSERT INTO groups (id, name, description, kind, icon, company, parent_id) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', %s);"
	insertPartnerHandlerMemberStm   = "INSERT INTO members (id, username, email, group_id) VALUES ('%s', '%s', '%s', '%s');"
	insertPartnerHandlerCustomerStm = "INSERT INTO partners_customers (id, username, partner_id, request_status, name, contact_name, contact_phone, email, location) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s');"
)

var _ = Describe("partner handler", Ordered, func() {
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
		partnerSvc := service.NewAuthzPartnerService(service.NewPartnerService(s, accountsSvc), accountsSvc, s)
		srv = handlers.NewServiceHandler(nil, nil, nil, nil, nil, partnerSvc, accountsSvc)
	})

	AfterAll(func() {
		_ = s.Close()
	})

	Context("ListPartners", func() {
		It("returns only partner groups", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, uuid.New(), "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, uuid.New(), "Admin Org", "desc", "admin", "icon", "Red Hat", "NULL"))
			Expect(tx.Error).To(BeNil())

			resp, err := srv.ListPartners(context.TODO(), server.ListPartnersRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ListPartners200JSONResponse{})))

			body := resp.(server.ListPartners200JSONResponse)
			Expect(body).To(HaveLen(1))
			Expect(body[0].Name).To(Equal("Partner Org"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM members;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("CreatePartnerRequest", func() {
		It("creates a request for a regular user", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "regularuser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			body := api.PartnerRequestCreate{
				Name:         "My Company",
				ContactName:  "John",
				ContactPhone: "555-1234",
				Email:        "john@example.com",
				Location:     "NYC",
			}
			resp, err := srv.CreatePartnerRequest(ctx, server.CreatePartnerRequestRequestObject{
				Id:   partnerGroupID,
				Body: &body,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreatePartnerRequest201JSONResponse{})))

			created := resp.(server.CreatePartnerRequest201JSONResponse)
			Expect(created.Username).To(Equal("regularuser"))
			Expect(created.Partner.Id).To(Equal(partnerGroupID))
			Expect(created.Partner.Name).To(Equal("Partner Org"))
			Expect(created.Partner.Company).To(Equal("Acme"))
			Expect(created.Partner.Icon).To(Equal("icon"))
			Expect(created.CreatedAt).ToNot(BeZero())
			Expect(created.RequestStatus).To(Equal(api.PartnerRequestStatusPending))
		})

		It("returns 403 when user is not regular (partner member)", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "partneruser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			body := api.PartnerRequestCreate{
				Name: "Co", ContactName: "J", ContactPhone: "555", Email: "j@e.com", Location: "NY",
			}
			resp, err := srv.CreatePartnerRequest(ctx, server.CreatePartnerRequestRequestObject{
				Id:   partnerGroupID,
				Body: &body,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreatePartnerRequest403JSONResponse{})))
		})

		It("returns 400 when user already has an active request", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, uuid.New(), "regularuser", partnerGroupID, "pending", "Co", "J", "555", "j@e.com", "NY"))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "regularuser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			body := api.PartnerRequestCreate{
				Name: "Co2", ContactName: "J2", ContactPhone: "555", Email: "j2@e.com", Location: "LA",
			}
			resp, err := srv.CreatePartnerRequest(ctx, server.CreatePartnerRequestRequestObject{
				Id:   uuid.New(),
				Body: &body,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreatePartnerRequest400JSONResponse{})))
		})

		It("returns 400 for nil body", func() {
			authUser := auth.User{Username: "regularuser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.CreatePartnerRequest(ctx, server.CreatePartnerRequestRequestObject{
				Id:   uuid.New(),
				Body: nil,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreatePartnerRequest400JSONResponse{})))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM members;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("ListPartnerRequests", func() {
		It("returns all requests for the authenticated user", func() {
			partner1ID := uuid.New()
			partner2ID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partner1ID, "Partner1", "desc", "partner", "icon1", "Acme1", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partner2ID, "Partner2", "desc", "partner", "icon2", "Acme2", "NULL"))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, uuid.New(), "user1", partner1ID, "pending", "Co", "J", "555", "j@e.com", "NY"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, uuid.New(), "user1", partner2ID, "rejected", "Co", "J", "555", "j@e.com", "NY"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, uuid.New(), "otheruser", partner1ID, "pending", "Co", "J", "555", "j@e.com", "NY"))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "user1", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.ListPartnerRequests(ctx, server.ListPartnerRequestsRequestObject{})
			Expect(err).To(BeNil())

			body := resp.(server.ListPartnerRequests200JSONResponse)
			Expect(body).To(HaveLen(2))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("CancelPartnerRequest", func() {
		It("cancels a pending request", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())

			requestID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, requestID, "user1", partnerGroupID, "pending", "Co", "J", "555", "j@e.com", "NY"))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "user1", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.CancelPartnerRequest(ctx, server.CancelPartnerRequestRequestObject{Id: requestID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CancelPartnerRequest200Response{})))

			var status string
			tx = gormdb.Raw("SELECT request_status FROM partners_customers WHERE id = ?;", requestID).Scan(&status)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("cancelled"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("GetPartner", func() {
		It("returns partner group for a customer", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, uuid.New(), "customer1", partnerGroupID, "accepted", "Co", "J", "555", "j@e.com", "NY"))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "customer1", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.GetPartner(ctx, server.GetPartnerRequestObject{Id: partnerGroupID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.GetPartner200JSONResponse{})))

			body := resp.(server.GetPartner200JSONResponse)
			Expect(body.Name).To(Equal("Partner Org"))
		})

		It("returns 403 when user is not a customer", func() {
			authUser := auth.User{Username: "nobody", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.GetPartner(ctx, server.GetPartnerRequestObject{Id: uuid.New()})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.GetPartner403JSONResponse{})))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM members;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("LeavePartner", func() {
		It("removes partner relationship", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, uuid.New(), "customer1", partnerGroupID, "accepted", "Co", "J", "555", "j@e.com", "NY"))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "customer1", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.LeavePartner(ctx, server.LeavePartnerRequestObject{Id: partnerGroupID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.LeavePartner200Response{})))

			var status string
			tx = gormdb.Raw("SELECT request_status FROM partners_customers WHERE username = 'customer1';").Scan(&status)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("cancelled"))
		})

		It("returns 403 when user is not a customer", func() {
			authUser := auth.User{Username: "regularuser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.LeavePartner(ctx, server.LeavePartnerRequestObject{Id: uuid.New()})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.LeavePartner403JSONResponse{})))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM members;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("ListCustomers", func() {
		It("returns customers for a partner user", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, uuid.New(), "cust1", partnerGroupID, "accepted", "Co1", "J1", "555", "c1@e.com", "NY"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, uuid.New(), "cust2", partnerGroupID, "pending", "Co2", "J2", "555", "c2@e.com", "LA"))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "partneruser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.ListCustomers(ctx, server.ListCustomersRequestObject{})
			Expect(err).To(BeNil())

			body := resp.(server.ListCustomers200JSONResponse)
			Expect(body).To(HaveLen(2))
		})

		It("returns 403 when user is not a partner", func() {
			authUser := auth.User{Username: "regularuser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.ListCustomers(ctx, server.ListCustomersRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ListCustomers403JSONResponse{})))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM members;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("UpdatePartnerRequest", func() {
		It("accepts a customer request", func() {
			partnerGroupID := uuid.New()
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, requestID, "cust1", partnerGroupID, "pending", "Co", "J", "555", "c@e.com", "NY"))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "partneruser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			body := api.PartnerRequestUpdate{Status: api.PartnerRequestStatusAccepted}
			resp, err := srv.UpdatePartnerRequest(ctx, server.UpdatePartnerRequestRequestObject{
				Id:   requestID,
				Body: &body,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdatePartnerRequest200JSONResponse{})))

			updated := resp.(server.UpdatePartnerRequest200JSONResponse)
			Expect(updated.RequestStatus).To(Equal(api.PartnerRequestStatusAccepted))
		})

		It("rejects a request with reason", func() {
			partnerGroupID := uuid.New()
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, requestID, "cust1", partnerGroupID, "pending", "Co", "J", "555", "c@e.com", "NY"))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "partneruser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			reason := "Not a fit"
			body := api.PartnerRequestUpdate{Status: api.PartnerRequestStatusRejected, Reason: &reason}
			resp, err := srv.UpdatePartnerRequest(ctx, server.UpdatePartnerRequestRequestObject{
				Id:   requestID,
				Body: &body,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdatePartnerRequest200JSONResponse{})))

			updated := resp.(server.UpdatePartnerRequest200JSONResponse)
			Expect(updated.RequestStatus).To(Equal(api.PartnerRequestStatusRejected))
			Expect(*updated.Reason).To(Equal("Not a fit"))
		})

		It("returns 400 when rejecting without reason", func() {
			partnerGroupID := uuid.New()
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, requestID, "cust1", partnerGroupID, "pending", "Co", "J", "555", "c@e.com", "NY"))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "partneruser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			body := api.PartnerRequestUpdate{Status: api.PartnerRequestStatusRejected}
			resp, err := srv.UpdatePartnerRequest(ctx, server.UpdatePartnerRequestRequestObject{
				Id:   requestID,
				Body: &body,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdatePartnerRequest400JSONResponse{})))
		})

		It("returns 400 for nil body", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "partneruser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.UpdatePartnerRequest(ctx, server.UpdatePartnerRequestRequestObject{
				Id:   uuid.New(),
				Body: nil,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdatePartnerRequest400JSONResponse{})))
		})

		It("returns 403 when non-partner user updates request", func() {
			someGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, someGroupID, "Some Org", "desc", "partner", "icon", "SomeCo", "NULL"))
			Expect(tx.Error).To(BeNil())

			requestID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, requestID, "cust1", someGroupID, "pending", "Co", "J", "555", "c@e.com", "NY"))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "regularuser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			body := api.PartnerRequestUpdate{Status: api.PartnerRequestStatusAccepted}
			resp, err := srv.UpdatePartnerRequest(ctx, server.UpdatePartnerRequestRequestObject{
				Id:   requestID,
				Body: &body,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdatePartnerRequest403JSONResponse{})))
		})

		It("returns 403 when partner updates request from another group", func() {
			partnerGroupID := uuid.New()
			otherGroupID := uuid.New()
			requestID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, otherGroupID, "Other Org", "desc", "partner", "icon", "Other", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, requestID, "user1", otherGroupID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "partneruser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			body := api.PartnerRequestUpdate{Status: api.PartnerRequestStatusAccepted}
			resp, err := srv.UpdatePartnerRequest(ctx, server.UpdatePartnerRequestRequestObject{
				Id:   requestID,
				Body: &body,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdatePartnerRequest403JSONResponse{})))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM members;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("RemoveCustomer", func() {
		It("removes a customer from partner org", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, uuid.New(), "cust1", partnerGroupID, "accepted", "Co", "J", "555", "c@e.com", "NY"))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "partneruser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.RemoveCustomer(ctx, server.RemoveCustomerRequestObject{Username: "cust1"})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.RemoveCustomer200Response{})))

			var status string
			tx = gormdb.Raw("SELECT request_status FROM partners_customers WHERE username = 'cust1';").Scan(&status)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("cancelled"))
		})

		It("returns 403 when non-partner user removes customer", func() {
			authUser := auth.User{Username: "regularuser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.RemoveCustomer(ctx, server.RemoveCustomerRequestObject{Username: "cust1"})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.RemoveCustomer403JSONResponse{})))
		})

		It("returns 403 when partner removes customer from another group", func() {
			partnerGroupID := uuid.New()
			otherGroupID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerGroupStm, otherGroupID, "Other Org", "desc", "partner", "icon", "Other", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerHandlerCustomerStm, uuid.New(), "cust1", otherGroupID, "accepted", "Co", "J", "555", "c@e.com", "NY"))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "partneruser", Organization: "org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.RemoveCustomer(ctx, server.RemoveCustomerRequestObject{Username: "cust1"})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.RemoveCustomer403JSONResponse{})))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM members;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})
})
