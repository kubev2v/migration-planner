package rvtools_test

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"gorm.io/gorm"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/rvtools"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

var _ = Describe("RVTools Background Processing", Ordered, func() {
	var (
		s           store.Store
		gormdb      *gorm.DB
		riverClient *river.Client[pgx.Tx]
		dbPool      *pgxpool.Pool
		ctx         context.Context
		svc         *service.AssessmentService
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
		ctx = context.Background()

		// Create pgx pool for River
		dsn := "host=" + cfg.Database.Hostname +
			" user=" + cfg.Database.User +
			" password=" + cfg.Database.Password +
			" port=" + cfg.Database.Port +
			" dbname=" + cfg.Database.Name +
			" sslmode=disable"

		dbPool, err = pgxpool.New(ctx, dsn)
		Expect(err).To(BeNil())

		// Run River migrations to create necessary tables
		migrator, err := rivermigrate.New(riverpgxv5.New(dbPool), nil)
		Expect(err).To(BeNil())
		_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{})
		Expect(err).To(BeNil())

		// Initialize River workers
		workers := river.NewWorkers()
		river.AddWorker(workers, rvtools.NewRVToolsWorker(s, nil))

		// Create River client with test settings
		riverClient, err = river.NewClient(riverpgxv5.New(dbPool), &river.Config{
			Queues: map[string]river.QueueConfig{
				river.QueueDefault: {MaxWorkers: 4},
			},
			Workers:                     workers,
			CompletedJobRetentionPeriod: 10 * time.Second,
		})
		Expect(err).To(BeNil())

		// Start River
		err = riverClient.Start(ctx)
		Expect(err).To(BeNil())

		// Create assessment service with River client
		svc = service.NewAssessmentService(s, nil, riverClient)
	})

	AfterAll(func() {
		if riverClient != nil {
			_ = riverClient.Stop(ctx)
		}
		if dbPool != nil {
			dbPool.Close()
		}
		s.Close()
	})

	AfterEach(func() {
		gormdb.Exec("DELETE FROM river_job;")
		gormdb.Exec("DELETE FROM snapshots;")
		gormdb.Exec("DELETE FROM assessments;")
	})

	Describe("End-to-end processing", func() {
		It("processes minimal RVTools file successfully", func() {
			fileData := createMinimalTestExcel()

			form := mappers.AssessmentCreateForm{
				ID:          uuid.New(),
				Name:        "Minimal Test",
				OrgID:       "test-org",
				Username:    "testuser",
				Source:      service.SourceTypeRvtools,
				RVToolsFile: fileData,
			}

			assessment, err := svc.CreateRvtoolsAssessment(ctx, form)
			Expect(err).To(BeNil())
			Expect(assessment.Snapshots).To(HaveLen(1))

			snapshotID := assessment.Snapshots[0].ID

			// Wait for River to process the job
			Eventually(func() model.SnapshotStatus {
				snapshot, err := s.Snapshot().Get(ctx, snapshotID)
				if err != nil {
					return model.SnapshotStatusPending
				}
				return snapshot.Status
			}, "30s", "500ms").Should(Equal(model.SnapshotStatusReady))

			// Verify inventory was stored
			snapshot, err := s.Snapshot().Get(ctx, snapshotID)
			Expect(err).To(BeNil())
			Expect(snapshot.Inventory).ToNot(BeNil())
		})

		It("processes comprehensive RVTools file successfully", func() {
			fileData := createComprehensiveVMDataExcel()

			form := mappers.AssessmentCreateForm{
				ID:          uuid.New(),
				Name:        "Comprehensive Test",
				OrgID:       "test-org",
				Username:    "testuser",
				Source:      service.SourceTypeRvtools,
				RVToolsFile: fileData,
			}

			assessment, err := svc.CreateRvtoolsAssessment(ctx, form)
			Expect(err).To(BeNil())

			snapshotID := assessment.Snapshots[0].ID

			Eventually(func() model.SnapshotStatus {
				snapshot, err := s.Snapshot().Get(ctx, snapshotID)
				if err != nil {
					return model.SnapshotStatusPending
				}
				return snapshot.Status
			}, "60s", "1s").Should(Equal(model.SnapshotStatusReady))

			snapshot, err := s.Snapshot().Get(ctx, snapshotID)
			Expect(err).To(BeNil())
			Expect(snapshot.Inventory).ToNot(BeNil())
		})
	})

	Describe("Error handling", func() {
		It("handles invalid base64 data", func() {
			// Create a real assessment with pending snapshot
			assessment := model.Assessment{
				Name:       "Invalid Base64 Test",
				OrgID:      "test-org",
				Username:   "testuser",
				SourceType: "rvtools",
			}

			// Create pending snapshot
			snapshot := model.Snapshot{
				Status: model.SnapshotStatusPending,
			}

			// Create assessment with pending snapshot
			err := s.Assessment().Create(ctx, &assessment, &snapshot)
			Expect(err).To(BeNil())

			snapshotID := snapshot.ID

			// Manually create a job with invalid base64
			_, err = riverClient.Insert(ctx, rvtools.RVToolsJobArgs{
				SnapshotID:   snapshotID,
				AssessmentID: assessment.ID,
				RVToolsData:  "!!!invalid-base64!!!",
			}, nil)
			Expect(err).To(BeNil())

			// Wait for snapshot to transition to failed
			Eventually(func() model.SnapshotStatus {
				snapshot, err := s.Snapshot().Get(ctx, snapshotID)
				if err != nil {
					return model.SnapshotStatusPending
				}
				return snapshot.Status
			}, "10s", "500ms").Should(Equal(model.SnapshotStatusFailed))

			// Verify error message exists
			finalSnapshot, err := s.Snapshot().Get(ctx, snapshotID)
			Expect(err).To(BeNil())
			Expect(finalSnapshot.Error).ToNot(BeNil())
			Expect(*finalSnapshot.Error).To(ContainSubstring("base64"))
		})

		It("handles corrupted Excel file", func() {
			// Create a file that's not valid Excel
			fileData := []byte("This is not a valid Excel file")

			form := mappers.AssessmentCreateForm{
				ID:          uuid.New(),
				Name:        "Invalid File Test",
				OrgID:       "test-org",
				Username:    "testuser",
				Source:      service.SourceTypeRvtools,
				RVToolsFile: fileData,
			}

			assessment, err := svc.CreateRvtoolsAssessment(ctx, form)
			Expect(err).To(BeNil())
			snapshotID := assessment.Snapshots[0].ID

			// Wait for job to fail
			Eventually(func() model.SnapshotStatus {
				snapshot, err := s.Snapshot().Get(ctx, snapshotID)
				if err != nil {
					return model.SnapshotStatusPending
				}
				return snapshot.Status
			}, "30s", "500ms").Should(Equal(model.SnapshotStatusFailed))

			// Verify error message
			snapshot, err := s.Snapshot().Get(ctx, snapshotID)
			Expect(err).To(BeNil())
			Expect(snapshot.Error).ToNot(BeNil())
		})
	})

	Describe("Base64 encoding/decoding", func() {
		It("correctly encodes and decodes file data", func() {
			fileData := createMinimalTestExcel()
			encoded := base64.StdEncoding.EncodeToString(fileData)
			decoded, err := base64.StdEncoding.DecodeString(encoded)

			Expect(err).To(BeNil())
			Expect(decoded).To(Equal(fileData))
		})
	})
})
