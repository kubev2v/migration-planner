package store_test

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertPartnerStm      = "INSERT INTO partners_customers (id, username, partner_id, request_status, name, contact_name, contact_phone, email, location) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s');"
	insertPartnerGroupStm = "INSERT INTO groups (id, name, description, kind, icon, company) VALUES ('%s', '%s', 'desc', 'partner', 'icon', '%s');"
)

var _ = Describe("partner store", Ordered, func() {
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
		_ = s.Close()
	})

	Context("list", func() {
		var partner1ID, partner2ID uuid.UUID

		BeforeEach(func() {
			partner1ID = uuid.New()
			partner2ID = uuid.New()
			gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partner1ID, "Partner1", "Acme1"))
			gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partner2ID, "Partner2", "Acme2"))
		})

		It("successfully list all partners", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user1", partner1ID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user2", partner2ID, "accepted", "Name2", "Contact2", "555-0002", "user2@example.com", "Location2"))
			Expect(tx.Error).To(BeNil())

			partners, err := s.PartnerCustomer().List(context.TODO(), store.NewPartnerQueryFilter())
			Expect(err).To(BeNil())
			Expect(partners).To(HaveLen(2))
		})

		It("list all partners -- no partners", func() {
			partners, err := s.PartnerCustomer().List(context.TODO(), store.NewPartnerQueryFilter())
			Expect(err).To(BeNil())
			Expect(partners).To(HaveLen(0))
		})

		It("successfully list partners filtered by username", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user1", partner1ID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user2", partner2ID, "pending", "Name2", "Contact2", "555-0002", "user2@example.com", "Location2"))
			Expect(tx.Error).To(BeNil())

			partners, err := s.PartnerCustomer().List(context.TODO(), store.NewPartnerQueryFilter().ByUsername("user1"))
			Expect(err).To(BeNil())
			Expect(partners).To(HaveLen(1))
			Expect(partners[0].Username).To(Equal("user1"))
		})

		It("successfully list partners filtered by partner_id", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user1", partner1ID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user2", partner2ID, "pending", "Name2", "Contact2", "555-0002", "user2@example.com", "Location2"))
			Expect(tx.Error).To(BeNil())

			partners, err := s.PartnerCustomer().List(context.TODO(), store.NewPartnerQueryFilter().ByPartnerID(partner2ID.String()))
			Expect(err).To(BeNil())
			Expect(partners).To(HaveLen(1))
			Expect(partners[0].PartnerID).To(Equal(partner2ID.String()))
		})

		It("successfully list partners filtered by status", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user1", partner1ID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user2", partner2ID, "accepted", "Name2", "Contact2", "555-0002", "user2@example.com", "Location2"))
			Expect(tx.Error).To(BeNil())

			partners, err := s.PartnerCustomer().List(context.TODO(), store.NewPartnerQueryFilter().ByStatus(model.RequestStatusAccepted))
			Expect(err).To(BeNil())
			Expect(partners).To(HaveLen(1))
			Expect(partners[0].RequestStatus).To(Equal(model.RequestStatusAccepted))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("create", func() {
		var partnerID uuid.UUID

		BeforeEach(func() {
			partnerID = uuid.New()
			gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerID, "TestPartner", "TestCo"))
		})

		It("successfully creates a partner", func() {
			pc := model.PartnerCustomer{
				ID:           uuid.New(),
				Username:     "testuser",
				PartnerID:    partnerID.String(),
				Name:         "Test Partner",
				ContactName:  "John Doe",
				ContactPhone: "555-1234",
				Email:        "john@example.com",
				Location:     "New York",
			}

			created, err := s.PartnerCustomer().Create(context.TODO(), pc)
			Expect(err).To(BeNil())
			Expect(created).ToNot(BeNil())
			Expect(created.ID).ToNot(Equal(uuid.Nil))
			Expect(created.RequestStatus).To(Equal(model.RequestStatusPending))

			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM partners_customers;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("failed to create duplicate partner", func() {
			pc := model.PartnerCustomer{
				ID:           uuid.New(),
				Username:     "testuser",
				PartnerID:    partnerID.String(),
				Name:         "Test Partner",
				ContactName:  "John Doe",
				ContactPhone: "555-1234",
				Email:        "john@example.com",
				Location:     "New York",
			}

			_, err := s.PartnerCustomer().Create(context.TODO(), pc)
			Expect(err).To(BeNil())

			pc2 := model.PartnerCustomer{
				ID:           uuid.New(),
				Username:     "testuser",
				PartnerID:    partnerID.String(),
				Name:         "Another Partner",
				ContactName:  "Jane Doe",
				ContactPhone: "555-5678",
				Email:        "jane@example.com",
				Location:     "Boston",
			}

			_, err = s.PartnerCustomer().Create(context.TODO(), pc2)
			Expect(err).ToNot(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("get", func() {
		var partnerID uuid.UUID

		BeforeEach(func() {
			partnerID = uuid.New()
			gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerID, "Partner1", "Acme1"))
		})

		It("successfully get a partner by username", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user1", partnerID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			partner, err := s.PartnerCustomer().Get(context.TODO(), store.NewPartnerQueryFilter().ByUsername("user1"))
			Expect(err).To(BeNil())
			Expect(partner).ToNot(BeNil())
			Expect(partner.Username).To(Equal("user1"))
		})

		It("successfully get a partner by partner_id", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user1", partnerID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			partner, err := s.PartnerCustomer().Get(context.TODO(), store.NewPartnerQueryFilter().ByPartnerID(partnerID.String()))
			Expect(err).To(BeNil())
			Expect(partner).ToNot(BeNil())
			Expect(partner.PartnerID).To(Equal(partnerID.String()))
		})

		It("failed get a partner -- partner does not exist", func() {
			partner, err := s.PartnerCustomer().Get(context.TODO(), store.NewPartnerQueryFilter().ByUsername("nonexistent"))
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("record not found"))
			Expect(partner).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("update", func() {
		var partnerID uuid.UUID

		BeforeEach(func() {
			partnerID = uuid.New()
			gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerID, "Partner1", "Acme1"))
		})

		It("successfully update request_status", func() {
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, requestID, "user1", partnerID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			updated, err := s.PartnerCustomer().Update(context.TODO(), model.PartnerCustomer{
				ID:            requestID,
				RequestStatus: model.RequestStatusAccepted,
			})
			Expect(err).To(BeNil())
			Expect(updated).ToNot(BeNil())
			Expect(updated.RequestStatus).To(Equal(model.RequestStatusAccepted))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("cancel", func() {
		var partnerID uuid.UUID

		BeforeEach(func() {
			partnerID = uuid.New()
			gormdb.Exec(fmt.Sprintf(insertPartnerGroupStm, partnerID, "Partner1", "Acme1"))
		})

		It("successfully cancels a partner request", func() {
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, requestID, "user1", partnerID, "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			updated, err := s.PartnerCustomer().Update(context.TODO(), model.PartnerCustomer{
				ID:            requestID,
				RequestStatus: model.RequestStatusCancelled,
			})
			Expect(err).To(BeNil())
			Expect(updated.RequestStatus).To(Equal(model.RequestStatusCancelled))

			var count int
			tx = gormdb.Raw("SELECT COUNT(*) FROM partners_customers;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})
})
