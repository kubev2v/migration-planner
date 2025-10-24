package v1alpha1_test

import (
	"context"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertAgentStm              = "INSERT INTO agents (id, status, status_info, cred_url,source_id, version) VALUES ('%s', '%s', '%s', '%s', '%s', 'version_1');"
	insertSourceWithUsernameStm = "INSERT INTO sources (id, name, username, org_id) VALUES ('%s', 'source_name', '%s', '%s');"
	insertSourceOnPremisesStm   = "INSERT INTO sources (id, name, username, org_id, on_premises) VALUES ('%s', '%s', '%s', '%s', TRUE);"
	insertLabelStm              = "INSERT INTO labels (key, value, source_id) VALUES ('%s', '%s', '%s');"
	validCert                   = `
-----BEGIN CERTIFICATE-----
MIIDiTCCAnGgAwIBAgIUBvDjZ2irE/zWyKQxxRnPi3Ap5OowDQYJKoZIhvcNAQEL
BQAwVDELMAkGA1UEBhMCVVMxFDASBgNVBAcMC0dvdGhhbSBDaXR5MRowGAYDVQQK
DBFXYXluZSBFbnRlcnByaXNlczETMBEGA1UEAwwKYmF0bWFuLmNvbTAeFw0yNTA2
MTIxMjU1MzNaFw0yNjAzMDQxMjU1MzNaMFQxCzAJBgNVBAYTAlVTMRQwEgYDVQQH
DAtHb3RoYW0gQ2l0eTEaMBgGA1UECgwRV2F5bmUgRW50ZXJwcmlzZXMxEzARBgNV
BAMMCmJhdG1hbi5jb20wggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDk
w2USSDum5VXDR7uq3S7y2P+DJYB4k2cNcCJKkE0vSj6IlJ886YnohONGIPrZx2pa
5xZOR8yDaGzRTfRy5qt4X3RvctzEkDnXVSQFCOG1HoDmZ9EX7q9e0DlMX6tVV6Dm
Dv19C3zryHwA5zsG2xSVMJLLNlNbDmr+mgzNy9ot98MTRs8CszD/X0M7FkaYrmCo
9hUCm6ItU1R3rLLd60s2izso69zyjmW5ao8JuG9zfTKaL8Nvrt43xLLcMaR5iUTx
Fq29xHFk7YwmYPyH6lQQggQUGO7TMAidKGa9lSSmwKoB3HzSc+LP2ie34nY0wzyV
cXeKOYDbWQTZ2xMHDm/9AgMBAAGjUzBRMB0GA1UdDgQWBBShEoxZ2LSSMJaRFt9O
xZibvI5c+jAfBgNVHSMEGDAWgBShEoxZ2LSSMJaRFt9OxZibvI5c+jAPBgNVHRMB
Af8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQDNBS+evQj+4L7wNE/JReJ+MVen
ag8/v9/cBs0Yh+Rahglgp7iqzZDyrBwSUSRZ+BIVouHH8SuX2QPmW17Xy/IhNW6u
L0qx03is4pz+xjrXXIpKe+xlJGqYm/0DRLdDLBPSiMEtGm7sFQL8kBW5S6/1xg/B
4lX/tP70LbXygP+rkDjzVTRY3IVModi+fhKXxB3rWBH88IJTDYjQ0MmfXveLeTcK
TZUUZpsP4or19B48WSqiV/eMdCB/PxnFZYT1SyFLlDBiXolb+30HbGeeaF0bEg+u
1+6zqGMDx++ViZJ2IRU+rLETtnOwS3yV5dUCIRN7jZN1iLIjgcgs5XLKY1Ft
-----END CERTIFICATE-----
`
)

