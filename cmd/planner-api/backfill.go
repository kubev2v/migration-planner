package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/internal/util"
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

		return backfillAssessments(ctx, s, w, cl)
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

		inventory, err := toV2Inventory(assessment.Snapshots[0])
		if err != nil {
			zap.S().Errorw("converting inventory to v2", "id", assessment.ID, "error", err)
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

	zap.S().Infow("backfill completed",
		"total", len(assessments),
		"success", success,
		"no_snapshots_errors", noSnapshotsErrors,
		"inventory_convert_errors", inventoryConvertErrors,
		"event_build_errors", eventBuildErrors,
		"publish_errors", publishErrors,
	)
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

func toV2Inventory(snapshot model.Snapshot) (json.RawMessage, error) {
	if util.GetInventoryVersion(snapshot.Inventory) == model.SnapshotVersionV2 {
		return snapshot.Inventory, nil
	}

	var data v1alpha1.InventoryData
	if err := json.Unmarshal(snapshot.Inventory, &data); err != nil {
		return nil, fmt.Errorf("unmarshaling v1 inventory: %w", err)
	}

	v2 := v1alpha1.Inventory{
		Vcenter:   &data,
		VcenterId: data.Vcenter.Id,
		Clusters:  map[string]v1alpha1.InventoryData{},
	}

	return json.Marshal(v2)
}
