package mappers

import (
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/model"
	storemappers "github.com/kubev2v/migration-planner/internal/store/mappers"
	"github.com/kubev2v/migration-planner/internal/util"
)

func AssessmentInventoryFromJSON(id, assessmentID uuid.UUID, name string, isSubset bool, raw []byte) (model.AssessmentInventory, error) {
	// TODO: Replace all callers with NewAssessmentInventory. This requires introducing a new source model at the domain level.
	version := uint(util.GetInventoryVersion(raw))

	inventory, err := storemappers.UnmarshalInventory(raw, version)
	if err != nil {
		return model.AssessmentInventory{}, err
	}

	return model.NewAssessmentInventory(id, assessmentID, name, version, isSubset, inventory), nil
}
