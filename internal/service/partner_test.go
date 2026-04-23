package service_test

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertPartnerCustomerStm = "INSERT INTO partners_customers (id, username, partner_id, request_status, name, contact_name, contact_phone, email, location) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s');"
	insertPartnerGroupStm    = "INSERT INTO groups (id, name, description, kind, icon, company, parent_id) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', %s);"
	insertPartnerMemberStm   = "INSERT INTO members (id, username, email, group_id) VALUES ('%s', '%s', '%s', '%s');"
)

var _ = Describe("partner service", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
		srv    service.PartnerServicer
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
		srv = service.NewPartnerService(s, service.NewAccountsService(s))
	})

	AfterAll(func() {
		_ = s.Close()
	})

	Context("ListRequests", func() {
		var partnerGroupID1, partnerGroupID2 uuid.UUID

		BeforeEach(func() {
			partnerGroupID1 = uuid.New()
			partnerGroupID2 = uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerGroupID1, "Partner1", "desc", "partner", "icon", "Acme1", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerGroupID2, "Partner2", "desc", "partner", "icon", "Acme2", "NULL"))
			Expect(tx.Error).To(BeNil())
		})

		It("returns all requests for a user", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, uuid.New(), "user1", partnerGroupID1, "rejected", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, uuid.New(), "user1", partnerGroupID2, "accepted", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			partners, err := srv.ListRequests(context.TODO(), auth.User{Username: "user1"})
			Expect(err).To(BeNil())
			Expect(partners).To(HaveLen(2))
		})

		It("returns empty list when no requests", func() {
			partners, err := srv.ListRequests(context.TODO(), auth.User{Username: "nonexistent"})
			Expect(err).To(BeNil())
			Expect(partners).To(HaveLen(0))
		})

		It("returns all requests for the partner's group when called by a partner", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID1))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, uuid.New(), "user1", partnerGroupID1, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, uuid.New(), "user2", partnerGroupID1, "accepted", "Name2", "Contact2", "555-0002", "user2@example.com", "Location2"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, uuid.New(), "user3", partnerGroupID2, "pending", "Name3", "Contact3", "555-0003", "user3@example.com", "Location3"))
			Expect(tx.Error).To(BeNil())

			partners, err := srv.ListRequests(context.TODO(), auth.User{Username: "partneruser"})
			Expect(err).To(BeNil())
			Expect(partners).To(HaveLen(2))
			for _, p := range partners {
				Expect(p.PartnerID).To(Equal(partnerGroupID1.String()))
			}
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM members;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("CreateRequest", func() {
		It("successfully creates a pending request", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())

			pc := model.PartnerCustomer{
				Name:         "Name1",
				ContactName:  "Contact1",
				ContactPhone: "555-0001",
				Email:        "user1@example.com",
				Location:     "Location1",
			}

			created, err := srv.CreateRequest(context.TODO(), auth.User{Username: "user1"}, partnerGroupID.String(), pc)
			Expect(err).To(BeNil())
			Expect(created).ToNot(BeNil())
			Expect(created.ID).ToNot(Equal(uuid.Nil))
			Expect(created.RequestStatus).To(Equal(model.RequestStatusPending))
		})

		It("fails when partner group does not exist", func() {
			pc := model.PartnerCustomer{
				Name:         "Name1",
				ContactName:  "Contact1",
				ContactPhone: "555-0001",
				Email:        "user1@example.com",
				Location:     "Location1",
			}

			_, err := srv.CreateRequest(context.TODO(), auth.User{Username: "user1"}, uuid.New().String(), pc)
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		It("fails when partner ID is not a valid UUID", func() {
			pc := model.PartnerCustomer{
				Name:         "Name1",
				ContactName:  "Contact1",
				ContactPhone: "555-0001",
				Email:        "user1@example.com",
				Location:     "Location1",
			}

			_, err := srv.CreateRequest(context.TODO(), auth.User{Username: "user1"}, "not-a-uuid", pc)
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		It("fails when group exists but is not a partner", func() {
			adminGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, adminGroupID, "Admin Org", "desc", "admin", "icon", "Corp", "NULL"))
			Expect(tx.Error).To(BeNil())

			pc := model.PartnerCustomer{
				Name:         "Name1",
				ContactName:  "Contact1",
				ContactPhone: "555-0001",
				Email:        "user1@example.com",
				Location:     "Location1",
			}

			_, err := srv.CreateRequest(context.TODO(), auth.User{Username: "user1"}, adminGroupID.String(), pc)
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		It("fails when user already has an accepted request", func() {
			existingGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, existingGroupID, "Existing Partner", "desc", "partner", "icon", "Corp", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, uuid.New(), "user1", existingGroupID, "accepted", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			pc := model.PartnerCustomer{
				Name:         "Name1",
				ContactName:  "Contact1",
				ContactPhone: "555-0001",
				Email:        "user1@example.com",
				Location:     "Location1",
			}

			_, err := srv.CreateRequest(context.TODO(), auth.User{Username: "user1"}, uuid.New().String(), pc)
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrActiveRequestExists)
			Expect(ok).To(BeTrue())
		})

		It("fails when user already has a pending request", func() {
			existingGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, existingGroupID, "Existing Partner", "desc", "partner", "icon", "Corp", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, uuid.New(), "user1", existingGroupID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			pc := model.PartnerCustomer{
				Name:         "Name1",
				ContactName:  "Contact1",
				ContactPhone: "555-0001",
				Email:        "user1@example.com",
				Location:     "Location1",
			}

			_, err := srv.CreateRequest(context.TODO(), auth.User{Username: "user1"}, uuid.New().String(), pc)
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrActiveRequestExists)
			Expect(ok).To(BeTrue())
		})

		It("succeeds when user only has rejected requests", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, uuid.New(), "user1", partnerGroupID, "rejected", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			pc := model.PartnerCustomer{
				Name:         "Name1",
				ContactName:  "Contact1",
				ContactPhone: "555-0001",
				Email:        "user1@example.com",
				Location:     "Location1",
			}

			created, err := srv.CreateRequest(context.TODO(), auth.User{Username: "user1"}, partnerGroupID.String(), pc)
			Expect(err).To(BeNil())
			Expect(created).ToNot(BeNil())
			Expect(created.RequestStatus).To(Equal(model.RequestStatusPending))
		})

		It("full lifecycle: request -> reject -> request again -> accept -> user is customer", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{Username: "user1"}
			pc := model.PartnerCustomer{
				Name:         "Name1",
				ContactName:  "Contact1",
				ContactPhone: "555-0001",
				Email:        "user1@example.com",
				Location:     "Location1",
			}

			// First request
			created, err := srv.CreateRequest(context.TODO(), user, partnerGroupID.String(), pc)
			Expect(err).To(BeNil())

			// Reject it
			_, err = srv.UpdateRequest(context.TODO(), auth.User{Username: "partneruser"}, created.ID, model.Request{
				Status: model.RequestStatusRejected,
				Reason: "Not now",
			})
			Expect(err).To(BeNil())

			// Updating the rejected request again should fail
			_, err = srv.UpdateRequest(context.TODO(), auth.User{Username: "partneruser"}, created.ID, model.Request{
				Status: model.RequestStatusAccepted,
			})
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrInvalidRequest)
			Expect(ok).To(BeTrue())

			// User is still regular
			accountsSvc := service.NewAccountsService(s)
			identity, err := accountsSvc.GetIdentity(context.TODO(), user)
			Expect(err).To(BeNil())
			Expect(identity.Kind).To(Equal(service.KindRegular))

			// Second request to the same partner
			created2, err := srv.CreateRequest(context.TODO(), user, partnerGroupID.String(), pc)
			Expect(err).To(BeNil())
			Expect(created2.ID).ToNot(Equal(created.ID))

			// Accept it (pending request can be updated)
			_, err = srv.UpdateRequest(context.TODO(), auth.User{Username: "partneruser"}, created2.ID, model.Request{
				Status: model.RequestStatusAccepted,
			})
			Expect(err).To(BeNil())

			// User is now a customer
			identity, err = accountsSvc.GetIdentity(context.TODO(), user)
			Expect(err).To(BeNil())
			Expect(identity.Kind).To(Equal(service.KindCustomer))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM members;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("UpdateRequest", func() {
		var partnerGroupID uuid.UUID

		BeforeEach(func() {
			partnerGroupID = uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
		})

		It("successfully updates status to accepted", func() {
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, requestID, "user1", partnerGroupID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			updated, err := srv.UpdateRequest(context.TODO(), auth.User{Username: "partneruser"}, requestID, model.Request{
				Status: model.RequestStatusAccepted,
			})
			Expect(err).To(BeNil())
			Expect(updated).ToNot(BeNil())
			Expect(updated.RequestStatus).To(Equal(model.RequestStatusAccepted))
		})

		It("fails to update an already accepted request", func() {
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, requestID, "user1", partnerGroupID, "accepted", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			_, err := srv.UpdateRequest(context.TODO(), auth.User{Username: "partneruser"}, requestID, model.Request{
				Status: model.RequestStatusRejected,
				Reason: "Changed my mind",
			})
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrInvalidRequest)
			Expect(ok).To(BeTrue())
		})

		It("fails to update an already rejected request", func() {
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, requestID, "user1", partnerGroupID, "rejected", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			_, err := srv.UpdateRequest(context.TODO(), auth.User{Username: "partneruser"}, requestID, model.Request{
				Status: model.RequestStatusAccepted,
			})
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrInvalidRequest)
			Expect(ok).To(BeTrue())
		})

		It("fails to reject without reason", func() {
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, requestID, "user1", partnerGroupID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			_, err := srv.UpdateRequest(context.TODO(), auth.User{Username: "partneruser"}, requestID, model.Request{
				Status: model.RequestStatusRejected,
			})
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrInvalidRequest)
			Expect(ok).To(BeTrue())
		})

		It("successfully updates status to rejected with reason", func() {
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, requestID, "user1", partnerGroupID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			updated, err := srv.UpdateRequest(context.TODO(), auth.User{Username: "partneruser"}, requestID, model.Request{
				Status: model.RequestStatusRejected,
				Reason: "Not a good fit",
			})
			Expect(err).To(BeNil())
			Expect(updated).ToNot(BeNil())
			Expect(updated.RequestStatus).To(Equal(model.RequestStatusRejected))
			Expect(updated.Reason).ToNot(BeNil())
			Expect(*updated.Reason).To(Equal("Not a good fit"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("CancelRequest", func() {
		var partnerGroupID uuid.UUID

		BeforeEach(func() {
			partnerGroupID = uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
		})

		It("successfully cancels a request", func() {
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, requestID, "user1", partnerGroupID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			err := srv.CancelRequest(context.TODO(), auth.User{Username: "user1"}, requestID)
			Expect(err).To(BeNil())

			var status string
			tx = gormdb.Raw("SELECT request_status FROM partners_customers WHERE id = ?;", requestID).Scan(&status)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("cancelled"))
		})

		It("fails to cancel an accepted request", func() {
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, requestID, "user1", partnerGroupID, "accepted", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			err := srv.CancelRequest(context.TODO(), auth.User{Username: "user1"}, requestID)
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrInvalidRequest)
			Expect(ok).To(BeTrue())
		})

		It("fails to cancel a rejected request", func() {
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, requestID, "user1", partnerGroupID, "rejected", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			err := srv.CancelRequest(context.TODO(), auth.User{Username: "user1"}, requestID)
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrInvalidRequest)
			Expect(ok).To(BeTrue())
		})

		It("returns not found when request belongs to another user", func() {
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, requestID, "user1", partnerGroupID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			err := srv.CancelRequest(context.TODO(), auth.User{Username: "otheruser"}, requestID)
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("ListCustomers", func() {
		It("returns only accepted customers", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, uuid.New(), "user1", partnerGroupID, "accepted", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, uuid.New(), "user2", partnerGroupID, "pending", "Name2", "Contact2", "555-0002", "user2@example.com", "Location2"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, uuid.New(), "user3", partnerGroupID, "rejected", "Name3", "Contact3", "555-0003", "user3@example.com", "Location3"))
			Expect(tx.Error).To(BeNil())

			customers, err := srv.ListCustomers(context.TODO(), auth.User{Username: "partneruser"})
			Expect(err).To(BeNil())
			Expect(customers).To(HaveLen(1))
			Expect(customers[0].Username).To(Equal("user1"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM members;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("RemoveCustomer", func() {
		It("successfully removes an accepted customer", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, uuid.New(), "user1", partnerGroupID, "accepted", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			err := srv.RemoveCustomer(context.TODO(), auth.User{Username: "partneruser"}, "user1")
			Expect(err).To(BeNil())

			var status string
			tx = gormdb.Raw("SELECT request_status FROM partners_customers WHERE username = 'user1';").Scan(&status)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("cancelled"))
		})

		It("fails to remove a pending request", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, uuid.New(), "user1", partnerGroupID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			err := srv.RemoveCustomer(context.TODO(), auth.User{Username: "partneruser"}, "user1")
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		It("fails to remove a rejected request", func() {
			partnerGroupID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerGroupID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerMemberStm, uuid.New(), "partneruser", "p@acme.com", partnerGroupID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerCustomerStm, uuid.New(), "user1", partnerGroupID, "rejected", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			err := srv.RemoveCustomer(context.TODO(), auth.User{Username: "partneruser"}, "user1")
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM members;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})
})
