package eventwrap

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/estimations/engines"
	"github.com/kubev2v/migration-planner/pkg/estimations/estimation"
	"github.com/kubev2v/migration-planner/pkg/events"
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
	clusterID string,
	schemas []engines.Schema,
	userParams []estimation.Param,
) (map[engines.Schema]*service.MigrationAssessmentResult, error) {
	results, err := e.inner.CalculateMigrationEstimation(ctx, assessmentID, clusterID, schemas, userParams)
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
	clusterID string,
) (*service.MigrationComplexityResult, error) {
	result, err := e.inner.CalculateMigrationComplexity(ctx, assessmentID, clusterID)
	if err != nil {
		return nil, err
	}

	if err := e.publishUserAction(ctx, assessmentID, events.MigrationComplexityEventType); err != nil {
		return nil, err
	}
	return result, nil
}

func (e *EventEstimationService) CalculateOsDiskComplexity(ctx context.Context, assessmentID uuid.UUID, clusterID string) (*service.OsDiskComplexityResult, error) {
	return e.inner.CalculateOsDiskComplexity(ctx, assessmentID, clusterID)
}

func (e *EventEstimationService) ValidateParams(userParams []estimation.Param) error {
	return e.inner.ValidateParams(userParams)
}

func (e *EventEstimationService) BuildBaseParams(userParams []estimation.Param) []estimation.Param {
	return e.inner.BuildBaseParams(userParams)
}

func (e *EventEstimationService) BuildBucketParams(baseParams []estimation.Param, vmCount int, diskGB float64) []estimation.Param {
	return e.inner.BuildBucketParams(baseParams, vmCount, diskGB)
}

func (e *EventEstimationService) RunEstimation(schemas []engines.Schema, params []estimation.Param) (map[engines.Schema]*service.MigrationAssessmentResult, error) {
	return e.inner.RunEstimation(schemas, params)
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
	return e.outbox.Insert(ctx, eventType, ceBytes)
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
