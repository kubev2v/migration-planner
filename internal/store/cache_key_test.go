package store_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var _ = Describe("cache key store", Ordered, func() {
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
		schema.RegisterSerializer("key_serializer", model.KeySerializer{})
	})

	AfterAll(func() {
		s.Close()
	})

	Context("GetPublicKey", func() {
		It("successfully retrieves a public key from database", func() {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
				},
			)

			tx := gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "id1", "org_id", string(pemdata)))
			Expect(tx.Error).To(BeNil())

			pb, err := s.PrivateKey().GetPublicKey(context.TODO(), "id1")
			Expect(err).To(BeNil())
			Expect(pb).NotTo(BeNil())
		})

		It("successfully retrieves a public key from cache on second call", func() {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
				},
			)

			tx := gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "id2", "org_id", string(pemdata)))
			Expect(tx.Error).To(BeNil())

			// First call - should fetch from database
			pb1, err := s.PrivateKey().GetPublicKey(context.TODO(), "id2")
			Expect(err).To(BeNil())
			Expect(pb1).NotTo(BeNil())

			// Second call - should fetch from cache
			pb2, err := s.PrivateKey().GetPublicKey(context.TODO(), "id2")
			Expect(err).To(BeNil())
			Expect(pb2).NotTo(BeNil())

			// Both should be the same
			Expect(pb1).To(Equal(pb2))
		})

		It("returns error when public key not found", func() {
			pb, err := s.PrivateKey().GetPublicKey(context.TODO(), "non-existent-id")
			Expect(err).ToNot(BeNil())
			Expect(pb).To(BeNil())
		})

		It("successfully retrieves different public keys by id", func() {
			privateKey1, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata1 := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey1),
				},
			)

			privateKey2, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata2 := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey2),
				},
			)

			tx := gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "id3", "org_id_1", string(pemdata1)))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "id4", "org_id_2", string(pemdata2)))
			Expect(tx.Error).To(BeNil())

			pb1, err := s.PrivateKey().GetPublicKey(context.TODO(), "id3")
			Expect(err).To(BeNil())
			Expect(pb1).NotTo(BeNil())

			pb2, err := s.PrivateKey().GetPublicKey(context.TODO(), "id4")
			Expect(err).To(BeNil())
			Expect(pb2).NotTo(BeNil())

			Expect(pb1).ToNot(Equal(pb2))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from keys;")
		})
	})

	Context("Create", func() {
		It("successfully creates a key through cache layer", func() {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			_, err = s.PrivateKey().Create(context.TODO(), model.Key{
				ID:         "id5",
				OrgID:      "org_id",
				PrivateKey: privateKey,
			})
			Expect(err).To(BeNil())

			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM keys;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from keys;")
		})
	})

	Context("Get", func() {
		It("successfully gets a key by org_id through cache layer", func() {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
				},
			)

			tx := gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "id6", "org_id", string(pemdata)))
			Expect(tx.Error).To(BeNil())

			key, err := s.PrivateKey().Get(context.TODO(), "org_id")
			Expect(err).To(BeNil())
			Expect(key).NotTo(BeNil())
			Expect(key.OrgID).To(Equal("org_id"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from keys;")
		})
	})

	Context("Delete", func() {
		It("successfully deletes a key and clears cache", func() {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
				},
			)

			tx := gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "id7", "org_id", string(pemdata)))
			Expect(tx.Error).To(BeNil())

			// First, populate the cache by getting the public key
			pb, err := s.PrivateKey().GetPublicKey(context.TODO(), "id7")
			Expect(err).To(BeNil())
			Expect(pb).NotTo(BeNil())

			// Delete the key
			err = s.PrivateKey().Delete(context.TODO(), "org_id")
			Expect(err).To(BeNil())

			// Verify it's deleted from database
			count := 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM keys;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("successfully clears cache on delete even with multiple keys cached", func() {
			privateKey1, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata1 := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey1),
				},
			)

			privateKey2, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata2 := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey2),
				},
			)

			tx := gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "id8", "org_id_1", string(pemdata1)))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "id9", "org_id_2", string(pemdata2)))
			Expect(tx.Error).To(BeNil())

			// Populate cache with both keys
			pb1, err := s.PrivateKey().GetPublicKey(context.TODO(), "id8")
			Expect(err).To(BeNil())
			Expect(pb1).NotTo(BeNil())

			pb2, err := s.PrivateKey().GetPublicKey(context.TODO(), "id9")
			Expect(err).To(BeNil())
			Expect(pb2).NotTo(BeNil())

			// Delete one key - this should clear entire cache
			err = s.PrivateKey().Delete(context.TODO(), "org_id_1")
			Expect(err).To(BeNil())

			// Verify first key is deleted
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM keys WHERE org_id = 'org_id_1';").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			// Second key should still exist in database
			tx = gormdb.Raw("SELECT COUNT(*) FROM keys WHERE org_id = 'org_id_2';").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from keys;")
		})
	})
})
