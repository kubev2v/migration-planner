package mappers

import (
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

func PartnerRequestCreateToModel(req api.PartnerRequestCreate) model.PartnerCustomer {
	return model.PartnerCustomer{
		Name:         req.Name,
		ContactName:  req.ContactName,
		ContactPhone: req.ContactPhone,
		Email:        req.Email,
		Location:     req.Location,
	}
}

func PartnerRequestUpdateToModel(req api.PartnerRequestUpdate) model.Request {
	r := model.Request{
		Status: model.RequestStatus(req.Status),
	}
	if req.Reason != nil {
		r.Reason = *req.Reason
	}
	return r
}
