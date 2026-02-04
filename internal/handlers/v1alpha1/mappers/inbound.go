package mappers

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/auth"
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

func mapIpv4Fields(vmnetwork *v1alpha1.VmNetwork) (ipAddress, subnetMask, defaultGateway, dns string) {
	if vmnetwork == nil || vmnetwork.Ipv4 == nil {
		return "", "", "", ""
	}

	return vmnetwork.Ipv4.IpAddress, vmnetwork.Ipv4.SubnetMask, vmnetwork.Ipv4.DefaultGateway, vmnetwork.Ipv4.Dns
}

func SourceFormApi(resource v1alpha1.SourceCreate) mappers.SourceCreateForm {
	httpUrl, httpsUrl, noProxy := mapProxyFields(resource.Proxy)
	ipAddress, subnetMask, defaultGateway, dns := mapIpv4Fields(resource.Network)

	form := mappers.SourceCreateForm{
		Name:           string(resource.Name),
		Labels:         mapLabels(resource.Labels),
		HttpUrl:        httpUrl,
		HttpsUrl:       httpsUrl,
		NoProxy:        noProxy,
		IpAddress:      ipAddress,
		SubnetMask:     subnetMask,
		DefaultGateway: defaultGateway,
		Dns:            dns,
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
	ipAddress, subnetMask, defaultGateway, dns := mapIpv4Fields(resource.Network)

	form := mappers.SourceUpdateForm{
		HttpUrl:        httpUrl,
		HttpsUrl:       httpsUrl,
		NoProxy:        noProxy,
		IpAddress:      &ipAddress,
		SubnetMask:     &subnetMask,
		DefaultGateway: &defaultGateway,
		Dns:            &dns,
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

// Assessment-related mappers

func ClusterRequirementsRequestToForm(apiReq v1alpha1.ClusterRequirementsRequest) mappers.ClusterRequirementsRequestForm {
	form := mappers.ClusterRequirementsRequestForm{
		ClusterID:               apiReq.ClusterId,
		CpuOverCommitRatio:      string(apiReq.CpuOverCommitRatio),
		MemoryOverCommitRatio:   string(apiReq.MemoryOverCommitRatio),
		WorkerNodeCPU:           apiReq.WorkerNodeCPU,
		WorkerNodeThreads:       0,
		WorkerNodeMemory:        apiReq.WorkerNodeMemory,
		ControlPlaneSchedulable: false,
	}
	if apiReq.ControlPlaneSchedulable != nil {
		form.ControlPlaneSchedulable = *apiReq.ControlPlaneSchedulable
	}
	if apiReq.WorkerNodeThreads != nil {
		form.WorkerNodeThreads = *apiReq.WorkerNodeThreads
	}
	return form
}

func AssessmentFormToCreateForm(resource v1alpha1.AssessmentForm, user auth.User) mappers.AssessmentCreateForm {
	form := mappers.AssessmentCreateForm{
		ID:       uuid.New(),
		Name:     resource.Name,
		OrgID:    user.Organization,
		Username: user.Username,
		Source:   resource.SourceType,
	}

	// Set owner fields from user context (like username)
	if user.FirstName != "" {
		form.OwnerFirstName = &user.FirstName
	}
	if user.LastName != "" {
		form.OwnerLastName = &user.LastName
	}

	// Set source ID if provided
	if resource.SourceId != nil {
		form.SourceID = resource.SourceId
	}

	// Set inventory if provided
	if resource.Inventory != nil {
		data, _ := json.Marshal(resource.Inventory) // cannot fail. it has been already validated
		form.Inventory = data
	}

	return form
}

func InventoryToForm(inventory v1alpha1.Inventory) mappers.InventoryForm {
	return mappers.InventoryForm{
		Data: inventory,
	}
}
