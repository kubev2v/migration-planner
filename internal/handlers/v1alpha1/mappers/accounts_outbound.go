package mappers

import (
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store/model"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func GroupToApi(group model.Group) api.Group {
	result := api.Group{
		Id:        group.ID,
		Name:      group.Name,
		Kind:      api.GroupKind(group.Kind),
		Icon:      group.Icon,
		Company:   group.Company,
		CreatedAt: group.CreatedAt,
	}

	if group.Description != "" {
		result.Description = &group.Description
	}
	if group.UpdatedAt != nil {
		result.UpdatedAt = *group.UpdatedAt
	}

	return result
}

func GroupListToApi(groups model.GroupList) api.GroupList {
	result := make(api.GroupList, len(groups))
	for i, group := range groups {
		result[i] = GroupToApi(group)
	}
	return result
}

func MemberToApi(member model.Member) api.Member {
	result := api.Member{
		Username:  member.Username,
		Email:     openapi_types.Email(member.Email),
		GroupId:   member.GroupID,
		CreatedAt: member.CreatedAt,
	}

	if member.UpdatedAt != nil {
		result.UpdatedAt = member.UpdatedAt
	}

	return result
}

func MemberListToApi(members model.MemberList) api.MemberList {
	result := make(api.MemberList, len(members))
	for i, member := range members {
		result[i] = MemberToApi(member)
	}
	return result
}

func IdentityToApi(identity service.Identity) api.Identity {
	return api.Identity{
		Username:  identity.Username,
		Kind:      api.IdentityKind(identity.Kind),
		GroupId:   identity.GroupID,
		PartnerId: identity.PartnerID,
	}
}
