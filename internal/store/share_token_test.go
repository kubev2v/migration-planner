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
	insertShareTokenStm = "INSERT INTO share_tokens (token, source_id) VALUES ('%s', '%s');"
)

var _ = Describe("share token store", Ordered, func() {
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

	Context("Create", func() {
		It("successfully creates a share token", func() {
			sourceID := uuid.New()
			// First create a source to associate with the share token
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "test-user", "test-org"))
			Expect(tx.Error).To(BeNil())

			shareToken := model.NewShareToken(sourceID, "test-token-123")

			result, err := s.ShareToken().Create(context.TODO(), shareToken)
			Expect(err).To(BeNil())
			Expect(result).NotTo(BeNil())
			Expect(result.Token).To(Equal("test-token-123"))
			Expect(result.SourceID).To(Equal(sourceID))
		})

		It("fails to create share token with non-existent source", func() {
			nonExistentSourceID := uuid.New()
			shareToken := model.NewShareToken(nonExistentSourceID, "test-token-456")

			result, err := s.ShareToken().Create(context.TODO(), shareToken)
			Expect(err).NotTo(BeNil())
			Expect(result).To(BeNil())
		})

		It("fails to create share token with duplicate token", func() {
			sourceID := uuid.New()
			// First create a source
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source-2", "test-user", "test-org"))
			Expect(tx.Error).To(BeNil())

			// Create first share token
			shareToken1 := model.NewShareToken(sourceID, "duplicate-token")
			result1, err := s.ShareToken().Create(context.TODO(), shareToken1)
			Expect(err).To(BeNil())
			Expect(result1).NotTo(BeNil())

			// Try to create second share token with same token
			sourceID2 := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID2, "test-source-3", "test-user", "test-org"))
			Expect(tx.Error).To(BeNil())

			shareToken2 := model.NewShareToken(sourceID2, "duplicate-token")
			result2, err := s.ShareToken().Create(context.TODO(), shareToken2)
			Expect(err).NotTo(BeNil())
			Expect(result2).To(BeNil())
		})

		It("fails to create multiple share tokens for the same source (unique constraint)", func() {
			sourceID := uuid.New()
			// First create a source
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source-unique", "test-user", "test-org"))
			Expect(tx.Error).To(BeNil())

			// Create first share token for the source
			shareToken1 := model.NewShareToken(sourceID, "first-token")
			result1, err := s.ShareToken().Create(context.TODO(), shareToken1)
			Expect(err).To(BeNil())
			Expect(result1).NotTo(BeNil())
			Expect(result1.SourceID).To(Equal(sourceID))
			Expect(result1.Token).To(Equal("first-token"))

			// Try to create second share token for the SAME source - should fail due to unique constraint
			shareToken2 := model.NewShareToken(sourceID, "second-token")
			result2, err := s.ShareToken().Create(context.TODO(), shareToken2)
			Expect(err).NotTo(BeNil())
			Expect(result2).To(BeNil())
			Expect(err).To(Equal(gorm.ErrDuplicatedKey))

			// Verify only one token exists for this source
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM share_tokens WHERE source_id = ?", sourceID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("GetByToken", func() {
		It("successfully gets share token by token", func() {
			sourceID := uuid.New()

			// Create source and share token
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "test-user", "test-org"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, "find-me-token", sourceID))
			Expect(tx.Error).To(BeNil())

			result, err := s.ShareToken().GetByToken(context.TODO(), "find-me-token")
			Expect(err).To(BeNil())
			Expect(result).NotTo(BeNil())
			Expect(result.Token).To(Equal("find-me-token"))
			Expect(result.SourceID).To(Equal(sourceID))
		})

		It("returns ErrRecordNotFound for non-existent token", func() {
			result, err := s.ShareToken().GetByToken(context.TODO(), "non-existent-token")
			Expect(err).To(Equal(store.ErrRecordNotFound))
			Expect(result).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("GetBySourceID", func() {
		It("successfully gets share token by source ID", func() {
			sourceID := uuid.New()

			// Create source and share token
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "test-user", "test-org"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, "source-token", sourceID))
			Expect(tx.Error).To(BeNil())

			result, err := s.ShareToken().GetBySourceID(context.TODO(), sourceID)
			Expect(err).To(BeNil())
			Expect(result).NotTo(BeNil())
			Expect(result.Token).To(Equal("source-token"))
			Expect(result.SourceID).To(Equal(sourceID))
		})

		It("returns ErrRecordNotFound for non-existent source ID", func() {
			nonExistentSourceID := uuid.New()
			result, err := s.ShareToken().GetBySourceID(context.TODO(), nonExistentSourceID)
			Expect(err).To(Equal(store.ErrRecordNotFound))
			Expect(result).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("Delete", func() {
		It("successfully deletes share token by source ID", func() {
			sourceID := uuid.New()

			// Create source and share token
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "test-user", "test-org"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, "delete-me-token", sourceID))
			Expect(tx.Error).To(BeNil())

			// Verify token exists
			result, err := s.ShareToken().GetBySourceID(context.TODO(), sourceID)
			Expect(err).To(BeNil())
			Expect(result).NotTo(BeNil())

			// Delete the token
			err = s.ShareToken().Delete(context.TODO(), sourceID)
			Expect(err).To(BeNil())

			// Verify token is deleted
			result, err = s.ShareToken().GetBySourceID(context.TODO(), sourceID)
			Expect(err).To(Equal(store.ErrRecordNotFound))
			Expect(result).To(BeNil())
		})

		It("does not error when deleting non-existent share token", func() {
			nonExistentSourceID := uuid.New()
			err := s.ShareToken().Delete(context.TODO(), nonExistentSourceID)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("GetSourceByToken", func() {
		It("successfully gets source by share token with preloaded relations", func() {
			sourceID := uuid.New()

			// Create source and share token
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source-name", "test-user", "test-org"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, "source-lookup-token", sourceID))
			Expect(tx.Error).To(BeNil())

			result, err := s.ShareToken().GetSourceByToken(context.TODO(), "source-lookup-token")
			Expect(err).To(BeNil())
			Expect(result).NotTo(BeNil())
			Expect(result.ID.String()).To(Equal(sourceID.String()))
			Expect(result.Name).To(Equal("test-source-name"))
			Expect(result.Username).To(Equal("test-user"))
			Expect(result.OrgID).To(Equal("test-org"))
		})

		It("returns ErrRecordNotFound for non-existent token", func() {
			result, err := s.ShareToken().GetSourceByToken(context.TODO(), "non-existent-token")
			Expect(err).To(Equal(store.ErrRecordNotFound))
			Expect(result).To(BeNil())
		})

		It("returns ErrRecordNotFound for empty token", func() {
			result, err := s.ShareToken().GetSourceByToken(context.TODO(), "")
			Expect(err).To(Equal(store.ErrRecordNotFound))
			Expect(result).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("ListByOrgID", func() {
		It("successfully lists share tokens by organization ID", func() {
			orgID := "test-org"
			sourceID1 := uuid.New()
			sourceID2 := uuid.New()
			sourceID3 := uuid.New() // Different org

			// Create sources for different organizations
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID1, "store-source1", "user1", orgID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID2, "store-source2", "user2", orgID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID3, "store-source3", "user3", "different-org"))
			Expect(tx.Error).To(BeNil())

			// Create share tokens
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, "token-1", sourceID1))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, "token-2", sourceID2))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, "token-3", sourceID3))
			Expect(tx.Error).To(BeNil())

			result, err := s.ShareToken().ListByOrgID(context.TODO(), orgID)
			Expect(err).To(BeNil())
			Expect(len(result)).To(Equal(2))

			// Verify only tokens for the specified organization are returned
			foundTokens := make(map[string]uuid.UUID)
			for _, token := range result {
				foundTokens[token.Token] = token.SourceID
			}
			Expect(foundTokens["token-1"]).To(Equal(sourceID1))
			Expect(foundTokens["token-2"]).To(Equal(sourceID2))
			_, found := foundTokens["token-3"]
			Expect(found).To(BeFalse()) // Should not include token-3 from different org
		})

		It("returns empty list for organization with no share tokens", func() {
			result, err := s.ShareToken().ListByOrgID(context.TODO(), "nonexistent-org")
			Expect(err).To(BeNil())
			Expect(len(result)).To(Equal(0))
		})

		It("returns empty list when organization has sources but no share tokens", func() {
			orgID := "org-without-tokens"
			sourceID := uuid.New()

			// Create source without share token
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "source-no-token", "user", orgID))
			Expect(tx.Error).To(BeNil())

			result, err := s.ShareToken().ListByOrgID(context.TODO(), orgID)
			Expect(err).To(BeNil())
			Expect(len(result)).To(Equal(0))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})
})
