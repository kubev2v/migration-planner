package image

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
)

const (
	// ImageExpirationTime define the expiration of the image download URL
	ImageExpirationTime = 4 * time.Hour
)

// HMACKey generates a hex string representing n random bytes
//
// This string is intended to be used as a private key for signing and
// verifying jwt tokens. Specifically ones used for downloading images
// when using rhsso auth and the image service.
func HMACKey(n int) (string, error) {
	buf := make([]byte, n)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}

func GenerateDownloadURLByToken(baseURL, id, privateKey, name string) (string, *strfmt.DateTime, error) {
	token, err := JWTForSymmetricKey([]byte(privateKey), ImageExpirationTime, id)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to sign image URL")
	}

	exp, err := ParseExpiration(token)
	if err != nil {
		return "", nil, err
	}

	path := fmt.Sprintf("%s/%s/%s.ova", "/api/v1/image/bytoken/", token, name)
	shortURL, err := buildURL(baseURL, path, false, map[string]string{})
	if err != nil {
		return "", nil, err
	}
	return shortURL, exp, err
}

func JWTForSymmetricKey(key []byte, expiration time.Duration, sub string) (string, error) {
	exp := time.Now().Add(expiration).Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": exp,
		"sub": sub,
	})

	return token.SignedString(key)
}

func ParseExpiration(tokenString string) (*strfmt.DateTime, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.Errorf("malformed token claims in url")
	}
	exp, ok := claims["exp"].(float64)
	if !ok {
		return nil, errors.Errorf("token missing 'exp' claim")
	}
	expTime := time.Unix(int64(exp), 0)
	expiresAt := strfmt.DateTime(expTime)

	return &expiresAt, nil
}

func buildURL(baseURL string, suffix string, insecure bool, params map[string]string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse image service base URL")
	}
	downloadURL := url.URL{
		Scheme: base.Scheme,
		Host:   base.Host,
		Path:   path.Join(base.Path, suffix),
	}
	queryValues := url.Values{}
	for k, v := range params {
		if v != "" {
			queryValues.Set(k, v)
		}
	}
	downloadURL.RawQuery = queryValues.Encode()
	if insecure {
		downloadURL.Scheme = "http"
	}
	return downloadURL.String(), nil
}

// IsTokenNearExpiry parses a JWT without verification and checks if it expires within provided days.
func IsTokenNearExpiry(tokenStr string, days int) bool {
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return true // can't parse → treat as expired
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return true
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return true
	}
	return time.Until(exp.Time) < time.Duration(days)*24*time.Hour
}