var _ = Describe("source handler", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
		authz  service.Authz
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
		authz = service.NewNoopAuthzService(s)
	})

	AfterAll(func() {
		s.Close()
	})

	Context("list", func() {
		It("successfully list all the sources", func() {
			sourceID := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			sourceID = uuid.NewString()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "batman", "batman"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.ListSources(ctx, server.ListSourcesRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.ListSources200JSONResponse{}).String()))
			Expect(resp).To(HaveLen(1))
		})

		It("successfully list all the sources -- on premises", func() {
			sourceID := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceOnPremisesStm, sourceID, "source1", "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			sourceID = uuid.NewString()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceOnPremisesStm, sourceID, "source2", "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.ListSources(ctx, server.ListSourcesRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.ListSources200JSONResponse{}).String()))
			Expect(resp).To(HaveLen(2))

			count := 0
			tx = gormdb.Raw("SELECT count(*) from sources where on_premises IS TRUE;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(2))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("create", func() {
		It("successfully creates a source", func() {
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.CreateSource(ctx, server.CreateSourceRequestObject{
				Body: &v1alpha1.CreateSourceJSONRequestBody{
					Name: "test",
				},
			})
			Expect(err).To(BeNil())
			source, ok := resp.(server.CreateSource201JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(source.Name).To(Equal("test"))

			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			emailDomain := ""
			tx = gormdb.Raw("SELECT email_domain FROM sources LIMIT 1;").Scan(&emailDomain)
			Expect(tx.Error).To(BeNil())
			Expect(emailDomain).To(Equal("admin.example.com"))
		})

		It("successfully creates a source -- with proxy paramters defined", func() {
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			toStrPtr := func(s string) *string {
				return &s
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.CreateSource(ctx, server.CreateSourceRequestObject{
				Body: &v1alpha1.CreateSourceJSONRequestBody{
					Name: "test",
					Proxy: &v1alpha1.AgentProxy{
						HttpUrl:  toStrPtr("http://example.com"),
						HttpsUrl: toStrPtr("https://example.com"),
						NoProxy:  toStrPtr("noproxy"),
					},
				},
			})
			Expect(err).To(BeNil())
			source, ok := resp.(server.CreateSource201JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(source.Name).To(Equal("test"))

			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("failed to create a source -- proxy paramters not valid", func() {
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			toStrPtr := func(s string) *string {
				return &s
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.CreateSource(ctx, server.CreateSourceRequestObject{
				Body: &v1alpha1.CreateSourceJSONRequestBody{
					Name: "test",
					Proxy: &v1alpha1.AgentProxy{
						HttpUrl:  toStrPtr("http"),
						HttpsUrl: toStrPtr("https://example.com"),
						NoProxy:  toStrPtr("noproxy"),
					},
				},
			})
			Expect(err).To(BeNil())
			_, ok := resp.(server.CreateSource400JSONResponse)
			Expect(ok).To(BeTrue())

			count := 1
			tx := gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("successfully creates a source -- with certificate chain defined", func() {
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			toStrPtr := func(s string) *string {
				return &s
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.CreateSource(ctx, server.CreateSourceRequestObject{
				Body: &v1alpha1.CreateSourceJSONRequestBody{
					Name:             "test",
					CertificateChain: toStrPtr(validCert),
				},
			})
			Expect(err).To(BeNil())
			source, ok := resp.(server.CreateSource201JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(source.Name).To(Equal("test"))

			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("failed to create a source -- invalid certificate chain", func() {
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			toStrPtr := func(s string) *string {
				return &s
			}

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.CreateSource(ctx, server.CreateSourceRequestObject{
				Body: &v1alpha1.CreateSourceJSONRequestBody{
					Name:             "test",
					CertificateChain: toStrPtr("invalid cert"),
				},
			})
			Expect(err).To(BeNil())
			_, ok := resp.(server.CreateSource400JSONResponse)
			Expect(ok).To(BeTrue())

			count := 1
			tx := gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("get", func() {
		It("successfully retrieve the source", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			secondSourceID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, secondSourceID, "batman", "batman"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", secondSourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.GetSource(ctx, server.GetSourceRequestObject{Id: firstSourceID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSource200JSONResponse{}).String()))

			source := resp.(server.GetSource200JSONResponse)
			Expect(source.Id.String()).To(Equal(firstSourceID.String()))
			Expect(source.Agent).NotTo(BeNil())
			Expect(source.Agent.Id.String()).To(Equal(firstAgentID.String()))
		})

		It("successfully retrieve the source -- with infra fields", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			// Insert image_infra data with proxy, SSH key, and network config
			insertImageInfraStm := `INSERT INTO image_infras
				(source_id, http_proxy_url, https_proxy_url, no_proxy_domains, ssh_public_key, ip_address, subnet_mask, default_gateway, dns)
				VALUES ('%s', 'http://proxy.example.com', 'https://proxy.example.com', 'localhost,127.0.0.1', 'ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ', '192.168.1.100', '24', '192.168.1.1', '8.8.8.8');`
			tx = gormdb.Exec(fmt.Sprintf(insertImageInfraStm, firstSourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.GetSource(ctx, server.GetSourceRequestObject{Id: firstSourceID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSource200JSONResponse{}).String()))

			source := resp.(server.GetSource200JSONResponse)
			Expect(source.Id.String()).To(Equal(firstSourceID.String()))
			Expect(source.Agent).NotTo(BeNil())
			Expect(source.Agent.Id.String()).To(Equal(firstAgentID.String()))

			// Verify infra fields
			Expect(source.Infra).NotTo(BeNil())

			// Verify proxy
			Expect(source.Infra.Proxy).NotTo(BeNil())
			Expect(source.Infra.Proxy.HttpUrl).NotTo(BeNil())
			Expect(*source.Infra.Proxy.HttpUrl).To(Equal("http://proxy.example.com"))
			Expect(source.Infra.Proxy.HttpsUrl).NotTo(BeNil())
			Expect(*source.Infra.Proxy.HttpsUrl).To(Equal("https://proxy.example.com"))
			Expect(source.Infra.Proxy.NoProxy).NotTo(BeNil())
			Expect(*source.Infra.Proxy.NoProxy).To(Equal("localhost,127.0.0.1"))

			// Verify SSH public key
			Expect(source.Infra.SshPublicKey).NotTo(BeNil())
			Expect(*source.Infra.SshPublicKey).To(Equal("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ"))

			// Verify VM network
			Expect(source.Infra.VmNetwork).NotTo(BeNil())
			Expect(source.Infra.VmNetwork.Ipv4).NotTo(BeNil())
			Expect(source.Infra.VmNetwork.Ipv4.IpAddress).To(Equal("192.168.1.100"))
			Expect(source.Infra.VmNetwork.Ipv4.SubnetMask).To(Equal("24"))
			Expect(source.Infra.VmNetwork.Ipv4.DefaultGateway).To(Equal("192.168.1.1"))
			Expect(source.Infra.VmNetwork.Ipv4.Dns).To(Equal("8.8.8.8"))
		})

		It("successfully retrieve the source -- with partial infra fields", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			// Insert image_infra data with only SSH key (no proxy, no network)
			insertImageInfraStm := `INSERT INTO image_infras
				(source_id, ssh_public_key)
				VALUES ('%s', 'ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ');`
			tx = gormdb.Exec(fmt.Sprintf(insertImageInfraStm, firstSourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.GetSource(ctx, server.GetSourceRequestObject{Id: firstSourceID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSource200JSONResponse{}).String()))

			source := resp.(server.GetSource200JSONResponse)
			Expect(source.Id.String()).To(Equal(firstSourceID.String()))

			// Verify infra fields
			Expect(source.Infra).NotTo(BeNil())

			// Verify proxy is nil (no proxy data)
			Expect(source.Infra.Proxy).To(BeNil())

			// Verify SSH public key exists
			Expect(source.Infra.SshPublicKey).NotTo(BeNil())
			Expect(*source.Infra.SshPublicKey).To(Equal("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ"))

			// Verify VM network is nil (no network data)
			Expect(source.Infra.VmNetwork).To(BeNil())
		})

		It("successfully retrieve the source -- with labels", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			secondSourceID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, secondSourceID, "batman", "batman"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", secondSourceID))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertLabelStm, "key", "value", firstSourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.GetSource(ctx, server.GetSourceRequestObject{Id: firstSourceID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSource200JSONResponse{}).String()))

			source := resp.(server.GetSource200JSONResponse)
			Expect(source.Id.String()).To(Equal(firstSourceID.String()))
			Expect(source.Agent).NotTo(BeNil())
			Expect(source.Agent.Id.String()).To(Equal(firstAgentID.String()))
			Expect(source.Labels).ToNot(BeNil())
			Expect(*source.Labels).To(HaveLen(1))
			labels := *source.Labels
			Expect(labels[0].Key).To(Equal("key"))
			Expect(labels[0].Value).To(Equal("value"))
		})

		It("failed to get source - 404", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			secondSourceID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, secondSourceID, "batman", "batman"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", secondSourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.GetSource(ctx, server.GetSourceRequestObject{Id: uuid.New()})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSource404JSONResponse{}).String()))
		})

		It("failed to get source - 403", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			secondSourceID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, secondSourceID, "batman", "batman"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", secondSourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "joker",
				Organization: "joker",
				EmailDomain:  "joker.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.GetSource(ctx, server.GetSourceRequestObject{Id: firstSourceID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSource403JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from labels;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM image_infras;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("delete", func() {
		It("successfully deletes all the sources", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			secondSourceID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, secondSourceID, "batman", "batman"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.New(), "not-connected", "status-info-1", "cred_url-1", secondSourceID))
			Expect(tx.Error).To(BeNil())

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			_, err := srv.DeleteSources(context.TODO(), server.DeleteSourcesRequestObject{})
			Expect(err).To(BeNil())

			count := 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM SOURCES;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("successfully deletes a source", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			_, err := srv.DeleteSource(ctx, server.DeleteSourceRequestObject{Id: firstSourceID})
			Expect(err).To(BeNil())

			count := 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM SOURCES;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			count = 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM AGENTS;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("fails to delete a source -- under user's scope", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "user",
				Organization: "user",
				EmailDomain:  "user.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.DeleteSource(ctx, server.DeleteSourceRequestObject{Id: firstSourceID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteSource403JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("update source", func() {
		It("fails to update source with invalid label key", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			invalidLabels := []v1alpha1.Label{
				{Key: "-invalid-key", Value: "valid-value"},
			}
			resp, err := srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: sourceID,
				Body: &v1alpha1.SourceUpdate{
					Labels: &invalidLabels,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource400JSONResponse{}).String()))
		})

		It("fails to update source with invalid label value", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			invalidLabels := []v1alpha1.Label{
				{Key: "valid-key", Value: "invalid value with space"},
			}
			resp, err := srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: sourceID,
				Body: &v1alpha1.SourceUpdate{
					Labels: &invalidLabels,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource400JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM labels;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("update on prem", func() {
		It("successfully update source on prem", func() {
			firstSourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.UpdateInventory(ctx, server.UpdateInventoryRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.UpdateInventory{
					AgentId: uuid.New(),
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))

			// agent must be created
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM agents;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			vCenterID := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT v_center_id FROM SOURCES where id = '%s';", firstSourceID)).Scan(&vCenterID)
			Expect(tx.Error).To(BeNil())
			Expect(vCenterID).To(Equal("vcenter"))

			onPrem := false
			tx = gormdb.Raw(fmt.Sprintf("SELECT on_premises FROM SOURCES where id = '%s';", firstSourceID)).Scan(&onPrem)
			Expect(tx.Error).To(BeNil())
			Expect(onPrem).To(BeTrue())
		})

		It("successfully update source on prem -- same vcenter", func() {
			firstSourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.UpdateInventory(ctx, server.UpdateInventoryRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.UpdateInventory{
					AgentId: uuid.New(),
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))

			vCenterID := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT v_center_id FROM SOURCES where id = '%s';", firstSourceID)).Scan(&vCenterID)
			Expect(tx.Error).To(BeNil())
			Expect(vCenterID).To(Equal("vcenter"))

			onPrem := false
			tx = gormdb.Raw(fmt.Sprintf("SELECT on_premises FROM SOURCES where id = '%s';", firstSourceID)).Scan(&onPrem)
			Expect(tx.Error).To(BeNil())
			Expect(onPrem).To(BeTrue())

			updateResp, err := srv.UpdateInventory(ctx, server.UpdateInventoryRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.UpdateInventory{
					AgentId: uuid.New(),
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(updateResp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))
		})

		It("fails to update source on prem -- different vcenter", func() {
			firstSourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), authz)
			resp, err := srv.UpdateInventory(ctx, server.UpdateInventoryRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.UpdateInventory{
					AgentId: uuid.New(),
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))

			vCenterID := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT v_center_id FROM SOURCES where id = '%s';", firstSourceID)).Scan(&vCenterID)
			Expect(tx.Error).To(BeNil())
			Expect(vCenterID).To(Equal("vcenter"))

			onPrem := false
			tx = gormdb.Raw(fmt.Sprintf("SELECT on_premises FROM SOURCES where id = '%s';", firstSourceID)).Scan(&onPrem)
			Expect(tx.Error).To(BeNil())
			Expect(onPrem).To(BeTrue())

			resp, err = srv.UpdateInventory(ctx, server.UpdateInventoryRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.UpdateInventory{
					AgentId: uuid.New(),
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "another-vcenter-id",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory400JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})
})
