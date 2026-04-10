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
	insertPartnerStm = "INSERT INTO partners_customers (id, username, partner_id, request_status, name, contact_name, contact_phone, email, location) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s');"
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
		It("successfully list all partners", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user1", "partner1", "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user2", "partner2", "accepted", "Name2", "Contact2", "555-0002", "user2@example.com", "Location2"))
			Expect(tx.Error).To(BeNil())

			partners, err := s.Partner().List(context.TODO(), store.NewPartnerQueryFilter())
			Expect(err).To(BeNil())
			Expect(partners).To(HaveLen(2))
		})

		It("list all partners -- no partners", func() {
			partners, err := s.Partner().List(context.TODO(), store.NewPartnerQueryFilter())
			Expect(err).To(BeNil())
			Expect(partners).To(HaveLen(0))
		})

		It("successfully list partners filtered by username", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user1", "partner1", "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user2", "partner2", "pending", "Name2", "Contact2", "555-0002", "user2@example.com", "Location2"))
			Expect(tx.Error).To(BeNil())

			partners, err := s.Partner().List(context.TODO(), store.NewPartnerQueryFilter().ByUsername("user1"))
			Expect(err).To(BeNil())
			Expect(partners).To(HaveLen(1))
			Expect(partners[0].Username).To(Equal("user1"))
		})

		It("successfully list partners filtered by partner_id", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user1", "partner1", "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user2", "partner2", "pending", "Name2", "Contact2", "555-0002", "user2@example.com", "Location2"))
			Expect(tx.Error).To(BeNil())

			partners, err := s.Partner().List(context.TODO(), store.NewPartnerQueryFilter().ByPartnerID("partner2"))
			Expect(err).To(BeNil())
			Expect(partners).To(HaveLen(1))
			Expect(partners[0].PartnerID).To(Equal("partner2"))
		})

		It("successfully list partners filtered by status", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user1", "partner1", "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user2", "partner2", "accepted", "Name2", "Contact2", "555-0002", "user2@example.com", "Location2"))
			Expect(tx.Error).To(BeNil())

			partners, err := s.Partner().List(context.TODO(), store.NewPartnerQueryFilter().ByStatus(model.RequestStatusAccepted))
			Expect(err).To(BeNil())
			Expect(partners).To(HaveLen(1))
			Expect(partners[0].RequestStatus).To(Equal(model.RequestStatusAccepted))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
		})
	})

	Context("create", func() {
		It("successfully creates a partner", func() {
			pc := model.PartnerCustomer{
				ID:           uuid.New(),
				Username:     "testuser",
				PartnerID:    "partner123",
				Name:         "Test Partner",
				ContactName:  "John Doe",
				ContactPhone: "555-1234",
				Email:        "john@example.com",
				Location:     "New York",
			}

			created, err := s.Partner().Create(context.TODO(), pc)
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
				PartnerID:    "partner123",
				Name:         "Test Partner",
				ContactName:  "John Doe",
				ContactPhone: "555-1234",
				Email:        "john@example.com",
				Location:     "New York",
			}

			_, err := s.Partner().Create(context.TODO(), pc)
			Expect(err).To(BeNil())

			pc2 := model.PartnerCustomer{
				ID:           uuid.New(),
				Username:     "testuser",
				PartnerID:    "partner123",
				Name:         "Another Partner",
				ContactName:  "Jane Doe",
				ContactPhone: "555-5678",
				Email:        "jane@example.com",
				Location:     "Boston",
			}

			_, err = s.Partner().Create(context.TODO(), pc2)
			Expect(err).ToNot(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
		})
	})

	Context("get", func() {
		It("successfully get a partner by username", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user1", "partner1", "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			partner, err := s.Partner().Get(context.TODO(), store.NewPartnerQueryFilter().ByUsername("user1"))
			Expect(err).To(BeNil())
			Expect(partner).ToNot(BeNil())
			Expect(partner.Username).To(Equal("user1"))
		})

		It("successfully get a partner by partner_id", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user1", "partner1", "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			partner, err := s.Partner().Get(context.TODO(), store.NewPartnerQueryFilter().ByPartnerID("partner1"))
			Expect(err).To(BeNil())
			Expect(partner).ToNot(BeNil())
			Expect(partner.PartnerID).To(Equal("partner1"))
		})

		It("failed get a partner -- partner does not exist", func() {
			partner, err := s.Partner().Get(context.TODO(), store.NewPartnerQueryFilter().ByUsername("nonexistent"))
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("record not found"))
			Expect(partner).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
		})
	})

	Context("update", func() {
		It("successfully update request_status", func() {
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, requestID, "user1", "partner1", "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())

			updated, err := s.Partner().Update(context.TODO(), model.PartnerCustomer{
				ID:            requestID,
				RequestStatus: model.RequestStatusAccepted,
			})
			Expect(err).To(BeNil())
			Expect(updated).ToNot(BeNil())
			Expect(updated.RequestStatus).To(Equal(model.RequestStatusAccepted))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
		})
	})

	Context("delete", func() {
		It("successfully delete a partner", func() {
			requestID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertPartnerStm, requestID, "user1", "partner1", "pending", "Name1", "Contact1", "555-0001", "user1@example.com", "Location1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertPartnerStm, uuid.New(), "user2", "partner2", "pending", "Name2", "Contact2", "555-0002", "user2@example.com", "Location2"))
			Expect(tx.Error).To(BeNil())

			err := s.Partner().Delete(context.TODO(), requestID)
			Expect(err).To(BeNil())

			var count int
			tx = gormdb.Raw("SELECT COUNT(*) FROM partners_customers;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM partners_customers;")
		})
	})
})
