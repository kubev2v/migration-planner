package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertPrivateKeyStm = "INSERT INTO keys (id, org_id, private_key) VALUES ('%s', '%s', '%s');"
)

var _ = Describe("agent authentication", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
	)

	BeforeEach(func() {
		cfg, err := config.NewDefault()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
	})

	AfterEach(func() {
		s.Close()
	})

	Context("authenticator", func() {
		It("successfully authenticate agent", func() {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
				},
			)

			tx := gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "kid", "GothamCity", string(pemdata)))
			Expect(tx.Error).To(BeNil())

			token := generateAgentToken("kid", "my_source", "GothamCity", privateKey)

			agentAuthenticator := auth.NewAgentAuthenticator(s)

			agentJwt, err := agentAuthenticator.Authenticate(token)
			Expect(err).To(BeNil())
			Expect(agentJwt.OrgID).To(Equal("GothamCity"))
			Expect(agentJwt.SourceID).To(Equal("my_source"))
			Expect(agentJwt.Issuer).To(Equal("test"))

			Expect(agentJwt.ExpireAt.After(time.Now())).To(BeTrue())
		})

		It("failed to authenticate agent -- public key is missing", func() {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
				},
			)

			tx := gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "kid", "GothamCity", string(pemdata)))
			Expect(tx.Error).To(BeNil())

			token := generateAgentToken("missing-key-kid", "my_source", "GothamCity", privateKey)

			agentAuthenticator := auth.NewAgentAuthenticator(s)

			_, err = agentAuthenticator.Authenticate(token)
			Expect(err).ToNot(BeNil())
		})

		It("failed to authenticate agent -- wrong public key", func() {
			signingKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			keyUsedToVerify, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(keyUsedToVerify),
				},
			)

			tx := gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "kid", "GothamCity", string(pemdata)))
			Expect(tx.Error).To(BeNil())

			token := generateAgentToken("kid", "my_source", "GothamCity", signingKey)

			agentAuthenticator := auth.NewAgentAuthenticator(s)

			_, err = agentAuthenticator.Authenticate(token)
			Expect(err).ToNot(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from keys;")
		})
	})

	Context("authenticator middleware", func() {
		It("successfully authenticate agent", func() {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).To(BeNil())

			pemdata := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
				},
			)

			tx := gormdb.Exec(fmt.Sprintf(insertPrivateKeyStm, "1234_kid", "org_id", string(pemdata)))
			Expect(tx.Error).To(BeNil())

			token := generateAgentToken("1234_kid", "my_source", "org_id", privateKey)

			agentAuthenticator := auth.NewAgentAuthenticator(s)
			h := &handler{}
			ts := httptest.NewServer(agentAuthenticator.Authenticator(h))
			defer ts.Close()

			req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
			Expect(err).To(BeNil())
			req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))

			resp, rerr := http.DefaultClient.Do(req)
			Expect(rerr).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from keys;")
		})
	})

})

func generateAgentToken(kid, sourceID, orgID string, signingKey *rsa.PrivateKey) string {
	type jwtToken struct {
		SourceID string `json:"source_id"`
		jwt.RegisteredClaims
	}

	// Create claims with multiple fields populated
	claims := jwtToken{
		sourceID,
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "test",
			Subject:   orgID,
			ID:        "1",
			Audience:  []string{"somebody_else"},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	ss, err := token.SignedString(signingKey)
	Expect(err).To(BeNil())

	return ss
}
