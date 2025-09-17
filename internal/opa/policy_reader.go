package opa

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// PolicyReader Handle policy discovery and file reading
type PolicyReader struct{}

func NewPolicyReader() *PolicyReader {
	return &PolicyReader{}
}

// ReadPolicies Read all .rego policy files from the specified directory
func (pr *PolicyReader) ReadPolicies(policiesDir string) (map[string]string, error) {

	policies := make(map[string]string)

	entries, err := os.ReadDir(policiesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read policies directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".rego") ||
			strings.HasSuffix(entry.Name(), "_test.rego") {
			continue // Skip test files
		}

		path := filepath.Join(policiesDir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read policy file %s: %w", path, err)
		}

		policies[entry.Name()] = string(content)
		zap.S().Named("opa").Debugf("Read policy: %s", entry.Name())
	}

	if len(policies) == 0 {
		return nil, fmt.Errorf("no .rego policy files found in directory: %s", policiesDir)
	}

	zap.S().Named("opa").Infof("Successfully read %d policy files from: %s", len(policies), policiesDir)
	return policies, nil
}
