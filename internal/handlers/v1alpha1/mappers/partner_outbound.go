package mappers

import (
	"fmt"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

func PartnerRequestToApi(pc model.PartnerCustomer) (api.PartnerRequest, error) {
	if pc.Partner == nil {
		return api.PartnerRequest{}, fmt.Errorf("partner relation not loaded for request %s", pc.ID)
	}
	r := api.PartnerRequest{
		Id:            pc.ID,
		Username:      pc.Username,
		RequestStatus: mapRequestStatus(pc.RequestStatus),
		Name:          pc.Name,
		ContactName:   pc.ContactName,
		ContactPhone:  pc.ContactPhone,
		Email:         pc.Email,
		Location:      pc.Location,
		Reason:        pc.Reason,
		AcceptedAt:    pc.AcceptedAt,
		TerminatedAt:  pc.TerminatedAt,
		CreatedAt:     pc.CreatedAt,
	}
	r.Partner = api.PartnerSummary{
		Id:      pc.Partner.ID,
		Name:    pc.Partner.Name,
		Company: pc.Partner.Company,
		Icon:    pc.Partner.Icon,
	}
	return r, nil
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

func PartnerRequestListToApi(pcs model.PartnerCustomerList) (api.PartnerRequestList, error) {
	result := make(api.PartnerRequestList, len(pcs))
	for i, pc := range pcs {
		r, err := PartnerRequestToApi(pc)
		if err != nil {
			return nil, err
		}
		result[i] = r
	}
	return result, nil
}
