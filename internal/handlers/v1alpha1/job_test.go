package v1alpha1_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"time"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/client"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("job handler", Ordered, func() {
	var (
		s           store.Store
		testServer  *httptest.Server
		sizerClient *client.SizerClient
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())
		s = store.NewStore(db)

		// Create a minimal sizer client to prevent nil pointer panics
		// This test server responds to health checks to satisfy sizer service requirements
		testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		sizerClient = client.NewSizerClient(testServer.URL, 5*time.Second)
	})

	AfterAll(func() {
		if testServer != nil {
			testServer.Close()
		}
		s.Close()
	})

	Context("GetJob", func() {
		It("returns 404 when job not found", func() {
			user := auth.User{
				Username:     "test-user",
				Organization: "test-org",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), service.NewJobService(s, nil), service.NewSizerService(sizerClient, s))
			resp, err := srv.GetJob(ctx, server.GetJobRequestObject{Id: 123})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.GetJob404JSONResponse{}).String()))
		})
	})

	Context("CancelJob", func() {
		It("returns 404 when job not found", func() {
			user := auth.User{
				Username:     "test-user",
				Organization: "test-org",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), service.NewJobService(s, nil), service.NewSizerService(sizerClient, s))
			resp, err := srv.CancelJob(ctx, server.CancelJobRequestObject{Id: 123})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CancelJob404JSONResponse{}).String()))
		})
	})
})
