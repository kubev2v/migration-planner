package service

import (
	"context"
	"errors"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
)

func (h *ServiceHandler) ListAgents(ctx context.Context, request server.ListAgentsRequestObject) (server.ListAgentsResponseObject, error) {
	qf := store.NewAgentQueryFilter()
	if user, found := auth.UserFromContext(ctx); found {
		qf = qf.ByUsername(user.Username)
	}
	result, err := h.store.Agent().List(ctx, qf, store.NewAgentQueryOptions().WithIncludeSoftDeleted(false))
	if err != nil {
		return nil, err
	}
	return server.ListAgents200JSONResponse(mappers.AgentListToApi(result)), nil
}

func (h *ServiceHandler) DeleteAgent(ctx context.Context, request server.DeleteAgentRequestObject) (server.DeleteAgentResponseObject, error) {
	agent, err := h.store.Agent().Get(ctx, request.Id.String())
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return server.DeleteAgent404JSONResponse{Message: "agent not found"}, nil
		}
		return server.DeleteAgent500JSONResponse{}, nil
	}

	if user, found := auth.UserFromContext(ctx); found {
		if agent.Username != user.Username {
			return server.DeleteAgent403JSONResponse{}, nil
		}
	}

	if agent.DeletedAt.Valid {
		return server.DeleteAgent200JSONResponse(mappers.AgentToApi(*agent)), nil
	}
	if agent.Associated {
		return server.DeleteAgent400JSONResponse{Message: "cannot delete an associated agent"}, nil
	}

	// remove the agent
	softDeletion := true
	if err := h.store.Agent().Delete(ctx, request.Id.String(), softDeletion); err != nil {
		return server.DeleteAgent500JSONResponse{}, nil
	}

	return server.DeleteAgent200JSONResponse(mappers.AgentToApi(*agent)), nil
}
