package v1alpha1_test

import (
	"context"
	"reflect"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("job handler", Ordered, func() {
	var (
		s store.Store
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())
		s = store.NewStore(db)
	})

	AfterAll(func() {
		s.Close()
	})

	Context("GetJob", func() {
		It("returns 404 when job not found", func() {
			user := auth.User{
				Username:     "test-user",
				Organization: "test-org",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), service.NewJobService(s, nil))
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

			srv := handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil), service.NewJobService(s, nil))
			resp, err := srv.CancelJob(ctx, server.CancelJobRequestObject{Id: 123})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.CancelJob404JSONResponse{}).String()))
		})
	})
})
