package duckdb_parser

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDuckDBParser(t *testing.T) {
	RegisterFailHandler(Fail)
	// Existing fixture builders require *testing.T, so prepare files before the specs.
	prepareHostNICInputs(t)
	RunSpecs(t, "DuckDB Parser Suite")
}
