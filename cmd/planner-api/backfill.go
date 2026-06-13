package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/events"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var backfillCmd = &cobra.Command{
	Use:   "backfill",
	Short: "Backfill existing assessments into Kafka",
	Long:  "Reads all assessments from the database and publishes AssessmentCreated events to Kafka, backfilling historical data.",
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := log.InitLog(zap.NewAtomicLevelAt(zap.InfoLevel))
		defer func() { _ = logger.Sync() }()

		undo := zap.ReplaceGlobals(logger)
		defer undo()

		cfg, err := config.New()
		if err != nil {
			zap.S().Fatalw("reading configuration", "error", err)
		}

		zap.S().Info("Initializing data store")
		db, err := store.InitDB(cfg)
		if err != nil {
			zap.S().Fatalw("initializing data store", "error", err)
		}

		s := store.NewStore(db)
		defer func() { _ = s.Close() }()

		ctx := context.Background()

		w, cl, err := createEventWriter(ctx, cfg)
		if err != nil {
			zap.S().Fatalw("initializing kafka producer", "error", err)
		}

		if err := backfillAssessments(ctx, s, w, cl); err != nil {
			zap.S().Errorf("completed with errors: %f", err)
			return nil
		}

		zap.S().Infow("completed successfully without any error")
		return nil
	},
}

func backfillAssessments(ctx context.Context, s store.Store, writer events.Writer, writerClose func()) error {
	defer writerClose()

	assessments, err := s.Assessment().List(ctx, nil)
	if err != nil {
		return fmt.Errorf("listing assessments: %w", err)
	}

	zap.S().Infow("found assessments to backfill", "count", len(assessments))

	partnerIDs, err := resolvePartnerIDs(ctx, s, assessments)
	if err != nil {
		return fmt.Errorf("resolving partner IDs: %w", err)
	}

	var success, noSnapshotsErrors, inventoryConvertErrors, eventBuildErrors, publishErrors int
	for _, assessment := range assessments {
		if len(assessment.Snapshots) == 0 {
			zap.S().Warnw("skipping assessment with no snapshots", "id", assessment.ID)
			noSnapshotsErrors++
			continue
		}

		apiAssessment, err := mappers.AssessmentToApi(assessment)
		if err != nil {
			zap.S().Errorw("converting assessment", "id", assessment.ID, "error", err)
			inventoryConvertErrors++
			continue
		}

		inventory, err := json.Marshal(apiAssessment.Snapshots[0].Inventory)
		if err != nil {
			zap.S().Errorw("marshaling inventory", "id", assessment.ID, "error", err)
			inventoryConvertErrors++
			continue
		}

		payload := events.NewAssessmentCreatedPayload(events.AssessmentData{
			ID:         assessment.ID.String(),
			SnapshotID: assessment.Snapshots[0].ID,
			Inventory:  inventory,
			Name:       assessment.Name,
			OrgID:      assessment.OrgID,
			Username:   assessment.Username,
			SourceType: assessment.SourceType,
			PartnerID:  partnerIDs[assessment.ID.String()],
			CreatedAt:  assessment.CreatedAt,
			UpdatedAt:  assessment.UpdatedAt,
		})

		ceBytes, err := events.BuildCloudEvent(events.AssessmentCreatedEventType, payload)
		if err != nil {
			zap.S().Errorw("building cloud event", "id", assessment.ID, "error", err)
			eventBuildErrors++
			continue
		}

		if err := writer.Write(ctx, events.GenericTopic, ceBytes); err != nil {
			zap.S().Errorw("publishing event", "id", assessment.ID, "error", err)
			publishErrors++
			continue
		}

		success++
	}

	if publishErrors > 0 || inventoryConvertErrors > 0 || eventBuildErrors > 0 || noSnapshotsErrors > 0 {
		return fmt.Errorf(
			"backfill completed with errors: success=%d no_snapshots=%d inventory_convert=%d event_build=%d publish=%d",
			success,
			noSnapshotsErrors,
			inventoryConvertErrors,
			eventBuildErrors,
			publishErrors,
		)
	}

	return nil
}

func resolvePartnerIDs(ctx context.Context, s store.Store, assessments model.AssessmentList) (map[string]*string, error) {
	ids := make([]string, len(assessments))
	for i, a := range assessments {
		ids[i] = a.ID.String()
	}

	relsByID, err := s.Authz().ListBulkRelationship(ctx, ids)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*string, len(assessments))
	for _, a := range assessments {
		id := a.ID.String()
		for _, rel := range relsByID[id] {
			if rel.Relation == model.ViewerRelation && rel.Subject.Kind == model.OrgSubject {
				partnerID := rel.Subject.ID
				result[id] = &partnerID
				break
			}
		}
	}
	return result, nil
}
