package v1alpha1_test

import (
	"context"
	"errors"

	model2 "github.com/kubev2v/migration-planner/internal/model"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/api/server"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type assessmentV2ServiceStub struct {
	service.AssessmentServicer
	assessment          *model2.Assessment
	deleteErr           error
	deletedAssessmentID uuid.UUID
	deletedVCenterID    uuid.UUID
}

func (s *assessmentV2ServiceStub) GetAssessment(_ context.Context, id uuid.UUID) (*model2.Assessment, error) {
	if id != s.assessment.ID {
		return nil, service.NewErrAssessmentNotFound(id)
	}
	return s.assessment, nil
}

func (s *assessmentV2ServiceStub) DeleteVCenterInventory(_ context.Context, assessmentID, vcenterID uuid.UUID) error {
	s.deletedAssessmentID = assessmentID
	s.deletedVCenterID = vcenterID
	return s.deleteErr
}

var _ = Describe("assessment v2 handler", func() {
	var (
		vcenterID1 = uuid.MustParse("11111111-1111-4111-8111-111111111111")
		vcenterID2 = uuid.MustParse("22222222-2222-4222-8222-222222222222")
		clusterID1 = uuid.MustParse("33333333-3333-4333-8333-333333333333")
		clusterID2 = uuid.MustParse("44444444-4444-4444-8444-444444444444")
	)

	inventory := func(vcenterID string, isSubset bool, totalVMs int, clusterID uuid.UUID) model2.AssessmentInventory {
		return model2.AssessmentInventory{
			VCenterID: vcenterID,
			IsSubset:  isSubset,
			Version:   model.InventoryVersionV2,
			Inventory: model2.Inventory{
				VcenterId: vcenterID,
				Vcenter: &model2.InventoryData{
					Vms: model2.VMs{
						Total: totalVMs,
					},
				},
				Clusters: map[string]model2.InventoryData{
					clusterID.String(): {
						Vms: model2.VMs{
							Total: totalVMs,
						},
					},
				},
			},
		}
	}

	getAssessment := func(sourceType string, main model2.AssessmentInventory, subset []model2.AssessmentInventory) server.GetAssessmentV2200JSONResponse {
		GinkgoHelper()

		assessment := &model2.Assessment{
			ID:                uuid.New(),
			SourceType:        sourceType,
			MainInventory:     main,
			SubsetInventories: subset,
		}
		handler := handlers.NewServiceHandler(nil, &assessmentV2ServiceStub{assessment: assessment}, nil, nil, nil, nil, nil, nil)

		response, err := handler.GetAssessmentV2(context.Background(), server.GetAssessmentV2RequestObject{Id: assessment.ID})
		Expect(err).NotTo(HaveOccurred())
		Expect(response).To(BeAssignableToTypeOf(server.GetAssessmentV2200JSONResponse{}))
		return response.(server.GetAssessmentV2200JSONResponse)
	}

	Context("GetAssessmentV2", func() {
		It("returns RVTools aggregate data and only the individual vCenters",
			func() {
				main := inventory("", false, 30, uuid.Nil)
				subset := []model2.AssessmentInventory{
					inventory(vcenterID1.String(), true, 10, clusterID1),
					inventory(vcenterID2.String(), true, 20, clusterID2),
				}

				response := getAssessment(service.SourceTypeRvtools, main, subset)

				Expect(response.Data.Data.Vms.Total).To(Equal(30))
				Expect(response.Vcenters).To(ConsistOf(
					HaveField("Id", vcenterID1.String()),
					HaveField("Id", vcenterID2.String()),
				))
			},
		)

		It("returns agent main inventory data with one vCenter subresource",
			func() {
				main := inventory(vcenterID1.String(), false, 30, clusterID1)

				s1 := inventory(vcenterID1.String(), true, 10, clusterID2)

				response := getAssessment(service.SourceTypeAgent, main, []model2.AssessmentInventory{s1})

				Expect(response.Data.Id).To(Equal(vcenterID1.String()))
				Expect(response.Data.Data.Vms.Total).To(Equal(30))
				Expect(response.Vcenters).To(ConsistOf(HaveField("Id", vcenterID1.String())))
			},
		)
	})

	Context("DeleteVCenterInventory", func() {
		var assessmentID uuid.UUID
		var stub *assessmentV2ServiceStub
		var handler *handlers.ServiceHandler

		BeforeEach(func() {
			assessmentID = uuid.New()
			stub = &assessmentV2ServiceStub{}
			handler = handlers.NewServiceHandler(nil, stub, nil, nil, nil, nil, nil, nil)
		})

		deleteVCenter := func() server.DeleteVCenterResponseObject {
			GinkgoHelper()
			response, err := handler.DeleteVCenter(context.Background(), server.DeleteVCenterRequestObject{
				Id: assessmentID, Vid: vcenterID1,
			})
			Expect(err).NotTo(HaveOccurred())
			return response
		}

		It("passes both IDs to the service and returns 204", func() {
			response := deleteVCenter()
			Expect(response).To(BeAssignableToTypeOf(server.DeleteVCenter204Response{}))
			Expect(stub.deletedAssessmentID).To(Equal(assessmentID))
			Expect(stub.deletedVCenterID).To(Equal(vcenterID1))
		})

		It("returns 404 when the service cannot find the vCenter", func() {
			stub.deleteErr = service.NewErrResourceNotFound(vcenterID1, "vcenter")
			Expect(deleteVCenter()).To(BeAssignableToTypeOf(server.DeleteVCenter404JSONResponse{}))
		})

		It("returns 403 when deletion is forbidden", func() {
			stub.deleteErr = service.NewErrForbidden("assessment", assessmentID.String())
			Expect(deleteVCenter()).To(BeAssignableToTypeOf(server.DeleteVCenter403JSONResponse{}))
		})

		It("returns 500 for an unexpected service error", func() {
			stub.deleteErr = errors.New("database unavailable")
			Expect(deleteVCenter()).To(BeAssignableToTypeOf(server.DeleteVCenter500JSONResponse{}))
		})
	})
})
