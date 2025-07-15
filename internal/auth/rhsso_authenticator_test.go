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
			Expect(user.EmailDomain).To(Equal("gothamcity.com"))
		})

		// FIXME: enable when token validation enabled again
		PIt("fails to authenticate -- wrong signing method", func() {
			sToken, keyFn := generateInvalidTokenWrongSigningMethod()
			authenticator, err := auth.NewRHSSOAuthenticatorWithKeyFn(keyFn)
			Expect(err).To(BeNil())

			_, err = authenticator.Authenticate(sToken)
			Expect(err).ToNot(BeNil())
		})

		It("successfully validate the token -- no orgID", func() {
			sToken, keyFn := generateCustomToken("user@company.com", "", "user@company.com")
			authenticator, err := auth.NewRHSSOAuthenticatorWithKeyFn(keyFn)
			Expect(err).To(BeNil())

			user, err := authenticator.Authenticate(sToken)
			Expect(err).To(BeNil())
			Expect(user.Username).To(Equal("user@company.com"))
			Expect(user.Organization).To(Equal("company.com"))
		})

		It("failed validate the token -- email is missing", func() {
			sToken, keyFn := generateCustomToken("user", "", "")
			authenticator, err := auth.NewRHSSOAuthenticatorWithKeyFn(keyFn)
			Expect(err).To(BeNil())

			_, err = authenticator.Authenticate(sToken)
			Expect(err).ToNot(BeNil())
		})

		It("failed to set email domain -- email is malformated", func() {
			sToken, keyFn := generateCustomToken("user", "", "some-email")
			authenticator, err := auth.NewRHSSOAuthenticatorWithKeyFn(keyFn)
			Expect(err).To(BeNil())

			user, err := authenticator.Authenticate(sToken)
			Expect(err).ToNot(BeNil())
			Expect(user.EmailDomain).To(BeEmpty())
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
			req.Header.Add("X-Authorization", fmt.Sprintf("Bearer %s", sToken))

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
			req.Header.Add("X-Authorization", fmt.Sprintf("Bearer %s", sToken))

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
		Username string `json:"username"`
		OrgID    string `json:"org_id"`
		Email    string `json:"email"`
		jwt.RegisteredClaims
	}

	// Create claims with multiple fields populated
	claims := TokenClaims{
		"batman",
		"GothamCity",
		"batman@gothamcity.com",
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

// func generateInvalidValidToken(missingClaim string) (string, func(t *jwt.Token) (any, error)) {
// 	type TokenClaims struct {
// 		Username string `json:"preferred_username"`
// 		OrgID    string `json:"org_id"`
// 		jwt.RegisteredClaims
// 	}

// 	registedClaims := jwt.RegisteredClaims{
// 		ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
// 		IssuedAt:  jwt.NewNumericDate(time.Now()),
// 		NotBefore: jwt.NewNumericDate(time.Now()),
// 		Issuer:    "test",
// 		Subject:   "somebody",
// 		ID:        "1",
// 		Audience:  []string{"somebody_else"},
// 	}

// 	switch missingClaim {
// 	case "exp_at":
// 		registedClaims.ExpiresAt = nil
// 	}

// 	// Create claims with multiple fields populated
// 	claims := TokenClaims{
// 		"batman",
// 		"GothamCity",
// 		registedClaims,
// 	}

// 	// generate a pair of keys RSA
// 	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
// 	Expect(err).To(BeNil())

// 	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
// 	ss, err := token.SignedString(privateKey)
// 	Expect(err).To(BeNil())

// 	return ss, func(t *jwt.Token) (any, error) {
// 		return privateKey.Public(), nil
// 	}
// }

func generateInvalidTokenWrongSigningMethod() (string, func(t *jwt.Token) (any, error)) {
	type TokenClaims struct {
		Username string `json:"username"`
		OrgID    string `json:"org_id"`
		Email    string `json:"email"`
		jwt.RegisteredClaims
	}

	// Create claims with multiple fields populated
	claims := TokenClaims{
		"batman",
		"GothamCity",
		"batman@gothamcity.com",
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

func generateCustomToken(username string, orgID string, email string) (string, func(t *jwt.Token) (any, error)) {
	type TokenClaims struct {
		Username string `json:"username"`
		OrgID    string `json:"org_id,omitempty"`
		Email    string `json:"email,omitempty"`
		jwt.RegisteredClaims
	}

	// Create claims with multiple fields populated
	claims := TokenClaims{
		username,
		orgID,
		email,
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
