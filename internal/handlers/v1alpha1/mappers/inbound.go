package mappers

import (
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/util"
)

// mapLabels converts API labels to map[string]string
func mapLabels(apiLabels *[]v1alpha1.Label) map[string]string {
	if apiLabels == nil {
		return nil
	}
	labels := make(map[string]string, len(*apiLabels))
	for _, label := range *apiLabels {
		labels[label.Key] = label.Value
	}
	return labels
}

// mapProxyFields extracts proxy fields from API proxy struct
func mapProxyFields(proxy *v1alpha1.AgentProxy) (httpUrl, httpsUrl, noProxy string) {
	if proxy == nil {
		return "", "", ""
	}

	return util.DerefString(proxy.HttpUrl),
		util.DerefString(proxy.HttpsUrl),
		util.DerefString(proxy.NoProxy)
}

// mapProxyFieldsForUpdate extracts proxy fields for update form (returns pointers)
func mapProxyFieldsForUpdate(proxy *v1alpha1.AgentProxy) (httpUrl, httpsUrl, noProxy *string) {
	if proxy == nil {
		return nil, nil, nil
	}
	return proxy.HttpUrl, proxy.HttpsUrl, proxy.NoProxy
}

func SourceFormApi(resource v1alpha1.SourceCreate) mappers.SourceCreateForm {
	httpUrl, httpsUrl, noProxy := mapProxyFields(resource.Proxy)

	form := mappers.SourceCreateForm{
		Name:     string(resource.Name),
		Labels:   mapLabels(resource.Labels),
		HttpUrl:  httpUrl,
		HttpsUrl: httpsUrl,
		NoProxy:  noProxy,
	}

	if resource.SshPublicKey != nil {
		form.SshPublicKey = string(*resource.SshPublicKey)
	}
	if resource.CertificateChain != nil {
		form.CertificateChain = string(*resource.CertificateChain)
	}

	return form
}

func SourceUpdateFormApi(resource v1alpha1.SourceUpdate) mappers.SourceUpdateForm {
	httpUrl, httpsUrl, noProxy := mapProxyFieldsForUpdate(resource.Proxy)

	form := mappers.SourceUpdateForm{
		HttpUrl:  httpUrl,
		HttpsUrl: httpsUrl,
		NoProxy:  noProxy,
	}

	if resource.Name != nil {
		form.Name = (*string)(resource.Name)
	}
	if resource.SshPublicKey != nil {
		sshKey := string(*resource.SshPublicKey)
		form.SshPublicKey = &sshKey
	}
	if resource.CertificateChain != nil {
		certChain := string(*resource.CertificateChain)
		form.CertificateChain = &certChain
	}

	// Handle labels conversion - convert to simple form structure
	if resource.Labels != nil {
		labels := make([]mappers.Label, len(*resource.Labels))
		for i, label := range *resource.Labels {
			labels[i] = mappers.Label{Key: label.Key, Value: label.Value}
		}
		form.Labels = labels
	}

	return form
}
