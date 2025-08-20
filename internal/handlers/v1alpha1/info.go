package v1alpha1

import (
	"context"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/pkg/version"
)

// (GET /api/v1/info)
func (s *ServiceHandler) GetInfo(ctx context.Context, request server.GetInfoRequestObject) (server.GetInfoResponseObject, error) {
	versionInfo := version.Get()

	response := v1alpha1.Info{
		GitCommit:   versionInfo.GitCommit,
		VersionName: versionInfo.GitVersion,
	}

	return server.GetInfo200JSONResponse(response), nil
}
