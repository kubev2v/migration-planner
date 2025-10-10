package store_test

import (
	"context"
	"sync"
	"time"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

var _ = Describe("ZedTokenStore", Ordered, func() {
	var (
		gormDB     *gorm.DB
		tokenStore *store.ZedTokenStore
	)

	BeforeAll(func() {
		var err error
		ctx := context.Background()
		cfg, err := config.New()
		Expect(err).To(BeNil())

		gormDB, err = store.InitDB(cfg)
		Expect(err).To(BeNil())

		var count int64
		err = gormDB.WithContext(ctx).Raw("SELECT COUNT(*) FROM zed_token").Scan(&count).Error
		Expect(err).To(BeNil())

		if count == 0 {
			err = gormDB.WithContext(ctx).Exec("INSERT INTO zed_token (id, token) VALUES (1, '')").Error
			Expect(err).To(BeNil())
		} else if count > 1 {
			err = gormDB.WithContext(ctx).Exec("DELETE FROM zed_token").Error
			Expect(err).To(BeNil())
			err = gormDB.WithContext(ctx).Exec("INSERT INTO zed_token (id, token) VALUES (1, '')").Error
			Expect(err).To(BeNil())
		}

		tokenStore = store.NewZedTokenStore(gormDB)
		Expect(tokenStore).ToNot(BeNil())
	})

	BeforeEach(func() {
		ctx := context.Background()
		err := gormDB.WithContext(ctx).Exec("UPDATE zed_token SET token = '' WHERE id = 1").Error
		Expect(err).To(BeNil())
	})

	Context("Read Operations", func() {
		It("should read empty token successfully", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			token, err := tokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(token).To(BeNil())
		})

		It("should read existing token successfully", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			testToken := "test-token-123"

			err := gormDB.Exec("UPDATE zed_token SET token = ? WHERE id = 1", testToken).Error
			Expect(err).To(BeNil())

			token, err := tokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(token).ToNot(BeNil())
			Expect(*token).To(Equal(testToken))
		})
	})

	Context("Write Operations", func() {
		It("should write token successfully", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			testToken := "write-test-token-456"

			err := tokenStore.Write(ctx, testToken)
			Expect(err).To(BeNil())

			var storedToken string
			err = gormDB.Raw("SELECT token FROM zed_token LIMIT 1").Scan(&storedToken).Error
			Expect(err).To(BeNil())
			Expect(storedToken).To(Equal(testToken))
		})

		It("should overwrite existing token", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			initialToken := "initial-token"
			newToken := "updated-token"

			err := tokenStore.Write(ctx, initialToken)
			Expect(err).To(BeNil())

			err = tokenStore.Write(ctx, newToken)
			Expect(err).To(BeNil())

			var storedToken string
			err = gormDB.Raw("SELECT token FROM zed_token LIMIT 1").Scan(&storedToken).Error
			Expect(err).To(BeNil())
			Expect(storedToken).To(Equal(newToken))
		})

		It("should handle empty token write", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := tokenStore.Write(ctx, "")
			Expect(err).To(BeNil())

			var storedToken string
			err = gormDB.Raw("SELECT token FROM zed_token LIMIT 1").Scan(&storedToken).Error
			Expect(err).To(BeNil())
			Expect(storedToken).To(Equal(""))
		})

		It("should handle context cancellation during write", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()

			err := tokenStore.Write(ctx, "test-token")
			_ = err
		})
	})

	Context("Concurrent Operations", func() {
		It("should handle concurrent reads", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			testToken := "concurrent-read-token"

			err := tokenStore.Write(ctx, testToken)
			Expect(err).To(BeNil())

			var wg sync.WaitGroup
			results := make([]string, 5)
			errors := make([]error, 5)

			for i := range 5 {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					token, err := tokenStore.Read(ctx)
					errors[index] = err
					if err == nil && token != nil {
						results[index] = *token
					}
				}(i)
			}

			wg.Wait()

			for i := range 5 {
				Expect(errors[i]).To(BeNil())
				Expect(results[i]).To(Equal(testToken))
			}
		})

		It("should serialize concurrent writes", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			var wg sync.WaitGroup
			tokens := []string{"token1", "token2", "token3", "token4", "token5"}
			errors := make([]error, len(tokens))

			for i, token := range tokens {
				wg.Add(1)
				go func(index int, t string) {
					defer wg.Done()
					errors[index] = tokenStore.Write(ctx, t)
				}(i, token)
			}

			wg.Wait()

			for i := 0; i < len(tokens); i++ {
				Expect(errors[i]).To(BeNil())
			}

			finalToken, err := tokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(tokens).To(ContainElement(*finalToken))
		})

		It("should handle mixed concurrent reads and writes", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			initialToken := "initial"
			newToken := "updated"

			err := tokenStore.Write(ctx, initialToken)
			Expect(err).To(BeNil())

			var wg sync.WaitGroup
			readResults := make([]string, 3)
			readErrors := make([]error, 3)
			writeErrors := make([]error, 2)

			for i := 0; i < 3; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					time.Sleep(time.Duration(index*10) * time.Millisecond)
					token, err := tokenStore.Read(ctx)
					readErrors[index] = err
					if err == nil && token != nil {
						readResults[index] = *token
					}
				}(i)
			}

			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					time.Sleep(time.Duration(index*15) * time.Millisecond)
					writeErrors[index] = tokenStore.Write(ctx, newToken)
				}(i)
			}

			wg.Wait()

			for i := 0; i < 3; i++ {
				Expect(readErrors[i]).To(BeNil())
			}
			for i := 0; i < 2; i++ {
				Expect(writeErrors[i]).To(BeNil())
			}

			validTokens := []string{initialToken, newToken}
			for i := 0; i < 3; i++ {
				Expect(validTokens).To(ContainElement(readResults[i]))
			}
		})
	})

	Context("Lock Consistency", func() {
		It("should ensure read-write consistency", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			token1 := "consistency-test-1"
			token2 := "consistency-test-2"

			err := tokenStore.Write(ctx, token1)
			Expect(err).To(BeNil())

			readToken, err := tokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(*readToken).To(Equal(token1))

			err = tokenStore.Write(ctx, token2)
			Expect(err).To(BeNil())

			readToken, err = tokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(*readToken).To(Equal(token2))
		})

		It("should handle rapid read-write cycles", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			baseToken := "rapid-test"

			for i := 0; i < 10; i++ {
				token := baseToken + "-" + string(rune('0'+i))

				err := tokenStore.Write(ctx, token)
				Expect(err).To(BeNil())

				readToken, err := tokenStore.Read(ctx)
				Expect(err).To(BeNil())
				Expect(*readToken).To(Equal(token))
			}
		})
	})
})
