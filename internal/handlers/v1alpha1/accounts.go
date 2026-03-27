package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// (GET /api/v1/identity)
func (h *ServiceHandler) GetIdentity(ctx context.Context, request server.GetIdentityRequestObject) (server.GetIdentityResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("get_identity").
		Build()

	authUser := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("username", authUser.Username).Log()

	identity, err := h.accountsSrv.GetIdentity(ctx, authUser)
	if err != nil {
		logger.Error(err).Log()
		return server.GetIdentity500JSONResponse{Message: fmt.Sprintf("failed to get identity: %v", err)}, nil
	}

	logger.Success().WithString("username", identity.Username).Log()
	return server.GetIdentity200JSONResponse(mappers.IdentityToApi(identity)), nil
}

// (GET /api/v1/groups)
func (h *ServiceHandler) ListGroups(ctx context.Context, request server.ListGroupsRequestObject) (server.ListGroupsResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("list_groups").
		Build()

	filter := store.NewGroupQueryFilter()
	if request.Params.Kind != nil {
		filter = filter.ByKind(string(*request.Params.Kind))
	}
	if request.Params.Name != nil {
		filter = filter.ByName(*request.Params.Name)
	}
	if request.Params.Company != nil {
		filter = filter.ByCompany(*request.Params.Company)
	}

	groups, err := h.accountsSrv.ListGroups(ctx, filter)
	if err != nil {
		logger.Error(err).Log()
		return server.ListGroups500JSONResponse{Message: fmt.Sprintf("failed to list groups: %v", err)}, nil
	}

	return server.ListGroups200JSONResponse(mappers.GroupListToApi(groups)), nil
}

// (POST /api/v1/groups)
func (h *ServiceHandler) CreateGroup(ctx context.Context, request server.CreateGroupRequestObject) (server.CreateGroupResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("create_group").
		Build()

	if request.Body == nil {
		return server.CreateGroup400JSONResponse{Message: "empty body"}, nil
	}

	if strings.TrimSpace(request.Body.Name) == "" {
		return server.CreateGroup400JSONResponse{Message: "name cannot be empty"}, nil
	}
	if strings.TrimSpace(request.Body.Company) == "" {
		return server.CreateGroup400JSONResponse{Message: "company cannot be empty"}, nil
	}

	group := mappers.GroupCreateToModel(*request.Body)

	created, err := h.accountsSrv.CreateGroup(ctx, group)
	if err != nil {
		switch err.(type) {
		case *service.ErrDuplicateKey:
			return server.CreateGroup400JSONResponse{Message: "group already exists"}, nil
		default:
			logger.Error(err).Log()
			return server.CreateGroup500JSONResponse{Message: fmt.Sprintf("failed to create group: %v", err)}, nil
		}
	}

	logger.Success().WithString("group_name", created.Name).Log()
	return server.CreateGroup201JSONResponse(mappers.GroupToApi(created)), nil
}

// (GET /api/v1/groups/{id})
func (h *ServiceHandler) GetGroup(ctx context.Context, request server.GetGroupRequestObject) (server.GetGroupResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("get_group").
		WithUUID("group_id", request.Id).
		Build()

	group, err := h.accountsSrv.GetGroup(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.GetGroup404JSONResponse{Message: "group not found"}, nil
		default:
			logger.Error(err).Log()
			return server.GetGroup500JSONResponse{Message: fmt.Sprintf("failed to get group: %v", err)}, nil
		}
	}

	logger.Success().WithString("group_name", group.Name).Log()
	return server.GetGroup200JSONResponse(mappers.GroupToApi(group)), nil
}

// (PUT /api/v1/groups/{id})
func (h *ServiceHandler) UpdateGroup(ctx context.Context, request server.UpdateGroupRequestObject) (server.UpdateGroupResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("update_group").
		WithUUID("group_id", request.Id).
		Build()

	if request.Body == nil {
		return server.UpdateGroup400JSONResponse{Message: "empty body"}, nil
	}

	if request.Body.Name != nil && strings.TrimSpace(*request.Body.Name) == "" {
		return server.UpdateGroup400JSONResponse{Message: "name cannot be empty"}, nil
	}
	if request.Body.Company != nil && strings.TrimSpace(*request.Body.Company) == "" {
		return server.UpdateGroup400JSONResponse{Message: "company cannot be empty"}, nil
	}

	existing, err := h.accountsSrv.GetGroup(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.UpdateGroup404JSONResponse{Message: "group not found"}, nil
		default:
			logger.Error(err).Log()
			return server.UpdateGroup500JSONResponse{Message: fmt.Sprintf("failed to get group: %v", err)}, nil
		}
	}

	updated := mappers.GroupUpdateToModel(*request.Body, existing)
	result, err := h.accountsSrv.UpdateGroup(ctx, updated)
	if err != nil {
		switch err.(type) {
		case *service.ErrDuplicateKey:
			return server.UpdateGroup400JSONResponse{Message: "group already exists"}, nil
		default:
			logger.Error(err).Log()
			return server.UpdateGroup500JSONResponse{Message: fmt.Sprintf("failed to update group: %v", err)}, nil
		}
	}

	logger.Success().WithString("group_name", result.Name).Log()
	return server.UpdateGroup200JSONResponse(mappers.GroupToApi(result)), nil
}

