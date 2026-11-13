package eventwrap

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-event-streamer/pkg/events"
	"github.com/kubev2v/migration-planner-common/estimations/engines"
	"github.com/kubev2v/migration-planner-common/estimations/estimation"
	svcmodel "github.com/kubev2v/migration-planner/internal/model"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
)

type EventEstimationService struct {
	inner  service.EstimationServicer
	store  store.Store
	outbox *OutboxService
}

func NewEventEstimationService(inner service.EstimationServicer, s store.Store) service.EstimationServicer {
	return &EventEstimationService{inner: inner, store: s, outbox: NewOutboxService(s)}
}

func (e *EventEstimationService) CalculateMigrationEstimation(
	ctx context.Context,
	assessmentID uuid.UUID,
	clusterIDs []string,
	schemas []engines.Schema,
	userParams []estimation.Param,
) (*svcmodel.MigrationEstimationResult, error) {
	results, err := e.inner.CalculateMigrationEstimation(ctx, assessmentID, clusterIDs, schemas, userParams)
	if err != nil {
		return nil, err
	}

	if err := e.publishUserAction(ctx, assessmentID, events.MigrationTimeEstimationEventType); err != nil {
		return nil, err
	}
	return results, nil
}

func (e *EventEstimationService) CalculateMigrationComplexity(
	ctx context.Context,
	assessmentID uuid.UUID,
	clusterIDs []string,
) (*svcmodel.MigrationComplexityResult, error) {
	result, err := e.inner.CalculateMigrationComplexity(ctx, assessmentID, clusterIDs)
	if err != nil {
		return nil, err
	}

	if err := e.publishUserAction(ctx, assessmentID, events.MigrationComplexityEventType); err != nil {
		return nil, err
	}
	return result, nil
}

func (e *EventEstimationService) CalculateMigrationEstimationByComplexity(
	ctx context.Context,
	assessmentID uuid.UUID,
	clusterIDs []string,
	schemas []engines.Schema,
	userParams []estimation.Param,
) (*svcmodel.MigrationEstimationByComplexityResult, error) {
	return e.inner.CalculateMigrationEstimationByComplexity(ctx, assessmentID, clusterIDs, schemas, userParams)
}

func (e *EventEstimationService) publishUserAction(ctx context.Context, assessmentID uuid.UUID, eventType string) error {
	assessment, err := e.store.Assessment().Get(ctx, assessmentID)
	if err != nil {
		return err
	}
	payload, err := buildEstimationPayload(eventType, assessment.Username, assessmentID.String())
	if err != nil {
		return err
	}
	ceBytes, err := events.BuildCloudEvent(eventType, payload)
	if err != nil {
		return err
	}
	return e.outbox.Insert(ctx, EventTypeKafka, ceBytes)
}

func buildEstimationPayload(eventType, username, assessmentID string) (events.UserActionEventPayload, error) {
	switch eventType {
	case events.MigrationComplexityEventType:
		return events.NewComplexityPayload(username, assessmentID), nil
	case events.MigrationTimeEstimationEventType:
		return events.NewTimeEstimationPayload(username, assessmentID), nil
	default:
		return events.UserActionEventPayload{}, fmt.Errorf("unknown estimation event type: %s", eventType)
	}
}
