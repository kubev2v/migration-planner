package agent

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"time"
)

type SelfSignedCertificateProvider struct {
	ip net.IP
}

func NewSelfSignedCertificateProvider(credentialAddr *net.TCPAddr) *SelfSignedCertificateProvider {
	ip := net.IPv4(0, 0, 0, 0)
	if credentialAddr != nil {
		ip = credentialAddr.IP
	}

	return &SelfSignedCertificateProvider{ip: ip}
}

func (s *SelfSignedCertificateProvider) GetCertificate(expire time.Time) (*x509.Certificate, *rsa.PrivateKey, error) {
	csr := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Issuer: pkix.Name{
			Organization: []string{"Red Hat"},
		},
		// TODO Verify if these values are OK.
		Subject: pkix.Name{
			Country:            []string{"US"},
			Organization:       []string{"Red Hat"},
			OrganizationalUnit: []string{"Assisted Migrations"},
		},
		IPAddresses:           []net.IP{s.ip},
		NotBefore:             time.Now(),
		NotAfter:              expire,
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate rsa private key")
	}

	certData, err := x509.CreateCertificate(rand.Reader, csr, csr, privateKey.Public(), privateKey)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(certData)
	if err != nil {
		return nil, nil, err
	}

	return cert, privateKey, nil
}
