package agent_test

import (
	"crypto/x509"
	"net"
	"time"

	"github.com/kubev2v/migration-planner/internal/agent"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Certification Provider", func() {
	Context("self signed certificate", func() {
		It("generates successfully -- ip 1.2.3.4", func() {
			certAddr, err := net.ResolveTCPAddr("tcp", "1.2.3.4:3333")
			Expect(err).To(BeNil())

			cp := agent.NewSelfSignedCertificateProvider(certAddr)
			cert, key, err := cp.GetCertificate(time.Now().Add(10 * time.Second))
			Expect(err).To(BeNil())
			Expect(key).ToNot(BeNil())

			data := x509.MarshalPKCS1PrivateKey(key)
			Expect(len(data) > 0).To(BeTrue())

			Expect(cert.IPAddresses).To(HaveLen(1))
			ip := cert.IPAddresses[0]
			Expect(ip.String()).To(Equal(net.ParseIP("1.2.3.4").String()))
			Expect(cert.Issuer.Organization).Should(ContainElement("Red Hat"))
			Expect(cert.Issuer.OrganizationalUnit).Should(ContainElement("Assisted Migrations"))
		})

		It("generates successfully -- credsURL invalid", func() {
			cp := agent.NewSelfSignedCertificateProvider(nil)
			cert, key, err := cp.GetCertificate(time.Now().Add(10 * time.Second))
			Expect(err).To(BeNil())
			Expect(key).ToNot(BeNil())

			data := x509.MarshalPKCS1PrivateKey(key)
			Expect(len(data) > 0).To(BeTrue())

			Expect(cert.IPAddresses).To(HaveLen(1))
			ip := cert.IPAddresses[0]
			Expect(ip.String()).To(Equal(net.ParseIP("0.0.0.0").String()))
			Expect(cert.Issuer.Organization).Should(ContainElement("Red Hat"))
			Expect(cert.Issuer.OrganizationalUnit).Should(ContainElement("Assisted Migrations"))
		})
	})
})
