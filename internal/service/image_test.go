package service_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/iso"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

// Same RHCOS live ISO as build/migration-planner-iso/config (used only by integration test).
const (
	rhcosTestISOURL      = "https://mirror.openshift.com/pub/openshift-v4/dependencies/rhcos/4.19/4.19.10/rhcos-4.19.10-x86_64-live-iso.x86_64.iso"
	rhcosTestISOChecksum = "7a47d0c7a9bf5edb143d52809e793af2d74731567b95d91c6225171a1c49b5ab"
)

func downloadRHCOSISO(destPath, url, wantSHA256 string) error {
	if _, err := os.Stat(destPath); err == nil {
		return nil
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	dl := iso.NewHttpDownloader(url, wantSHA256)
	if err := dl.Get(ctx, f); err != nil {
		_ = os.Remove(destPath)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = os.Remove(destPath)
		return err
	}
	return nil
}

func findModuleRootFromWD() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found starting from %s", dir)
		}
		dir = parent
	}
}

var _ = Describe("image service", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).NotTo(HaveOccurred())
		db, err := store.InitDB(cfg)
		Expect(err).NotTo(HaveOccurred())

		s = store.NewStore(db)
		gormdb = db
	})

	AfterAll(func() {
		_ = s.Close()
	})

	Describe("Validate", func() {
		var (
			imgSvc *service.ImageSvc
			tmpDir string
		)

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "image-svc-validate-*")
			Expect(err).NotTo(HaveOccurred())
			c, err := config.New()
			Expect(err).NotTo(HaveOccurred())
			c.Service.TempImagesDir = tmpDir
			imgSvc = service.NewImageSvc(s, c)
		})

		AfterEach(func() {
			_ = os.RemoveAll(tmpDir)
			gormdb.Exec("DELETE FROM labels;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM image_infras;")
			gormdb.Exec("DELETE FROM sources;")
		})

		It("returns an error for an invalid source ID string", func() {
			err := imgSvc.Validate(context.Background(), "not-a-uuid")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid source ID"))
		})

		It("returns an error when the source does not exist", func() {
			err := imgSvc.Validate(context.Background(), uuid.NewString())
			Expect(err).To(HaveOccurred())
		})

		It("succeeds when the source exists", func() {
			srcSvc := service.NewSourceService(s, nil)
			created, err := srcSvc.CreateSource(context.Background(), mappers.SourceCreateForm{
				Name: "validate-me-" + uuid.NewString(), OrgID: "admin", Username: "admin",
			})
			Expect(err).NotTo(HaveOccurred())

			err = imgSvc.Validate(context.Background(), created.ID.String())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("ValidateToken", func() {
		var (
			imgSvc   *service.ImageSvc
			tmpDir   string
			sourceID uuid.UUID
			tokenKey string
		)

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "image-svc-token-*")
			Expect(err).NotTo(HaveOccurred())
			c, err := config.New()
			Expect(err).NotTo(HaveOccurred())
			c.Service.TempImagesDir = tmpDir
			imgSvc = service.NewImageSvc(s, c)

			srcSvc := service.NewSourceService(s, nil)
			created, err := srcSvc.CreateSource(context.Background(), mappers.SourceCreateForm{
				Name: "token-test-" + uuid.NewString(), OrgID: "admin", Username: "admin",
			})
			Expect(err).NotTo(HaveOccurred())
			sourceID = created.ID
			full, err := s.Source().Get(context.Background(), sourceID)
			Expect(err).NotTo(HaveOccurred())
			tokenKey = full.ImageInfra.ImageTokenKey
			Expect(tokenKey).NotTo(BeEmpty())
		})

		AfterEach(func() {
			_ = os.RemoveAll(tmpDir)
			gormdb.Exec("DELETE FROM labels;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM image_infras;")
			gormdb.Exec("DELETE FROM sources;")
		})

		It("rejects a malformed token", func() {
			err := imgSvc.ValidateToken(context.Background(), "not-a-jwt")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ParseTokenETag", func() {
		var imgSvc *service.ImageSvc

		BeforeEach(func() {
			c, err := config.New()
			Expect(err).NotTo(HaveOccurred())
			imgSvc = service.NewImageSvc(s, c)
		})

		It("returns an error when the separator is missing", func() {
			_, _, err := imgSvc.ParseTokenETag("only-one-part")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid format"))
		})

		It("returns an error when the token part is empty", func() {
			_, _, err := imgSvc.ParseTokenETag("+etagvalue")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing token or etag"))
		})

		It("returns an error when the etag part is empty", func() {
			_, _, err := imgSvc.ParseTokenETag("tokensegment+")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing token or etag"))
		})
	})

	Describe("GenerateDownloadURL", func() {
		var (
			imgSvc *service.ImageSvc
			tmpDir string
		)

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "image-svc-gendurl-*")
			Expect(err).NotTo(HaveOccurred())
			c, err := config.New()
			Expect(err).NotTo(HaveOccurred())
			c.Service.TempImagesDir = tmpDir
			imgSvc = service.NewImageSvc(s, c)
		})

		AfterEach(func() {
			_ = os.RemoveAll(tmpDir)
			gormdb.Exec("DELETE FROM keys;")
			gormdb.Exec("DELETE FROM labels;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM image_infras;")
			gormdb.Exec("DELETE FROM sources;")
		})

		It("returns ErrResourceNotFound when the source does not exist", func() {
			_, _, err := imgSvc.GenerateDownloadURL(context.Background(), uuid.New())
			Expect(err).To(HaveOccurred())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})
	})

	// Full OVA build needs RHCOS ISO, data/ignition.template, data/MigrationAssessment.ovf, data/persistence-disk.vmdk.
	// pkg/image resolves those paths relative to the process working directory, so we chdir to the module root.
	Describe("GenerateDownloadURL integration", Ordered, func() {
		var (
			repoRoot string
			origWd   string
			isoPath  string
			imgSvc   *service.ImageSvc
			tmpDir   string
		)

		BeforeAll(func() {
			var err error
			origWd, err = os.Getwd()
			Expect(err).NotTo(HaveOccurred())

			repoRoot, err = findModuleRootFromWD()
			Expect(err).NotTo(HaveOccurred())
			vmdk := filepath.Join(repoRoot, "data", "persistence-disk.vmdk")
			if _, err := os.Stat(vmdk); err != nil {
				Skip(fmt.Sprintf("data/persistence-disk.vmdk not found (%v); add it to run this test", err))
			}

			isoPath = filepath.Join(repoRoot, "rhcos-live-iso.x86_64.iso")
			if _, err := os.Stat(isoPath); err != nil {
				isoPath = filepath.Join(os.TempDir(), "migration-planner-rhcos-integration.iso")
				Expect(downloadRHCOSISO(isoPath, rhcosTestISOURL, rhcosTestISOChecksum)).To(Succeed())
			}

			Expect(os.Setenv("MIGRATION_PLANNER_ISO_PATH", isoPath)).To(Succeed())
		})

		BeforeEach(func() {
			Expect(os.Chdir(repoRoot)).To(Succeed())

			var err error
			tmpDir, err = os.MkdirTemp("", "image-svc-gendurl-int-*")
			Expect(err).NotTo(HaveOccurred())
			c, err := config.New()
			Expect(err).NotTo(HaveOccurred())
			c.Service.TempImagesDir = tmpDir
			imgSvc = service.NewImageSvc(s, c)
		})

		AfterEach(func() {
			_ = os.RemoveAll(tmpDir)
			gormdb.Exec("DELETE FROM keys;")
			gormdb.Exec("DELETE FROM labels;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM image_infras;")
			gormdb.Exec("DELETE FROM sources;")

			Expect(os.Chdir(origWd)).To(Succeed())
		})

		AfterAll(func() {
			_ = os.Unsetenv("MIGRATION_PLANNER_ISO_PATH")
			if isoPath != "" {
				_ = os.Remove(isoPath)
			}
		})

		It("downloads the ISO, generates one OVA per etag, and returns stable URLs for concurrent callers", func() {
			srcSvc := service.NewSourceService(s, nil)
			created, err := srcSvc.CreateSource(context.Background(), mappers.SourceCreateForm{
				Name: "ova-int-" + uuid.NewString(), OrgID: "admin", Username: "admin",
			})
			Expect(err).NotTo(HaveOccurred())

			const workers = 8
			type result struct {
				downloadURL string
				expiresAt   time.Time
				err         error
			}
			ch := make(chan result, workers)
			for i := 0; i < workers; i++ {
				go func() {
					u, exp, err := imgSvc.GenerateDownloadURL(context.Background(), created.ID)
					ch <- result{downloadURL: u, expiresAt: exp, err: err}
				}()
			}

			var firstURL string
			for i := 0; i < workers; i++ {
				r := <-ch
				Expect(r.err).NotTo(HaveOccurred())
				Expect(r.downloadURL).NotTo(BeEmpty())
				Expect(r.expiresAt).NotTo(BeZero())
				if firstURL == "" {
					firstURL = r.downloadURL
				}

				parsed, err := url.Parse(r.downloadURL)
				Expect(err).NotTo(HaveOccurred())
				Expect(parsed.Path).To(ContainSubstring("/api/v1/image/bytoken/"))

				// Path: .../bytoken/{token+etag}/{sourceName}.ova
				segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
				Expect(len(segments)).To(BeNumerically(">=", 4))
				tokenETag := segments[len(segments)-2]
				tok, etag, err := imgSvc.ParseTokenETag(tokenETag)
				Expect(err).NotTo(HaveOccurred())
				Expect(tok).NotTo(BeEmpty())
				Expect(etag).NotTo(BeEmpty())

				ovaPath := imgSvc.FilePath(etag)
				fi, err := os.Stat(ovaPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(fi.Size()).To(BeNumerically(">", 1100000000), "OVA file should be fully written when GenerateDownloadURL returns")
			}

			matches, err := filepath.Glob(filepath.Join(tmpDir, "*.ova"))
			Expect(err).NotTo(HaveOccurred())
			Expect(matches).To(HaveLen(1), "singleflight should produce one file per etag")
		})
	})
})
