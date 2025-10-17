package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/agent/common"
	client "github.com/kubev2v/migration-planner/internal/api/client/agent"
	"github.com/kubev2v/migration-planner/internal/auth"
)

var _ Planner = (*planner)(nil)

var (
	ErrEmptyResponse = errors.New("empty response")
	ErrSourceGone    = errors.New("source is gone")
	ErrUnauthorized  = errors.New("agent is not authorized")
)

func NewPlanner(client *client.ClientWithResponses) Planner {
	return &planner{
		client: client,
	}
}

type planner struct {
	client *client.ClientWithResponses
}

func (p *planner) UpdateSourceStatus(ctx context.Context, id uuid.UUID, params api.SourceStatusUpdate) error {
	resp, err := p.client.UpdateSourceInventoryWithResponse(ctx, id, params, func(ctx context.Context, req *http.Request) error {
		if jwt, found := p.jwtFromContext(ctx); found {
			req.Header.Add(auth.AgentTokenHeader, jwt)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if resp.HTTPResponse != nil {
		defer resp.HTTPResponse.Body.Close()
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("failed to update inventory: %s", resp.Status())
	}

	return nil
}

func (p *planner) UpdateAgentStatus(ctx context.Context, id uuid.UUID, params api.AgentStatusUpdate) error {
	resp, err := p.client.UpdateAgentStatusWithResponse(ctx, id, params, func(ctx context.Context, req *http.Request) error {
		if jwt, found := p.jwtFromContext(ctx); found {
			req.Header.Add(auth.AgentTokenHeader, jwt)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if resp.HTTPResponse != nil {
		defer resp.HTTPResponse.Body.Close()
	}

	switch resp.StatusCode() {
	case http.StatusOK:
		return nil
	case http.StatusCreated:
		return nil
	case http.StatusGone:
		return ErrSourceGone
	case http.StatusUnauthorized:
		return ErrUnauthorized
	default:
		return fmt.Errorf("failed to update agent status: %s", resp.Status())
	}
}

func (p *planner) jwtFromContext(ctx context.Context) (string, bool) {
	val := ctx.Value(common.JwtKey)
	if val == nil {
		return "", false
	}
	return val.(string), true
}
