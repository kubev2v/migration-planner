package store_test

import (
	"context"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SourceSubsetInventory Store", Ordered, func() {
	var (
		s          store.Store
		testSource *model.Source
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
	})

	AfterAll(func() {
		_ = s.Close()
	})

	BeforeEach(func() {
		// Create a test source
		testSource = &model.Source{
			ID:       uuid.New(),
			Name:     "Test Source",
			Username: "testuser",
			OrgID:    "testorg",
		}
		_, err := s.Source().Create(context.TODO(), *testSource)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		// Clean up test data - ignore errors if source already deleted (e.g., in CASCADE test)
		if testSource != nil {
			_ = s.Source().Delete(context.TODO(), testSource.ID)
		}
	})

	Describe("Create", func() {
		It("should create a new source inventory successfully", func() {
			inventory := model.SourceSubsetInventory{
				ID:         uuid.New(),
				Name:       "Test Subset",
				SourceID:   testSource.ID,
				VCenterID:  "vcenter-1",
				VMsCount:   10,
				Inventory:  []byte(`{"vcenter": {"id": "vcenter-1"}}`),
				UpdateType: "auto",
			}

			created, err := s.SourceSubsetInventory().Create(context.TODO(), inventory)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).NotTo(BeNil())
			Expect(created.ID).To(Equal(inventory.ID))
			Expect(created.Name).To(Equal("Test Subset"))
			Expect(created.SourceID).To(Equal(testSource.ID))
			Expect(created.VCenterID).To(Equal("vcenter-1"))
			Expect(created.VMsCount).To(Equal(10))
			Expect(created.UpdateType).To(Equal("auto"))
			Expect(created.CreatedAt).NotTo(BeZero())
			Expect(created.UpdatedAt).NotTo(BeZero())
		})

		It("should set default update_type to auto if not specified", func() {
			inventory := model.SourceSubsetInventory{
				ID:        uuid.New(),
				Name:      "Test Subset",
				SourceID:  testSource.ID,
				Inventory: []byte(`{}`),
			}

			created, err := s.SourceSubsetInventory().Create(context.TODO(), inventory)
			Expect(err).NotTo(HaveOccurred())
			Expect(created.UpdateType).To(Equal("auto"))
		})

		It("should fail with foreign key constraint if source doesn't exist", func() {
			inventory := model.SourceSubsetInventory{
				ID:        uuid.New(),
				Name:      "Test Subset",
				SourceID:  uuid.New(), // Non-existent source
				Inventory: []byte(`{}`),
			}

			_, err := s.SourceSubsetInventory().Create(context.TODO(), inventory)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Get", func() {
		var testInventory *model.SourceSubsetInventory

		BeforeEach(func() {
			inventory := model.SourceSubsetInventory{
				ID:         uuid.New(),
				Name:       "Test Subset",
				SourceID:   testSource.ID,
				VCenterID:  "vcenter-1",
				VMsCount:   5,
				Inventory:  []byte(`{"vcenter": {"id": "vcenter-1"}}`),
				UpdateType: "auto",
			}
			var err error
			testInventory, err = s.SourceSubsetInventory().Create(context.TODO(), inventory)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should retrieve source inventory by ID", func() {
			retrieved, err := s.SourceSubsetInventory().Get(context.TODO(), testInventory.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved).NotTo(BeNil())
			Expect(retrieved.ID).To(Equal(testInventory.ID))
			Expect(retrieved.Name).To(Equal("Test Subset"))
			Expect(retrieved.SourceID).To(Equal(testSource.ID))
			Expect(retrieved.VMsCount).To(Equal(5))
		})

		It("should return ErrRecordNotFound for non-existent ID", func() {
			_, err := s.SourceSubsetInventory().Get(context.TODO(), uuid.New())
			Expect(err).To(Equal(store.ErrRecordNotFound))
		})
	})

	Describe("Update", func() {
		var testInventory *model.SourceSubsetInventory

		BeforeEach(func() {
			inventory := model.SourceSubsetInventory{
				ID:         uuid.New(),
				Name:       "Original Name",
				SourceID:   testSource.ID,
				VMsCount:   5,
				Inventory:  []byte(`{"original": "data"}`),
				UpdateType: "auto",
			}
			var err error
			testInventory, err = s.SourceSubsetInventory().Create(context.TODO(), inventory)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update source inventory successfully", func() {
			testInventory.Name = "Updated Name"
			testInventory.VMsCount = 15
			testInventory.Inventory = []byte(`{"updated": "data"}`)
			testInventory.UpdateType = "manual"

			updated, err := s.SourceSubsetInventory().Update(context.TODO(), *testInventory)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.Name).To(Equal("Updated Name"))
			Expect(updated.VMsCount).To(Equal(15))
			Expect(updated.UpdateType).To(Equal("manual"))
			Expect(updated.UpdatedAt).To(BeTemporally(">", testInventory.CreatedAt))
		})

		It("should return ErrRecordNotFound when updating non-existent inventory", func() {
			nonExistent := model.SourceSubsetInventory{
				ID:        uuid.New(),
				Name:      "Does Not Exist",
				SourceID:  testSource.ID,
				VMsCount:  99,
				Inventory: []byte(`{}`),
			}

			_, err := s.SourceSubsetInventory().Update(context.TODO(), nonExistent)
			Expect(err).To(Equal(store.ErrRecordNotFound))
		})

		It("should update zero-value fields (VMsCount=0, VCenterID=\"\")", func() {
			testInventory.VMsCount = 0   // Zero value
			testInventory.VCenterID = "" // Empty string
			testInventory.Name = "Zero Values"

			updated, err := s.SourceSubsetInventory().Update(context.TODO(), *testInventory)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.VMsCount).To(Equal(0))
			Expect(updated.VCenterID).To(Equal(""))
			Expect(updated.Name).To(Equal("Zero Values"))

			// Verify it's actually persisted in DB, not just returned
			fetched, err := s.SourceSubsetInventory().Get(context.TODO(), testInventory.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetched.VMsCount).To(Equal(0))
			Expect(fetched.VCenterID).To(Equal(""))
		})
	})

	Describe("Delete", func() {
		var testInventory *model.SourceSubsetInventory

		BeforeEach(func() {
			inventory := model.SourceSubsetInventory{
				ID:        uuid.New(),
				Name:      "Test Subset",
				SourceID:  testSource.ID,
				Inventory: []byte(`{}`),
			}
			var err error
			testInventory, err = s.SourceSubsetInventory().Create(context.TODO(), inventory)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete source inventory successfully", func() {
			err := s.SourceSubsetInventory().Delete(context.TODO(), testInventory.ID)
			Expect(err).NotTo(HaveOccurred())

			// Verify it's deleted
			_, err = s.SourceSubsetInventory().Get(context.TODO(), testInventory.ID)
			Expect(err).To(Equal(store.ErrRecordNotFound))
		})

		It("should not error when deleting non-existent inventory", func() {
			err := s.SourceSubsetInventory().Delete(context.TODO(), uuid.New())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("List", func() {
		var createdIDs []uuid.UUID

		BeforeEach(func() {
			createdIDs = make([]uuid.UUID, 0, 3)
			// Create multiple inventories
			for i := 0; i < 3; i++ {
				inventory := model.SourceSubsetInventory{
					ID:        uuid.New(),
					Name:      "Subset " + string(rune('A'+i)),
					SourceID:  testSource.ID,
					VMsCount:  i + 1,
					Inventory: []byte(`{}`),
				}
				created, err := s.SourceSubsetInventory().Create(context.TODO(), inventory)
				Expect(err).NotTo(HaveOccurred())
				createdIDs = append(createdIDs, created.ID)
			}
		})

		It("should list all source inventories without filter", func() {
			inventories, err := s.SourceSubsetInventory().List(context.TODO(), nil)
			Expect(err).NotTo(HaveOccurred())

			// Extract IDs from returned inventories
			returnedIDs := make([]uuid.UUID, len(inventories))
			for i, inv := range inventories {
				returnedIDs[i] = inv.ID
			}

			// Verify exactly the created inventories are returned (and no leaked rows)
			Expect(returnedIDs).To(ConsistOf(createdIDs))
		})

		It("should filter source inventories by source_id", func() {
			filter := store.NewSourceSubsetInventoryQueryFilter().BySourceID(testSource.ID)
			inventories, err := s.SourceSubsetInventory().List(context.TODO(), filter)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(inventories)).To(Equal(3))
			for _, inv := range inventories {
				Expect(inv.SourceID).To(Equal(testSource.ID))
			}
		})

	})

	Describe("CASCADE Delete", func() {
		It("should delete all source inventories when source is deleted", func() {
			// Create multiple inventories for the source
			inventory1 := model.SourceSubsetInventory{
				ID:        uuid.New(),
				Name:      "Subset 1",
				SourceID:  testSource.ID,
				Inventory: []byte(`{}`),
			}
			inventory2 := model.SourceSubsetInventory{
				ID:        uuid.New(),
				Name:      "Subset 2",
				SourceID:  testSource.ID,
				Inventory: []byte(`{}`),
			}

			_, err := s.SourceSubsetInventory().Create(context.TODO(), inventory1)
			Expect(err).NotTo(HaveOccurred())
			_, err = s.SourceSubsetInventory().Create(context.TODO(), inventory2)
			Expect(err).NotTo(HaveOccurred())

			// Verify inventories exist
			filter := store.NewSourceSubsetInventoryQueryFilter().BySourceID(testSource.ID)
			inventories, err := s.SourceSubsetInventory().List(context.TODO(), filter)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(inventories)).To(BeNumerically(">=", 2))

			// Delete the source
			err = s.Source().Delete(context.TODO(), testSource.ID)
			Expect(err).NotTo(HaveOccurred())

			// Verify all inventories are deleted (CASCADE)
			inventories, err = s.SourceSubsetInventory().List(context.TODO(), filter)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(inventories)).To(Equal(0))
		})
	})
})
