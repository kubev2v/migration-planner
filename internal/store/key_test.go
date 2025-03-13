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

const (
	insertPrivateKeyStm = "INSERT INTO keys (id, org_id, private_key) VALUES ('%s', '%s', '%s');"
)

var _ = Describe("key store", Ordered, func() {
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

	Context("create", func() {
		It("successfully creates a key", func() {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			_, err = s.PrivateKey().Create(context.TODO(), model.Key{
				ID:         "id",
				OrgID:      "org_id",
				PrivateKey: privateKey,
			})
			Expect(err).To(BeNil())

			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM keys;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			var id string
			tx = gormdb.Raw("SELECT id FROM keys LIMIT 1;").Scan(&id)
			Expect(tx.Error).To(BeNil())
			Expect(id).To(Equal("id"))

			pemdata := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
				},
			)
			var keyContent string
			tx = gormdb.Raw("SELECT private_key FROM keys LIMIT 1;").Scan(&keyContent)
			Expect(tx.Error).To(BeNil())
			Expect(keyContent).To(Equal(string(pemdata)))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from keys;")
		})
	})

	Context("get", func() {
		It("successfully gets a get by org_id", func() {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
				},
			)

			tx := gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "id", "org_id", string(pemdata)))
			Expect(tx.Error).To(BeNil())

			key, err := s.PrivateKey().Get(context.TODO(), "org_id")
			Expect(err).To(BeNil())

			pemdata1 := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(key.PrivateKey),
				},
			)
			Expect(string(pemdata1)).To(Equal(string(pemdata)))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from keys;")
		})
	})

	Context("delete", func() {
		It("successfully gets a get by org_id", func() {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
				},
			)

			tx := gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "id", "org_id", string(pemdata)))
			Expect(tx.Error).To(BeNil())

			err = s.PrivateKey().Delete(context.TODO(), "org_id")
			Expect(err).To(BeNil())

			count := 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM keys;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from keys;")
		})
	})

	Context("public keys", func() {
		It("successfully gets a list of public keys", func() {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
				},
			)

			tx := gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "id", "org_id", string(pemdata)))
			Expect(tx.Error).To(BeNil())

			pb, err := s.PrivateKey().GetPublicKeys(context.TODO())
			Expect(err).To(BeNil())

			Expect(pb).To(HaveLen(1))
			Expect(pb["id"]).To(Not(BeNil()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from keys;")
		})
	})
})
