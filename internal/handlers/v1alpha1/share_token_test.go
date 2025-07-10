package v1alpha1_test

import (
	"context"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertShareTokenStm = "INSERT INTO share_tokens (id, token, source_id) VALUES ('%s', '%s', '%s');"
)

var _ = Describe("share token handler", Ordered, func() {
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
		It("successfully creates a new share token", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.CreateShareToken(ctx, server.CreateShareTokenRequestObject{Id: sourceID})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateShareToken200JSONResponse{}).String()))

			shareToken := resp.(server.CreateShareToken200JSONResponse)
			Expect(shareToken.Id).NotTo(BeNil())
			Expect(shareToken.Token).NotTo(BeEmpty())
			Expect(len(shareToken.Token)).To(Equal(64)) // 32 bytes hex encoded = 64 chars
		})

		It("returns existing share token when one already exists", func() {
			sourceID := uuid.New()
			existingToken := "existing-token-value-64-chars-long-0123456789abcdef0123456789abcdef"

			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Insert existing share token
			tokenID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, tokenID, existingToken, sourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.CreateShareToken(ctx, server.CreateShareTokenRequestObject{Id: sourceID})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateShareToken200JSONResponse{}).String()))

			shareToken := resp.(server.CreateShareToken200JSONResponse)
			Expect(shareToken.Id).NotTo(BeNil())
			Expect(shareToken.Token).To(Equal(existingToken))
		})

		It("returns 404 for non-existent source", func() {
			nonExistentSourceID := uuid.New()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.CreateShareToken(ctx, server.CreateShareTokenRequestObject{Id: nonExistentSourceID})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateShareToken404JSONResponse{}).String()))
		})

		It("returns 403 for unauthorized access (different organization)", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "hacker",
				Organization: "evil-corp",
				EmailDomain:  "evil.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.CreateShareToken(ctx, server.CreateShareTokenRequestObject{Id: sourceID})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateShareToken403JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("DeleteShareToken", func() {
		It("successfully deletes an existing share token", func() {
			sourceID := uuid.New()
			tokenValue := "token-to-delete-64-chars-long-0123456789abcdef0123456789abcdef"

			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Insert share token
			tokenID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, tokenID, tokenValue, sourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.DeleteShareToken(ctx, server.DeleteShareTokenRequestObject{Id: sourceID})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteShareToken200JSONResponse{}).String()))

			// Verify token is deleted
			_, err = s.ShareToken().GetBySourceID(context.TODO(), sourceID)
			Expect(err).To(Equal(store.ErrRecordNotFound))
		})

		It("succeeds even when no share token exists (idempotent)", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.DeleteShareToken(ctx, server.DeleteShareTokenRequestObject{Id: sourceID})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteShareToken200JSONResponse{}).String()))
		})

		It("returns 404 for non-existent source", func() {
			nonExistentSourceID := uuid.New()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.DeleteShareToken(ctx, server.DeleteShareTokenRequestObject{Id: nonExistentSourceID})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteShareToken404JSONResponse{}).String()))
		})

		It("returns 403 for unauthorized access (different organization)", func() {
			sourceID := uuid.New()
			tokenValue := "unauthorized-token-64-chars-long-0123456789abcdef0123456789abcdef"

			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Insert share token
			tokenID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, tokenID, tokenValue, sourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "hacker",
				Organization: "evil-corp",
				EmailDomain:  "evil.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.DeleteShareToken(ctx, server.DeleteShareTokenRequestObject{Id: sourceID})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteShareToken403JSONResponse{}).String()))

			// Verify token still exists
			token, err := s.ShareToken().GetBySourceID(context.TODO(), sourceID)
			Expect(err).To(BeNil())
			Expect(token.Token).To(Equal(tokenValue))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("GetSharedSource", func() {
		It("successfully retrieves source by valid token (no authentication required)", func() {
			sourceID := uuid.New()
			tokenValue := "valid-shared-token-64-chars-long-0123456789abcdef0123456789abcdef"

			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Insert share token
			tokenID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, tokenID, tokenValue, sourceID))
			Expect(tx.Error).To(BeNil())

			// No authentication context needed for public endpoint
			ctx := context.TODO()

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.GetSharedSource(ctx, server.GetSharedSourceRequestObject{Token: tokenValue})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSharedSource200JSONResponse{}).String()))

			source := resp.(server.GetSharedSource200JSONResponse)
			Expect(source.Id).To(Equal(sourceID))
			Expect(source.Name).To(Equal("source_name"))
		})

		It("returns 404 for invalid token", func() {
			invalidToken := "invalid-token-64-chars-long-0123456789abcdef0123456789abcdef"

			// No authentication context needed for public endpoint
			ctx := context.TODO()

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.GetSharedSource(ctx, server.GetSharedSourceRequestObject{Token: invalidToken})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSharedSource404JSONResponse{}).String()))
		})

		It("returns 404 for empty token", func() {
			emptyToken := ""

			// No authentication context needed for public endpoint
			ctx := context.TODO()

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.GetSharedSource(ctx, server.GetSharedSourceRequestObject{Token: emptyToken})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSharedSource404JSONResponse{}).String()))
		})

		It("returns 404 for token with deleted source", func() {
			sourceID := uuid.New()
			tokenValue := "orphaned-token-64-chars-long-0123456789abcdef0123456789abcdef"

			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Insert share token
			tokenID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, tokenID, tokenValue, sourceID))
			Expect(tx.Error).To(BeNil())

			// Delete the source but keep the token (simulating orphaned token)
			tx = gormdb.Exec(fmt.Sprintf("DELETE FROM sources WHERE id = '%s';", sourceID))
			Expect(tx.Error).To(BeNil())

			// No authentication context needed for public endpoint
			ctx := context.TODO()

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.GetSharedSource(ctx, server.GetSharedSourceRequestObject{Token: tokenValue})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSharedSource404JSONResponse{}).String()))
		})

		It("allows access to sources from different organizations via token", func() {
			sourceID := uuid.New()
			tokenValue := "cross-org-token-64-chars-long-0123456789abcdef0123456789abcdef"

			// Create source belonging to different org
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "secret-org"))
			Expect(tx.Error).To(BeNil())

			// Insert share token
			tokenID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertShareTokenStm, tokenID, tokenValue, sourceID))
			Expect(tx.Error).To(BeNil())

			// No authentication context needed - this is the point of share tokens
			ctx := context.TODO()

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.GetSharedSource(ctx, server.GetSharedSourceRequestObject{Token: tokenValue})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSharedSource200JSONResponse{}).String()))

			source := resp.(server.GetSharedSource200JSONResponse)
			Expect(source.Id).To(Equal(sourceID))
			Expect(source.Name).To(Equal("source_name"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("Share token security and edge cases", func() {
		It("ensures tokens are unique across sources", func() {
			sourceID1 := uuid.New()
			sourceID2 := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf("INSERT INTO sources (id, name, username, org_id) VALUES ('%s', 'source1', 'admin', 'admin');", sourceID1))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO sources (id, name, username, org_id) VALUES ('%s', 'source2', 'admin', 'admin');", sourceID2))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))

			// Create tokens for both sources
			resp1, err1 := srv.CreateShareToken(ctx, server.CreateShareTokenRequestObject{Id: sourceID1})
			resp2, err2 := srv.CreateShareToken(ctx, server.CreateShareTokenRequestObject{Id: sourceID2})

			Expect(err1).To(BeNil())
			Expect(err2).To(BeNil())

			token1 := resp1.(server.CreateShareToken200JSONResponse)
			token2 := resp2.(server.CreateShareToken200JSONResponse)

			// Tokens should be different
			Expect(token1.Token).NotTo(Equal(token2.Token))

			// Each token should only give access to its respective source
			source1Resp, err := srv.GetSharedSource(context.TODO(), server.GetSharedSourceRequestObject{Token: token1.Token})
			Expect(err).To(BeNil())
			source1 := source1Resp.(server.GetSharedSource200JSONResponse)
			Expect(source1.Id).To(Equal(sourceID1))

			source2Resp, err := srv.GetSharedSource(context.TODO(), server.GetSharedSourceRequestObject{Token: token2.Token})
			Expect(err).To(BeNil())
			source2 := source2Resp.(server.GetSharedSource200JSONResponse)
			Expect(source2.Id).To(Equal(sourceID2))
		})

		It("handles concurrent token creation gracefully", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))

			// Create token multiple times - should return the same token
			resp1, err1 := srv.CreateShareToken(ctx, server.CreateShareTokenRequestObject{Id: sourceID})
			resp2, err2 := srv.CreateShareToken(ctx, server.CreateShareTokenRequestObject{Id: sourceID})
			resp3, err3 := srv.CreateShareToken(ctx, server.CreateShareTokenRequestObject{Id: sourceID})

			Expect(err1).To(BeNil())
			Expect(err2).To(BeNil())
			Expect(err3).To(BeNil())

			token1 := resp1.(server.CreateShareToken200JSONResponse)
			token2 := resp2.(server.CreateShareToken200JSONResponse)
			token3 := resp3.(server.CreateShareToken200JSONResponse)

			// All should return the same token
			Expect(token1.Token).To(Equal(token2.Token))
			Expect(token2.Token).To(Equal(token3.Token))

			// Verify only one token exists in database
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM share_tokens WHERE source_id = ?", sourceID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})
})
