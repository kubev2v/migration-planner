package model

import (
	"github.com/kubev2v/migration-planner-common/estimations/complexity"
	"github.com/kubev2v/migration-planner-common/estimations/engines"
	"github.com/kubev2v/migration-planner-common/estimations/estimation"
)

type BucketEstimations map[int]map[string]*estimation.Result
type ComplexityMatrix map[complexity.Score]map[complexity.Score]complexity.Score

// MigrationComplexityResult holds the output of a complexity estimation run.
type MigrationComplexityResult struct {
	ComplexityByDisk   []complexity.DiskComplexityEntry // scores 1–4, always 4 entries
	ComplexityByOS     []complexity.OSDifficultyEntry   // scores 0–4, always 5 entries
	ComplexityByOSName []complexity.OSNameEntry         // one entry per distinct OS name
	DiskSizeRatings    map[string]complexity.Score      // static tier label → score lookup
	OSRatings          map[string]complexity.Score      // per-inventory OS name → score
}

type MigrationEstimationResult struct {
	Estimation        map[engines.Schema]*estimation.Result
	EstimationContext EstimationContext
}

type MigrationEstimationByComplexityResult struct {
	Buckets           []complexity.OSDiskEntry
	BucketEstimations BucketEstimations
	EstimationContext EstimationContext
	ComplexityMatrix  ComplexityMatrix
}

type EstimationContext struct {
	Schemas    []engines.Schema
	BaseParams []estimation.Param
}
