package v1alpha1_test

import (
	"context"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func createTestAssessmentForEnhancementDataHandler(id uuid.UUID, username, orgID string) *model.Assessment {
	return &model.Assessment{
		ID:       id,
		Name:     "test-assessment",
		OrgID:    orgID,
		Username: username,
	}
}

var _ = Describe("enhancement data handler", func() {
	var (
		mockStore    *MockStore
		handler      *handlers.ServiceHandler
		ctx          context.Context
		user         auth.User
		assessmentID uuid.UUID
	)

	BeforeEach(func() {
		mockStore = NewMockStore()
		user = auth.User{
			Username:     "test-user",
			Organization: "test-org",
		}
		ctx = auth.NewTokenContext(context.Background(), user)
		assessmentID = uuid.New()

		mockStore.assessments[assessmentID] = createTestAssessmentForEnhancementDataHandler(assessmentID, user.Username, user.Organization)
		handler = handlers.NewServiceHandler(
			nil,
			service.NewAssessmentService(mockStore, nil, nil),
			nil,
			nil,
			nil,
			nil,
			nil,
			service.NewAssessmentEnhancementDataService(mockStore),
		)
	})

	Describe("SaveAssessmentEnhancementData", func() {
		It("returns 200 with valid request", func() {
			env := api.DeployedEnvironmentInputEnvironment("on_premises")
			request := &api.EnhancementData{
				DeployedEnvironment: &api.DeployedEnvironmentInput{
					Environment: &env,
				},
			}

			resp, err := handler.SaveAssessmentEnhancementData(ctx, server.SaveAssessmentEnhancementDataRequestObject{
				Id:   assessmentID,
				Body: request,
			})

			Expect(err).To(BeNil())
			Expect(resp).NotTo(BeNil())
			response, ok := resp.(server.SaveAssessmentEnhancementData200JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(response.DeployedEnvironment).NotTo(BeNil())
			Expect(*response.DeployedEnvironment.Environment).To(Equal(api.DeployedEnvironmentInputEnvironment("on_premises")))
		})

		It("returns 200 with empty data", func() {
			request := &api.EnhancementData{}

			resp, err := handler.SaveAssessmentEnhancementData(ctx, server.SaveAssessmentEnhancementDataRequestObject{
				Id:   assessmentID,
				Body: request,
			})

			Expect(err).To(BeNil())
			_, ok := resp.(server.SaveAssessmentEnhancementData200JSONResponse)
			Expect(ok).To(BeTrue())
		})

		It("returns 400 when body is nil", func() {
			resp, err := handler.SaveAssessmentEnhancementData(ctx, server.SaveAssessmentEnhancementDataRequestObject{
				Id:   assessmentID,
				Body: nil,
			})

			Expect(err).To(BeNil())
			_, ok := resp.(server.SaveAssessmentEnhancementData400JSONResponse)
			Expect(ok).To(BeTrue())
		})

		It("returns 404 when assessment does not exist", func() {
			request := &api.EnhancementData{}

			resp, err := handler.SaveAssessmentEnhancementData(ctx, server.SaveAssessmentEnhancementDataRequestObject{
				Id:   uuid.New(),
				Body: request,
			})

			Expect(err).To(BeNil())
			_, ok := resp.(server.SaveAssessmentEnhancementData404JSONResponse)
			Expect(ok).To(BeTrue())
		})

		It("returns 400 when an enum field has an invalid value", func() {
			env := api.DeployedEnvironmentInputEnvironment("not_a_real_environment")
			request := &api.EnhancementData{
				DeployedEnvironment: &api.DeployedEnvironmentInput{
					Environment: &env,
				},
			}

			resp, err := handler.SaveAssessmentEnhancementData(ctx, server.SaveAssessmentEnhancementDataRequestObject{
				Id:   assessmentID,
				Body: request,
			})

			Expect(err).To(BeNil())
			_, ok := resp.(server.SaveAssessmentEnhancementData400JSONResponse)
			Expect(ok).To(BeTrue())
		})

		It("returns 400 when a counter is negative", func() {
			negative := -1
			request := &api.EnhancementData{
				VmwareVersionCounts: &api.VMwareVersionCountsInput{
					PerpetualLicensesCount: &negative,
				},
			}

			resp, err := handler.SaveAssessmentEnhancementData(ctx, server.SaveAssessmentEnhancementDataRequestObject{
				Id:   assessmentID,
				Body: request,
			})

			Expect(err).To(BeNil())
			_, ok := resp.(server.SaveAssessmentEnhancementData400JSONResponse)
			Expect(ok).To(BeTrue())
		})

		It("returns 403 when authz denies access", func() {
			forbiddenHandler := handlers.NewServiceHandler(
				nil,
				&ForbiddenAssessmentService{},
				nil, nil, nil, nil, nil,
				service.NewAssessmentEnhancementDataService(mockStore),
			)

			request := &api.EnhancementData{}

			resp, err := forbiddenHandler.SaveAssessmentEnhancementData(ctx, server.SaveAssessmentEnhancementDataRequestObject{
				Id:   assessmentID,
				Body: request,
			})

			Expect(err).To(BeNil())
			_, ok := resp.(server.SaveAssessmentEnhancementData403JSONResponse)
			Expect(ok).To(BeTrue())
		})

		It("overwrites previous data on second save", func() {
			env1 := api.DeployedEnvironmentInputEnvironment("on_premises")
			first := &api.EnhancementData{
				DeployedEnvironment: &api.DeployedEnvironmentInput{
					Environment: &env1,
				},
			}

			_, err := handler.SaveAssessmentEnhancementData(ctx, server.SaveAssessmentEnhancementDataRequestObject{
				Id:   assessmentID,
				Body: first,
			})
			Expect(err).To(BeNil())

			env2 := api.DeployedEnvironmentInputEnvironment("on_cloud")
			second := &api.EnhancementData{
				DeployedEnvironment: &api.DeployedEnvironmentInput{
					Environment: &env2,
				},
			}

			resp, err := handler.SaveAssessmentEnhancementData(ctx, server.SaveAssessmentEnhancementDataRequestObject{
				Id:   assessmentID,
				Body: second,
			})
			Expect(err).To(BeNil())
			response, ok := resp.(server.SaveAssessmentEnhancementData200JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(*response.DeployedEnvironment.Environment).To(Equal(api.DeployedEnvironmentInputEnvironment("on_cloud")))

			getResp, err := handler.GetAssessmentEnhancementData(ctx, server.GetAssessmentEnhancementDataRequestObject{
				Id: assessmentID,
			})
			Expect(err).To(BeNil())
			getResponse, ok := getResp.(server.GetAssessmentEnhancementData200JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(*getResponse.DeployedEnvironment.Environment).To(Equal(api.DeployedEnvironmentInputEnvironment("on_cloud")))
		})
	})

	Describe("GetAssessmentEnhancementData", func() {
		It("returns 200 with empty data when no inputs saved", func() {
			resp, err := handler.GetAssessmentEnhancementData(ctx, server.GetAssessmentEnhancementDataRequestObject{
				Id: assessmentID,
			})

			Expect(err).To(BeNil())
			response, ok := resp.(server.GetAssessmentEnhancementData200JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(response.DeployedEnvironment).To(BeNil())
		})

		It("returns 200 with stored data after save", func() {
			env := api.DeployedEnvironmentInputEnvironment("managed_services")
			request := &api.EnhancementData{
				DeployedEnvironment: &api.DeployedEnvironmentInput{
					Environment: &env,
				},
			}

			_, err := handler.SaveAssessmentEnhancementData(ctx, server.SaveAssessmentEnhancementDataRequestObject{
				Id:   assessmentID,
				Body: request,
			})
			Expect(err).To(BeNil())

			resp, err := handler.GetAssessmentEnhancementData(ctx, server.GetAssessmentEnhancementDataRequestObject{
				Id: assessmentID,
			})

			Expect(err).To(BeNil())
			response, ok := resp.(server.GetAssessmentEnhancementData200JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(response.DeployedEnvironment).NotTo(BeNil())
			Expect(*response.DeployedEnvironment.Environment).To(Equal(api.DeployedEnvironmentInputEnvironment("managed_services")))
		})

		It("returns 404 when assessment does not exist", func() {
			resp, err := handler.GetAssessmentEnhancementData(ctx, server.GetAssessmentEnhancementDataRequestObject{
				Id: uuid.New(),
			})

			Expect(err).To(BeNil())
			_, ok := resp.(server.GetAssessmentEnhancementData404JSONResponse)
			Expect(ok).To(BeTrue())
		})

		It("returns 403 when authz denies access", func() {
			forbiddenHandler := handlers.NewServiceHandler(
				nil,
				&ForbiddenAssessmentService{},
				nil, nil, nil, nil, nil,
				service.NewAssessmentEnhancementDataService(mockStore),
			)

			resp, err := forbiddenHandler.GetAssessmentEnhancementData(ctx, server.GetAssessmentEnhancementDataRequestObject{
				Id: assessmentID,
			})

			Expect(err).To(BeNil())
			_, ok := resp.(server.GetAssessmentEnhancementData403JSONResponse)
			Expect(ok).To(BeTrue())
		})
	})
})
