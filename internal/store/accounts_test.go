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
	insertGroupStm  = "INSERT INTO groups (id, name, description, kind, icon, company, parent_id) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', %s);"
	insertMemberStm = "INSERT INTO members (id, username, email, group_id) VALUES ('%s', '%s', '%s', '%s');"
)

var _ = Describe("accounts store", Ordered, func() {
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

	Context("groups", func() {
		Context("list", func() {
			It("successfully lists all groups", func() {
				orgID1 := uuid.New()
				orgID2 := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID1, "Org One", "First org", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID2, "Org Two", "Second org", "admin", "icon", "Red Hat", "NULL"))
				Expect(tx.Error).To(BeNil())

				orgs, err := s.Accounts().ListGroups(context.TODO(), store.NewGroupQueryFilter())
				Expect(err).To(BeNil())
				Expect(orgs).To(HaveLen(2))
			})

			It("lists groups filtered by kind", func() {
				orgID1 := uuid.New()
				orgID2 := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID1, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID2, "Admin Org", "desc", "admin", "icon", "Red Hat", "NULL"))
				Expect(tx.Error).To(BeNil())

				orgs, err := s.Accounts().ListGroups(context.TODO(), store.NewGroupQueryFilter().ByKind("partner"))
				Expect(err).To(BeNil())
				Expect(orgs).To(HaveLen(1))
				Expect(orgs[0].Kind).To(Equal("partner"))
			})

			It("lists groups filtered by name", func() {
				orgID1 := uuid.New()
				orgID2 := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID1, "Acme Consulting", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID2, "Platform Admin", "desc", "admin", "icon", "Red Hat", "NULL"))
				Expect(tx.Error).To(BeNil())

				orgs, err := s.Accounts().ListGroups(context.TODO(), store.NewGroupQueryFilter().ByName("acme"))
				Expect(err).To(BeNil())
				Expect(orgs).To(HaveLen(1))
				Expect(orgs[0].Name).To(Equal("Acme Consulting"))
			})

			It("lists groups filtered by company", func() {
				orgID1 := uuid.New()
				orgID2 := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID1, "Org One", "desc", "partner", "icon", "Acme Corp", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID2, "Org Two", "desc", "partner", "icon", "Globex Inc", "NULL"))
				Expect(tx.Error).To(BeNil())

				orgs, err := s.Accounts().ListGroups(context.TODO(), store.NewGroupQueryFilter().ByCompany("acme"))
				Expect(err).To(BeNil())
				Expect(orgs).To(HaveLen(1))
				Expect(orgs[0].Company).To(Equal("Acme Corp"))
			})

			It("lists groups filtered by parent ID", func() {
				parentID := uuid.New()
				childID := uuid.New()
				otherID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, parentID, "Parent Org", "desc", "admin", "icon", "Red Hat", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertGroupStm, childID, "Child Org", "desc", "partner", "icon", "Acme", fmt.Sprintf("'%s'", parentID)))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertGroupStm, otherID, "Other Org", "desc", "partner", "icon", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())

				orgs, err := s.Accounts().ListGroups(context.TODO(), store.NewGroupQueryFilter().ByParentID(parentID))
				Expect(err).To(BeNil())
				Expect(orgs).To(HaveLen(1))
				Expect(orgs[0].Name).To(Equal("Child Org"))
			})

			It("lists groups with members preloaded", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID, "Org With Members", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertMemberStm, uuid.New(), "user1", "user1@acme.com", orgID))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertMemberStm, uuid.New(), "user2", "user2@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				orgs, err := s.Accounts().ListGroups(context.TODO(), store.NewGroupQueryFilter())
				Expect(err).To(BeNil())
				Expect(orgs).To(HaveLen(1))
				Expect(orgs[0].Members).To(HaveLen(2))
			})

			It("lists no groups when none exist", func() {
				orgs, err := s.Accounts().ListGroups(context.TODO(), store.NewGroupQueryFilter())
				Expect(err).To(BeNil())
				Expect(orgs).To(HaveLen(0))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("get", func() {
			It("successfully gets a group", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID, "Test Org", "A test org", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				org, err := s.Accounts().GetGroup(context.TODO(), orgID)
				Expect(err).To(BeNil())
				Expect(org.ID).To(Equal(orgID))
				Expect(org.Name).To(Equal("Test Org"))
				Expect(org.Description).To(Equal("A test org"))
				Expect(org.Kind).To(Equal("partner"))
				Expect(org.Company).To(Equal("Acme"))
			})

			It("gets a group with members preloaded", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertMemberStm, uuid.New(), "testuser", "test@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				org, err := s.Accounts().GetGroup(context.TODO(), orgID)
				Expect(err).To(BeNil())
				Expect(org.Members).To(HaveLen(1))
				Expect(org.Members[0].Username).To(Equal("testuser"))
			})

			It("fails to get non-existent group", func() {
				_, err := s.Accounts().GetGroup(context.TODO(), uuid.New())
				Expect(err).To(Equal(store.ErrRecordNotFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("create", func() {
			It("successfully creates a group", func() {
				orgID := uuid.New()
				org := model.Group{
					ID:          orgID,
					Name:        "New Org",
					Description: "A new org",
					Kind:        "partner",
					Icon:        "icon",
					Company:     "Acme",
				}

				created, err := s.Accounts().CreateGroup(context.TODO(), org)
				Expect(err).To(BeNil())
				Expect(created.ID).To(Equal(orgID))
				Expect(created.Name).To(Equal("New Org"))

				var count int
				tx := gormdb.Raw("SELECT COUNT(*) FROM groups;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("successfully creates a group with parent", func() {
				parentID := uuid.New()
				childID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, parentID, "Parent", "desc", "admin", "icon", "Red Hat", "NULL"))
				Expect(tx.Error).To(BeNil())

				child := model.Group{
					ID:       childID,
					Name:     "Child",
					Kind:     "partner",
					Icon:     "icon",
					ParentID: &parentID,
				}
				created, err := s.Accounts().CreateGroup(context.TODO(), child)
				Expect(err).To(BeNil())
				Expect(created.ParentID).ToNot(BeNil())
				Expect(*created.ParentID).To(Equal(parentID))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("update", func() {
			It("successfully updates a group", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID, "Original Name", "Original desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				org := model.Group{
					ID:          orgID,
					Name:        "Updated Name",
					Description: "Updated desc",
					Kind:        "partner",
					Icon:        "icon",
					Company:     "Acme",
				}
				updated, err := s.Accounts().UpdateGroup(context.TODO(), org)
				Expect(err).To(BeNil())
				Expect(updated.Name).To(Equal("Updated Name"))
				Expect(updated.Description).To(Equal("Updated desc"))
				Expect(updated.UpdatedAt).ToNot(BeNil())
			})

			It("fails to update non-existent group", func() {
				org := model.Group{
					ID:   uuid.New(),
					Name: "Does Not Exist",
					Kind: "partner",
					Icon: "icon",
				}
				_, err := s.Accounts().UpdateGroup(context.TODO(), org)
				Expect(err).To(Equal(store.ErrRecordNotFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("delete", func() {
			It("successfully deletes a group", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID, "To Delete", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				err := s.Accounts().DeleteGroup(context.TODO(), orgID)
				Expect(err).To(BeNil())

				var count int
				tx = gormdb.Raw("SELECT COUNT(*) FROM groups;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(0))
			})

			It("does not fail when deleting non-existent group", func() {
				err := s.Accounts().DeleteGroup(context.TODO(), uuid.New())
				Expect(err).To(BeNil())
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})
	})

	Context("members", func() {
		Context("list", func() {
			It("successfully lists all members", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertMemberStm, uuid.New(), "user1", "user1@test.com", orgID))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertMemberStm, uuid.New(), "user2", "user2@test.com", orgID))
				Expect(tx.Error).To(BeNil())

				members, err := s.Accounts().ListMembers(context.TODO(), store.NewMemberQueryFilter())
				Expect(err).To(BeNil())
				Expect(members).To(HaveLen(2))
			})

			It("lists members filtered by group ID", func() {
				orgID1 := uuid.New()
				orgID2 := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID1, "Org One", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID2, "Org Two", "desc", "partner", "icon", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertMemberStm, uuid.New(), "user1", "user1@test.com", orgID1))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertMemberStm, uuid.New(), "user2", "user2@test.com", orgID2))
				Expect(tx.Error).To(BeNil())

				members, err := s.Accounts().ListMembers(context.TODO(), store.NewMemberQueryFilter().ByGroupID(orgID1))
				Expect(err).To(BeNil())
				Expect(members).To(HaveLen(1))
				Expect(members[0].Username).To(Equal("user1"))
			})

			It("lists no members when none exist", func() {
				members, err := s.Accounts().ListMembers(context.TODO(), store.NewMemberQueryFilter())
				Expect(err).To(BeNil())
				Expect(members).To(HaveLen(0))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("get", func() {
			It("successfully gets a member", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertMemberStm, uuid.New(), "testuser", "test@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				member, err := s.Accounts().GetMember(context.TODO(), "testuser")
				Expect(err).To(BeNil())
				Expect(member.ID).ToNot(Equal(uuid.Nil))
				Expect(member.Username).To(Equal("testuser"))
				Expect(member.Email).To(Equal("test@acme.com"))
				Expect(member.GroupID).To(Equal(orgID))
			})

			It("fails to get non-existent member", func() {
				_, err := s.Accounts().GetMember(context.TODO(), "nonexistent")
				Expect(err).To(Equal(store.ErrRecordNotFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("create", func() {
			It("successfully creates a member", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				member := model.Member{
					Username: "newuser",
					Email:    "new@acme.com",
					GroupID:  orgID,
				}

				created, err := s.Accounts().CreateMember(context.TODO(), member)
				Expect(err).To(BeNil())
				Expect(created.ID).ToNot(Equal(uuid.Nil))
				Expect(created.Username).To(Equal("newuser"))
				Expect(created.GroupID).To(Equal(orgID))

				var count int
				tx = gormdb.Raw("SELECT COUNT(*) FROM members;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("fails to create member with duplicate username", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertMemberStm, uuid.New(), "dupuser", "dup1@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				member2 := model.Member{
					Username: "dupuser",
					Email:    "dup2@acme.com",
					GroupID:  orgID,
				}
				_, err := s.Accounts().CreateMember(context.TODO(), member2)
				Expect(err).ToNot(BeNil())
				Expect(err).To(Equal(store.ErrDuplicateKey))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("update", func() {
			It("successfully updates a member", func() {
				orgID := uuid.New()
				memberID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertMemberStm, memberID, "updateuser", "old@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				member := model.Member{
					ID:       memberID,
					Username: "updateuser",
					Email:    "new@acme.com",
					GroupID:  orgID,
				}
				updated, err := s.Accounts().UpdateMember(context.TODO(), member)
				Expect(err).To(BeNil())
				Expect(updated.Email).To(Equal("new@acme.com"))
				Expect(updated.UpdatedAt).ToNot(BeNil())
			})

			It("successfully updates username", func() {
				orgID := uuid.New()
				memberID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertMemberStm, memberID, "oldname", "user@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				member := model.Member{
					ID:       memberID,
					Username: "newname",
					Email:    "user@acme.com",
					GroupID:  orgID,
				}
				updated, err := s.Accounts().UpdateMember(context.TODO(), member)
				Expect(err).To(BeNil())
				Expect(updated.Username).To(Equal("newname"))
				Expect(updated.ID).To(Equal(memberID))

				// Verify old username no longer resolves
				var count int
				tx = gormdb.Raw("SELECT COUNT(*) FROM members WHERE username = 'oldname';").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(0))

				// Verify new username exists
				tx = gormdb.Raw("SELECT COUNT(*) FROM members WHERE username = 'newname';").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("fails to update non-existent member", func() {
				member := model.Member{
					ID:       uuid.New(),
					Username: "nonexistent",
					Email:    "no@acme.com",
				}
				_, err := s.Accounts().UpdateMember(context.TODO(), member)
				Expect(err).To(Equal(store.ErrRecordNotFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("delete", func() {
			It("successfully deletes a member", func() {
				orgID := uuid.New()
				memberID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertMemberStm, memberID, "deleteuser", "del@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				err := s.Accounts().DeleteMember(context.TODO(), memberID)
				Expect(err).To(BeNil())

				var count int
				tx = gormdb.Raw("SELECT COUNT(*) FROM members;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(0))
			})

			It("does not fail when deleting non-existent member", func() {
				err := s.Accounts().DeleteMember(context.TODO(), uuid.New())
				Expect(err).To(BeNil())
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})
	})
})
