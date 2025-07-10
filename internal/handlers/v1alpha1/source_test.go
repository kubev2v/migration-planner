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
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
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
			gormdb.Exec("DELETE FROM share_tokens;")
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
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
			gormdb.Exec("DELETE FROM share_tokens;")
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.GetSource(ctx, server.GetSourceRequestObject{Id: firstSourceID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSource200JSONResponse{}).String()))

			source := resp.(server.GetSource200JSONResponse)
			Expect(source.Id.String()).To(Equal(firstSourceID.String()))
			Expect(source.Agent).NotTo(BeNil())
			Expect(source.Agent.Id.String()).To(Equal(firstAgentID.String()))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.GetSource(ctx, server.GetSourceRequestObject{Id: firstSourceID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetSource403JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE from labels;")
			gormdb.Exec("DELETE FROM agents;")
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.DeleteSource(ctx, server.DeleteSourceRequestObject{Id: firstSourceID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteSource403JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.SourceUpdateOnPrem{
					AgentId: uuid.New(),
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource200JSONResponse{}).String()))

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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.SourceUpdateOnPrem{
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource200JSONResponse{}).String()))

			vCenterID := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT v_center_id FROM SOURCES where id = '%s';", firstSourceID)).Scan(&vCenterID)
			Expect(tx.Error).To(BeNil())
			Expect(vCenterID).To(Equal("vcenter"))

			onPrem := false
			tx = gormdb.Raw(fmt.Sprintf("SELECT on_premises FROM SOURCES where id = '%s';", firstSourceID)).Scan(&onPrem)
			Expect(tx.Error).To(BeNil())
			Expect(onPrem).To(BeTrue())

			resp, err = srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.SourceUpdateOnPrem{
					AgentId: uuid.New(),
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource200JSONResponse{}).String()))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.SourceUpdateOnPrem{
					AgentId: uuid.New(),
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "vcenter",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource200JSONResponse{}).String()))

			vCenterID := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT v_center_id FROM SOURCES where id = '%s';", firstSourceID)).Scan(&vCenterID)
			Expect(tx.Error).To(BeNil())
			Expect(vCenterID).To(Equal("vcenter"))

			onPrem := false
			tx = gormdb.Raw(fmt.Sprintf("SELECT on_premises FROM SOURCES where id = '%s';", firstSourceID)).Scan(&onPrem)
			Expect(tx.Error).To(BeNil())
			Expect(onPrem).To(BeTrue())

			resp, err = srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: firstSourceID,
				Body: &v1alpha1.SourceUpdateOnPrem{
					AgentId: uuid.New(),
					Inventory: v1alpha1.Inventory{
						Vcenter: v1alpha1.VCenter{
							Id: "another-vcenter-id",
						},
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource400JSONResponse{}).String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("delete source with share token", func() {
		It("deletes share token when source is deleted via handler", func() {
			sourceID := uuid.New()

			// Create source
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Create share token for the source
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO share_tokens (token, source_id) VALUES ('%s', '%s');", "test-token", sourceID))
			Expect(tx.Error).To(BeNil())

			// Verify share token exists
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM share_tokens WHERE source_id = ?", sourceID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
				EmailDomain:  "admin.example.com",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			// Delete the source via handler
			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.DeleteSource(ctx, server.DeleteSourceRequestObject{Id: sourceID})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteSource200JSONResponse{}).String()))

			// Verify source is deleted
			count = 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM sources WHERE id = ?", sourceID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			// Verify share token is automatically deleted due to CASCADE
			count = 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM share_tokens WHERE source_id = ?", sourceID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("deletes all share tokens when all sources are deleted via handler", func() {
			sourceID1 := uuid.New()
			sourceID2 := uuid.New()

			// Create two sources with different org_ids to avoid unique constraint violation
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID1, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID2, "user1", "group1"))
			Expect(tx.Error).To(BeNil())

			// Create share tokens for both sources
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO share_tokens (token, source_id) VALUES ('%s', '%s');", "test-token-1", sourceID1))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf("INSERT INTO share_tokens (token, source_id) VALUES ('%s', '%s');", "test-token-2", sourceID2))
			Expect(tx.Error).To(BeNil())

			// Verify both share tokens exist
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM share_tokens").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(2))

			// Delete all sources via handler
			srv := handlers.NewServiceHandler(service.NewSourceService(s), service.NewShareTokenService(s))
			resp, err := srv.DeleteSources(context.TODO(), server.DeleteSourcesRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.DeleteSources200JSONResponse{}).String()))

			// Verify all sources are deleted
			count = 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM sources").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			// Verify all share tokens are deleted due to CASCADE
			count = 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM share_tokens").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM share_tokens;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})
})
