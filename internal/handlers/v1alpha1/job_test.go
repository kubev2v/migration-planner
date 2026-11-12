package v1alpha1_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/client"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	svcmodel "github.com/kubev2v/migration-planner/internal/model"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
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
		_ = s.Close()
	})

	Context("GetJob", func() {
		It("returns 404 when job not found", func() {
			user := auth.User{
				Username:     "test-user",
				Organization: "test-org",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil, nil), service.NewJobService(s, nil, ""), service.NewSizerService(sizerClient, s), nil, nil, nil, nil)
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil, nil), service.NewJobService(s, nil, ""), service.NewSizerService(sizerClient, s), nil, nil, nil, nil)
			resp, err := srv.CancelJob(ctx, server.CancelJobRequestObject{Id: 123})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CancelJob404JSONResponse{}).String()))
		})
	})

	Context("CreateRVToolsAssessment - name validation", func() {
		var user auth.User
		var ctx context.Context
		var srv *handlers.ServiceHandler

		// Helper function to create a multipart reader for testing.
		// File content is prefixed with XLSX/ZIP magic bytes for validation.
		createMultipartReader := func(name string, fileContent string) *multipart.Reader {
			var b bytes.Buffer
			w := multipart.NewWriter(&b)

			// Add name field
			namePart, _ := w.CreateFormField("name")
			_, _ = io.WriteString(namePart, name)

			// Add file field with XLSX magic bytes prefix
			filePart, _ := w.CreateFormFile("file", "test.xlsx")
			_, _ = filePart.Write([]byte{0x50, 0x4B, 0x03, 0x04})
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
			srv = handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil, nil), service.NewJobService(s, nil, ""), service.NewSizerService(sizerClient, s), nil, nil, nil, nil)
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

type rvtoolsFailingReader struct{ err error }

func (r rvtoolsFailingReader) Read([]byte) (int, error) { return 0, r.err }

type rvtoolsFailingJobStore struct {
	store.Job
	err error
}

func (s *rvtoolsFailingJobStore) Get(context.Context, int64) (*store.JobRow, error) {
	return nil, s.err
}

type rvtoolsHandlerStore struct {
	store.Store
	job store.Job
}

func (s *rvtoolsHandlerStore) Job() store.Job { return s.job }

var _ = Describe("RVTools safe API errors", func() {

	DescribeTable("does not expose upload implementation errors", func(version int, problem string) {
		cause := errors.New("reading /var/lib/agent/private.xlsx: SELECT secret FROM internal")
		ctx := auth.NewTokenContext(context.Background(), auth.User{Username: "user", Organization: "org"})
		srv := handlers.NewServiceHandler(nil, &assessmentV2ServiceStub{assessment: &svcmodel.Assessment{}}, nil, nil, nil, nil, nil, nil)
		body := multipart.NewReader(rvtoolsFailingReader{cause}, "boundary")
		expectedStatus := http.StatusBadRequest
		expectedMessage := "failed to parse form"
		if problem != "read" {
			var data bytes.Buffer
			writer := multipart.NewWriter(&data)
			if version != 3 {
				Expect(writer.WriteField("name", "assessment")).To(Succeed())
			}
			part, err := writer.CreateFormFile("file", "private.xlsx")
			Expect(err).NotTo(HaveOccurred())
			_, err = part.Write([]byte("not an Excel workbook"))
			Expect(err).NotTo(HaveOccurred())
			Expect(writer.Close()).To(Succeed())
			body = multipart.NewReader(&data, writer.Boundary())
			if problem == "save" {
				GinkgoT().Setenv("TMPDIR", filepath.Join(GinkgoT().TempDir(), "missing"))
				expectedStatus = http.StatusInternalServerError
				expectedMessage = "failed to create temp file"
			} else {
				expectedMessage = "file is not a valid Excel file"
			}
		}
		recorder := httptest.NewRecorder()
		switch version {
		case 3:
			response, err := srv.UploadRvtoolsToAssessment(ctx, server.UploadRvtoolsToAssessmentRequestObject{Body: body})
			Expect(err).NotTo(HaveOccurred())
			Expect(response.VisitUploadRvtoolsToAssessmentResponse(recorder)).To(Succeed())
		default:
			response, err := srv.CreateRVToolsAssessmentV2(ctx, server.CreateRVToolsAssessmentV2RequestObject{Body: body})
			Expect(err).NotTo(HaveOccurred())
			Expect(response.VisitCreateRVToolsAssessmentV2Response(recorder)).To(Succeed())
		}
		Expect(recorder.Code).To(Equal(expectedStatus))
		Expect(recorder.Body.String()).To(MatchJSON(`{"message":"` + expectedMessage + `"}`))
	},
		Entry("v2 read failure", 2, "read"),
		Entry("v2 save failure", 2, "save"),
		Entry("v2 invalid signature", 2, "signature"),
		Entry("update read failure", 3, "read"), Entry("update save failure", 3, "save"), Entry("update invalid signature", 3, "signature"),
	)

	It("hides job-store errors from polling and cancellation responses", func() {
		cause := errors.New("SELECT private_column FROM /private/database")
		st := &rvtoolsHandlerStore{job: &rvtoolsFailingJobStore{err: cause}}
		jobs := service.NewJobService(st, nil, "")
		srv := handlers.NewServiceHandler(nil, nil, jobs, nil, nil, nil, nil, nil)
		ctx := auth.NewTokenContext(context.Background(), auth.User{Username: "user", Organization: "org"})
		recorder := httptest.NewRecorder()
		response, err := srv.GetJob(ctx, server.GetJobRequestObject{Id: 1})
		Expect(err).NotTo(HaveOccurred())
		Expect(response.VisitGetJobResponse(recorder)).To(Succeed())
		Expect(recorder.Code).To(Equal(http.StatusInternalServerError))
		Expect(recorder.Body.String()).To(MatchJSON(`{"message":"failed to get job"}`))
		recorder = httptest.NewRecorder()
		cancelled, err := srv.CancelJob(ctx, server.CancelJobRequestObject{Id: 1})
		Expect(err).NotTo(HaveOccurred())
		Expect(cancelled.VisitCancelJobResponse(recorder)).To(Succeed())
		Expect(recorder.Code).To(Equal(http.StatusInternalServerError))
		Expect(recorder.Body.String()).To(MatchJSON(`{"message":"failed to cancel job"}`))
	})
})
