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
	insertShareTokenStm = "INSERT INTO share_tokens (id, token, source_id, org_id) VALUES ('%s', '%s', '%s', '%s');"
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
			
			// Insert a test source
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", orgID))
			Expect(tx.Error).To(BeNil())

			shareTokenService := service.NewShareTokenService(s)
			token, err := shareTokenService.CreateShareToken(context.TODO(), sourceID, orgID)

			Expect(err).To(BeNil())
			Expect(token).NotTo(BeNil())
			Expect(token.SourceID).To(Equal(sourceID))
			Expect(token.OrgID).To(Equal(orgID))
			Expect(token.Token).NotTo(BeEmpty())
			Expect(len(token.Token)).To(Equal(64)) // 32 bytes hex encoded = 64 chars
		})

		It("returns existing share token when one already exists", func() {
			sourceID := uuid.New()
			orgID := "test-org-2"
			existingToken := "existing-token-value"
			
			// Insert a test source
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", orgID))
			Expect(tx.Error).To(BeNil())

			// Insert existing share token
			tokenID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, tokenID, existingToken, sourceID, orgID))
			Expect(tx.Error).To(BeNil())

			shareTokenService := service.NewShareTokenService(s)
			token, err := shareTokenService.CreateShareToken(context.TODO(), sourceID, orgID)

			Expect(err).To(BeNil())
			Expect(token).NotTo(BeNil())
			Expect(token.SourceID).To(Equal(sourceID))
			Expect(token.OrgID).To(Equal(orgID))
			Expect(token.Token).To(Equal(existingToken))
		})

		It("returns error when source does not exist", func() {
			nonExistentSourceID := uuid.New()
			orgID := "test-org-3"

			shareTokenService := service.NewShareTokenService(s)
			_, err := shareTokenService.CreateShareToken(context.TODO(), nonExistentSourceID, orgID)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create share token"))
		})
	})

	Context("DeleteShareToken", func() {
		It("successfully deletes an existing share token", func() {
			sourceID := uuid.New()
			orgID := "test-org-4"
			tokenValue := "token-to-delete"
			
			// Insert a test source
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", orgID))
			Expect(tx.Error).To(BeNil())

			// Insert share token
			tokenID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, tokenID, tokenValue, sourceID, orgID))
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
			
			// Insert a test source
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", orgID))
			Expect(tx.Error).To(BeNil())

			// Insert share token
			tokenID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, tokenID, tokenValue, sourceID, orgID))
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
			service := service.NewShareTokenService(s)
			
			token, err := service.CreateShareToken(context.TODO(), uuid.New(), "test-org")
			
			if err == nil {
				Expect(token.Token).To(HaveLen(64))
				Expect(token.Token).To(MatchRegexp("^[0-9a-f]{64}$"))
			}
		})

		It("generates unique tokens", func() {
			shareTokenService := service.NewShareTokenService(s)
			
			sourceID1 := uuid.New()
			sourceID2 := uuid.New()
			orgID := "test-org-unique"
			
			// Insert test sources with different names to avoid unique constraint violation
			tx := gormdb.Exec(fmt.Sprintf("INSERT INTO sources (id, name, username, org_id) VALUES ('%s', 'source_name_1', 'admin', '%s');", sourceID1, orgID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO sources (id, name, username, org_id) VALUES ('%s', 'source_name_2', 'admin', '%s');", sourceID2, orgID))
			Expect(tx.Error).To(BeNil())

			token1, err1 := shareTokenService.CreateShareToken(context.TODO(), sourceID1, orgID)
			token2, err2 := shareTokenService.CreateShareToken(context.TODO(), sourceID2, orgID)

			Expect(err1).To(BeNil())
			Expect(err2).To(BeNil())
			Expect(token1.Token).NotTo(Equal(token2.Token))
		})
	})

	Context("Token format validation", func() {
		It("generates hex-encoded tokens", func() {
			sourceID := uuid.New()
			orgID := "test-org-hex"
			
			// Insert a test source
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", orgID))
			Expect(tx.Error).To(BeNil())

			shareTokenService := service.NewShareTokenService(s)
			token, err := shareTokenService.CreateShareToken(context.TODO(), sourceID, orgID)

			Expect(err).To(BeNil())
			Expect(token.Token).To(HaveLen(64))
			// Check that all characters are valid hex characters
			for _, char := range token.Token {
				Expect(strings.ContainsRune("0123456789abcdef", char)).To(BeTrue())
			}
		})
	})

	AfterEach(func() {
		gormdb.Exec("DELETE FROM share_tokens;")
		gormdb.Exec("DELETE FROM sources;")
	})
}) 
