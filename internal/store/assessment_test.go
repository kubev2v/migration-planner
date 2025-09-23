package store_test

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertAssessmentStm = "INSERT INTO assessments (id, name, org_id, username, owner_first_name, owner_last_name, source_type, source_id) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', %s);"
	insertSnapshotStm   = "INSERT INTO snapshots (assessment_id, inventory) VALUES ('%s', '%s');"
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
		s.Close()
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

		It("successfully list assessments with snapshots", func() {
			assessmentID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID, "assessment1", "org1", "user1", "John", "Doe", "inventory", "NULL"))
			Expect(tx.Error).To(BeNil())

			// Add snapshots
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, assessmentID, `{"vcenter": {"id": "test"}}`))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, assessmentID, `{"vcenter": {"id": "test2"}}`))
			Expect(tx.Error).To(BeNil())

			assessments, err := s.Assessment().List(context.TODO(), store.NewAssessmentQueryFilter())
			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(1))
			Expect(assessments[0].Snapshots).To(HaveLen(2))
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
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("get", func() {
		It("successfully get an assessment", func() {
			assessmentID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID, "test-assessment", "org1", "testuser", "John", "Doe", "inventory", "NULL"))
			Expect(tx.Error).To(BeNil())

			// Add a snapshot
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, assessmentID, `{"vcenter": {"id": "test"}}`))
			Expect(tx.Error).To(BeNil())

			assessment, err := s.Assessment().Get(context.TODO(), assessmentID)
			Expect(err).To(BeNil())
			Expect(assessment).ToNot(BeNil())
			Expect(assessment.ID).To(Equal(assessmentID))
			Expect(assessment.Name).To(Equal("test-assessment"))
			Expect(assessment.OrgID).To(Equal("org1"))
			Expect(assessment.Username).To(Equal("testuser"))
			Expect(assessment.SourceType).To(Equal("inventory"))
			Expect(assessment.Snapshots).To(HaveLen(1))
		})

		It("failed to get assessment - assessment does not exist", func() {
			nonExistentID := uuid.New()

			assessment, err := s.Assessment().Get(context.TODO(), nonExistentID)
			Expect(err).ToNot(BeNil())
			Expect(err).To(Equal(store.ErrRecordNotFound))
			Expect(assessment).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("create", func() {
		It("successfully creates an assessment with inventory", func() {
			assessmentID := uuid.New()
			inventory := api.Inventory{
				Vcenter: api.VCenter{Id: "test-vcenter"},
				Vms:     api.VMs{Total: 10},
				Infra:   api.Infra{TotalHosts: 5},
			}

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "inventory",
			}

			created, err := s.Assessment().Create(context.TODO(), assessment, inventory)
			Expect(err).To(BeNil())
			Expect(created).ToNot(BeNil())
			Expect(created.ID).To(Equal(assessmentID))
			Expect(created.Name).To(Equal("test-assessment"))
			Expect(created.Snapshots).To(HaveLen(1))

			// Verify in database
			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM assessments;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("successfully creates an assessment with owner fields", func() {
			assessmentID := uuid.New()
			inventory := api.Inventory{
				Vcenter: api.VCenter{Id: "test-vcenter"},
				Vms:     api.VMs{Total: 10},
				Infra:   api.Infra{TotalHosts: 5},
			}

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
			}

			created, err := s.Assessment().Create(context.TODO(), assessment, inventory)
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
			inventory := api.Inventory{
				Vcenter: api.VCenter{Id: "test-vcenter"},
				Vms:     api.VMs{Total: 10},
				Infra:   api.Infra{TotalHosts: 5},
			}

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "agent",
				SourceID:   &sourceID,
			}

			created, err := s.Assessment().Create(context.TODO(), assessment, inventory)
			Expect(err).To(BeNil())
			Expect(created).ToNot(BeNil())
			Expect(created.ID).To(Equal(assessmentID))
			Expect(created.SourceID).ToNot(BeNil())
			Expect(created.SourceID.String()).To(Equal(sourceID.String()))
		})

		It("fails to create assessment with non-existent source_id", func() {
			nonExistentSourceID := uuid.New()
			assessmentID := uuid.New()
			inventory := api.Inventory{
				Vcenter: api.VCenter{Id: "test-vcenter"},
				Vms:     api.VMs{Total: 10},
				Infra:   api.Infra{TotalHosts: 5},
			}

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "agent",
				SourceID:   &nonExistentSourceID,
			}

			_, err := s.Assessment().Create(context.TODO(), assessment, inventory)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("foreign key constraint"))
		})

		It("fails to create assessment with duplicate name in same org", func() {
			assessmentID1 := uuid.New()
			assessmentID2 := uuid.New()
			inventory := api.Inventory{
				Vcenter: api.VCenter{Id: "test-vcenter"},
				Vms:     api.VMs{Total: 10},
				Infra:   api.Infra{TotalHosts: 5},
			}

			assessment1 := model.Assessment{
				ID:         assessmentID1,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "inventory",
			}

			_, err := s.Assessment().Create(context.TODO(), assessment1, inventory)
			Expect(err).To(BeNil())

			// Try to create another assessment with same name in same org
			assessment2 := model.Assessment{
				ID:         assessmentID2,
				Name:       "test-assessment", // Same name
				OrgID:      "org1",            // Same org
				SourceType: "rvtools",
			}

			_, err = s.Assessment().Create(context.TODO(), assessment2, inventory)
			Expect(err).ToNot(BeNil())
		})

		It("successfully creates assessments with same name in different orgs", func() {
			assessmentID1 := uuid.New()
			assessmentID2 := uuid.New()
			inventory := api.Inventory{
				Vcenter: api.VCenter{Id: "test-vcenter"},
				Vms:     api.VMs{Total: 10},
				Infra:   api.Infra{TotalHosts: 5},
			}

			assessment1 := model.Assessment{
				ID:         assessmentID1,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "inventory",
			}

			_, err := s.Assessment().Create(context.TODO(), assessment1, inventory)
			Expect(err).To(BeNil())

			// Create assessment with same name but different org
			assessment2 := model.Assessment{
				ID:         assessmentID2,
				Name:       "test-assessment", // Same name
				OrgID:      "org2",            // Different org
				SourceType: "rvtools",
			}

			_, err = s.Assessment().Create(context.TODO(), assessment2, inventory)
			Expect(err).To(BeNil())

			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM assessments;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(2))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
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
			inventory := api.Inventory{
				Vcenter: api.VCenter{Id: "test-vcenter"},
				Vms:     api.VMs{Total: 10},
				Infra:   api.Infra{TotalHosts: 5},
			}

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "agent",
				SourceID:   &sourceID,
			}

			created, err := s.Assessment().Create(context.TODO(), assessment, inventory)
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
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("update", func() {
		It("successfully updates assessment name", func() {
			assessmentID := uuid.New()
			inventory := api.Inventory{
				Vcenter: api.VCenter{Id: "test-vcenter"},
				Vms:     api.VMs{Total: 10},
				Infra:   api.Infra{TotalHosts: 5},
			}

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "original-name",
				OrgID:      "org1",
				SourceType: "inventory",
			}

			_, err := s.Assessment().Create(context.TODO(), assessment, inventory)
			Expect(err).To(BeNil())

			newName := "updated-name"
			updated, err := s.Assessment().Update(context.TODO(), assessmentID, &newName, nil)
			Expect(err).To(BeNil())
			Expect(updated).ToNot(BeNil())
			Expect(updated.Name).To(Equal("updated-name"))
			Expect(updated.UpdatedAt).ToNot(BeNil())
		})

		It("successfully adds new snapshot to assessment", func() {
			assessmentID := uuid.New()
			inventory1 := api.Inventory{
				Vcenter: api.VCenter{Id: "test-vcenter-1"},
				Vms:     api.VMs{Total: 10},
				Infra:   api.Infra{TotalHosts: 5},
			}

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "inventory",
			}

			created, err := s.Assessment().Create(context.TODO(), assessment, inventory1)
			Expect(err).To(BeNil())
			Expect(created.Snapshots).To(HaveLen(1))

			// Add new snapshot
			inventory2 := api.Inventory{
				Vcenter: api.VCenter{Id: "test-vcenter-2"},
				Vms:     api.VMs{Total: 15},
				Infra:   api.Infra{TotalHosts: 7},
			}

			updated, err := s.Assessment().Update(context.TODO(), assessmentID, nil, &inventory2)
			Expect(err).To(BeNil())
			Expect(updated).ToNot(BeNil())

			// Verify new snapshot was added
			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(2))
		})

		It("successfully updates both name and adds snapshot", func() {
			assessmentID := uuid.New()
			inventory1 := api.Inventory{
				Vcenter: api.VCenter{Id: "test-vcenter-1"},
				Vms:     api.VMs{Total: 10},
				Infra:   api.Infra{TotalHosts: 5},
			}

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "original-name",
				OrgID:      "org1",
				SourceType: "inventory",
			}

			_, err := s.Assessment().Create(context.TODO(), assessment, inventory1)
			Expect(err).To(BeNil())

			// Update both name and add snapshot
			newName := "updated-name"
			inventory2 := api.Inventory{
				Vcenter: api.VCenter{Id: "test-vcenter-2"},
				Vms:     api.VMs{Total: 15},
				Infra:   api.Infra{TotalHosts: 7},
			}

			updated, err := s.Assessment().Update(context.TODO(), assessmentID, &newName, &inventory2)
			Expect(err).To(BeNil())
			Expect(updated).ToNot(BeNil())
			Expect(updated.Name).To(Equal("updated-name"))
			Expect(updated.UpdatedAt).ToNot(BeNil())

			// Verify new snapshot was added
			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID).Scan(&count)
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
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("delete", func() {
		It("successfully deletes an assessment", func() {
			assessmentID := uuid.New()
			inventory := api.Inventory{
				Vcenter: api.VCenter{Id: "test-vcenter"},
				Vms:     api.VMs{Total: 10},
				Infra:   api.Infra{TotalHosts: 5},
			}

			assessment := model.Assessment{
				ID:         assessmentID,
				Name:       "test-assessment",
				OrgID:      "org1",
				SourceType: "inventory",
			}

			_, err := s.Assessment().Create(context.TODO(), assessment, inventory)
			Expect(err).To(BeNil())

			// Verify assessment and snapshot exist
			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM assessments;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			// Delete assessment
			err = s.Assessment().Delete(context.TODO(), assessmentID)
			Expect(err).To(BeNil())

			// Verify assessment and snapshots are deleted
			tx = gormdb.Raw("SELECT COUNT(*) FROM assessments;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("does not fail when deleting non-existent assessment", func() {
			nonExistentID := uuid.New()

			err := s.Assessment().Delete(context.TODO(), nonExistentID)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
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
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})
})
