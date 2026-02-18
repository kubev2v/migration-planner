package v1alpha1_test

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), service.NewJobService(s, nil), service.NewSizerService(sizerClient, s), nil)
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), service.NewJobService(s, nil), service.NewSizerService(sizerClient, s), nil)
			resp, err := srv.CancelJob(ctx, server.CancelJobRequestObject{Id: 123})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CancelJob404JSONResponse{}).String()))
		})
	})

	Context("CreateRVToolsAssessment - name validation", func() {
		var user auth.User
		var ctx context.Context
		var srv *handlers.ServiceHandler

		// Helper function to create a multipart reader for testing
		createMultipartReader := func(name string, fileContent string) *multipart.Reader {
			var b bytes.Buffer
			w := multipart.NewWriter(&b)

			// Add name field
			namePart, _ := w.CreateFormField("name")
			_, _ = io.WriteString(namePart, name)

			// Add file field
			filePart, _ := w.CreateFormFile("file", "test.xlsx")
			_, _ = io.WriteString(filePart, fileContent)

			_ = w.Close()

			return multipart.NewReader(&b, w.Boundary())
		}

		BeforeEach(func() {
			user = auth.User{
				Username:     "test-user",
				Organization: "test-org",
				FirstName:    "Test",
				LastName:     "User",
			}
			ctx = auth.NewTokenContext(context.TODO(), user)
			srv = handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), service.NewJobService(s, nil), service.NewSizerService(sizerClient, s), nil)
		})

		It("returns 400 when name is empty", func() {
			reader := createMultipartReader("", "file content")

			resp, err := srv.CreateRVToolsAssessment(ctx, server.CreateRVToolsAssessmentRequestObject{
				Body: reader,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateRVToolsAssessment400JSONResponse{}).String()))

			errorResp := resp.(server.CreateRVToolsAssessment400JSONResponse)
			Expect(errorResp.Message).To(ContainSubstring("The provided name"))
			Expect(errorResp.Message).To(ContainSubstring("invalid"))
		})

		It("returns 400 when name contains spaces", func() {
			reader := createMultipartReader("invalid name with spaces", "file content")

			resp, err := srv.CreateRVToolsAssessment(ctx, server.CreateRVToolsAssessmentRequestObject{
				Body: reader,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateRVToolsAssessment400JSONResponse{}).String()))

			errorResp := resp.(server.CreateRVToolsAssessment400JSONResponse)
			Expect(errorResp.Message).To(ContainSubstring("The provided name"))
			Expect(errorResp.Message).To(ContainSubstring("invalid"))
		})

		It("returns 400 when name contains special characters", func() {
			reader := createMultipartReader("invalid@name#with$special%chars", "file content")

			resp, err := srv.CreateRVToolsAssessment(ctx, server.CreateRVToolsAssessmentRequestObject{
				Body: reader,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateRVToolsAssessment400JSONResponse{}).String()))

			errorResp := resp.(server.CreateRVToolsAssessment400JSONResponse)
			Expect(errorResp.Message).To(ContainSubstring("The provided name"))
			Expect(errorResp.Message).To(ContainSubstring("invalid"))
		})

		It("returns 400 when name exceeds 100 characters", func() {
			// Create a name with 101 characters
			longName := strings.Repeat("a", 101)
			reader := createMultipartReader(longName, "file content")

			resp, err := srv.CreateRVToolsAssessment(ctx, server.CreateRVToolsAssessmentRequestObject{
				Body: reader,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateRVToolsAssessment400JSONResponse{}).String()))

			errorResp := resp.(server.CreateRVToolsAssessment400JSONResponse)
			Expect(errorResp.Message).To(ContainSubstring("The provided name"))
			Expect(errorResp.Message).To(ContainSubstring("invalid"))
		})

		It("returns 400 when name contains slash", func() {
			reader := createMultipartReader("invalid/name", "file content")

			resp, err := srv.CreateRVToolsAssessment(ctx, server.CreateRVToolsAssessmentRequestObject{
				Body: reader,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateRVToolsAssessment400JSONResponse{}).String()))

			errorResp := resp.(server.CreateRVToolsAssessment400JSONResponse)
			Expect(errorResp.Message).To(ContainSubstring("The provided name"))
			Expect(errorResp.Message).To(ContainSubstring("invalid"))
		})

		It("returns 400 when name contains backslash", func() {
			reader := createMultipartReader("invalid\\name", "file content")

			resp, err := srv.CreateRVToolsAssessment(ctx, server.CreateRVToolsAssessmentRequestObject{
				Body: reader,
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CreateRVToolsAssessment400JSONResponse{}).String()))

			errorResp := resp.(server.CreateRVToolsAssessment400JSONResponse)
			Expect(errorResp.Message).To(ContainSubstring("The provided name"))
			Expect(errorResp.Message).To(ContainSubstring("invalid"))
		})
	})
})
