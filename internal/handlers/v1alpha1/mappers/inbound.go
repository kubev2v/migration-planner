package mappers

import (
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
)

func SourceFormApi(resource v1alpha1.SourceCreate) mappers.SourceCreateForm {
	form := mappers.SourceCreateForm{
		Name: resource.Name,
	}

	if resource.SshPublicKey != nil {
		form.SshPublicKey = *resource.SshPublicKey
	}

	if resource.Proxy != nil {
		if resource.Proxy.HttpUrl != nil {
			form.HttpUrl = *resource.Proxy.HttpUrl
		}
		if resource.Proxy.HttpsUrl != nil {
			form.HttpsUrl = *resource.Proxy.HttpsUrl
		}
		if resource.Proxy.NoProxy != nil {
			form.NoProxy = *resource.Proxy.NoProxy
		}
	}

	if resource.CertificateChain != nil {
		form.CertificateChain = *resource.CertificateChain
	}

	return form
}
