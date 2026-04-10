package v1alpha1

import (
	"context"
	"fmt"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// (GET /api/v1/partners)
func (h *ServiceHandler) ListPartners(ctx context.Context, request server.ListPartnersRequestObject) (server.ListPartnersResponseObject, error) {
	logger := log.NewDebugLogger("partner_handler").
		WithContext(ctx).
		Operation("list_partners").
		Build()

	groups, err := h.partnerSrv.ListPartners(ctx)
	if err != nil {
		logger.Error(err).Log()
		return server.ListPartners500JSONResponse{Message: fmt.Sprintf("failed to list partners: %v", err)}, nil
	}

	logger.Success().WithInt("count", len(groups)).Log()
	return server.ListPartners200JSONResponse(mappers.GroupListToApi(groups)), nil
}

// (GET /api/v1/partners/requests)
func (h *ServiceHandler) ListPartnerRequests(ctx context.Context, request server.ListPartnerRequestsRequestObject) (server.ListPartnerRequestsResponseObject, error) {
	logger := log.NewDebugLogger("partner_handler").
		WithContext(ctx).
		Operation("list_partner_requests").
		Build()

	authUser := auth.MustHaveUser(ctx)

	requests, err := h.partnerSrv.ListRequests(ctx, authUser)
	if err != nil {
		logger.Error(err).Log()
		return server.ListPartnerRequests500JSONResponse{Message: fmt.Sprintf("failed to list requests: %v", err)}, nil
	}

	logger.Success().WithInt("count", len(requests)).Log()
	return server.ListPartnerRequests200JSONResponse(mappers.PartnerRequestListToApi(requests)), nil
}

// (POST /api/v1/partners/{id}/request)
func (h *ServiceHandler) CreatePartnerRequest(ctx context.Context, request server.CreatePartnerRequestRequestObject) (server.CreatePartnerRequestResponseObject, error) {
	logger := log.NewDebugLogger("partner_handler").
		WithContext(ctx).
		Operation("create_partner_request").
		WithString("partner_id", request.Id.String()).
		Build()

	if request.Body == nil {
		return server.CreatePartnerRequest400JSONResponse{Message: "empty body"}, nil
	}

	authUser := auth.MustHaveUser(ctx)
	pc := mappers.PartnerRequestCreateToModel(*request.Body)

	created, err := h.partnerSrv.CreateRequest(ctx, authUser, request.Id.String(), pc)
	if err != nil {
		switch err.(type) {
		case *service.ErrInvalidRequest:
			return server.CreatePartnerRequest400JSONResponse{Message: err.Error()}, nil
		case *service.ErrActiveRequestExists:
			return server.CreatePartnerRequest400JSONResponse{Message: err.Error()}, nil
		case *service.ErrResourceNotFound:
			return server.CreatePartnerRequest404JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.CreatePartnerRequest500JSONResponse{Message: fmt.Sprintf("failed to create request: %v", err)}, nil
		}
	}

	logger.Success().WithString("username", created.Username).Log()
	return server.CreatePartnerRequest201JSONResponse(mappers.PartnerRequestToApi(*created)), nil
}

// (DELETE /api/v1/partners/requests/{id})
func (h *ServiceHandler) CancelPartnerRequest(ctx context.Context, request server.CancelPartnerRequestRequestObject) (server.CancelPartnerRequestResponseObject, error) {
	logger := log.NewDebugLogger("partner_handler").
		WithContext(ctx).
		Operation("cancel_partner_request").
		WithString("request_id", request.Id.String()).
		Build()

	authUser := auth.MustHaveUser(ctx)

	if err := h.partnerSrv.CancelRequest(ctx, authUser, request.Id); err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.CancelPartnerRequest404JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.CancelPartnerRequest500JSONResponse{Message: fmt.Sprintf("failed to cancel request: %v", err)}, nil
		}
	}

	logger.Success().Log()
	return server.CancelPartnerRequest200Response{}, nil
}

// (GET /api/v1/partners/{id})
func (h *ServiceHandler) GetPartner(ctx context.Context, request server.GetPartnerRequestObject) (server.GetPartnerResponseObject, error) {
	logger := log.NewDebugLogger("partner_handler").
		WithContext(ctx).
		Operation("get_partner").
		WithString("partner_id", request.Id.String()).
		Build()

	authUser := auth.MustHaveUser(ctx)

	group, err := h.partnerSrv.GetPartner(ctx, authUser, request.Id.String())
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.GetPartner404JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.GetPartner500JSONResponse{Message: fmt.Sprintf("failed to get partner: %v", err)}, nil
		}
	}

	logger.Success().WithString("partner_name", group.Name).Log()
	return server.GetPartner200JSONResponse(mappers.GroupToApi(group)), nil
}

