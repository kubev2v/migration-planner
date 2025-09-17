package store_test

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertImageStm = "INSERT INTO image_infras (source_id, http_proxy_url, https_proxy_url, no_proxy_domains, certificate_chain) VALUES ('%s', '%s', '%s', '%s', '%s');"
)

var _ = Describe("image infra store", Ordered, func() {
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

	Context("create", func() {
		It("successfully create an image", func() {
			sourceID := uuid.New()
			m := model.Source{
				ID:       sourceID,
				Username: "admin",
				OrgID:    "org",
			}
			source, err := s.Source().Create(context.TODO(), m)
			Expect(err).To(BeNil())
			Expect(source).NotTo(BeNil())

			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM sources;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			// create the image
			image := model.ImageInfra{
				SourceID:         m.ID,
				HttpProxyUrl:     "http",
				HttpsProxyUrl:    "https",
				NoProxyDomains:   "noproxy",
				CertificateChain: "certs",
			}

			img, err := s.ImageInfra().Create(context.TODO(), image)
			Expect(err).To(BeNil())
			Expect(img).ToNot(BeNil())

			count = -1
			tx = gormdb.Raw("SELECT COUNT(*) FROM image_infras;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from sources;")
		})
	})

	Context("get", func() {
		It("successfully get a source with image infra", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "source1", "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertImageStm, sourceID, "http", "https", "noproxy", "certs"))
			Expect(tx.Error).To(BeNil())

			source, err := s.Source().Get(context.TODO(), sourceID)
			Expect(err).To(BeNil())
			Expect(source).ToNot(BeNil())

			Expect(source.ImageInfra.HttpProxyUrl).To(Equal("http"))
			Expect(source.ImageInfra.HttpsProxyUrl).To(Equal("https"))
			Expect(source.ImageInfra.NoProxyDomains).To(Equal("noproxy"))
			Expect(source.ImageInfra.CertificateChain).To(Equal("certs"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from sources;")
		})
	})

	Context("list", func() {
		It("successfully list sources with image infra", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "source1", "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertImageStm, sourceID, "http", "https", "noproxy", "certs"))
			Expect(tx.Error).To(BeNil())

			sources, err := s.Source().List(context.TODO(), store.NewSourceQueryFilter())
			Expect(err).To(BeNil())
			Expect(sources).To(HaveLen(1))

			Expect(sources[0].ImageInfra.HttpProxyUrl).To(Equal("http"))
			Expect(sources[0].ImageInfra.HttpsProxyUrl).To(Equal("https"))
			Expect(sources[0].ImageInfra.NoProxyDomains).To(Equal("noproxy"))
			Expect(sources[0].ImageInfra.CertificateChain).To(Equal("certs"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from sources;")
		})
	})

	Context("update", func() {
		It("successfully updates an existing image infra", func() {
			sourceID := uuid.New()
			// create source
			m := model.Source{ID: sourceID, Username: "admin", OrgID: "org"}
			src, err := s.Source().Create(context.TODO(), m)
			Expect(err).To(BeNil())
			Expect(src).NotTo(BeNil())

			// create initial image infra
			img, err := s.ImageInfra().Create(context.TODO(), model.ImageInfra{
				SourceID:         sourceID,
				HttpProxyUrl:     "http1",
				HttpsProxyUrl:    "https1",
				NoProxyDomains:   "np1",
				CertificateChain: "cert1",
			})
			Expect(err).To(BeNil())
			Expect(img).ToNot(BeNil())

			// update fields
			img.HttpProxyUrl = "http2"
			img.HttpsProxyUrl = "https2"
			img.NoProxyDomains = "np2"
			img.CertificateChain = "cert2"

			upd, err := s.ImageInfra().Update(context.TODO(), *img)
			Expect(err).To(BeNil())
			Expect(upd).ToNot(BeNil())
			Expect(upd.HttpProxyUrl).To(Equal("http2"))
			Expect(upd.HttpsProxyUrl).To(Equal("https2"))
			Expect(upd.NoProxyDomains).To(Equal("np2"))
			Expect(upd.CertificateChain).To(Equal("cert2"))

			// verify in DB
			var fetched model.ImageInfra
			gormdb.WithContext(context.TODO()).Where("source_id = ?", sourceID).First(&fetched)
			Expect(fetched.HttpProxyUrl).To(Equal("http2"))
			Expect(fetched.HttpsProxyUrl).To(Equal("https2"))
			Expect(fetched.NoProxyDomains).To(Equal("np2"))
			Expect(fetched.CertificateChain).To(Equal("cert2"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from image_infras;")
			gormdb.Exec("DELETE from sources;")
		})
	})
})
