package v1alpha1_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/google/uuid"
	server "github.com/kubev2v/migration-planner/internal/api/server/image"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

var _ = Describe("agent image by token", Ordered, func() {
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
		_ = s.Close()
	})

	AfterEach(func() {
		gormdb.Exec("DELETE FROM image_infras;")
		gormdb.Exec("DELETE FROM sources;")
	})

	It("serves the appliance image tar for a valid presigned URL", func() {
		dir := GinkgoT().TempDir()
		body := []byte("fake-oci-archive")
		Expect(os.WriteFile(filepath.Join(dir, "migration-planner-agent.tar"), body, 0o644)).To(Succeed())
		cfg, err := config.New()
		Expect(err).To(BeNil())
		oldIsoPath := cfg.Service.IsoPath
		DeferCleanup(func() { cfg.Service.IsoPath = oldIsoPath })
		cfg.Service.IsoPath = filepath.Join(dir, "agent.iso")

		sourceID := uuid.New()
		tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
		Expect(tx.Error).To(BeNil())
		tx = gormdb.Exec(fmt.Sprintf("INSERT INTO image_infras (source_id) VALUES ('%s');", sourceID))
		Expect(tx.Error).To(BeNil())

		sourceSrv := service.NewSourceService(s, nil)
		downloadURL, _, err := sourceSrv.GetAgentImageDownloadURL(context.Background(), sourceID)
		Expect(err).To(BeNil())

		parsed, err := url.Parse(downloadURL)
		Expect(err).To(BeNil())
		token := strings.Split(parsed.Path, "/")[5]
		Expect(token).NotTo(BeEmpty())
		Expect(strings.HasSuffix(parsed.Path, "/appliance")).To(BeTrue())

		h := handlers.NewImageHandler(s, cfg)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, parsed.Path, nil)
		ctx := context.WithValue(context.Background(), image.ResponseWriterKey, w)
		ctx = context.WithValue(ctx, image.RequestKey, r)
		resp, err := h.GetAgentImageByToken(ctx, server.GetAgentImageByTokenRequestObject{Token: token})
		Expect(err).To(BeNil())
		Expect(resp).To(BeNil())
		Expect(w.Code).To(Equal(http.StatusOK))
		Expect(w.Body.Bytes()).To(Equal(body))

		// Without a valid served version (unset until OMA-1941 lands) the
		// stamping guard must leave the stored version untouched.
		var storedVersion *string
		tx = gormdb.Raw("SELECT agent_version FROM image_infras WHERE source_id = ?", sourceID).Scan(&storedVersion)
		Expect(tx.Error).To(BeNil())
		Expect(storedVersion).To(BeNil())
	})

	It("rejects an invalid token and reports a missing tar", func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		oldIsoPath := cfg.Service.IsoPath
		DeferCleanup(func() { cfg.Service.IsoPath = oldIsoPath })
		cfg.Service.IsoPath = filepath.Join(GinkgoT().TempDir(), "agent.iso")
		h := handlers.NewImageHandler(s, cfg)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/image/bytoken/bad-token/appliance", nil)
		ctx := context.WithValue(context.Background(), image.ResponseWriterKey, w)
		ctx = context.WithValue(ctx, image.RequestKey, r)
		resp, err := h.GetAgentImageByToken(ctx, server.GetAgentImageByTokenRequestObject{Token: "bad-token"})
		Expect(err).To(BeNil())
		Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetAgentImageByToken401JSONResponse{}).String()))

		sourceID := uuid.New()
		tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
		Expect(tx.Error).To(BeNil())
		tx = gormdb.Exec(fmt.Sprintf("INSERT INTO image_infras (source_id) VALUES ('%s');", sourceID))
		Expect(tx.Error).To(BeNil())

		sourceSrv := service.NewSourceService(s, nil)
		downloadURL, _, err := sourceSrv.GetAgentImageDownloadURL(context.Background(), sourceID)
		Expect(err).To(BeNil())
		parsed, err := url.Parse(downloadURL)
		Expect(err).To(BeNil())
		token := strings.Split(parsed.Path, "/")[5]

		resp, err = h.GetAgentImageByToken(ctx, server.GetAgentImageByTokenRequestObject{Token: token})
		Expect(err).To(BeNil())
		Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetAgentImageByToken404JSONResponse{}).String()))
	})
})
