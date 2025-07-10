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
	insertShareTokenStm = "INSERT INTO share_tokens (id, token, source_id, org_id) VALUES ('%s', '%s', '%s', '%s');"
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

			shareToken := model.NewShareToken(sourceID, "test-org", "test-token-123")
			
			result, err := s.ShareToken().Create(context.TODO(), shareToken)
			Expect(err).To(BeNil())
			Expect(result).NotTo(BeNil())
			Expect(result.Token).To(Equal("test-token-123"))
			Expect(result.SourceID).To(Equal(sourceID))
			Expect(result.OrgID).To(Equal("test-org"))
		})

		It("fails to create share token with non-existent source", func() {
			nonExistentSourceID := uuid.New()
			shareToken := model.NewShareToken(nonExistentSourceID, "test-org", "test-token-456")
			
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
			shareToken1 := model.NewShareToken(sourceID, "test-org", "duplicate-token")
			result1, err := s.ShareToken().Create(context.TODO(), shareToken1)
			Expect(err).To(BeNil())
			Expect(result1).NotTo(BeNil())

			// Try to create second share token with same token
			sourceID2 := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID2, "test-source-3", "test-user", "test-org"))
			Expect(tx.Error).To(BeNil())

			shareToken2 := model.NewShareToken(sourceID2, "test-org", "duplicate-token")
			result2, err := s.ShareToken().Create(context.TODO(), shareToken2)
			Expect(err).NotTo(BeNil())
			Expect(result2).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("GetByToken", func() {
		It("successfully gets share token by token", func() {
			sourceID := uuid.New()
			shareTokenID := uuid.New()
			
			// Create source and share token
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "test-user", "test-org"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, shareTokenID, "find-me-token", sourceID, "test-org"))
			Expect(tx.Error).To(BeNil())

			result, err := s.ShareToken().GetByToken(context.TODO(), "find-me-token")
			Expect(err).To(BeNil())
			Expect(result).NotTo(BeNil())
			Expect(result.Token).To(Equal("find-me-token"))
			Expect(result.SourceID).To(Equal(sourceID))
			Expect(result.OrgID).To(Equal("test-org"))
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
			shareTokenID := uuid.New()
			
			// Create source and share token
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "test-user", "test-org"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, shareTokenID, "source-token", sourceID, "test-org"))
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
			shareTokenID := uuid.New()
			
			// Create source and share token
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source", "test-user", "test-org"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, shareTokenID, "delete-me-token", sourceID, "test-org"))
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
			shareTokenID := uuid.New()
			
			// Create source and share token
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "test-source-name", "test-user", "test-org"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, shareTokenID, "source-lookup-token", sourceID, "test-org"))
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
}) 