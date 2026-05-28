package eventwrap

import (
	"context"
	"time"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/events"
)

type EventSizerService struct {
	inner  service.SizerServicer
	store  store.Store
	outbox *OutboxService
}

func NewEventSizerService(inner service.SizerServicer, s store.Store) service.SizerServicer {
	return &EventSizerService{inner: inner, store: s, outbox: NewOutboxService(s)}
}

func (e *EventSizerService) CalculateClusterRequirements(
	ctx context.Context,
	assessmentID uuid.UUID,
	req *mappers.ClusterRequirementsRequestForm,
) (*api.ClusterRequirementsResponse, error) {
	result, err := e.inner.CalculateClusterRequirements(ctx, assessmentID, req)
	if err != nil {
		return nil, err
	}

	assessment, err := e.store.Assessment().Get(ctx, assessmentID)
	if err != nil {
		return nil, err
	}

	assessmentIDStr := assessmentID.String()
	payload := events.NewUserActionPayload(events.UserActionData{
		Username:     assessment.Username,
		AssessmentID: &assessmentIDStr,
		Timestamp:    time.Now().UTC(),
	})
	ceBytes, err := events.BuildCloudEvent(events.SizingEventType, payload)
	if err != nil {
		return nil, err
	}
	if err := e.outbox.Insert(ctx, events.SizingEventType, ceBytes); err != nil {
		return nil, err
	}

	return result, nil
}

func (e *EventSizerService) CalculateStandaloneClusterRequirements(
	ctx context.Context,
	req *mappers.StandaloneClusterRequirementsRequestForm,
) (*mappers.StandaloneClusterRequirementsResponseForm, error) {
	return e.inner.CalculateStandaloneClusterRequirements(ctx, req)
}

func (e *EventSizerService) GetClusterRequirementsInput(ctx context.Context, assessmentID uuid.UUID, clusterID string) (*mappers.ClusterRequirementsInputForm, error) {
	return e.inner.GetClusterRequirementsInput(ctx, assessmentID, clusterID)
}

func (e *EventSizerService) Health(ctx context.Context) error {
	return e.inner.Health(ctx)
}
