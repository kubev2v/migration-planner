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
		AcceptedAt:    pc.AcceptedAt,
		TerminatedAt:  pc.TerminatedAt,
	}
}

func mapRequestStatus(s model.RequestStatus) api.PartnerRequestStatus {
	switch s {
	case model.RequestStatusPending:
		return api.PartnerRequestStatusPending
	case model.RequestStatusAccepted:
		return api.PartnerRequestStatusAccepted
	case model.RequestStatusRejected:
		return api.PartnerRequestStatusRejected
	case model.RequestStatusCancelled:
		return api.PartnerRequestStatusCancelled
	default:
		return api.PartnerRequestStatus(s)
	}
}

func PartnerRequestListToApi(pcs model.PartnerCustomerList) api.PartnerRequestList {
	result := make(api.PartnerRequestList, len(pcs))
	for i, pc := range pcs {
		result[i] = PartnerRequestToApi(pc)
	}
	return result
}