// (DELETE /api/v1/groups/{id})
func (h *ServiceHandler) DeleteGroup(ctx context.Context, request server.DeleteGroupRequestObject) (server.DeleteGroupResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("delete_group").
		WithUUID("group_id", request.Id).
		Build()

	group, err := h.accountsSrv.GetGroup(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.DeleteGroup404JSONResponse{Message: "group not found"}, nil
		default:
			logger.Error(err).Log()
			return server.DeleteGroup500JSONResponse{Message: fmt.Sprintf("failed to get group: %v", err)}, nil
		}
	}

	if err := h.accountsSrv.DeleteGroup(ctx, request.Id); err != nil {
		logger.Error(err).Log()
		return server.DeleteGroup500JSONResponse{Message: fmt.Sprintf("failed to delete group: %v", err)}, nil
	}

	logger.Success().WithString("group_name", group.Name).Log()
	return server.DeleteGroup200JSONResponse(mappers.GroupToApi(group)), nil
}

// (GET /api/v1/groups/{id}/members)
func (h *ServiceHandler) ListGroupMembers(ctx context.Context, request server.ListGroupMembersRequestObject) (server.ListGroupMembersResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("list_group_members").
		WithUUID("group_id", request.Id).
		Build()

	members, err := h.accountsSrv.ListGroupMembers(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.ListGroupMembers404JSONResponse{Message: "group not found"}, nil
		default:
			logger.Error(err).Log()
			return server.ListGroupMembers500JSONResponse{Message: fmt.Sprintf("failed to list group members: %v", err)}, nil
		}
	}

	logger.Success().WithInt("count", len(members)).Log()
	return server.ListGroupMembers200JSONResponse(mappers.MemberListToApi(members)), nil
}

// (PUT /api/v1/groups/{id}/members/{username})
func (h *ServiceHandler) UpdateGroupMember(ctx context.Context, request server.UpdateGroupMemberRequestObject) (server.UpdateGroupMemberResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("update_group_member").
		WithUUID("group_id", request.Id).
		WithString("username", request.Username).
		Build()

	if request.Body == nil {
		return server.UpdateGroupMember400JSONResponse{Message: "empty body"}, nil
	}

	existing, err := h.accountsSrv.GetMember(ctx, request.Username)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.UpdateGroupMember404JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.UpdateGroupMember500JSONResponse{Message: fmt.Sprintf("failed to get member: %v", err)}, nil
		}
	}

	updated := mappers.MemberUpdateToModel(*request.Body, existing)

	result, err := h.accountsSrv.UpdateGroupMember(ctx, request.Id, request.Username, updated)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.UpdateGroupMember404JSONResponse{Message: err.Error()}, nil
		case *service.ErrMembershipMismatch:
			return server.UpdateGroupMember400JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.UpdateGroupMember500JSONResponse{Message: fmt.Sprintf("failed to update member: %v", err)}, nil
		}
	}

	logger.Success().WithString("username", result.Username).Log()
	return server.UpdateGroupMember200JSONResponse(mappers.MemberToApi(result)), nil
}

// (DELETE /api/v1/groups/{id}/members/{username})
func (h *ServiceHandler) RemoveGroupMember(ctx context.Context, request server.RemoveGroupMemberRequestObject) (server.RemoveGroupMemberResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("remove_group_member").
		WithUUID("group_id", request.Id).
		WithString("username", request.Username).
		Build()

	err := h.accountsSrv.RemoveGroupMember(ctx, request.Id, request.Username)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.RemoveGroupMember404JSONResponse{Message: err.Error()}, nil
		case *service.ErrMembershipMismatch:
			return server.RemoveGroupMember400JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.RemoveGroupMember500JSONResponse{Message: fmt.Sprintf("failed to remove member from group: %v", err)}, nil
		}
	}

	logger.Success().Log()
	return server.RemoveGroupMember200Response{}, nil
}

// (POST /api/v1/groups/{id}/members)
func (h *ServiceHandler) CreateGroupMember(ctx context.Context, request server.CreateGroupMemberRequestObject) (server.CreateGroupMemberResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("create_group_member").
		WithUUID("group_id", request.Id).
		Build()

	if request.Body == nil {
		return server.CreateGroupMember400JSONResponse{Message: "empty body"}, nil
	}

	member := mappers.MemberCreateToModel(*request.Body, request.Id)

	created, err := h.accountsSrv.CreateMember(ctx, member)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.CreateGroupMember404JSONResponse{Message: "group not found"}, nil
		case *service.ErrDuplicateKey:
			return server.CreateGroupMember409JSONResponse{Message: "member already exists"}, nil
		default:
			logger.Error(err).Log()
			return server.CreateGroupMember500JSONResponse{Message: fmt.Sprintf("failed to create member: %v", err)}, nil
		}
	}

	logger.Success().WithString("username", created.Username).Log()
	return server.CreateGroupMember201JSONResponse(mappers.MemberToApi(created)), nil
}
