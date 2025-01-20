package certprovider

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

const (
	RSAPrivateKeyBlockType = "RSA PRIVATE KEY"
	issuer                 = "Red Hat"
)

type SelfSignedCertificateProvider struct {
	org string
}

func NewSelfSignedCertificateProvider(org string) *SelfSignedCertificateProvider {
	return &SelfSignedCertificateProvider{org: org}
}

func (s *SelfSignedCertificateProvider) GetCACertificate(expire time.Time) (*x509.Certificate, *rsa.PrivateKey, error) {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Issuer: pkix.Name{
			Organization: []string{issuer},
		},
		Subject: pkix.Name{
			Organization: []string{s.org},
		},
		NotBefore:             time.Now(),
		NotAfter:              expire,
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// create our private and public key
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, fmt.Errorf("Cannot generate CA Key")
	}

	// create the CA
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, caPrivKey.Public(), caPrivKey)
	if err != nil {
		return nil, nil, err
	}

	caCert, err := x509.ParseCertificate(caBytes)
	if err != nil {
		return nil, nil, err
	}

	return caCert, caPrivKey, nil
}

func (s *SelfSignedCertificateProvider) GetCertificate(caCert *x509.Certificate, caKey *rsa.PrivateKey) (*x509.Certificate, *rsa.PrivateKey, error) {
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			Organization: caCert.Subject.Organization,
		},
		NotBefore:             time.Now(),
		NotAfter:              caCert.NotAfter,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate agent's web server private key: %w", err)
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, certPrivKey.Public(), caKey)
	if err != nil {
		return nil, nil, err
	}

	c, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, nil, err
	}

	return c, certPrivKey, nil
}

func (s *SelfSignedCertificateProvider) ConvertToPEM(cert *x509.Certificate, key *rsa.PrivateKey) ([]byte, []byte) {
	certPEM := new(bytes.Buffer)
	_ = pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})

	privKeyPEM := new(bytes.Buffer)
	_ = pem.Encode(privKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	return certPEM.Bytes(), privKeyPEM.Bytes()
}
