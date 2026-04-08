package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	internalclient "github.com/kubev2v/migration-planner/internal/api/client"
	"go.uber.org/zap"
)

const (
	apiV1GroupsPath   = "/api/v1/groups"
	apiV1IdentityPath = "/api/v1/identity"
)

func (s *plannerService) GetIdentity() (*v1alpha1.Identity, error) {
	zap.S().Infof("[PlannerService] Get identity [user: %s]", s.credentials.Username)

	res, err := s.api.GetRequest(apiV1IdentityPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseGetIdentityResponse(res)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity. status: %d, err: %v", res.StatusCode, err)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get identity. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}
	return parsed.JSON200, nil
}

func (s *plannerService) CreateGroup(req v1alpha1.GroupCreate) (*v1alpha1.Group, error) {
	zap.S().Infof("[PlannerService] Create group [user: %s]", s.credentials.Username)

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	res, err := s.api.PostRequest(apiV1GroupsPath, reqBody)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseCreateGroupResponse(res)
	if err != nil {
		return nil, fmt.Errorf("failed to create group. status: %d, err: %v", res.StatusCode, err)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create group. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}
	return parsed.JSON201, nil
}

func (s *plannerService) DeleteGroup(id uuid.UUID) error {
	zap.S().Infof("[PlannerService] Delete group [user: %s]", s.credentials.Username)

	res, err := s.api.DeleteRequest(path.Join(apiV1GroupsPath, id.String()))
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("failed to delete group. status: %d, body: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (s *plannerService) CreateGroupMember(groupID uuid.UUID, req v1alpha1.MemberCreate) (*v1alpha1.Member, error) {
	zap.S().Infof("[PlannerService] Create group member [user: %s, group: %s]", s.credentials.Username, groupID)

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	res, err := s.api.PostRequest(path.Join(apiV1GroupsPath, groupID.String(), "members"), reqBody)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseCreateGroupMemberResponse(res)
	if err != nil {
		return nil, fmt.Errorf("failed to create group member. status: %d, err: %v", res.StatusCode, err)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create group member. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}
	return parsed.JSON201, nil
}

func (s *plannerService) DeleteGroupMember(groupID uuid.UUID, username string) error {
	zap.S().Infof("[PlannerService] Delete group member [user: %s, group: %s, member: %s]", s.credentials.Username, groupID, username)

	res, err := s.api.DeleteRequest(path.Join(apiV1GroupsPath, groupID.String(), "members", username))
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("failed to delete group member. status: %d, body: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
