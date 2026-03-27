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
	insertAccountsGroupStm  = "INSERT INTO groups (id, name, description, kind, icon, company, parent_id) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', %s);"
	insertAccountsMemberStm = "INSERT INTO members (id, username, email, group_id) VALUES ('%s', '%s', '%s', '%s');"
)

var _ = Describe("accounts service", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
		svc    *service.AccountsService
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
		svc = service.NewAccountsService(s)
	})

	AfterAll(func() {
		_ = s.Close()
	})

	Context("GetIdentity", func() {
		It("returns regular identity from JWT when member not in DB", func() {
			authUser := auth.User{
				Username:     "jwtuser",
				Organization: "jwt-org-id",
			}

			identity, err := svc.GetIdentity(context.TODO(), authUser)
			Expect(err).To(BeNil())
			Expect(identity.Username).To(Equal("jwtuser"))
			Expect(identity.Kind).To(Equal("regular"))
			Expect(identity.GroupID).To(BeNil())
			Expect(identity.PartnerID).To(BeNil())
		})

		It("returns admin identity when member belongs to admin group", func() {
			orgID := uuid.New()
			userID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Admin Org", "desc", "admin", "icon", "Red Hat", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAccountsMemberStm, userID, "adminuser", "admin@rh.com", orgID))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{
				Username:     "adminuser",
				Organization: "jwt-org-id",
			}

			identity, err := svc.GetIdentity(context.TODO(), authUser)
			Expect(err).To(BeNil())
			Expect(identity.Username).To(Equal("adminuser"))
			Expect(identity.Kind).To(Equal("admin"))
			Expect(*identity.GroupID).To(Equal(orgID.String()))
			Expect(identity.PartnerID).To(BeNil())
		})

		It("returns partner identity when member belongs to partner group", func() {
			orgID := uuid.New()
			userID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Partner Org", "desc", "partner", "icon", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAccountsMemberStm, userID, "partneruser", "partner@acme.com", orgID))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{
				Username:     "partneruser",
				Organization: "jwt-org-id",
			}

			identity, err := svc.GetIdentity(context.TODO(), authUser)
			Expect(err).To(BeNil())
			Expect(identity.Username).To(Equal("partneruser"))
			Expect(identity.Kind).To(Equal("partner"))
			Expect(*identity.GroupID).To(Equal(orgID.String()))
			Expect(identity.PartnerID).ToNot(BeNil())
			Expect(*identity.PartnerID).To(Equal(orgID.String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM members;")
			gormdb.Exec("DELETE FROM groups;")
		})
	})

	Context("Groups", func() {
		Context("GetGroup", func() {
			It("returns the group", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				org, err := svc.GetGroup(context.TODO(), orgID)
				Expect(err).To(BeNil())
				Expect(org.ID).To(Equal(orgID))
				Expect(org.Name).To(Equal("Test Org"))
			})

			It("returns ErrResourceNotFound for missing group", func() {
				_, err := svc.GetGroup(context.TODO(), uuid.New())
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("CreateGroup", func() {
			It("creates a group", func() {
				orgID := uuid.New()
				org := model.Group{
					ID:   orgID,
					Name: "New Org",
					Kind: "partner",
					Icon: "icon",
				}

				created, err := svc.CreateGroup(context.TODO(), org)
				Expect(err).To(BeNil())
				Expect(created.ID).To(Equal(orgID))

				var count int
				tx := gormdb.Raw("SELECT COUNT(*) FROM groups;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("returns ErrDuplicateKey for duplicate group name", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Existing Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				org := model.Group{
					ID:   orgID,
					Name: "Existing Org",
					Kind: "partner",
					Icon: "icon",
				}
				_, err := svc.CreateGroup(context.TODO(), org)
				Expect(err).ToNot(BeNil())
				var dupKey *service.ErrDuplicateKey
				Expect(err).To(BeAssignableToTypeOf(dupKey))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("UpdateGroup", func() {
			It("updates a group", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Old Name", "old desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				org := model.Group{
					ID:          orgID,
					Name:        "New Name",
					Description: "new desc",
					Kind:        "partner",
					Icon:        "icon",
					Company:     "Acme",
				}
				updated, err := svc.UpdateGroup(context.TODO(), org)
				Expect(err).To(BeNil())
				Expect(updated.Name).To(Equal("New Name"))
			})

			It("returns ErrResourceNotFound for missing group", func() {
				org := model.Group{
					ID:   uuid.New(),
					Name: "Does Not Exist",
					Kind: "partner",
					Icon: "icon",
				}
				_, err := svc.UpdateGroup(context.TODO(), org)
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("DeleteGroup", func() {
			It("deletes a group", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "To Delete", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				err := svc.DeleteGroup(context.TODO(), orgID)
				Expect(err).To(BeNil())

				var count int
				tx = gormdb.Raw("SELECT COUNT(*) FROM groups;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(0))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM groups;")
			})
		})
	})

	Context("Members", func() {
		Context("GetMember", func() {
			It("returns the member", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsMemberStm, uuid.New(), "testuser", "test@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				member, err := svc.GetMember(context.TODO(), "testuser")
				Expect(err).To(BeNil())
				Expect(member.Username).To(Equal("testuser"))
			})

			It("returns ErrResourceNotFound for missing member", func() {
				_, err := svc.GetMember(context.TODO(), "nonexistent")
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("CreateMember", func() {
			It("creates a member", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				member := model.Member{
					Username: "newuser",
					Email:    "new@acme.com",
					GroupID:  orgID,
				}

				created, err := svc.CreateMember(context.TODO(), member)
				Expect(err).To(BeNil())
				Expect(created.Username).To(Equal("newuser"))
				Expect(created.ID).ToNot(Equal(uuid.Nil))

				var count int
				tx = gormdb.Raw("SELECT COUNT(*) FROM members;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("returns ErrDuplicateKey for duplicate username", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsMemberStm, uuid.New(), "dupuser", "dup@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				member := model.Member{
					Username: "dupuser",
					Email:    "dup2@acme.com",
					GroupID:  orgID,
				}
				_, err := svc.CreateMember(context.TODO(), member)
				Expect(err).ToNot(BeNil())
				var dupKey *service.ErrDuplicateKey
				Expect(err).To(BeAssignableToTypeOf(dupKey))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("UpdateGroupMember", func() {
			It("updates a member in the group", func() {
				orgID := uuid.New()
				userID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsMemberStm, userID, "updateme", "old@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				member := model.Member{
					Email: "new@acme.com",
				}
				updated, err := svc.UpdateGroupMember(context.TODO(), orgID, "updateme", member)
				Expect(err).To(BeNil())
				Expect(updated.Email).To(Equal("new@acme.com"))
			})

			It("returns ErrMembershipMismatch when member belongs to different group", func() {
				orgA := uuid.New()
				orgB := uuid.New()
				userID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgA, "Org A", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgB, "Org B", "desc", "partner", "icon", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsMemberStm, userID, "wrongorguser", "wrong@acme.com", orgA))
				Expect(tx.Error).To(BeNil())

				member := model.Member{
					Email: "updated@acme.com",
				}
				_, err := svc.UpdateGroupMember(context.TODO(), orgB, "wrongorguser", member)
				Expect(err).ToNot(BeNil())
				var mismatch *service.ErrMembershipMismatch
				Expect(err).To(BeAssignableToTypeOf(mismatch))
			})

			It("returns error when member not found", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				member := model.Member{
					Email: "ghost@acme.com",
				}
				_, err := svc.UpdateGroupMember(context.TODO(), orgID, "ghost", member)
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})
	})

	Context("Membership", func() {
		Context("ListGroupMembers", func() {
			It("lists members in the group", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsMemberStm, uuid.New(), "user1", "user1@acme.com", orgID))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsMemberStm, uuid.New(), "user2", "user2@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				members, err := svc.ListGroupMembers(context.TODO(), orgID)
				Expect(err).To(BeNil())
				Expect(members).To(HaveLen(2))
			})

			It("returns empty list for group with no members", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Empty Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				members, err := svc.ListGroupMembers(context.TODO(), orgID)
				Expect(err).To(BeNil())
				Expect(members).To(BeEmpty())
			})

			It("returns error for non-existent group", func() {
				_, err := svc.ListGroupMembers(context.TODO(), uuid.New())
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})

		Context("RemoveGroupMember", func() {
			It("returns error when group does not exist", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Some Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsMemberStm, uuid.New(), "someuser", "some@acme.com", orgID))
				Expect(tx.Error).To(BeNil())

				err := svc.RemoveGroupMember(context.TODO(), uuid.New(), "someuser")
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			It("returns error when member does not exist", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Test Org", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				err := svc.RemoveGroupMember(context.TODO(), orgID, "ghostuser")
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			It("returns ErrMembershipMismatch when member belongs to different group", func() {
				orgID := uuid.New()
				otherOrgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, orgID, "Org A", "desc", "partner", "icon", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsGroupStm, otherOrgID, "Org B", "desc", "partner", "icon", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsMemberStm, uuid.New(), "wrongorguser", "wrong@acme.com", otherOrgID))
				Expect(tx.Error).To(BeNil())

				err := svc.RemoveGroupMember(context.TODO(), orgID, "wrongorguser")
				Expect(err).ToNot(BeNil())
				var mismatch *service.ErrMembershipMismatch
				Expect(err).To(BeAssignableToTypeOf(mismatch))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM members;")
				gormdb.Exec("DELETE FROM groups;")
			})
		})
	})
})
