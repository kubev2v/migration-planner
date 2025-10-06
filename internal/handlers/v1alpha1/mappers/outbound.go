package mappers

import (
	"slices"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

func SourceToApi(s model.Source) api.Source {
	source := api.Source{
		Id:         s.ID,
		Inventory:  nil,
		CreatedAt:  s.CreatedAt,
		UpdatedAt:  s.UpdatedAt,
		OnPremises: s.OnPremises,
		Name:       s.Name,
	}

	if s.Inventory != nil {
		source.Inventory = &s.Inventory.Data
	}

	if len(s.Labels) > 0 {
		labels := make([]api.Label, 0, len(s.Labels))
		for _, label := range s.Labels {
			labels = append(labels, api.Label{Key: label.Key, Value: label.Value})
		}
		source.Labels = &labels
	}

	// Map ImageInfra fields to API infra
	source.Infra = &struct {
		Proxy        *api.AgentProxy            `json:"proxy,omitempty"`
		SshPublicKey *api.ValidatedSSHPublicKey `json:"sshPublicKey" validate:"omitnil,ssh_key"`
		VmNetwork    *api.VmNetwork             `json:"vmNetwork,omitempty"`
	}{}

	// Map proxy fields
	if s.ImageInfra.HttpProxyUrl != "" || s.ImageInfra.HttpsProxyUrl != "" || s.ImageInfra.NoProxyDomains != "" {
		source.Infra.Proxy = &api.AgentProxy{}
		if s.ImageInfra.HttpProxyUrl != "" {
			source.Infra.Proxy.HttpUrl = &s.ImageInfra.HttpProxyUrl
		}
		if s.ImageInfra.HttpsProxyUrl != "" {
			source.Infra.Proxy.HttpsUrl = &s.ImageInfra.HttpsProxyUrl
		}
		if s.ImageInfra.NoProxyDomains != "" {
			source.Infra.Proxy.NoProxy = &s.ImageInfra.NoProxyDomains
		}
	}

	// Map SSH public key
	if s.ImageInfra.SshPublicKey != "" {
		source.Infra.SshPublicKey = &s.ImageInfra.SshPublicKey
	}

	// Map VM network fields
	if s.ImageInfra.IpAddress != "" || s.ImageInfra.SubnetMask != "" || s.ImageInfra.DefaultGateway != "" || s.ImageInfra.Dns != "" {
		source.Infra.VmNetwork = &api.VmNetwork{
			Ipv4: &api.Ipv4Config{
				IpAddress:      s.ImageInfra.IpAddress,
				SubnetMask:     s.ImageInfra.SubnetMask,
				DefaultGateway: s.ImageInfra.DefaultGateway,
				Dns:            s.ImageInfra.Dns,
			},
		}
	}

	// We are mapping only the first agent based on created_at timestamp and ignore the rest for now.
	// TODO:
	// Remark: If multiple agents are deployed, we pass only the first one based on created_at timestamp
	// while other agents in up-to-date states exists.
	// Which one should be presented in the API response?
	if len(s.Agents) == 0 {
		return source
	}

	slices.SortFunc(s.Agents, func(a model.Agent, b model.Agent) int {
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}
		return 0
	})
	agent := AgentToApi(s.Agents[0])
	source.Agent = &agent

	return source
}

func SourceListToApi(sources ...model.SourceList) api.SourceList {
	sourceList := []api.Source{}
	for _, source := range sources {
		for _, s := range source {
			sourceList = append(sourceList, SourceToApi(s))
		}
	}
	return sourceList
}

func AgentToApi(a model.Agent) api.Agent {
	return api.Agent{
		Id:            a.ID,
		Status:        api.StringToAgentStatus(a.Status),
		StatusInfo:    a.StatusInfo,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
		CredentialUrl: a.CredUrl,
		Version:       a.Version,
	}
}

func AssessmentToApi(a model.Assessment) api.Assessment {
	assessment := api.Assessment{
		Id:             a.ID,
		Name:           a.Name,
		OwnerFirstName: a.OwnerFirstName,
		OwnerLastName:  a.OwnerLastName,
		CreatedAt:      a.CreatedAt,
		Snapshots:      make([]api.Snapshot, len(a.Snapshots)),
	}

	// Convert snapshots
	for i, snapshot := range a.Snapshots {
		assessment.Snapshots[i] = api.Snapshot{
			CreatedAt: snapshot.CreatedAt,
		}
		if snapshot.Inventory != nil {
			assessment.Snapshots[i].Inventory = snapshot.Inventory.Data
		}
	}

	// Set source type based on source field
	sourceType := api.AssessmentSourceType(a.SourceType)
	assessment.SourceType = sourceType
	assessment.SourceId = a.SourceID

	return assessment
}

func AssessmentListToApi(assessments []model.Assessment) api.AssessmentList {
	assessmentList := make([]api.Assessment, len(assessments))
	for i, assessment := range assessments {
		assessmentList[i] = AssessmentToApi(assessment)
	}
	return assessmentList
}
