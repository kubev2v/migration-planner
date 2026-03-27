package mappers

import (
	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

func GroupCreateToModel(req api.GroupCreate) model.Group {
	group := model.Group{
		ID:          uuid.New(),
		Name:        req.Name,
		Description: req.Description,
		Kind:        string(req.Kind),
		Icon:        req.Icon,
		Company:     req.Company,
	}

	return group
}

func GroupUpdateToModel(req api.GroupUpdate, existing model.Group) model.Group {
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Icon != nil {
		existing.Icon = *req.Icon
	}
	if req.Company != nil {
		existing.Company = *req.Company
	}

	return existing
}

func MemberCreateToModel(req api.MemberCreate, groupID uuid.UUID) model.Member {
	return model.Member{
		Username: req.Username,
		Email:    string(req.Email),
		GroupID:  groupID,
	}
}

func MemberUpdateToModel(req api.MemberUpdate, existing model.Member) model.Member {
	if req.Email != nil {
		existing.Email = string(*req.Email)
	}
	return existing
}
