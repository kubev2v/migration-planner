package engines

import (
	"fmt"

	"github.com/kubev2v/migration-planner/pkg/estimations/estimation"
	"github.com/kubev2v/migration-planner/pkg/estimations/estimation/calculators"
)

// Schema identifies a named set of calculators to run.
type Schema string

const (
	SchemaNetworkBased   Schema = "network-based"
	SchemaStorageOffload Schema = "storage-offload"
)

var allSchemas = []Schema{SchemaNetworkBased, SchemaStorageOffload}

var schemaBuilders = map[Schema]func(*estimation.Engine){
	SchemaNetworkBased: func(e *estimation.Engine) {
		e.Register(calculators.NewStorageMigration())
		e.Register(calculators.NewPostMigrationTroubleShooting())
	},
	SchemaStorageOffload: func(e *estimation.Engine) {
		e.Register(calculators.NewStorageOffload())
		e.Register(calculators.NewPostMigrationTroubleShooting())
	},
}

// BuildEngines returns one Engine per requested schema.
// If schemas is nil or empty, all known schemas are used.
// Returns an error if any schema name is unrecognised.
func BuildEngines(schemas []Schema) (map[Schema]*estimation.Engine, error) {
	if len(schemas) == 0 {
		schemas = allSchemas
	}
	result := make(map[Schema]*estimation.Engine, len(schemas))
	for _, s := range schemas {
		builder, ok := schemaBuilders[s]
		if !ok {
			return nil, fmt.Errorf("unknown estimation schema %q", s)
		}
		e := estimation.NewEngine()
		builder(e)
		result[s] = e
	}
	return result, nil
}
