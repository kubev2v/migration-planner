package mappers

import (
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
)

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func SourceFormApi(resource v1alpha1.SourceCreate) mappers.SourceCreateForm {
	form := mappers.SourceCreateForm{
		Name:   resource.Name,
		Labels: resource.Labels,
		Proxy:  resource.Proxy,
	}

	form.SshPublicKey = derefString(resource.SshPublicKey)
	form.CertificateChain = derefString(resource.CertificateChain)

	return form
}

func SourceUpdateFormApi(resource v1alpha1.SourceUpdate) mappers.SourceUpdateForm {
	form := mappers.SourceUpdateForm{}

	form.Name = resource.Name
	form.Labels = resource.Labels
	form.SshPublicKey = resource.SshPublicKey
	form.CertificateChain = resource.CertificateChain
	form.Proxy = resource.Proxy

	return form
}
