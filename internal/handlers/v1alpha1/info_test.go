package v1alpha1

import (
	"context"
	"testing"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/stretchr/testify/assert"
)

func TestGetInfo(t *testing.T) {
	handler := &ServiceHandler{}

	ctx := context.Background()
	request := server.GetInfoRequestObject{}

	response, err := handler.GetInfo(ctx, request)

	assert.NoError(t, err)
	assert.NotNil(t, response)

	// Cast to the success response type
	successResponse, ok := response.(server.GetInfo200JSONResponse)
	assert.True(t, ok, "Response should be GetInfo200JSONResponse")

	// Check that we have the expected fields (may be empty in tests without build flags)
	// GitCommit could be empty in development/test builds
	assert.NotNil(t, successResponse.GitCommit)
	// GitVersion should at least have the default "unknown" value
	assert.NotEmpty(t, successResponse.VersionName, "GitVersion should not be empty")
}
