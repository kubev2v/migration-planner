package service_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertSourceStm     = "INSERT INTO sources (id, name, username, org_id, inventory) VALUES ('%s', '%s', '%s', '%s', '%s');"
	insertAssessmentStm = "INSERT INTO assessments (id, name, org_id, source_type, source_id) VALUES ('%s', '%s', '%s', '%s', %s);"
	insertSnapshotStm   = "INSERT INTO snapshots (assessment_id, inventory) VALUES ('%s', '%s');"
)

var _ = Describe("assessment service", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
		svc    *service.AssessmentService
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
		svc = service.NewAssessmentService(s)
	})

	AfterAll(func() {
		s.Close()
	})

	Context("ListAssessments", func() {
		BeforeEach(func() {
			// Create test data
			assessment1ID := uuid.New()
			assessment2ID := uuid.New()
			assessment3ID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessment1ID, "Test Assessment 1", "org1", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessment2ID, "Another Test", "org1", service.SourceTypeRvtools, "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessment3ID, "Production Assessment", "org2", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())
		})

		It("lists all assessments for an organization", func() {
			filter := service.NewAssessmentFilter("org1")
			assessments, err := svc.ListAssessments(context.TODO(), filter)

			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(2))
			for _, assessment := range assessments {
				Expect(assessment.OrgID).To(Equal("org1"))
			}
		})

		It("filters assessments by source", func() {
			filter := service.NewAssessmentFilter("org1").WithSource(service.SourceTypeInventory)
			assessments, err := svc.ListAssessments(context.TODO(), filter)

			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(1))
			Expect(assessments[0].SourceType).To(Equal(service.SourceTypeInventory))
		})

		It("filters assessments by name pattern", func() {
			filter := service.NewAssessmentFilter("org1").WithNameLike("Test")
			assessments, err := svc.ListAssessments(context.TODO(), filter)

			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(2))
			for _, assessment := range assessments {
				Expect(assessment.Name).To(ContainSubstring("Test"))
			}
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("ListAssessments with IncludeDefault", func() {
		BeforeEach(func() {
			// Seed the database first to create default assessment
			err := s.Seed()
			Expect(err).To(BeNil())

			// Create regular test assessments
			assessment1ID := uuid.New()
			assessment2ID := uuid.New()
			assessment3ID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessment1ID, "Test Assessment 1", "org1", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessment2ID, "Another Test", "org1", service.SourceTypeRvtools, "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessment3ID, "Production Assessment", "org2", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())
		})

		It("includes default assessment when IncludeDefault is true", func() {
			filter := service.NewAssessmentFilter("org1").WithDefaultInventory()
			assessments, err := svc.ListAssessments(context.TODO(), filter)

			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(3)) // 2 regular org1 assessments + 1 default

			// Verify we have the default assessment
			var hasDefault bool
			for _, assessment := range assessments {
				if strings.ToLower(assessment.Name) == "example" && assessment.OrgID == "example" {
					hasDefault = true
					break
				}
			}
			Expect(hasDefault).To(BeTrue(), "should include default assessment")
		})

		It("excludes default assessment when IncludeDefault is false", func() {
			filter := service.NewAssessmentFilter("org1") // IncludeDefault is false by default
			assessments, err := svc.ListAssessments(context.TODO(), filter)

			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(2)) // Only 2 regular org1 assessments

			// Verify no default assessment is included
			for _, assessment := range assessments {
				Expect(assessment.ID).ToNot(Equal(uuid.UUID{}), "should not include default assessment")
				Expect(assessment.OrgID).To(Equal("org1"), "should only include org1 assessments")
			}
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("GetAssessment", func() {
		It("successfully gets an assessment", func() {
			assessmentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "Test Assessment", "org1", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())

			assessment, err := svc.GetAssessment(context.TODO(), assessmentID)

			Expect(err).To(BeNil())
			Expect(assessment).ToNot(BeNil())
			Expect(assessment.ID).To(Equal(assessmentID))
			Expect(assessment.Name).To(Equal("Test Assessment"))
		})

		It("returns error for non-existent assessment", func() {
			nonExistentID := uuid.New()

			assessment, err := svc.GetAssessment(context.TODO(), nonExistentID)

			Expect(err).ToNot(BeNil())
			Expect(assessment).To(BeNil())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("assessment %s not found", nonExistentID)))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("CreateAssessment", func() {
		Context("with inventory source", func() {
			It("successfully creates assessment with inventory", func() {
				inventory := v1alpha1.Inventory{
					Vcenter: v1alpha1.VCenter{Id: "test-vcenter"},
					Vms:     v1alpha1.VMs{Total: 10},
					Infra:   v1alpha1.Infra{TotalHosts: 5},
				}

				testAssessmentID := uuid.New()
				createForm := mappers.AssessmentCreateForm{
					ID:        testAssessmentID,
					Name:      "Test Assessment",
					OrgID:     "org1",
					Source:    service.SourceTypeInventory,
					Inventory: inventory,
				}

				assessment, err := svc.CreateAssessment(context.TODO(), createForm)

				Expect(err).To(BeNil())
				Expect(assessment).ToNot(BeNil())
				Expect(assessment.ID).To(Equal(testAssessmentID))
				Expect(assessment.Name).To(Equal("Test Assessment"))
				Expect(assessment.SourceType).To(Equal(service.SourceTypeInventory))
				Expect(assessment.SourceID).To(BeNil())
				Expect(assessment.Snapshots).To(HaveLen(1))
			})
		})

		Context("with source (sourceID)", func() {
			It("successfully creates assessment with valid source", func() {
				// Create a source with inventory first
				sourceID := uuid.New()
				inventoryJSON := `{"vcenter":{"id":"test-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`

				tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "user1", "org1", inventoryJSON))
				Expect(tx.Error).To(BeNil())

				testAssessmentID := uuid.New()
				createForm := mappers.AssessmentCreateForm{
					ID:       testAssessmentID,
					Name:     "Test Assessment",
					OrgID:    "org1",
					Source:   service.SourceTypeAgent,
					SourceID: &sourceID,
				}

				assessment, err := svc.CreateAssessment(context.TODO(), createForm)

				Expect(err).To(BeNil())
				Expect(assessment).ToNot(BeNil())
				Expect(assessment.ID).To(Equal(testAssessmentID))
				Expect(assessment.SourceType).To(Equal(service.SourceTypeAgent))
				Expect(assessment.SourceID).ToNot(BeNil())
				Expect(assessment.SourceID.String()).To(Equal(sourceID.String()))
				Expect(assessment.Snapshots).To(HaveLen(1))
			})

			It("fails when user orgID is different than source orgID", func() {
				// Create a source in different org
				sourceID := uuid.New()
				inventoryJSON := `{"vcenter":{"id":"test-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`

				tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "user1", "org2", inventoryJSON))
				Expect(tx.Error).To(BeNil())

				createForm := mappers.AssessmentCreateForm{
					ID:       uuid.New(),
					Name:     "Test Assessment",
					OrgID:    "org1", // Different org than source
					Source:   service.SourceTypeAgent,
					SourceID: &sourceID,
				}

				assessment, err := svc.CreateAssessment(context.TODO(), createForm)

				Expect(err).ToNot(BeNil())
				Expect(assessment).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("forbidden to create assessment from source id"))
			})

			It("fails when source has no inventory", func() {
				// Create a source without inventory
				sourceID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf("INSERT INTO sources (id, name, username, org_id, inventory) VALUES ('%s', '%s', '%s', '%s', %s);", sourceID, "test-source", "user1", "org1", "NULL"))
				Expect(tx.Error).To(BeNil())

				createForm := mappers.AssessmentCreateForm{
					ID:       uuid.New(),
					Name:     "Test Assessment",
					OrgID:    "org1",
					Source:   service.SourceTypeAgent,
					SourceID: &sourceID,
				}

				assessment, err := svc.CreateAssessment(context.TODO(), createForm)

				Expect(err).ToNot(BeNil())
				Expect(assessment).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("source has no inventory"))
			})

			It("fails when source does not exist", func() {
				nonExistentSourceID := uuid.New()

				createForm := mappers.AssessmentCreateForm{
					ID:       uuid.New(),
					Name:     "Test Assessment",
					OrgID:    "org1",
					Source:   service.SourceTypeAgent,
					SourceID: &nonExistentSourceID,
				}

				assessment, err := svc.CreateAssessment(context.TODO(), createForm)

				Expect(err).ToNot(BeNil())
				Expect(assessment).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("record not found"))
			})
		})

		Context("with rvtools source", func() {
			It("handles rvtools source (not implemented yet)", func() {
				createForm := mappers.AssessmentCreateForm{
					ID:     uuid.New(),
					Name:   "Test Assessment",
					OrgID:  "org1",
					Source: service.SourceTypeRvtools,
				}

				assessment, err := svc.CreateAssessment(context.TODO(), createForm)

				// Since rvtools is not implemented, it should create with empty inventory
				Expect(err).To(BeNil())
				Expect(assessment).ToNot(BeNil())
				Expect(assessment.SourceType).To(Equal(service.SourceTypeRvtools))
			})
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("UpdateAssessment", func() {
		Context("assessment with sourceID (source type)", func() {
			It("successfully updates assessment name and adds new snapshot", func() {
				// Create a source with inventory
				sourceID := uuid.New()
				inventoryJSON := `{"vcenter":{"id":"test-vcenter"},"vms":{"total":15},"infra":{"totalHosts":7}}`

				tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "user1", "org1", inventoryJSON))
				Expect(tx.Error).To(BeNil())

				// Create assessment with sourceID
				assessmentID := uuid.New()
				tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "Original Name", "org1", service.SourceTypeAgent, fmt.Sprintf("'%s'", sourceID)))
				Expect(tx.Error).To(BeNil())

				// Add initial snapshot
				tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, assessmentID.String(), `{"vcenter":{"id":"old-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`))
				Expect(tx.Error).To(BeNil())

				newName := "Updated Name"
				updatedAssessment, err := svc.UpdateAssessment(context.TODO(), assessmentID, &newName)

				Expect(err).To(BeNil())
				Expect(updatedAssessment).ToNot(BeNil())
				Expect(updatedAssessment.Name).To(Equal("Updated Name"))
				Expect(updatedAssessment.UpdatedAt).ToNot(BeNil())

				// Verify that a new snapshot was created from the source inventory
				var snapshotCount int
				tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID).Scan(&snapshotCount)
				Expect(tx.Error).To(BeNil())
				Expect(snapshotCount).To(Equal(2)) // Original + new snapshot from source
			})

			It("only updates name when name is provided without creating new snapshot", func() {
				// Create a source with inventory
				sourceID := uuid.New()
				inventoryJSON := `{"vcenter":{"id":"test-vcenter"},"vms":{"total":15},"infra":{"totalHosts":7}}`

				tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "user1", "org1", inventoryJSON))
				Expect(tx.Error).To(BeNil())

				// Create assessment with sourceID
				assessmentID := uuid.New()
				tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "Original Name", "org1", service.SourceTypeAgent, fmt.Sprintf("'%s'", sourceID)))
				Expect(tx.Error).To(BeNil())

				// Add initial snapshot
				tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, assessmentID.String(), `{"vcenter":{"id":"old-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`))
				Expect(tx.Error).To(BeNil())

				newName := "Updated Name"
				updatedAssessment, err := svc.UpdateAssessment(context.TODO(), assessmentID, &newName)

				Expect(err).To(BeNil())
				Expect(updatedAssessment.Name).To(Equal("Updated Name"))

				// Should still create new snapshot since source has inventory
				var snapshotCount int
				tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID).Scan(&snapshotCount)
				Expect(tx.Error).To(BeNil())
				Expect(snapshotCount).To(Equal(2))
			})

			It("creates a new snapshot on every PUT operation for sourceID-based assessments", func() {
				// Create a source with inventory
				sourceID := uuid.New()
				inventoryJSON := `{"vcenter":{"id":"test-vcenter"},"vms":{"total":15},"infra":{"totalHosts":7}}`

				tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "user1", "org1", inventoryJSON))
				Expect(tx.Error).To(BeNil())

				// Create assessment with sourceID
				assessmentID := uuid.New()
				tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "Original Name", "org1", service.SourceTypeAgent, fmt.Sprintf("'%s'", sourceID)))
				Expect(tx.Error).To(BeNil())

				// Add initial snapshot
				tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, assessmentID.String(), `{"vcenter":{"id":"old-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`))
				Expect(tx.Error).To(BeNil())

				// Verify initial state: 1 snapshot
				var snapshotCount int
				tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID).Scan(&snapshotCount)
				Expect(tx.Error).To(BeNil())
				Expect(snapshotCount).To(Equal(1))

				// First update
				newName1 := "Updated Name 1"
				_, err := svc.UpdateAssessment(context.TODO(), assessmentID, &newName1)
				Expect(err).To(BeNil())

				// Should have 2 snapshots now
				tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID).Scan(&snapshotCount)
				Expect(tx.Error).To(BeNil())
				Expect(snapshotCount).To(Equal(2))

				// Second update
				newName2 := "Updated Name 2"
				_, err = svc.UpdateAssessment(context.TODO(), assessmentID, &newName2)
				Expect(err).To(BeNil())

				// Should have 3 snapshots now
				tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID).Scan(&snapshotCount)
				Expect(tx.Error).To(BeNil())
				Expect(snapshotCount).To(Equal(3))

				// Third update
				newName3 := "Updated Name 3"
				_, err = svc.UpdateAssessment(context.TODO(), assessmentID, &newName3)
				Expect(err).To(BeNil())

				// Should have 4 snapshots now
				tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID).Scan(&snapshotCount)
				Expect(tx.Error).To(BeNil())
				Expect(snapshotCount).To(Equal(4))
			})

			It("successfully updates when source is deleted (source_id becomes NULL)", func() {
				// Create a source first
				sourceID := uuid.New()
				inventoryJSON := `{"vcenter":{"id":"test-vcenter"},"vms":{"total":15},"infra":{"totalHosts":7}}`

				tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "user1", "org1", inventoryJSON))
				Expect(tx.Error).To(BeNil())

				// Create assessment with the sourceID
				assessmentID := uuid.New()
				tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "Test Assessment", "org1", service.SourceTypeAgent, fmt.Sprintf("'%s'", sourceID)))
				Expect(tx.Error).To(BeNil())

				// Add initial snapshot
				tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, assessmentID.String(), `{"vcenter":{"id":"old-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`))
				Expect(tx.Error).To(BeNil())

				// Delete the source (this will set source_id to NULL due to ON DELETE SET NULL)
				tx = gormdb.Exec("DELETE FROM sources WHERE id = ?", sourceID)
				Expect(tx.Error).To(BeNil())

				newName := "Updated Name"
				updatedAssessment, err := svc.UpdateAssessment(context.TODO(), assessmentID, &newName)

				// Should succeed and only update name (no new snapshot since source_id is now NULL)
				Expect(err).To(BeNil())
				Expect(updatedAssessment).ToNot(BeNil())
				Expect(updatedAssessment.Name).To(Equal("Updated Name"))

				// Verify that no new snapshot was created
				var snapshotCount int
				tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID).Scan(&snapshotCount)
				Expect(tx.Error).To(BeNil())
				Expect(snapshotCount).To(Equal(1)) // Only original snapshot
			})
		})

		Context("assessment without sourceID (inventory/rvtools type)", func() {
			It("successfully updates assessment name only (no new snapshot)", func() {
				// Create assessment without sourceID (inventory type)
				assessmentID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "Original Name", "org1", service.SourceTypeInventory, "NULL"))
				Expect(tx.Error).To(BeNil())

				// Add initial snapshot
				tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, assessmentID.String(), `{"vcenter":{"id":"test-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`))
				Expect(tx.Error).To(BeNil())

				newName := "Updated Name"
				updatedAssessment, err := svc.UpdateAssessment(context.TODO(), assessmentID, &newName)

				Expect(err).To(BeNil())
				Expect(updatedAssessment).ToNot(BeNil())
				Expect(updatedAssessment.Name).To(Equal("Updated Name"))
				Expect(updatedAssessment.UpdatedAt).ToNot(BeNil())

				// Verify that no new snapshot was created (only name update)
				var snapshotCount int
				tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID).Scan(&snapshotCount)
				Expect(tx.Error).To(BeNil())
				Expect(snapshotCount).To(Equal(1)) // Only original snapshot
			})

			It("successfully updates rvtools assessment name only", func() {
				// Create assessment without sourceID (rvtools type)
				assessmentID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "Original Name", "org1", service.SourceTypeRvtools, "NULL"))
				Expect(tx.Error).To(BeNil())

				newName := "Updated Name"
				updatedAssessment, err := svc.UpdateAssessment(context.TODO(), assessmentID, &newName)

				Expect(err).To(BeNil())
				Expect(updatedAssessment.Name).To(Equal("Updated Name"))
				Expect(updatedAssessment.SourceType).To(Equal(service.SourceTypeRvtools))
			})

			It("maintains only one snapshot after multiple updates for non-sourceID assessments", func() {
				// Create assessment without sourceID (inventory type)
				assessmentID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "Original Name", "org1", service.SourceTypeInventory, "NULL"))
				Expect(tx.Error).To(BeNil())

				// Add initial snapshot
				tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, assessmentID.String(), `{"vcenter":{"id":"test-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`))
				Expect(tx.Error).To(BeNil())

				// Verify initial state: 1 snapshot
				var snapshotCount int
				tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID).Scan(&snapshotCount)
				Expect(tx.Error).To(BeNil())
				Expect(snapshotCount).To(Equal(1))

				// First update
				newName1 := "Updated Name 1"
				_, err := svc.UpdateAssessment(context.TODO(), assessmentID, &newName1)
				Expect(err).To(BeNil())

				// Should still have only 1 snapshot (no new snapshot created)
				tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID).Scan(&snapshotCount)
				Expect(tx.Error).To(BeNil())
				Expect(snapshotCount).To(Equal(1))

				// Second update
				newName2 := "Updated Name 2"
				_, err = svc.UpdateAssessment(context.TODO(), assessmentID, &newName2)
				Expect(err).To(BeNil())

				// Should still have only 1 snapshot
				tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID).Scan(&snapshotCount)
				Expect(tx.Error).To(BeNil())
				Expect(snapshotCount).To(Equal(1))

				// Third update
				newName3 := "Updated Name 3"
				_, err = svc.UpdateAssessment(context.TODO(), assessmentID, &newName3)
				Expect(err).To(BeNil())

				// Should still have only 1 snapshot
				tx = gormdb.Raw("SELECT COUNT(*) FROM snapshots WHERE assessment_id = ?", assessmentID).Scan(&snapshotCount)
				Expect(tx.Error).To(BeNil())
				Expect(snapshotCount).To(Equal(1))
			})
		})

		It("fails when assessment does not exist", func() {
			nonExistentID := uuid.New()
			newName := "Updated Name"

			updatedAssessment, err := svc.UpdateAssessment(context.TODO(), nonExistentID, &newName)

			Expect(err).ToNot(BeNil())
			Expect(updatedAssessment).To(BeNil())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("assessment %s not found", nonExistentID)))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("DeleteAssessment", func() {
		It("successfully deletes an assessment", func() {
			assessmentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID.String(), "Test Assessment", "org1", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())

			err := svc.DeleteAssessment(context.TODO(), assessmentID)

			Expect(err).To(BeNil())

			// Verify assessment is deleted
			var count int
			tx = gormdb.Raw("SELECT COUNT(*) FROM assessments WHERE id = ?", assessmentID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("fails when assessment does not exist", func() {
			nonExistentID := uuid.New()

			err := svc.DeleteAssessment(context.TODO(), nonExistentID)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("assessment %s not found", nonExistentID)))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("AssessmentFilter", func() {
		It("creates filter with orgID and chains methods", func() {
			filter := service.NewAssessmentFilter("org1").
				WithSource(service.SourceTypeInventory).
				WithSourceID("source-123").
				WithNameLike("test").
				WithLimit(10).
				WithOffset(5)

			Expect(filter.OrgID).To(Equal("org1"))
			Expect(filter.Source).To(Equal(service.SourceTypeInventory))
			Expect(filter.SourceID).To(Equal("source-123"))
			Expect(filter.NameLike).To(Equal("test"))
			Expect(filter.Limit).To(Equal(10))
			Expect(filter.Offset).To(Equal(5))
		})

		It("filter does not include default inventory by default", func() {
			filter := service.NewAssessmentFilter("org1")
			Expect(filter.IncludeDefault).To(BeFalse())
		})

		It("filter includes default inventory when WithDefaultInventory is called", func() {
			filter := service.NewAssessmentFilter("org1").WithDefaultInventory()
			Expect(filter.IncludeDefault).To(BeTrue())
		})

		It("chains WithDefaultInventory with other methods", func() {
			filter := service.NewAssessmentFilter("org1").
				WithSource(service.SourceTypeInventory).
				WithDefaultInventory().
				WithNameLike("test")

			Expect(filter.OrgID).To(Equal("org1"))
			Expect(filter.Source).To(Equal(service.SourceTypeInventory))
			Expect(filter.IncludeDefault).To(BeTrue())
			Expect(filter.NameLike).To(Equal("test"))
		})
	})

	Context("Transaction rollback tests", func() {
		BeforeEach(func() {
			// Clean up any existing data
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})

		Context("CreateAssessment failures", func() {
			It("rolls back database when assessment creation fails with orgID mismatch", func() {
				// Create a source in different org
				sourceID := uuid.New()
				inventoryJSON := `{"vcenter":{"id":"test-vcenter"},"vms":{"total":10},"infra":{"totalHosts":5}}`

				tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "user1", "org2", inventoryJSON))
				Expect(tx.Error).To(BeNil())

				// Verify source exists
				var sourceCount int64
				gormdb.Table("sources").Count(&sourceCount)
				Expect(sourceCount).To(Equal(int64(1)))

				// Verify no assessments or snapshots exist initially
				var assessmentCount, snapshotCount int64
				gormdb.Table("assessments").Count(&assessmentCount)
				gormdb.Table("snapshots").Count(&snapshotCount)
				Expect(assessmentCount).To(Equal(int64(0)))
				Expect(snapshotCount).To(Equal(int64(0)))

				// Attempt to create assessment with wrong orgID (should fail)
				createForm := mappers.AssessmentCreateForm{
					ID:       uuid.New(),
					Name:     "Test Assessment",
					OrgID:    "org1", // Different org than source (org2)
					Source:   service.SourceTypeAgent,
					SourceID: &sourceID,
				}

				assessment, err := svc.CreateAssessment(context.TODO(), createForm)

				// Verify the operation failed
				Expect(err).ToNot(BeNil())
				Expect(assessment).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("forbidden to create assessment from source id"))

				// Verify database is clean - no assessments or snapshots were created
				gormdb.Table("assessments").Count(&assessmentCount)
				gormdb.Table("snapshots").Count(&snapshotCount)
				Expect(assessmentCount).To(Equal(int64(0)))
				Expect(snapshotCount).To(Equal(int64(0)))

				// Verify source still exists (wasn't affected)
				gormdb.Table("sources").Count(&sourceCount)
				Expect(sourceCount).To(Equal(int64(1)))
			})

			It("rolls back database when assessment creation fails with missing inventory", func() {
				// Create a source without inventory
				sourceID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf("INSERT INTO sources (id, name, username, org_id, inventory) VALUES ('%s', '%s', '%s', '%s', NULL);", sourceID, "test-source", "user1", "org1"))
				Expect(tx.Error).To(BeNil())

				// Verify source exists
				var sourceCount int64
				gormdb.Table("sources").Count(&sourceCount)
				Expect(sourceCount).To(Equal(int64(1)))

				// Verify no assessments or snapshots exist initially
				var assessmentCount, snapshotCount int64
				gormdb.Table("assessments").Count(&assessmentCount)
				gormdb.Table("snapshots").Count(&snapshotCount)
				Expect(assessmentCount).To(Equal(int64(0)))
				Expect(snapshotCount).To(Equal(int64(0)))

				// Attempt to create assessment with source that has no inventory (should fail)
				createForm := mappers.AssessmentCreateForm{
					ID:       uuid.New(),
					Name:     "Test Assessment",
					OrgID:    "org1",
					Source:   service.SourceTypeAgent,
					SourceID: &sourceID,
				}

				assessment, err := svc.CreateAssessment(context.TODO(), createForm)

				// Verify the operation failed
				Expect(err).ToNot(BeNil())
				Expect(assessment).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("source has no inventory"))

				// Verify database is clean - no assessments or snapshots were created
				gormdb.Table("assessments").Count(&assessmentCount)
				gormdb.Table("snapshots").Count(&snapshotCount)
				Expect(assessmentCount).To(Equal(int64(0)))
				Expect(snapshotCount).To(Equal(int64(0)))

				// Verify source still exists (wasn't affected)
				gormdb.Table("sources").Count(&sourceCount)
				Expect(sourceCount).To(Equal(int64(1)))
			})

			It("rolls back database when assessment creation fails with non-existent source", func() {
				// Use non-existent source ID
				nonExistentSourceID := uuid.New()

				// Verify no data exists initially
				var sourceCount, assessmentCount, snapshotCount int64
				gormdb.Table("sources").Count(&sourceCount)
				gormdb.Table("assessments").Count(&assessmentCount)
				gormdb.Table("snapshots").Count(&snapshotCount)
				Expect(sourceCount).To(Equal(int64(0)))
				Expect(assessmentCount).To(Equal(int64(0)))
				Expect(snapshotCount).To(Equal(int64(0)))

				// Attempt to create assessment with non-existent source (should fail)
				createForm := mappers.AssessmentCreateForm{
					ID:       uuid.New(),
					Name:     "Test Assessment",
					OrgID:    "org1",
					Source:   service.SourceTypeAgent,
					SourceID: &nonExistentSourceID,
				}

				assessment, err := svc.CreateAssessment(context.TODO(), createForm)

				// Verify the operation failed
				Expect(err).ToNot(BeNil())
				Expect(assessment).To(BeNil())

				// Verify database is clean - no assessments or snapshots were created
				gormdb.Table("assessments").Count(&assessmentCount)
				gormdb.Table("snapshots").Count(&snapshotCount)
				Expect(assessmentCount).To(Equal(int64(0)))
				Expect(snapshotCount).To(Equal(int64(0)))

				// Verify no sources exist
				gormdb.Table("sources").Count(&sourceCount)
				Expect(sourceCount).To(Equal(int64(0)))
			})
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})
})
