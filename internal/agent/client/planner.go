package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	client "github.com/kubev2v/migration-planner/internal/api/client/agent"
)

var _ Planner = (*planner)(nil)

var (
	ErrEmptyResponse = errors.New("empty response")
)

func NewPlanner(client *client.ClientWithResponses) Planner {
	return &planner{
		client: client,
	}
}

type planner struct {
	client *client.ClientWithResponses
}

func (p *planner) UpdateSourceStatus(ctx context.Context, id uuid.UUID, params api.SourceStatusUpdate, rcb ...client.RequestEditorFn) error {
	resp, err := p.client.ReplaceSourceStatusWithResponse(ctx, id, params, rcb...)
	if err != nil {
		return err
	}
	if resp.HTTPResponse != nil {
		defer resp.HTTPResponse.Body.Close()
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("update source status failed: %s", resp.Status())
	}

	return nil
}
