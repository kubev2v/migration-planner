package service_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertShareTokenStm          = "INSERT INTO share_tokens (token, source_id) VALUES ('%s', '%s');"
	insertSourceWithInventoryStm = "INSERT INTO sources (id, name, username, org_id, inventory) VALUES ('%s', 'source_name', '%s', '%s', '{\"vcenter\":{\"id\":\"test-vcenter\"}}');"
)

var _ = Describe("ShareTokenService", Ordered, func() {
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

	Context("CreateShareToken", func() {
		It("successfully creates a new share token when none exists", func() {
			sourceID := uuid.New()
			orgID := "test-org"

			// Insert a test source with inventory
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithInventoryStm, sourceID, "admin", orgID))
			Expect(tx.Error).To(BeNil())

			shareTokenService := service.NewShareTokenService(s)
			token, err := shareTokenService.CreateShareToken(context.TODO(), sourceID)

			Expect(err).To(BeNil())
			Expect(token).NotTo(BeNil())
			Expect(token.SourceID).To(Equal(sourceID))
			Expect(token.Token).NotTo(BeEmpty())
			Expect(len(token.Token)).To(Equal(64)) // 32 bytes hex encoded = 64 chars
		})

		It("returns existing share token when one already exists", func() {
			sourceID := uuid.New()
			orgID := "test-org-2"
			existingToken := "existing-token-value"

			// Insert a test source with inventory
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithInventoryStm, sourceID, "admin", orgID))
			Expect(tx.Error).To(BeNil())

			// Insert existing share token
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, existingToken, sourceID))
			Expect(tx.Error).To(BeNil())

			shareTokenService := service.NewShareTokenService(s)
			token, err := shareTokenService.CreateShareToken(context.TODO(), sourceID)

			Expect(err).To(BeNil())
			Expect(token).NotTo(BeNil())
			Expect(token.SourceID).To(Equal(sourceID))
			Expect(token.Token).To(Equal(existingToken))
		})

		It("returns error when source does not exist", func() {
			nonExistentSourceID := uuid.New()

			shareTokenService := service.NewShareTokenService(s)
			_, err := shareTokenService.CreateShareToken(context.TODO(), nonExistentSourceID)

			Expect(err).To(HaveOccurred())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("source %s not found", nonExistentSourceID)))
		})

		It("returns error when source exists but has no inventory", func() {
			sourceID := uuid.New()
			orgID := "test-org-no-inventory"

			// Insert a test source without inventory using the old statement
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", orgID))
			Expect(tx.Error).To(BeNil())

			shareTokenService := service.NewShareTokenService(s)
			_, err := shareTokenService.CreateShareToken(context.TODO(), sourceID)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("source %s does not have an updated inventory", sourceID)))
		})

		It("handles unique constraint violation gracefully and returns existing token", func() {
			sourceID := uuid.New()
			orgID := "test-org-constraint"

			// Insert a test source with inventory
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithInventoryStm, sourceID, "admin", orgID))
			Expect(tx.Error).To(BeNil())

			shareTokenService := service.NewShareTokenService(s)

			// Create first token
			token1, err1 := shareTokenService.CreateShareToken(context.TODO(), sourceID)
			Expect(err1).To(BeNil())
			Expect(token1).NotTo(BeNil())
			Expect(token1.SourceID).To(Equal(sourceID))

			// Try to create second token for same source - should return the existing one
			token2, err2 := shareTokenService.CreateShareToken(context.TODO(), sourceID)
			Expect(err2).To(BeNil())
			Expect(token2).NotTo(BeNil())
			Expect(token2.SourceID).To(Equal(sourceID))

			// Both calls should return the same token
			Expect(token1.Token).To(Equal(token2.Token))

			// Verify only one token exists in database
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM share_tokens WHERE source_id = ?", sourceID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})
	})

	Context("DeleteShareToken", func() {
		It("successfully deletes an existing share token", func() {
			sourceID := uuid.New()
			orgID := "test-org-4"
			tokenValue := "token-to-delete"

			// Insert a test source with inventory
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithInventoryStm, sourceID, "admin", orgID))
			Expect(tx.Error).To(BeNil())

			// Insert share token
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, tokenValue, sourceID))
			Expect(tx.Error).To(BeNil())

			shareTokenService := service.NewShareTokenService(s)
			err := shareTokenService.DeleteShareToken(context.TODO(), sourceID)

			Expect(err).To(BeNil())

			// Verify token is deleted
			_, err = s.ShareToken().GetBySourceID(context.TODO(), sourceID)
			Expect(err).To(Equal(store.ErrRecordNotFound))
		})

		It("does not return error when trying to delete non-existent share token", func() {
			nonExistentSourceID := uuid.New()

			shareTokenService := service.NewShareTokenService(s)
			err := shareTokenService.DeleteShareToken(context.TODO(), nonExistentSourceID)

			Expect(err).To(BeNil())
		})
	})

	Context("GetSourceByToken", func() {
		It("successfully retrieves source by valid token", func() {
			sourceID := uuid.New()
			orgID := "test-org-5"
			tokenValue := "valid-token"

			// Insert a test source with inventory
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithInventoryStm, sourceID, "admin", orgID))
			Expect(tx.Error).To(BeNil())

			// Insert share token
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, tokenValue, sourceID))
			Expect(tx.Error).To(BeNil())

			shareTokenService := service.NewShareTokenService(s)
			source, err := shareTokenService.GetSourceByToken(context.TODO(), tokenValue)

			Expect(err).To(BeNil())
			Expect(source).NotTo(BeNil())
			Expect(source.ID).To(Equal(sourceID))
			Expect(source.OrgID).To(Equal(orgID))
		})

		It("returns ErrResourceNotFound for invalid token", func() {
			invalidToken := "invalid-token"

			shareTokenService := service.NewShareTokenService(s)
			_, err := shareTokenService.GetSourceByToken(context.TODO(), invalidToken)

			Expect(err).To(HaveOccurred())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		It("returns ErrResourceNotFound for empty token", func() {
			shareTokenService := service.NewShareTokenService(s)
			_, err := shareTokenService.GetSourceByToken(context.TODO(), "")

			Expect(err).To(HaveOccurred())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})
	})

	Context("generateRandomToken", func() {
		It("generates a valid 64-character hex string", func() {
			sourceID := uuid.New()
			orgID := "test-org-hex"

			// Insert a test source with inventory
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithInventoryStm, sourceID, "admin", orgID))
			Expect(tx.Error).To(BeNil())

			service := service.NewShareTokenService(s)
			token, err := service.CreateShareToken(context.TODO(), sourceID)

			Expect(err).To(BeNil())
			Expect(token.Token).To(HaveLen(64))
			Expect(token.Token).To(MatchRegexp("^[0-9a-f]{64}$"))
		})

		It("generates unique tokens", func() {
			shareTokenService := service.NewShareTokenService(s)

			sourceID1 := uuid.New()
			sourceID2 := uuid.New()
			orgID := "test-org-unique"

			// Insert test sources with different names to avoid unique constraint violation
			tx := gormdb.Exec(fmt.Sprintf("INSERT INTO sources (id, name, username, org_id, inventory) VALUES ('%s', 'source_name_1', 'admin', '%s', '{\"vcenter\":{\"id\":\"test-vcenter\"}}');", sourceID1, orgID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO sources (id, name, username, org_id, inventory) VALUES ('%s', 'source_name_2', 'admin', '%s', '{\"vcenter\":{\"id\":\"test-vcenter\"}}');", sourceID2, orgID))
			Expect(tx.Error).To(BeNil())

			token1, err1 := shareTokenService.CreateShareToken(context.TODO(), sourceID1)
			token2, err2 := shareTokenService.CreateShareToken(context.TODO(), sourceID2)

			Expect(err1).To(BeNil())
			Expect(err2).To(BeNil())
			Expect(token1.Token).NotTo(Equal(token2.Token))
		})
	})

	Context("Token format validation", func() {
		It("generates hex-encoded tokens", func() {
			sourceID := uuid.New()
			orgID := "test-org-hex-validation"

			// Insert a test source with inventory
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithInventoryStm, sourceID, "admin", orgID))
			Expect(tx.Error).To(BeNil())

			shareTokenService := service.NewShareTokenService(s)
			token, err := shareTokenService.CreateShareToken(context.TODO(), sourceID)

			Expect(err).To(BeNil())
			Expect(token.Token).To(HaveLen(64))
			// Check that all characters are valid hex characters
			for _, char := range token.Token {
				Expect(strings.ContainsRune("0123456789abcdef", char)).To(BeTrue())
			}
		})
	})

	Context("GetShareTokenBySourceID", func() {
		It("successfully retrieves share token by source ID", func() {
			sourceID := uuid.New()
			orgID := "test-org-get"

			// Insert a test source with inventory
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithInventoryStm, sourceID, "admin", orgID))
			Expect(tx.Error).To(BeNil())

			// Insert share token
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, "test-token-by-source", sourceID))
			Expect(tx.Error).To(BeNil())

			shareTokenService := service.NewShareTokenService(s)
			token, err := shareTokenService.GetShareTokenBySourceID(context.TODO(), sourceID)

			Expect(err).To(BeNil())
			Expect(token).NotTo(BeNil())
			Expect(token.SourceID).To(Equal(sourceID))
			Expect(token.Token).To(Equal("test-token-by-source"))
		})

		It("returns ErrResourceNotFound for non-existent share token", func() {
			nonExistentSourceID := uuid.New()

			shareTokenService := service.NewShareTokenService(s)
			_, err := shareTokenService.GetShareTokenBySourceID(context.TODO(), nonExistentSourceID)

			Expect(err).To(HaveOccurred())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})
	})

	Context("ListShareTokens", func() {
		It("successfully lists share tokens for the organization", func() {
			orgID := "test-org-list"
			sourceID1 := uuid.New()
			sourceID2 := uuid.New()
			sourceID3 := uuid.New() // Different org

			// Insert test sources with unique names
			tx := gormdb.Exec(fmt.Sprintf("INSERT INTO sources (id, name, username, org_id, inventory) VALUES ('%s', 'list-source1', 'user1', '%s', '{\"vcenter\":{\"id\":\"test-vcenter\"}}');", sourceID1, orgID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO sources (id, name, username, org_id, inventory) VALUES ('%s', 'list-source2', 'user2', '%s', '{\"vcenter\":{\"id\":\"test-vcenter\"}}');", sourceID2, orgID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO sources (id, name, username, org_id, inventory) VALUES ('%s', 'list-source3', 'user3', 'different-org', '{\"vcenter\":{\"id\":\"test-vcenter\"}}');", sourceID3))
			Expect(tx.Error).To(BeNil())

			// Insert share tokens
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, "token-1", sourceID1))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, "token-2", sourceID2))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, "token-3", sourceID3))
			Expect(tx.Error).To(BeNil())

			shareTokenService := service.NewShareTokenService(s)
			tokens, err := shareTokenService.ListShareTokens(context.TODO(), orgID)

			Expect(err).To(BeNil())
			Expect(len(tokens)).To(Equal(2))

			// Verify only tokens for the specified organization are returned
			foundTokens := make(map[string]uuid.UUID)
			for _, token := range tokens {
				foundTokens[token.Token] = token.SourceID
			}
			Expect(foundTokens["token-1"]).To(Equal(sourceID1))
			Expect(foundTokens["token-2"]).To(Equal(sourceID2))
			_, found := foundTokens["token-3"]
			Expect(found).To(BeFalse()) // Should not include token-3 from different org
		})

		It("returns empty list for organization with no share tokens", func() {
			shareTokenService := service.NewShareTokenService(s)
			tokens, err := shareTokenService.ListShareTokens(context.TODO(), "empty-org")

			Expect(err).To(BeNil())
			Expect(len(tokens)).To(Equal(0))
		})
	})

	AfterEach(func() {
		gormdb.Exec("DELETE FROM share_tokens;")
		gormdb.Exec("DELETE FROM sources;")
	})
})
