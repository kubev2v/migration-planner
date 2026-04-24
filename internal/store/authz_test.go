package store_test

import (
	"context"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

var _ = Describe("authz store", Ordered, func() {
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

	Context("WriteRelationships", func() {
		BeforeEach(func() {
			gormdb.Exec("DELETE FROM relations")
		})

		It("writes owner relation", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				Build()

			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			var count int
			gormdb.Raw("SELECT COUNT(*) FROM relations").Scan(&count)
			Expect(count).To(Equal(1))
		})

		It("is idempotent (upsert)", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				Build()

			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			// Write the same relation again
			err = s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			var count int
			gormdb.Raw("SELECT COUNT(*) FROM relations").Scan(&count)
			Expect(count).To(Equal(1))
		})

		It("writes multiple relations in one batch", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewUserSubject("bob")).
				With(model.NewOrgResource("acme"), model.MemberRelation, model.NewUserSubject("alice")).
				Build()

			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			var count int
			gormdb.Raw("SELECT COUNT(*) FROM relations").Scan(&count)
			Expect(count).To(Equal(3))
		})

		It("handles mixed touch and delete operations", func() {
			// First write a relation
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewUserSubject("bob")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			// Now delete one and add another in the same batch
			updates = store.NewRelationshipBuilder().
				Without(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewUserSubject("bob")).
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewUserSubject("charlie")).
				Build()
			err = s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			rels, err := s.Authz().ListRelationships(context.TODO(), model.NewAssessmentResource("assess1"))
			Expect(err).To(BeNil())
			Expect(rels).To(HaveLen(2))

			subjects := []string{rels[0].Subject.ID, rels[1].Subject.ID}
			Expect(subjects).To(ContainElement("alice"))
			Expect(subjects).To(ContainElement("charlie"))
			Expect(subjects).NotTo(ContainElement("bob"))
		})

		It("does nothing with empty updates", func() {
			err := s.Authz().WriteRelationships(context.TODO(), nil)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM relations")
		})
	})

	Context("DeleteRelationships", func() {
		It("deletes all relations for a specific resource", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewUserSubject("bob")).
				With(model.NewAssessmentResource("assess2"), model.OwnerRelation, model.NewUserSubject("charlie")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			err = s.Authz().DeleteRelationships(context.TODO(), model.NewAssessmentResource("assess1"))
			Expect(err).To(BeNil())

			var count int
			gormdb.Raw("SELECT COUNT(*) FROM relations").Scan(&count)
			Expect(count).To(Equal(1))

			// assess2 should still exist
			rels, err := s.Authz().ListRelationships(context.TODO(), model.NewAssessmentResource("assess2"))
			Expect(err).To(BeNil())
			Expect(rels).To(HaveLen(1))
		})

		It("deletes all relations for a resource type when ID is empty", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				With(model.NewAssessmentResource("assess2"), model.OwnerRelation, model.NewUserSubject("bob")).
				With(model.NewOrgResource("acme"), model.MemberRelation, model.NewUserSubject("alice")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			// Delete all assessment relations
			err = s.Authz().DeleteRelationships(context.TODO(), model.Resource{Type: model.AssessmentResource})
			Expect(err).To(BeNil())

			var count int
			gormdb.Raw("SELECT COUNT(*) FROM relations").Scan(&count)
			Expect(count).To(Equal(1)) // only org relation remains
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM relations")
		})
	})

	Context("ListRelationships", func() {
		It("lists all relations for a resource", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewUserSubject("bob")).
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewOrgSubject("acme")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			rels, err := s.Authz().ListRelationships(context.TODO(), model.NewAssessmentResource("assess1"))
			Expect(err).To(BeNil())
			Expect(rels).To(HaveLen(3))
		})

		It("returns empty slice when no relations exist", func() {
			rels, err := s.Authz().ListRelationships(context.TODO(), model.NewAssessmentResource("nonexistent"))
			Expect(err).To(BeNil())
			Expect(rels).To(BeEmpty())
		})

		It("converts to correct model types", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			rels, err := s.Authz().ListRelationships(context.TODO(), model.NewAssessmentResource("assess1"))
			Expect(err).To(BeNil())
			Expect(rels).To(HaveLen(1))
			Expect(rels[0].ResourceType).To(Equal(model.AssessmentResource))
			Expect(rels[0].ResourceID).To(Equal("assess1"))
			Expect(rels[0].Relation).To(Equal(model.OwnerRelation))
			Expect(rels[0].Subject.Kind).To(Equal(model.UserSubject))
			Expect(rels[0].Subject.ID).To(Equal("alice"))
			Expect(rels[0].String()).To(Equal("assessment:assess1#owner@user:alice"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM relations")
		})
	})

	Context("ListResources", func() {
		It("returns resources the user owns", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				With(model.NewAssessmentResource("assess2"), model.OwnerRelation, model.NewUserSubject("alice")).
				With(model.NewAssessmentResource("assess3"), model.OwnerRelation, model.NewUserSubject("bob")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			resources, err := s.Authz().ListResources(context.TODO(), "alice", model.AssessmentResource)
			Expect(err).To(BeNil())
			Expect(resources).To(HaveLen(2))

			ids := []string{resources[0].ID, resources[1].ID}
			Expect(ids).To(ContainElement("assess1"))
			Expect(ids).To(ContainElement("assess2"))
		})

		It("returns resources shared via viewer relation", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewUserSubject("bob")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			resources, err := s.Authz().ListResources(context.TODO(), "bob", model.AssessmentResource)
			Expect(err).To(BeNil())
			Expect(resources).To(HaveLen(1))
			Expect(resources[0].ID).To(Equal("assess1"))
			// Viewer only gets read permission
			Expect(resources[0].Permissions).To(ConsistOf(model.ReadPermission))
		})

		It("resolves indirect access via org membership", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewOrgSubject("acme")).
				With(model.NewOrgResource("acme"), model.MemberRelation, model.NewUserSubject("bob")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			// Bob should see assess1 via org membership
			resources, err := s.Authz().ListResources(context.TODO(), "bob", model.AssessmentResource)
			Expect(err).To(BeNil())
			Expect(resources).To(HaveLen(1))
			Expect(resources[0].ID).To(Equal("assess1"))
			Expect(resources[0].Permissions).To(ConsistOf(model.ReadPermission))
		})

		It("combines direct and indirect access", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("bob")).
				With(model.NewAssessmentResource("assess2"), model.ViewerRelation, model.NewOrgSubject("acme")).
				With(model.NewOrgResource("acme"), model.MemberRelation, model.NewUserSubject("bob")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			resources, err := s.Authz().ListResources(context.TODO(), "bob", model.AssessmentResource)
			Expect(err).To(BeNil())
			Expect(resources).To(HaveLen(2))
		})

		It("owner gets all permissions", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			resources, err := s.Authz().ListResources(context.TODO(), "alice", model.AssessmentResource)
			Expect(err).To(BeNil())
			Expect(resources).To(HaveLen(1))
			Expect(resources[0].Permissions).To(ConsistOf(
				model.ReadPermission, model.EditPermission, model.SharePermission, model.DeletePermission,
			))
		})

		It("returns empty when user has no access", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			resources, err := s.Authz().ListResources(context.TODO(), "nobody", model.AssessmentResource)
			Expect(err).To(BeNil())
			Expect(resources).To(BeEmpty())
		})

		It("does not grant access to non-members of an org", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewOrgSubject("acme")).
				With(model.NewOrgResource("acme"), model.MemberRelation, model.NewUserSubject("alice")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			// bob is not a member of acme
			resources, err := s.Authz().ListResources(context.TODO(), "bob", model.AssessmentResource)
			Expect(err).To(BeNil())
			Expect(resources).To(BeEmpty())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM relations")
		})
	})

	Context("GetPermissions", func() {
		It("returns all permissions for owner", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			resource, err := s.Authz().GetPermissions(context.TODO(), "alice", model.NewAssessmentResource("assess1"))
			Expect(err).To(BeNil())
			Expect(resource.ID).To(Equal("assess1"))
			Expect(resource.Permissions).To(ConsistOf(
				model.ReadPermission, model.EditPermission, model.SharePermission, model.DeletePermission,
			))
		})

		It("returns read permission for viewer", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewUserSubject("bob")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			resource, err := s.Authz().GetPermissions(context.TODO(), "bob", model.NewAssessmentResource("assess1"))
			Expect(err).To(BeNil())
			Expect(resource.Permissions).To(ConsistOf(model.ReadPermission))
		})

		It("returns no permissions for unrelated user", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			resource, err := s.Authz().GetPermissions(context.TODO(), "nobody", model.NewAssessmentResource("assess1"))
			Expect(err).To(BeNil())
			Expect(resource.Permissions).To(BeEmpty())
		})

		It("resolves permissions via org membership", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewOrgSubject("acme")).
				With(model.NewOrgResource("acme"), model.MemberRelation, model.NewUserSubject("bob")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			resource, err := s.Authz().GetPermissions(context.TODO(), "bob", model.NewAssessmentResource("assess1"))
			Expect(err).To(BeNil())
			Expect(resource.Permissions).To(ConsistOf(model.ReadPermission))
		})

		It("deduplicates permissions from multiple paths", func() {
			// Bob is both a direct viewer and a member of an org that has viewer access
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewUserSubject("bob")).
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewOrgSubject("acme")).
				With(model.NewOrgResource("acme"), model.MemberRelation, model.NewUserSubject("bob")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			resource, err := s.Authz().GetPermissions(context.TODO(), "bob", model.NewAssessmentResource("assess1"))
			Expect(err).To(BeNil())
			// Should have read only once, not duplicated
			Expect(resource.Permissions).To(ConsistOf(model.ReadPermission))
		})

		It("preserves resource type and ID in response", func() {
			resource, err := s.Authz().GetPermissions(context.TODO(), "alice", model.NewAssessmentResource("assess1"))
			Expect(err).To(BeNil())
			Expect(resource.Type).To(Equal(model.AssessmentResource))
			Expect(resource.ID).To(Equal("assess1"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM relations")
		})
	})

	Context("ListBulkRelationship", func() {
		It("returns relationships grouped by resource ID", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewUserSubject("bob")).
				With(model.NewAssessmentResource("assess2"), model.OwnerRelation, model.NewUserSubject("charlie")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			result, err := s.Authz().ListBulkRelationship(context.TODO(), []string{"assess1", "assess2"})
			Expect(err).To(BeNil())
			Expect(result).To(HaveLen(2))
			Expect(result["assess1"]).To(HaveLen(2))
			Expect(result["assess2"]).To(HaveLen(1))
			Expect(result["assess2"][0].Subject.ID).To(Equal("charlie"))
		})

		It("returns empty map when no IDs match", func() {
			result, err := s.Authz().ListBulkRelationship(context.TODO(), []string{"nonexistent1", "nonexistent2"})
			Expect(err).To(BeNil())
			Expect(result).To(BeEmpty())
		})

		It("returns only requested resource IDs", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				With(model.NewAssessmentResource("assess2"), model.OwnerRelation, model.NewUserSubject("bob")).
				With(model.NewAssessmentResource("assess3"), model.OwnerRelation, model.NewUserSubject("charlie")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			result, err := s.Authz().ListBulkRelationship(context.TODO(), []string{"assess1", "assess3"})
			Expect(err).To(BeNil())
			Expect(result).To(HaveLen(2))
			Expect(result).To(HaveKey("assess1"))
			Expect(result).To(HaveKey("assess3"))
			Expect(result).NotTo(HaveKey("assess2"))
		})

		It("handles single resource ID", func() {
			updates := store.NewRelationshipBuilder().
				With(model.NewAssessmentResource("assess1"), model.OwnerRelation, model.NewUserSubject("alice")).
				With(model.NewAssessmentResource("assess1"), model.ViewerRelation, model.NewOrgSubject("acme")).
				Build()
			err := s.Authz().WriteRelationships(context.TODO(), updates)
			Expect(err).To(BeNil())

			result, err := s.Authz().ListBulkRelationship(context.TODO(), []string{"assess1"})
			Expect(err).To(BeNil())
			Expect(result).To(HaveLen(1))
			Expect(result["assess1"]).To(HaveLen(2))
		})

		It("handles empty ID list", func() {
			result, err := s.Authz().ListBulkRelationship(context.TODO(), []string{})
			Expect(err).To(BeNil())
			Expect(result).To(BeEmpty())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM relations")
		})
	})
})