// (DELETE /api/v1/partners/{id})
func (h *ServiceHandler) LeavePartner(ctx context.Context, request server.LeavePartnerRequestObject) (server.LeavePartnerResponseObject, error) {
	logger := log.NewDebugLogger("partner_handler").
		WithContext(ctx).
		Operation("leave_partner").
		WithString("partner_id", request.Id.String()).
		Build()

	authUser := auth.MustHaveUser(ctx)

	if err := h.partnerSrv.LeavePartner(ctx, authUser, request.Id.String()); err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.LeavePartner404JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.LeavePartner500JSONResponse{Message: fmt.Sprintf("failed to leave partner: %v", err)}, nil
		}
	}

	logger.Success().Log()
	return server.LeavePartner200Response{}, nil
}

// (GET /api/v1/customers)
func (h *ServiceHandler) ListCustomers(ctx context.Context, request server.ListCustomersRequestObject) (server.ListCustomersResponseObject, error) {
	logger := log.NewDebugLogger("partner_handler").
		WithContext(ctx).
		Operation("list_customers").
		Build()

	authUser := auth.MustHaveUser(ctx)

	customers, err := h.partnerSrv.ListCustomers(ctx, authUser)
	if err != nil {
		switch err.(type) {
		case *service.ErrInvalidRequest:
			return server.ListCustomers400JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			return server.ListCustomers403JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.ListCustomers500JSONResponse{Message: fmt.Sprintf("failed to list customers: %v", err)}, nil
		}
	}

	logger.Success().WithInt("count", len(customers)).Log()
	return server.ListCustomers200JSONResponse(mappers.PartnerRequestListToApi(customers)), nil
}

// (PUT /api/v1/partners/requests/{id})
func (h *ServiceHandler) UpdatePartnerRequest(ctx context.Context, request server.UpdatePartnerRequestRequestObject) (server.UpdatePartnerRequestResponseObject, error) {
	logger := log.NewDebugLogger("partner_handler").
		WithContext(ctx).
		Operation("update_partner_request").
		WithString("request_id", request.Id.String()).
		Build()

	if request.Body == nil {
		return server.UpdatePartnerRequest400JSONResponse{Message: "empty body"}, nil
	}

	switch request.Body.Status {
	case api.PartnerRequestUpdateStatusAccepted, api.PartnerRequestUpdateStatusRejected:
	default:
		return server.UpdatePartnerRequest400JSONResponse{Message: "invalid status"}, nil
	}

	authUser := auth.MustHaveUser(ctx)
	req := mappers.PartnerRequestUpdateToModel(*request.Body)

	updated, err := h.partnerSrv.UpdateRequest(ctx, authUser, request.Id, req)
	if err != nil {
		switch err.(type) {
		case *service.ErrInvalidRequest:
			return server.UpdatePartnerRequest400JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			return server.UpdatePartnerRequest403JSONResponse{Message: err.Error()}, nil
		case *service.ErrResourceNotFound:
			return server.UpdatePartnerRequest404JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.UpdatePartnerRequest500JSONResponse{Message: fmt.Sprintf("failed to update request: %v", err)}, nil
		}
	}

	logger.Success().WithString("username", updated.Username).WithString("status", string(updated.RequestStatus)).Log()
	return server.UpdatePartnerRequest200JSONResponse(mappers.PartnerRequestToApi(*updated)), nil
}

// (DELETE /api/v1/customers/{username})
func (h *ServiceHandler) RemoveCustomer(ctx context.Context, request server.RemoveCustomerRequestObject) (server.RemoveCustomerResponseObject, error) {
	logger := log.NewDebugLogger("partner_handler").
		WithContext(ctx).
		Operation("remove_customer").
		WithString("username", request.Username).
		Build()

	authUser := auth.MustHaveUser(ctx)

	if err := h.partnerSrv.RemoveCustomer(ctx, authUser, request.Username); err != nil {
		switch err.(type) {
		case *service.ErrInvalidRequest:
			return server.RemoveCustomer400JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			return server.RemoveCustomer403JSONResponse{Message: err.Error()}, nil
		case *service.ErrResourceNotFound:
			return server.RemoveCustomer404JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.RemoveCustomer500JSONResponse{Message: fmt.Sprintf("failed to remove customer: %v", err)}, nil
		}
	}

	logger.Success().Log()
	return server.RemoveCustomer200Response{}, nil
}
