package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
)

const (
	insertRelationStm = "INSERT INTO relations (resource, resource_id, relation, subject_namespace, subject_id) VALUES ('%s', '%s', '%s', '%s', '%s');"
)

func ctxWithUser(username, org string) context.Context {
	return auth.NewTokenContext(context.Background(), auth.User{
		Username:     username,
		Organization: org,
	})
}

var _ = Describe("authz assessment service", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
		svc    service.AssessmentServicer
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
		inner := service.NewAssessmentService(s, nil)
		svc = service.NewAuthzAssessmentService(inner, s)
	})

	AfterAll(func() {
		_ = s.Close()
	})

	Context("ListAssessments", func() {
		BeforeEach(func() {
			// Create assessments for different users
			a1 := uuid.New()
			a2 := uuid.New()
			a3 := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, a1, "Assessment 1", "org1", "user1", "John", "Doe", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, a2, "Assessment 2", "org1", "user1", "John", "Doe", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAssessmentStm, a3, "Assessment 3", "org2", "user2", "Jane", "Smith", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())

			// user1 owns a1 and a2
			tx = gormdb.Exec(fmt.Sprintf(insertRelationStm, "assessment", a1, "owner", "user", "user1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertRelationStm, "assessment", a2, "owner", "user", "user1"))
			Expect(tx.Error).To(BeNil())
			// user2 owns a3
			tx = gormdb.Exec(fmt.Sprintf(insertRelationStm, "assessment", a3, "owner", "user", "user2"))
			Expect(tx.Error).To(BeNil())
		})

		It("returns only assessments the user has relations to", func() {
			ctx := ctxWithUser("user1", "org1")
			filter := service.NewAssessmentFilter("user1", "org1")

			assessments, err := svc.ListAssessments(ctx, filter)

			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(2))
			for _, a := range assessments {
				Expect(a.Username).To(Equal("user1"))
			}
		})

		It("returns empty list when user has no relations", func() {
			ctx := ctxWithUser("unknown-user", "org1")
			filter := service.NewAssessmentFilter("unknown-user", "org1")

			assessments, err := svc.ListAssessments(ctx, filter)

			Expect(err).To(BeNil())
			Expect(assessments).To(BeEmpty())
		})

		It("viewer relation grants read access for listing", func() {
			// Give user3 viewer access to one of user1's assessments
			var resourceID string
			tx := gormdb.Raw("SELECT resource_id FROM relations WHERE subject_id = 'user1' LIMIT 1").Scan(&resourceID)
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertRelationStm, "assessment", resourceID, "viewer", "user", "user3"))
			Expect(tx.Error).To(BeNil())

			ctx := ctxWithUser("user3", "org1")
			filter := &service.AssessmentFilter{}

			assessments, err := svc.ListAssessments(ctx, filter)

			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM relations;")
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
		})
	})

	Context("GetAssessment", func() {
		var assessmentID uuid.UUID

		BeforeEach(func() {
			assessmentID = uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID, "Test Assessment", "org1", "user1", "John", "Doe", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())
		})

		It("returns assessment when user has read permission (owner)", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertRelationStm, "assessment", assessmentID, "owner", "user", "user1"))
			Expect(tx.Error).To(BeNil())

			ctx := ctxWithUser("user1", "org1")
			assessment, err := svc.GetAssessment(ctx, assessmentID)

			Expect(err).To(BeNil())
			Expect(assessment).ToNot(BeNil())
			Expect(assessment.ID).To(Equal(assessmentID))
		})

		It("returns assessment when user has viewer relation", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertRelationStm, "assessment", assessmentID, "viewer", "user", "viewer-user"))
			Expect(tx.Error).To(BeNil())

			ctx := ctxWithUser("viewer-user", "org1")
			assessment, err := svc.GetAssessment(ctx, assessmentID)

			Expect(err).To(BeNil())
			Expect(assessment).ToNot(BeNil())
			Expect(assessment.ID).To(Equal(assessmentID))
		})

		It("returns ErrForbidden when user has no permission", func() {
			ctx := ctxWithUser("unauthorized-user", "org1")
			assessment, err := svc.GetAssessment(ctx, assessmentID)

			Expect(err).ToNot(BeNil())
			Expect(assessment).To(BeNil())
			var forbidden *service.ErrForbidden
			Expect(errors.As(err, &forbidden)).To(BeTrue())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM relations;")
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
		})
	})

	Context("CreateAssessment", func() {
		It("creates assessment and writes owner relation atomically", func() {
			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{
				VcenterId: "test-vcenter",
				Vcenter: &v1alpha1.InventoryData{
					Vms:   v1alpha1.VMs{Total: 10},
					Infra: v1alpha1.Infra{TotalHosts: 5},
				},
			})

			testID := uuid.New()
			createForm := mappers.AssessmentCreateForm{
				ID:        testID,
				Name:      "Authz Assessment",
				OrgID:     "org1",
				Username:  "user1",
				Source:    service.SourceTypeInventory,
				Inventory: inventoryJSON,
			}

			ctx := ctxWithUser("user1", "org1")
			assessment, err := svc.CreateAssessment(ctx, createForm)

			Expect(err).To(BeNil())
			Expect(assessment).ToNot(BeNil())
			Expect(assessment.ID).To(Equal(testID))

			// Verify owner relation was created
			var count int64
			gormdb.Raw("SELECT COUNT(*) FROM relations WHERE resource = 'assessment' AND resource_id = ? AND relation = 'owner' AND subject_id = 'user1'", testID.String()).Scan(&count)
			Expect(count).To(Equal(int64(1)))
		})

		It("the created assessment is visible via ListAssessments", func() {
			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{
				VcenterId: "test-vcenter",
				Vcenter: &v1alpha1.InventoryData{
					Vms:   v1alpha1.VMs{Total: 10},
					Infra: v1alpha1.Infra{TotalHosts: 5},
				},
			})

			createForm := mappers.AssessmentCreateForm{
				ID:        uuid.New(),
				Name:      "Visible Assessment",
				OrgID:     "org1",
				Username:  "user1",
				Source:    service.SourceTypeInventory,
				Inventory: inventoryJSON,
			}

			ctx := ctxWithUser("user1", "org1")
			_, err := svc.CreateAssessment(ctx, createForm)
			Expect(err).To(BeNil())

			// List should find it through authz
			filter := &service.AssessmentFilter{}
			assessments, err := svc.ListAssessments(ctx, filter)
			Expect(err).To(BeNil())
			Expect(assessments).To(HaveLen(1))
			Expect(assessments[0].Name).To(Equal("Visible Assessment"))
		})

		It("rolls back both assessment and relation on inner service failure", func() {
			createForm := mappers.AssessmentCreateForm{
				ID:       uuid.New(),
				Name:     "Fail Assessment",
				OrgID:    "org1",
				Username: "user1",
				Source:   service.SourceTypeAgent,
				SourceID: func() *uuid.UUID { id := uuid.New(); return &id }(), // non-existent source
			}

			ctx := ctxWithUser("user1", "org1")
			assessment, err := svc.CreateAssessment(ctx, createForm)

			Expect(err).ToNot(BeNil())
			Expect(assessment).To(BeNil())

			// Verify no relation was leaked
			var count int64
			gormdb.Raw("SELECT COUNT(*) FROM relations WHERE resource = 'assessment' AND resource_id = ?", createForm.ID.String()).Scan(&count)
			Expect(count).To(Equal(int64(0)))

			// Verify no assessment was leaked
			gormdb.Raw("SELECT COUNT(*) FROM assessments WHERE id = ?", createForm.ID).Scan(&count)
			Expect(count).To(Equal(int64(0)))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM relations;")
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
		})
	})

	Context("UpdateAssessment", func() {
		var assessmentID uuid.UUID

		BeforeEach(func() {
			assessmentID = uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID, "Original Name", "org1", "user1", "John", "Doe", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSnapshotStm, assessmentID, `{"vcenter_id":"test","vcenter":{"vms":{"total":10},"infra":{"totalHosts":5}}}`))
			Expect(tx.Error).To(BeNil())
		})

		It("updates when user has edit permission (owner)", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertRelationStm, "assessment", assessmentID, "owner", "user", "user1"))
			Expect(tx.Error).To(BeNil())

			ctx := ctxWithUser("user1", "org1")
			newName := "Updated Name"
			assessment, err := svc.UpdateAssessment(ctx, assessmentID, &newName)

			Expect(err).To(BeNil())
			Expect(assessment).ToNot(BeNil())
			Expect(assessment.Name).To(Equal("Updated Name"))
		})

		It("returns ErrForbidden when user has only viewer relation", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertRelationStm, "assessment", assessmentID, "viewer", "user", "viewer-user"))
			Expect(tx.Error).To(BeNil())

			ctx := ctxWithUser("viewer-user", "org1")
			newName := "Should Fail"
			assessment, err := svc.UpdateAssessment(ctx, assessmentID, &newName)

			Expect(err).ToNot(BeNil())
			Expect(assessment).To(BeNil())
			var forbidden *service.ErrForbidden
			Expect(errors.As(err, &forbidden)).To(BeTrue())
		})

		It("returns ErrForbidden when user has no relation", func() {
			ctx := ctxWithUser("unauthorized-user", "org1")
			newName := "Should Fail"
			assessment, err := svc.UpdateAssessment(ctx, assessmentID, &newName)

			Expect(err).ToNot(BeNil())
			Expect(assessment).To(BeNil())
			var forbidden *service.ErrForbidden
			Expect(errors.As(err, &forbidden)).To(BeTrue())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM relations;")
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
		})
	})

	Context("DeleteAssessment", func() {
		var assessmentID uuid.UUID

		BeforeEach(func() {
			assessmentID = uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertAssessmentStm, assessmentID, "To Delete", "org1", "user1", "John", "Doe", service.SourceTypeInventory, "NULL"))
			Expect(tx.Error).To(BeNil())
		})

		It("deletes assessment and cleans up relations when user has delete permission", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertRelationStm, "assessment", assessmentID, "owner", "user", "user1"))
			Expect(tx.Error).To(BeNil())

			ctx := ctxWithUser("user1", "org1")
			err := svc.DeleteAssessment(ctx, assessmentID)

			Expect(err).To(BeNil())

			// Verify assessment is deleted
			var assessmentCount int64
			gormdb.Raw("SELECT COUNT(*) FROM assessments WHERE id = ?", assessmentID).Scan(&assessmentCount)
			Expect(assessmentCount).To(Equal(int64(0)))

			// Verify relations are cleaned up
			var relationCount int64
			gormdb.Raw("SELECT COUNT(*) FROM relations WHERE resource = 'assessment' AND resource_id = ?", assessmentID.String()).Scan(&relationCount)
			Expect(relationCount).To(Equal(int64(0)))
		})

		It("returns ErrForbidden when user has only viewer relation", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertRelationStm, "assessment", assessmentID, "viewer", "user", "viewer-user"))
			Expect(tx.Error).To(BeNil())

			ctx := ctxWithUser("viewer-user", "org1")
			err := svc.DeleteAssessment(ctx, assessmentID)

			Expect(err).ToNot(BeNil())
			var forbidden *service.ErrForbidden
			Expect(errors.As(err, &forbidden)).To(BeTrue())

			// Verify assessment still exists
			var count int64
			gormdb.Raw("SELECT COUNT(*) FROM assessments WHERE id = ?", assessmentID).Scan(&count)
			Expect(count).To(Equal(int64(1)))
		})

		It("returns ErrForbidden when user has no relation", func() {
			ctx := ctxWithUser("unauthorized-user", "org1")
			err := svc.DeleteAssessment(ctx, assessmentID)

			Expect(err).ToNot(BeNil())
			var forbidden *service.ErrForbidden
			Expect(errors.As(err, &forbidden)).To(BeTrue())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM relations;")
			gormdb.Exec("DELETE FROM snapshots;")
			gormdb.Exec("DELETE FROM assessments;")
		})
	})
})
