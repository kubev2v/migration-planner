package store_test

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertAssessmentStm          = "INSERT INTO assessments (id, name, org_id, username, owner_first_name, owner_last_name, source_type, source_id) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', %s);"
	insertAssessmentInventoryStm = "INSERT INTO assessment_inventories (id, assessment_id, name, vcenter_id, vms_count, hosts_count, networks_count, datastores_count, inventory) VALUES (gen_random_uuid()::text, '%s', 'test', '', 0, 0, 0, 0, '%s');"
)

var _ = Describe("assessment store", Ordered, func() {
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
		It("successfully list all assessments", func() {
			assessmentID1 := uuid.New()
			assessmentID2 := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID1, "assessment1", "org1", "user1", "John", "Doe", "inventory", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID2, "assessment2", "org1", "user2", "John", "Doe", "rvtools", "NULL"))
			Expect(tx.Error).To(BeNil())

			assessments, err := s.Assessment().List(context.TODO(), store.NewAssessmentQueryFilter())
			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(2))

			// Verify owner fields are properly loaded
			for _, assessment := range assessments {
				Expect(assessment.OwnerFirstName).ToNot(BeNil())
				Expect(*assessment.OwnerFirstName).To(Equal("John"))
				Expect(assessment.OwnerLastName).ToNot(BeNil())
				Expect(*assessment.OwnerLastName).To(Equal("Doe"))
			}
		})

		It("successfully list assessments filtered by org_id", func() {
			assessmentID1 := uuid.New()
			assessmentID2 := uuid.New()
			assessmentID3 := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID1, "assessment1", "org1", "user1", "John", "Doe", "inventory", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID2, "assessment2", "org2", "user2", "John", "Doe", "rvtools", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID3, "assessment3", "org1", "user3", "John", "Doe", "agent", "NULL"))
			Expect(tx.Error).To(BeNil())

			assessments, err := s.Assessment().List(context.TODO(), store.NewAssessmentQueryFilter().WithOrgID("org1"))
			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(2))

			for _, assessment := range assessments {
				Expect(assessment.OrgID).To(Equal("org1"))
			}
		})

		It("successfully list assessments filtered by source", func() {
			assessmentID1 := uuid.New()
			assessmentID2 := uuid.New()
			assessmentID3 := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID1, "assessment1", "org1", "user1", "John", "Doe", "inventory", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID2, "assessment2", "org1", "user2", "John", "Doe", "rvtools", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID3, "assessment3", "org1", "user3", "John", "Doe", "agent", "NULL"))
			Expect(tx.Error).To(BeNil())

			assessments, err := s.Assessment().List(context.TODO(), store.NewAssessmentQueryFilter().WithSourceType("rvtools"))
			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(1))
			Expect(assessments[0].SourceType).To(Equal("rvtools"))
		})

		It("successfully list assessments with inventories", func() {
			assessmentID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID, "assessment1", "org1", "user1", "John", "Doe", "inventory", "NULL"))
			Expect(tx.Error).To(BeNil())

			// Add inventories
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentInventoryStm, assessmentID, `{"vcenter": {"id": "test"}}`))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentInventoryStm, assessmentID, `{"vcenter": {"id": "test2"}}`))
			Expect(tx.Error).To(BeNil())

			assessments, err := s.Assessment().List(context.TODO(), store.NewAssessmentQueryFilter())
			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(1))
			Expect(assessments[0].Inventories).To(HaveLen(2))
		})

		It("keeps the main inventory first even when a subset is newer", func() {
			older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			created, err := s.Assessment().Create(context.TODO(), model.Assessment{
				ID: uuid.New(), Name: "main-order", OrgID: "org1", SourceType: "inventory",
				Inventories: []model.AssessmentInventory{
					{ID: uuid.New(), Name: "old", CreatedAt: older, Inventory: []byte(`{}`)},
					{ID: uuid.New(), Name: "subset", IsSubset: true, CreatedAt: older.Add(2 * time.Hour), Inventory: []byte(`{}`)},
					{ID: uuid.New(), Name: "main", CreatedAt: older.Add(time.Hour), Inventory: []byte(`{}`)},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(created.Inventories).To(HaveExactElements(
				HaveField("Name", "main"), HaveField("Name", "old"), HaveField("Name", "subset"),
			))
			listed, err := s.Assessment().List(context.TODO(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(listed).To(HaveLen(1))
			Expect(listed[0].Inventories).To(HaveExactElements(
				HaveField("Name", "main"), HaveField("Name", "old"), HaveField("Name", "subset"),
			))
		})

		It("list assessments - no assessments", func() {
			assessments, err := s.Assessment().List(context.TODO(), store.NewAssessmentQueryFilter())
			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(0))
		})

		It("successfully handles assessments with NULL owner fields (backward compatibility)", func() {
			assessmentID := uuid.New()

			// Insert assessment with NULL owner fields
			tx := gormdb.Exec("INSERT INTO assessments (id, name, org_id, username, owner_first_name, owner_last_name, source_type, source_id) VALUES ($1, $2, $3, $4, NULL, NULL, $5, NULL);",
				assessmentID, "legacy-assessment", "org1", "legacy-user", "inventory")
			Expect(tx.Error).To(BeNil())

			assessments, err := s.Assessment().List(context.TODO(), store.NewAssessmentQueryFilter())
			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(1))

			// Verify NULL owner fields are handled correctly
			assessment := assessments[0]
			Expect(assessment.OwnerFirstName).To(BeNil())
			Expect(assessment.OwnerLastName).To(BeNil())
			Expect(assessment.Username).To(Equal("legacy-user"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM assessment_inventories;")
			gormdb.Exec("DELETE FROM assessment_inventories;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("get", func() {
		It("successfully get an assessment", func() {
			assessmentID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID, "test-assessment", "org1", "testuser", "John", "Doe", "inventory", "NULL"))
			Expect(tx.Error).To(BeNil())

			// Add an inventory
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentInventoryStm, assessmentID, `{"vcenter": {"id": "test"}}`))
			Expect(tx.Error).To(BeNil())

			assessment, err := s.Assessment().Get(context.TODO(), assessmentID)
			Expect(err).To(BeNil())
			Expect(assessment).ToNot(BeNil())
			Expect(assessment.ID).To(Equal(assessmentID))
			Expect(assessment.Name).To(Equal("test-assessment"))
			Expect(assessment.OrgID).To(Equal("org1"))
			Expect(assessment.Username).To(Equal("testuser"))
			Expect(assessment.SourceType).To(Equal("inventory"))
			Expect(assessment.Inventories).To(HaveLen(1))
		})

		It("failed to get assessment - assessment does not exist", func() {
			nonExistentID := uuid.New()

			assessment, err := s.Assessment().Get(context.TODO(), nonExistentID)
			Expect(err).ToNot(BeNil())
			Expect(err).To(Equal(store.ErrRecordNotFound))
			Expect(assessment).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM assessment_inventories;")
			gormdb.Exec("DELETE FROM assessment_inventories;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("create", func() {
		It("successfully creates an assessment with inventory", func() {
			assessmentID := uuid.New()
			inventoryJSON := []byte(`{"vcenter":{"id":"test-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`)

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "inventory",
				Inventories: []model.AssessmentInventory{
					{ID: uuid.New(), Name: "test", Inventory: inventoryJSON, Version: 1},
				},
			}

			created, err := s.Assessment().Create(context.TODO(), assessment)
			Expect(err).To(BeNil())
			Expect(created).ToNot(BeNil())
			Expect(created.ID).To(Equal(assessmentID))
			Expect(created.Name).To(Equal("test-assessment"))
			Expect(created.Inventories).To(HaveLen(1))

			// Verify in database
			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM assessments;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			tx = gormdb.Raw("SELECT COUNT(*) FROM assessment_inventories;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("successfully creates an assessment with owner fields", func() {
			assessmentID := uuid.New()
			inventoryJSON := []byte(`{"vcenter":{"id":"test-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`)

			ownerFirstName := "Alice"
			ownerLastName := "Johnson"
			assessment := model.Assessment{
				ID:             assessmentID,
				Name:           "test-assessment-with-owner",
				OrgID:          "org1",
				Username:       "alice",
				OwnerFirstName: &ownerFirstName,
				OwnerLastName:  &ownerLastName,
				SourceType:     "inventory",
				Inventories: []model.AssessmentInventory{
					{ID: uuid.New(), Name: "test", Inventory: inventoryJSON, Version: 1},
				},
			}

			created, err := s.Assessment().Create(context.TODO(), assessment)
			Expect(err).To(BeNil())
			Expect(created).ToNot(BeNil())
			Expect(created.ID).To(Equal(assessmentID))
			Expect(created.Name).To(Equal("test-assessment-with-owner"))

			// Verify owner fields are stored and returned correctly
			Expect(created.OwnerFirstName).ToNot(BeNil())
			Expect(*created.OwnerFirstName).To(Equal("Alice"))
			Expect(created.OwnerLastName).ToNot(BeNil())
			Expect(*created.OwnerLastName).To(Equal("Johnson"))

			// Verify by retrieving from database
			retrieved, err := s.Assessment().Get(context.TODO(), assessmentID)
			Expect(err).To(BeNil())
			Expect(retrieved.OwnerFirstName).ToNot(BeNil())
			Expect(*retrieved.OwnerFirstName).To(Equal("Alice"))
			Expect(retrieved.OwnerLastName).ToNot(BeNil())
			Expect(*retrieved.OwnerLastName).To(Equal("Johnson"))
		})

		It("successfully creates an assessment with valid source_id", func() {
			// First create a source
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "user1", "org1"))
			Expect(tx.Error).To(BeNil())

			assessmentID := uuid.New()
			inventoryJSON := []byte(`{"vcenter":{"id":"test-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`)

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "agent",
				SourceID:   &sourceID,
				Inventories: []model.AssessmentInventory{
					{ID: uuid.New(), Name: "test", Inventory: inventoryJSON, Version: 1},
				},
			}

			created, err := s.Assessment().Create(context.TODO(), assessment)
			Expect(err).To(BeNil())
			Expect(created).ToNot(BeNil())
			Expect(created.ID).To(Equal(assessmentID))
			Expect(created.SourceID).ToNot(BeNil())
			Expect(created.SourceID.String()).To(Equal(sourceID.String()))
		})

		It("successfully creates an assessment with subset inventories", func() {
			assessmentID := uuid.New()
			assessment := model.Assessment{
				ID: assessmentID, Name: "test-assessment-with-subsets", OrgID: "org1", Username: "user1", SourceType: "agent",
				Inventories: []model.AssessmentInventory{
					{ID: uuid.New(), Name: "main", Inventory: []byte(`{"vms":{"total":10}}`), Version: 1},
					{ID: uuid.New(), Name: "subset-a", IsSubset: true, VCenterID: "vc", VMsCount: 3, Inventory: []byte(`{"vms":{"total":3}}`), Version: 1},
					{ID: uuid.New(), Name: "subset-b", IsSubset: true, VCenterID: "vc", VMsCount: 7, Inventory: []byte(`{"vms":{"total":7}}`), Version: 1},
				},
			}
			created, err := s.Assessment().Create(context.TODO(), assessment)
			Expect(err).NotTo(HaveOccurred())
			Expect(created.Inventories).To(ConsistOf(
				And(HaveField("Name", "main"), HaveField("IsSubset", false)),
				And(HaveField("Name", "subset-a"), HaveField("IsSubset", true), HaveField("VMsCount", 3)),
				And(HaveField("Name", "subset-b"), HaveField("IsSubset", true), HaveField("VMsCount", 7)),
			))
			for _, inventory := range created.Inventories {
				Expect(inventory.AssessmentID).To(Equal(assessmentID))
			}
			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM assessment_inventories WHERE assessment_id = ? AND is_subset", assessmentID).Scan(&count)
			Expect(tx.Error).NotTo(HaveOccurred())
			Expect(count).To(Equal(2))
			retrieved, err := s.Assessment().Get(context.TODO(), assessmentID)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved.Inventories).To(HaveLen(3))
		})

		It("fails to create assessment with non-existent source_id", func() {
			nonExistentSourceID := uuid.New()
			assessmentID := uuid.New()
			inventoryJSON := []byte(`{"vcenter":{"id":"test-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`)

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "agent",
				SourceID:   &nonExistentSourceID,
				Inventories: []model.AssessmentInventory{
					{ID: uuid.New(), Name: "test", Inventory: inventoryJSON, Version: 1},
				},
			}

			_, err := s.Assessment().Create(context.TODO(), assessment)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("foreign key constraint"))
		})

		It("fails to create assessment with duplicate name in same org", func() {
			assessmentID1 := uuid.New()
			assessmentID2 := uuid.New()
			inventoryJSON := []byte(`{"vcenter":{"id":"test-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`)

			assessment1 := model.Assessment{
				ID:         assessmentID1,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "inventory",
				Inventories: []model.AssessmentInventory{
					{ID: uuid.New(), Name: "test", Inventory: inventoryJSON, Version: 1},
				},
			}

			_, err := s.Assessment().Create(context.TODO(), assessment1)
			Expect(err).To(BeNil())

			// Try to create another assessment with same name in same org
			assessment2 := model.Assessment{
				ID:         assessmentID2,
				Name:       "test-assessment", // Same name
				OrgID:      "org1",            // Same org
				SourceType: "rvtools",
				Inventories: []model.AssessmentInventory{
					{ID: uuid.New(), Name: "test", Inventory: inventoryJSON, Version: 1},
				},
			}

			_, err = s.Assessment().Create(context.TODO(), assessment2)
			Expect(err).ToNot(BeNil())
		})

		It("successfully creates assessments with same name in different orgs", func() {
			assessmentID1 := uuid.New()
			assessmentID2 := uuid.New()
			inventoryJSON := []byte(`{"vcenter":{"id":"test-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`)

			assessment1 := model.Assessment{
				ID:         assessmentID1,
				Username:   "user2",
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "inventory",
				Inventories: []model.AssessmentInventory{
					{ID: uuid.New(), Name: "test", Inventory: inventoryJSON, Version: 1},
				},
			}

			_, err := s.Assessment().Create(context.TODO(), assessment1)
			Expect(err).To(BeNil())

			// Create assessment with same name but different org
			assessment2 := model.Assessment{
				ID:         assessmentID2,
				Name:       "test-assessment", // Same name
				Username:   "user1",
				OrgID:      "org2", // Different org
				SourceType: "rvtools",
				Inventories: []model.AssessmentInventory{
					{ID: uuid.New(), Name: "test", Inventory: inventoryJSON, Version: 1},
				},
			}

			_, err = s.Assessment().Create(context.TODO(), assessment2)
			Expect(err).To(BeNil())

			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM assessments;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(2))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM assessment_inventories;")
			gormdb.Exec("DELETE FROM assessment_inventories;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("source_id behavior", func() {
		It("sets source_id to null when source is deleted (ON DELETE SET NULL)", func() {
			// First create a source
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "user1", "org1"))
			Expect(tx.Error).To(BeNil())

			// Create an assessment with this source_id
			assessmentID := uuid.New()
			inventoryJSON := []byte(`{"vcenter":{"id":"test-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`)

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "agent",
				SourceID:   &sourceID,
			}

			assessment.Inventories = []model.AssessmentInventory{
				{ID: uuid.New(), Name: "test", Inventory: inventoryJSON, Version: 1},
			}

			created, err := s.Assessment().Create(context.TODO(), assessment)
			Expect(err).To(BeNil())
			Expect(created.SourceID).ToNot(BeNil())
			Expect(created.SourceID.String()).To(Equal(sourceID.String()))

			// Delete the source
			tx = gormdb.Exec("DELETE FROM sources WHERE id = ?", sourceID)
			Expect(tx.Error).To(BeNil())

			// Verify that the assessment's source_id is now null
			retrieved, err := s.Assessment().Get(context.TODO(), assessmentID)
			Expect(err).To(BeNil())
			Expect(retrieved.SourceID).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM assessment_inventories;")
			gormdb.Exec("DELETE FROM assessment_inventories;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("update", func() {
		It("successfully updates assessment name", func() {
			assessmentID := uuid.New()
			inventoryJSON := []byte(`{"vcenter":{"id":"test-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`)

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "original-name",
				OrgID:      "org1",
				SourceType: "inventory",
				Inventories: []model.AssessmentInventory{
					{ID: uuid.New(), Name: "test", Inventory: inventoryJSON, Version: 1},
				},
			}

			_, err := s.Assessment().Create(context.TODO(), assessment)
			Expect(err).To(BeNil())

			newName := "updated-name"
			updated, err := s.Assessment().Update(context.TODO(), assessmentID, &newName, nil)
			Expect(err).To(BeNil())
			Expect(updated).ToNot(BeNil())
			Expect(updated.Name).To(Equal("updated-name"))
			Expect(updated.UpdatedAt).ToNot(BeNil())
		})

		It("successfully adds new inventory to assessment", func() {
			assessmentID := uuid.New()
			inventory1JSON := []byte(`{"vcenter":{"id":"test-vcenter-1"},"vms":{"total":10},"infra":{"totalHosts":5}}`)

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "inventory",
				Inventories: []model.AssessmentInventory{
					{ID: uuid.New(), Name: "test", Inventory: inventory1JSON, Version: 1},
				},
			}

			created, err := s.Assessment().Create(context.TODO(), assessment)
			Expect(err).To(BeNil())
			Expect(created.Inventories).To(HaveLen(1))

			// Add new inventory
			inventory2JSON := []byte(`{"vcenter":{"id":"test-vcenter-2"},"vms":{"total":15},"infra":{"totalHosts":7}}`)
			newInventory := model.AssessmentInventory{
				ID:           uuid.New(),
				Name:         "new-inventory",
				VCenterID:    "test-vcenter-2",
				VMsCount:     15,
				HostsCount:   7,
				Inventory:    inventory2JSON,
				AssessmentID: assessmentID,
			}

			updated, err := s.Assessment().Update(context.TODO(), assessmentID, nil, []model.AssessmentInventory{newInventory})
			Expect(err).To(BeNil())
			Expect(updated).ToNot(BeNil())

			// Verify new inventory was added
			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM assessment_inventories WHERE assessment_id = ?", assessmentID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(2))
		})

		It("successfully updates both name and adds inventory", func() {
			assessmentID := uuid.New()
			inventory1JSON := []byte(`{"vcenter":{"id":"test-vcenter-1"},"vms":{"total":10},"infra":{"totalHosts":5}}`)

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "original-name",
				OrgID:      "org1",
				SourceType: "inventory",
				Inventories: []model.AssessmentInventory{
					{ID: uuid.New(), Name: "test", Inventory: inventory1JSON, Version: 1},
				},
			}

			_, err := s.Assessment().Create(context.TODO(), assessment)
			Expect(err).To(BeNil())

			// Update both name and add inventory
			newName := "updated-name"
			inventory2JSON := []byte(`{"vcenter":{"id":"test-vcenter-2"},"vms":{"total":15},"infra":{"totalHosts":7}}`)
			newInventory := model.AssessmentInventory{
				ID:           uuid.New(),
				Name:         "new-inventory",
				VCenterID:    "test-vcenter-2",
				VMsCount:     15,
				HostsCount:   7,
				Inventory:    inventory2JSON,
				AssessmentID: assessmentID,
			}

			updated, err := s.Assessment().Update(context.TODO(), assessmentID, &newName, []model.AssessmentInventory{newInventory})
			Expect(err).To(BeNil())
			Expect(updated).ToNot(BeNil())
			Expect(updated.Name).To(Equal("updated-name"))
			Expect(updated.UpdatedAt).ToNot(BeNil())

			// Verify new inventory was added
			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM assessment_inventories WHERE assessment_id = ?", assessmentID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(2))
		})

		It("fails to update non-existent assessment", func() {
			nonExistentID := uuid.New()
			newName := "updated-name"

			_, err := s.Assessment().Update(context.TODO(), nonExistentID, &newName, nil)
			Expect(err).ToNot(BeNil())
			Expect(err).To(Equal(store.ErrRecordNotFound))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM assessment_inventories;")
			gormdb.Exec("DELETE FROM assessment_inventories;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("delete", func() {
		It("successfully deletes an assessment", func() {
			assessmentID := uuid.New()
			inventoryJSON := []byte(`{"vcenter":{"id":"test-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`)

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "inventory",
			}

			assessment.Inventories = []model.AssessmentInventory{
				{ID: uuid.New(), Name: "test", Inventory: inventoryJSON, Version: 1},
			}

			_, err := s.Assessment().Create(context.TODO(), assessment)
			Expect(err).To(BeNil())

			// Verify assessment and inventory exist
			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM assessments;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			tx = gormdb.Raw("SELECT COUNT(*) FROM assessment_inventories;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			// Delete assessment
			err = s.Assessment().Delete(context.TODO(), assessmentID)
			Expect(err).To(BeNil())

			// Verify assessment and inventories are deleted
			tx = gormdb.Raw("SELECT COUNT(*) FROM assessments;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			tx = gormdb.Raw("SELECT COUNT(*) FROM assessment_inventories;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("does not fail when deleting non-existent assessment", func() {
			nonExistentID := uuid.New()

			err := s.Assessment().Delete(context.TODO(), nonExistentID)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM assessment_inventories;")
			gormdb.Exec("DELETE FROM assessment_inventories;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("filters", func() {
		BeforeEach(func() {
			// Create a source first for the agent assessment
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, "12345678-1234-1234-1234-123456789012", "test-source", "user1", "org2"))
			Expect(tx.Error).To(BeNil())

			// Create test data
			assessment1ID := uuid.New()
			assessment2ID := uuid.New()
			assessment3ID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessment1ID.String(), "Test Assessment 1", "org1", "testuser1", "John", "Doe", "inventory", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessment2ID.String(), "Another Test", "org1", "testuser2", "John", "Doe", "rvtools", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessment3ID.String(), "Production Assessment", "org2", "produser", "John", "Doe", "agent", "'12345678-1234-1234-1234-123456789012'"))
			Expect(tx.Error).To(BeNil())
		})

		It("filters by name pattern", func() {
			assessments, err := s.Assessment().List(context.TODO(), store.NewAssessmentQueryFilter().WithNameLike("Test"))
			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(2))

			for _, assessment := range assessments {
				Expect(assessment.Name).To(ContainSubstring("Test"))
			}
		})

		It("filters by source ID", func() {
			sourceID := "12345678-1234-1234-1234-123456789012"
			assessments, err := s.Assessment().List(context.TODO(), store.NewAssessmentQueryFilter().WithSourceID(sourceID))
			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(1))
			Expect(assessments[0].SourceID.String()).To(Equal(sourceID))
		})

		It("combines multiple filters", func() {
			assessments, err := s.Assessment().List(context.TODO(),
				store.NewAssessmentQueryFilter().
					WithOrgID("org1").
					WithSourceType("inventory"))
			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(1))
			Expect(assessments[0].OrgID).To(Equal("org1"))
			Expect(assessments[0].SourceType).To(Equal("inventory"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM assessment_inventories;")
			gormdb.Exec("DELETE FROM assessment_inventories;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})
})
