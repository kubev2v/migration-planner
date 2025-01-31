package image

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/golang-jwt/jwt/v4"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/pkg/errors"
)

var jwtPayloadRegexp = regexp.MustCompile(`^.+\.(.+)\..+`)

type payload struct {
	Sub string `json:"sub"` // used by OCM tokens
}

const (
	// ImageExpirationTime define the expiration of the image download URL
	ImageExpirationTime = 4 * time.Hour
)

func GenerateDownloadURLByToken(baseUrl string, source *model.Source) (string, *strfmt.DateTime, error) {
	token, err := JWTForSymmetricKey([]byte(source.ImageTokenKey), ImageExpirationTime, source.ID.String())
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to sign image URL")
	}

	exp, err := ParseExpiration(token)
	if err != nil {
		return "", nil, err
	}

	path := fmt.Sprintf("%s/%s/%s.ova", "/api/v1/image/bytoken/", token, source.Name)
	shortURL, err := buildURL(baseUrl, path, false, map[string]string{})
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

func ValidateToken(ctx context.Context, token string, keyFunc func(token *jwt.Token) (interface{}, error)) error {
	parsedToken, err := jwt.Parse(token, keyFunc)
	if err != nil {
		return fmt.Errorf("unauthorized: %v", err)
	}

	return parsedToken.Claims.Valid()
}

func IdFromJWT(jwt string) (string, error) {
	match := jwtPayloadRegexp.FindStringSubmatch(jwt)

	if len(match) != 2 {
		return "", fmt.Errorf("failed to parse JWT from URL")
	}

	decoded, err := base64.RawStdEncoding.DecodeString(match[1])
	if err != nil {
		return "", err
	}

	var p payload
	err = json.Unmarshal(decoded, &p)
	if err != nil {
		return "", err
	}

	switch {
	case p.Sub != "":
		return p.Sub, nil
	}

	return "", fmt.Errorf("sub ID not found in token")
}
