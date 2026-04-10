package mappers

import (
	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

func PartnerRequestToApi(pc model.PartnerCustomer) api.PartnerRequest {
	partnerID, _ := uuid.Parse(pc.PartnerID)
	return api.PartnerRequest{
		Id:            pc.ID,
		Username:      pc.Username,
		PartnerId:     partnerID,
		RequestStatus: mapRequestStatus(pc.RequestStatus),
		Name:          pc.Name,
		ContactName:   pc.ContactName,
		ContactPhone:  pc.ContactPhone,
		Email:         pc.Email,
		Location:      pc.Location,
		Reason:        pc.Reason,
	}
}

func mapRequestStatus(s model.RequestStatus) api.PartnerRequestRequestStatus {
	switch s {
	case model.RequestStatusPending:
		return api.PartnerRequestRequestStatusAwaiting
	case model.RequestStatusAccepted:
		return api.PartnerRequestRequestStatusAccepted
	case model.RequestStatusRejected:
		return api.PartnerRequestRequestStatusRejected
	default:
		return api.PartnerRequestRequestStatus(s)
	}
}

func PartnerRequestListToApi(pcs model.PartnerCustomerList) api.PartnerRequestList {
	result := make(api.PartnerRequestList, len(pcs))
	for i, pc := range pcs {
		result[i] = PartnerRequestToApi(pc)
	}
	return result
}
