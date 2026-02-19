package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertAgentStm                 = "INSERT INTO agents (id, status, status_info, cred_url,source_id, version) VALUES ('%s', '%s', '%s', '%s', '%s', 'version_1');"
	insertSourceWithUsernameStm    = "INSERT INTO sources (id, name, username, org_id) VALUES ('%s', 'source_name', '%s', '%s');"
	insertSourceWithEmailDomainStm = "INSERT INTO sources (id, name, username, org_id, email_domain) VALUES ('%s', 'source_name', '%s', '%s', '%s');"
	insertSourceOnPremisesStm      = "INSERT INTO sources (id, name, username, org_id, on_premises) VALUES ('%s', '%s', '%s', '%s', TRUE);"
	insertLabelStm                 = "INSERT INTO labels (key, value, source_id) VALUES ('%s', '%s', '%s');"
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

			srv := service.NewSourceService(s, nil)
			resp, err := srv.ListSources(context.TODO(), service.NewSourceFilter(service.WithUsername("batman"), service.WithOrgID("batman")))
			Expect(err).To(BeNil())
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

			srv := service.NewSourceService(s, nil)
			resp, err := srv.ListSources(context.TODO(), service.NewSourceFilter(service.WithUsername("admin"), service.WithOrgID("admin")))
			Expect(err).To(BeNil())
			Expect(resp).To(HaveLen(2))

			count := 0
			tx = gormdb.Raw("SELECT count(*) from sources where on_premises IS TRUE;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(2))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM labels;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
			gormdb.Exec("DELETE FROM image_infras;")
		})
	})

	Context("create", func() {
		It("successfully creates a source", func() {
			srv := service.NewSourceService(s, nil)
			source, err := srv.CreateSource(context.TODO(), mappers.SourceCreateForm{Name: "test", OrgID: "admin", Username: "admin"})
			Expect(err).To(BeNil())
			Expect(source.Name).To(Equal("test"))

			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("successfully creates a source -- with proxy paramters defined", func() {
			srv := service.NewSourceService(s, nil)
			httpUrl := "http"
			httpsUrl := "https"
			noProxy := "noproxy"
			source, err := srv.CreateSource(context.TODO(), mappers.SourceCreateForm{
				Username: "admin",
				OrgID:    "admin",
				Name:     "test",
				HttpUrl:  httpUrl,
				HttpsUrl: httpsUrl,
				NoProxy:  noProxy,
			})
			Expect(err).To(BeNil())
			Expect(source.Name).To(Equal("test"))

			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("successfully creates a source -- with certificate chain defined", func() {
			srv := service.NewSourceService(s, nil)
			source, err := srv.CreateSource(context.TODO(), mappers.SourceCreateForm{
				Username:         "admin",
				OrgID:            "admin",
				Name:             "test",
				CertificateChain: "chain",
			})
			Expect(err).To(BeNil())
			Expect(source.Name).To(Equal("test"))

			count := 0
			tx := gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("successfully creates a source -- email domain defined", func() {
			srv := service.NewSourceService(s, nil)
			source, err := srv.CreateSource(context.TODO(), mappers.SourceCreateForm{Name: "test", OrgID: "admin", Username: "admin", EmailDomain: "domain.com"})
			Expect(err).To(BeNil())
			Expect(source.Name).To(Equal("test"))
			Expect(source.EmailDomain).ToNot(BeNil())
			Expect(*source.EmailDomain).To(Equal("domain.com"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM labels;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
			gormdb.Exec("DELETE FROM image_infras;")
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
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := service.NewSourceService(s, nil)
			source, err := srv.GetSource(ctx, firstSourceID)
			Expect(err).To(BeNil())
			Expect(source.ID.String()).To(Equal(firstSourceID.String()))
			Expect(source.Agents).To(HaveLen(1))
		})

		It("successfully retrieve the source -- with email domain", func() {
			firstSourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithEmailDomainStm, firstSourceID, "admin", "admin", "example.com"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := service.NewSourceService(s, nil)
			source, err := srv.GetSource(ctx, firstSourceID)
			Expect(err).To(BeNil())
			Expect(source.ID.String()).To(Equal(firstSourceID.String()))
			Expect(source.EmailDomain).ToNot(BeNil())
			Expect(*source.EmailDomain).To(Equal("example.com"))
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

			srv := service.NewSourceService(s, nil)
			_, err := srv.GetSource(context.TODO(), uuid.New())
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from labels;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
			gormdb.Exec("DELETE FROM image_infras;")
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

			srv := service.NewSourceService(s, nil)
			err := srv.DeleteSources(context.TODO())
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

			srv := service.NewSourceService(s, nil)
			err := srv.DeleteSource(context.TODO(), firstSourceID)
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

		AfterEach(func() {
			gormdb.Exec("DELETE FROM labels;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
			gormdb.Exec("DELETE FROM image_infras;")
		})
	})

	Context("update on prem", func() {
		It("successfully update source on prem", func() {
			firstSourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{
				VcenterId: "vcenter",
			})

			srv := service.NewSourceService(s, nil)
			_, err := srv.UpdateInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID:  firstSourceID,
				AgentID:   uuid.New(),
				VCenterID: "vcenter",
				Inventory: inventoryJSON,
			})
			Expect(err).To(BeNil())

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

			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{
				VcenterId: "vcenter",
			})

			srv := service.NewSourceService(s, nil)
			_, err := srv.UpdateInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID:  firstSourceID,
				AgentID:   uuid.New(),
				VCenterID: "vcenter",
				Inventory: inventoryJSON,
			})
			Expect(err).To(BeNil())

			vCenterID := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT v_center_id FROM SOURCES where id = '%s';", firstSourceID)).Scan(&vCenterID)
			Expect(tx.Error).To(BeNil())
			Expect(vCenterID).To(Equal("vcenter"))

			onPrem := false
			tx = gormdb.Raw(fmt.Sprintf("SELECT on_premises FROM SOURCES where id = '%s';", firstSourceID)).Scan(&onPrem)
			Expect(tx.Error).To(BeNil())
			Expect(onPrem).To(BeTrue())

			_, err = srv.UpdateInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID:  firstSourceID,
				AgentID:   uuid.New(),
				VCenterID: "vcenter",
				Inventory: inventoryJSON,
			})
			Expect(err).To(BeNil())
		})

		It("fails to update source on prem -- different vcenter", func() {
			firstSourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			inventory1JSON, _ := json.Marshal(v1alpha1.Inventory{
				VcenterId: "vcenter",
			})

			srv := service.NewSourceService(s, nil)
			_, err := srv.UpdateInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID:  firstSourceID,
				AgentID:   uuid.New(),
				VCenterID: "vcenter",
				Inventory: inventory1JSON,
			})
			Expect(err).To(BeNil())

			inventory2JSON, _ := json.Marshal(v1alpha1.Inventory{
				VcenterId: "another-vcenter-id",
			})

			// Now try to update with a different vCenter ID
			_, err = srv.UpdateInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID:  firstSourceID,
				AgentID:   uuid.New(),
				VCenterID: "another-vcenter-id",
				Inventory: inventory2JSON,
			})
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrInvalidVCenterID)
			Expect(ok).To(BeTrue())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("update-source", func() {
		It("successfully updates source", func() {
			// First create a source with initial data
			sourceID := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Create initial image_infra record
			initialImageInfra := model.ImageInfra{
				SourceID: uuid.MustParse(sourceID),
			}
			_, err := s.ImageInfra().Create(context.TODO(), initialImageInfra)
			Expect(err).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), nil, service.NewSizerService(nil, s), nil)
			newName := "updated-name"
			newLabels := []v1alpha1.Label{
				{Key: "env", Value: "prod"},
			}
			newSshKey := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCk83ddeteALlqCbO43E3ardbavFPboYIoFnlQZ3zVi+ls96c1x3P9DDWkNhuOgpQurull2y55Wm7HWLLK5hlk49s6tUuBDftH3XXfGMAmncBH9apGHxl0O+k/X1MrfhoEXHmmEwXTv+X6vC3BsZiazSOkKbIozHgnD7y1z83wuYWbbW0NYvgwhaoOtkWteKSJWwPxNaTwGCpj+RQ6xWygt5EbMSf7U3Ih2P1hcsa615zD5P2GSLxtLwWnHgWCylT/krdyIYlR1pqW9e/Iv2MKlGX6W1DSUxUz5BNxzCA8O53C0NSCeDFAhn9T8VE9U/RkGDtXBFJ8JVcmtM6S9buq5HZ12+0E0VCGFdmnvNT8XxdYrN0ff8f3DQI7ERgHEKQiqjrSPDv2+OMdv3nr3n5+tOBvQEn6aYDbnybILyrUP76UvLvjfgDTnnRxlkpw2Y43EtgtdeIUUo/VBSE9qfzRa21Pz3gBh6ZJE9xF+u6DstgvFigNJ7nMJoSktH5mzuBM= test@test"
			newCertChain := `-----BEGIN CERTIFICATE-----
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
-----END CERTIFICATE-----`
			newHttpProxy := "http://proxy.example.com:8080"
			newHttpsProxy := "https://proxy.example.com:8443"
			newNoProxy := "localhost,127.0.0.1"

			resp, err := srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: uuid.MustParse(sourceID),
				Body: &v1alpha1.SourceUpdate{
					Name:             util.ToStrPtr(newName),
					Labels:           &newLabels,
					SshPublicKey:     util.ToStrPtr(newSshKey),
					CertificateChain: util.ToStrPtr(newCertChain),
					Proxy: &v1alpha1.AgentProxy{
						HttpUrl:  util.ToStrPtr(newHttpProxy),
						HttpsUrl: util.ToStrPtr(newHttpsProxy),
						NoProxy:  util.ToStrPtr(newNoProxy),
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource200JSONResponse{}).String()))

			// Verify the updated source
			updatedSource, err := s.Source().Get(ctx, uuid.MustParse(sourceID))
			Expect(err).To(BeNil())
			Expect(updatedSource.Name).To(Equal(newName))
			Expect(updatedSource.Labels).To(HaveLen(1))
			Expect(updatedSource.Labels[0].Key).To(Equal("env"))
			Expect(updatedSource.Labels[0].Value).To(Equal("prod"))
			Expect(updatedSource.ImageInfra.SshPublicKey).To(Equal(newSshKey))
			Expect(updatedSource.ImageInfra.CertificateChain).To(Equal(newCertChain))
			Expect(updatedSource.ImageInfra.HttpProxyUrl).To(Equal(newHttpProxy))
			Expect(updatedSource.ImageInfra.HttpsProxyUrl).To(Equal(newHttpsProxy))
			Expect(updatedSource.ImageInfra.NoProxyDomains).To(Equal(newNoProxy))
		})

		It("returns 404 if source not found", func() {
			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), nil, service.NewSizerService(nil, s), nil)
			resp, err := srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id:   uuid.New(),
				Body: &v1alpha1.SourceUpdate{},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource404JSONResponse{}).String()))
		})

		It("returns 403 if user not authorized", func() {
			sourceID := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "owner-user", "owner-org"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "unauthorized",
				Organization: "unauthorized",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), nil, service.NewSizerService(nil, s), nil)
			resp, err := srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id:   uuid.MustParse(sourceID),
				Body: &v1alpha1.SourceUpdate{},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource403JSONResponse{}).String()))
		})

		It("successfully replaces existing labels", func() {
			sourceID := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Create initial image_infra record
			initialImageInfra := model.ImageInfra{
				SourceID: uuid.MustParse(sourceID),
			}
			_, err := s.ImageInfra().Create(context.TODO(), initialImageInfra)
			Expect(err).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), nil, service.NewSizerService(nil, s), nil)

			// First set initial labels
			initialLabels := []v1alpha1.Label{
				{Key: "env", Value: "dev"},
				{Key: "region", Value: "us-east"},
			}
			resp, err := srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: uuid.MustParse(sourceID),
				Body: &v1alpha1.SourceUpdate{
					Labels: &initialLabels,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource200JSONResponse{}).String()))

			// Verify initial labels were set
			updatedSource, err := s.Source().Get(ctx, uuid.MustParse(sourceID))
			Expect(err).To(BeNil())
			Expect(updatedSource.Labels).To(HaveLen(2))
			Expect(updatedSource.Labels[0].Key).To(Equal("env"))
			Expect(updatedSource.Labels[0].Value).To(Equal("dev"))
			Expect(updatedSource.Labels[1].Key).To(Equal("region"))
			Expect(updatedSource.Labels[1].Value).To(Equal("us-east"))

			// Now update with new labels
			newLabels := []v1alpha1.Label{
				{Key: "env", Value: "prod"},
				{Key: "tier", Value: "critical"},
			}
			resp, err = srv.UpdateSource(ctx, server.UpdateSourceRequestObject{
				Id: uuid.MustParse(sourceID),
				Body: &v1alpha1.SourceUpdate{
					Labels: &newLabels,
				},
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateSource200JSONResponse{}).String()))

			// Verify labels were replaced
			updatedSource, err = s.Source().Get(ctx, uuid.MustParse(sourceID))
			Expect(err).To(BeNil())
			Expect(updatedSource.Labels).To(HaveLen(2))
			Expect(updatedSource.Labels[0].Key).To(Equal("env"))
			Expect(updatedSource.Labels[0].Value).To(Equal("prod"))
			Expect(updatedSource.Labels[1].Key).To(Equal("tier"))
			Expect(updatedSource.Labels[1].Value).To(Equal("critical"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM labels;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
			gormdb.Exec("DELETE FROM image_infras;")
		})
	})

	Context("source filter", func() {
		It("filter has username set properlly", func() {
			f := service.NewSourceFilter(service.WithUsername("test"))
			Expect(f.Username).To(Equal("test"))
		})
		It("filter has id set properlly", func() {
			id := uuid.New()
			f := service.NewSourceFilter(service.WithSourceID(id))
			Expect(f.ID).To(Equal(id))
		})
	})
})
