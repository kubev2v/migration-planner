package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	api "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	client "github.com/kubev2v/migration-planner/internal/api/client/agent"
)

var _ Planner = (*planner)(nil)

var (
	ErrEmptyResponse = errors.New("empty response")
)

func NewPlanner(
	client *client.ClientWithResponses,
) Planner {
	return &planner{
		client: client,
	}
}

type planner struct {
	client *client.ClientWithResponses
}

func (p *planner) UpdateSourceInventory(ctx context.Context, id string, params api.SourceInventoryUpdate, rcb ...client.RequestEditorFn) error {
	resp, err := p.client.ReplaceSourceInventoryWithResponse(ctx, id, params, rcb...)
	if err != nil {
		return err
	}
	if resp.HTTPResponse != nil {
		defer resp.HTTPResponse.Body.Close()
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("update source inventory failed: %s", resp.Status())
	}

	return nil
}

func (p *planner) UpdateSourceStatus(ctx context.Context, id string, params api.SourceStatusUpdate, rcb ...client.RequestEditorFn) error {
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
