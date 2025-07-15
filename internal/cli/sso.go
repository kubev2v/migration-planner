package cli

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewCmdSSO() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sso private-key|token",
		Short: "Generate either the token or the signing private key",
	}

	cmd.AddCommand(newTokenCmd())
	cmd.AddCommand(newPrivateKeyCmd())

	return cmd
}

type tokenOptions struct {
	PrivateKey   string
	Username     string
	Organization string
}

func (o *tokenOptions) Bind(fs *pflag.FlagSet) {
	fs.StringVarP(&o.PrivateKey, "private-key", "", "", "private key used to sign the token")
	fs.StringVarP(&o.Organization, "org", "", "", "organization name")
	fs.StringVarP(&o.Username, "username", "", "", "username")
}

func newTokenCmd() *cobra.Command {
	o := &tokenOptions{}
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Generate a jwt",
		RunE: func(cmd *cobra.Command, args []string) error {
			privateKey, err := ParsePrivateKey(o.PrivateKey)
			if err != nil {
				return err
			}

			token, err := GenerateToken(o.Username, o.Organization, privateKey)
			if err != nil {
				return err
			}

			fmt.Println(token)
			return nil
		},
	}

	o.Bind(cmd.Flags())
	_ = cmd.MarkFlagRequired("private-key")
	_ = cmd.MarkFlagRequired("username")
	_ = cmd.MarkFlagRequired("org")

	return cmd
}

func newPrivateKeyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "private-key",
		Short: "Generate a private key",
		RunE: func(cmd *cobra.Command, args []string) error {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				return err
			}
			pemdata := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
				},
			)
			fmt.Println(string(pemdata))
			return nil
		},
	}
}

func ParsePrivateKey(content string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(content))
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func GenerateToken(username, organization string, privateKey *rsa.PrivateKey) (string, error) {
	type TokenClaims struct {
		Username string `json:"username"`
		OrgID    string `json:"org_id"`
		jwt.RegisteredClaims
	}

	// Create claims with multiple fields populated
	claims := TokenClaims{
		username,
		organization,
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

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(privateKey)
}
