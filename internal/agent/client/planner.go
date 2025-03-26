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
		return fmt.Errorf("failed to update source status: %s", resp.Status())
	}

	return nil
}

func (p *planner) Health(ctx context.Context) error {
	resp, err := p.client.HealthWithResponse(ctx, func(ctx context.Context, req *http.Request) error {
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
		return fmt.Errorf("health check failed with status: %s", resp.Status())
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
	if resp.StatusCode() == http.StatusGone {
		return ErrSourceGone
	}
	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("failed to update agent status: %s", resp.Status())
	}
	return nil
}

func (p *planner) jwtFromContext(ctx context.Context) (string, bool) {
	val := ctx.Value(common.JwtKey)
	if val == nil {
		return "", false
	}
	return val.(string), true
}
