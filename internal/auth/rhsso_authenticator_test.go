package auth_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kubev2v/migration-planner/internal/auth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("sso authentication", func() {
	Context("rh authentication", func() {
		It("successfully validate the token", func() {
			sToken, keyFn := generateValidToken()
			authenticator, err := auth.NewRHSSOAuthenticatorWithKeyFn(keyFn)
			Expect(err).To(BeNil())

			user, err := authenticator.Authenticate(sToken)
			Expect(err).To(BeNil())
			Expect(user.Username).To(Equal("batman"))
			Expect(user.Organization).To(Equal("GothamCity"))
		})

		It("fails to authenticate -- wrong signing method", func() {
			sToken, keyFn := generateInvalidTokenWrongSigningMethod()
			authenticator, err := auth.NewRHSSOAuthenticatorWithKeyFn(keyFn)
			Expect(err).To(BeNil())

			_, err = authenticator.Authenticate(sToken)
			Expect(err).ToNot(BeNil())
		})

		It("fails to authenticate -- issueAt claims is missing", func() {
			sToken, keyFn := generateInvalidValidToken("exp_at")
			authenticator, err := auth.NewRHSSOAuthenticatorWithKeyFn(keyFn)
			Expect(err).To(BeNil())

			_, err = authenticator.Authenticate(sToken)
			Expect(err).ToNot(BeNil())
		})

		It("successfully validate the token -- no orgID", func() {
			sToken, keyFn := generateCustomToken("user@company.com", nil)
			authenticator, err := auth.NewRHSSOAuthenticatorWithKeyFn(keyFn)
			Expect(err).To(BeNil())

			user, err := authenticator.Authenticate(sToken)
			Expect(err).To(BeNil())
			Expect(user.Username).To(Equal("user@company.com"))
			Expect(user.Organization).To(Equal("company.com"))
		})

		It("failed validate the token -- username malformatted", func() {
			sToken, keyFn := generateCustomToken("user@", nil)
			authenticator, err := auth.NewRHSSOAuthenticatorWithKeyFn(keyFn)
			Expect(err).To(BeNil())

			_, err = authenticator.Authenticate(sToken)
			Expect(err).ToNot(BeNil())
		})

		It("successfully validate the token -- username malformatted", func() {
			sToken, keyFn := generateCustomToken("@user", nil)
			authenticator, err := auth.NewRHSSOAuthenticatorWithKeyFn(keyFn)
			Expect(err).To(BeNil())

			user, err := authenticator.Authenticate(sToken)
			Expect(err).To(BeNil())
			Expect(user.Organization).To(Equal("user"))
		})
	})
	Context("rh auth middleware", func() {
		It("successfully authenticate", func() {
			sToken, keyFn := generateValidToken()
			authenticator, err := auth.NewRHSSOAuthenticatorWithKeyFn(keyFn)
			Expect(err).To(BeNil())

			h := &handler{}
			ts := httptest.NewServer(authenticator.Authenticator(h))
			defer ts.Close()

			req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
			Expect(err).To(BeNil())
			req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", sToken))

			resp, rerr := http.DefaultClient.Do(req)
			Expect(rerr).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))
		})

		It("failed to authenticate", func() {
			sToken, keyFn := generateInvalidTokenWrongSigningMethod()
			authenticator, err := auth.NewRHSSOAuthenticatorWithKeyFn(keyFn)
			Expect(err).To(BeNil())

			h := &handler{}
			ts := httptest.NewServer(authenticator.Authenticator(h))
			defer ts.Close()

			req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
			Expect(err).To(BeNil())
			req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", sToken))

			resp, rerr := http.DefaultClient.Do(req)
			Expect(rerr).To(BeNil())
			Expect(resp.StatusCode).To(Equal(401))
		})
	})
})

type handler struct{}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
}

func generateValidToken() (string, func(t *jwt.Token) (any, error)) {
	type TokenClaims struct {
		Username string `json:"preffered_username"`
		OrgID    string `json:"org_id"`
		jwt.RegisteredClaims
	}

	// Create claims with multiple fields populated
	claims := TokenClaims{
		"batman",
		"GothamCity",
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "test",
			Subject:   "somebody",
			ID:        "1",
			Audience:  []string{"somebody_else"},
		},
	}

	// generate a pair of keys RSA
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).To(BeNil())

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	ss, err := token.SignedString(privateKey)
	Expect(err).To(BeNil())

	return ss, func(t *jwt.Token) (any, error) {
		return privateKey.Public(), nil
	}
}

func generateInvalidValidToken(missingClaim string) (string, func(t *jwt.Token) (any, error)) {
	type TokenClaims struct {
		Username string `json:"preffered_username"`
		OrgID    string `json:"org_id"`
		jwt.RegisteredClaims
	}

	registedClaims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		NotBefore: jwt.NewNumericDate(time.Now()),
		Issuer:    "test",
		Subject:   "somebody",
		ID:        "1",
		Audience:  []string{"somebody_else"},
	}

	switch missingClaim {
	case "exp_at":
		registedClaims.ExpiresAt = nil
	}

	// Create claims with multiple fields populated
	claims := TokenClaims{
		"batman",
		"GothamCity",
		registedClaims,
	}

	// generate a pair of keys RSA
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).To(BeNil())

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	ss, err := token.SignedString(privateKey)
	Expect(err).To(BeNil())

	return ss, func(t *jwt.Token) (any, error) {
		return privateKey.Public(), nil
	}
}

func generateInvalidTokenWrongSigningMethod() (string, func(t *jwt.Token) (any, error)) {
	type TokenClaims struct {
		Username string `json:"preffered_username"`
		OrgID    string `json:"org_id"`
		jwt.RegisteredClaims
	}

	// Create claims with multiple fields populated
	claims := TokenClaims{
		"batman",
		"GothamCity",
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "test",
			Subject:   "somebody",
			ID:        "1",
			Audience:  []string{"somebody_else"},
		},
	}

	// generate a pair of keys ecdsa
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).To(BeNil())

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	ss, err := token.SignedString(privateKey)
	Expect(err).To(BeNil())

	return ss, func(t *jwt.Token) (any, error) {
		return privateKey.Public(), nil
	}
}

func generateCustomToken(username string, orgID *string) (string, func(t *jwt.Token) (any, error)) {
	type TokenClaims struct {
		Username string `json:"preffered_username"`
		OrgID    string `json:"org_id,omitempty"`
		jwt.RegisteredClaims
	}

	o := ""
	if orgID != nil {
		o = *orgID
	}

	// Create claims with multiple fields populated
	claims := TokenClaims{
		username,
		o,
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "test",
			Subject:   "somebody",
			ID:        "1",
			Audience:  []string{"somebody_else"},
		},
	}

	// generate a pair of keys RSA
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).To(BeNil())

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	ss, err := token.SignedString(privateKey)
	Expect(err).To(BeNil())

	return ss, func(t *jwt.Token) (any, error) {
		return privateKey.Public(), nil
	}
}
