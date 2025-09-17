package mappers

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/service"
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

// Assessment-related mappers

func AssessmentFormToCreateForm(resource v1alpha1.AssessmentForm, user auth.User) mappers.AssessmentCreateForm {
	form := mappers.AssessmentCreateForm{
		ID:       uuid.New(),
		Name:     resource.Name,
		OrgID:    user.Organization,
		Username: user.Username,
		Source:   resource.SourceType,
	}

	// Set source ID if provided
	if resource.SourceId != nil {
		form.SourceID = resource.SourceId
	}

	// Set inventory if provided
	if resource.Inventory != nil {
		form.Inventory = *resource.Inventory
	}

	return form
}

func InventoryToForm(inventory v1alpha1.Inventory) mappers.InventoryForm {
	return mappers.InventoryForm{
		Data: inventory,
	}
}

func AssessmentCreateFormFromMultipart(multipartBody *multipart.Reader, user auth.User) (mappers.AssessmentCreateForm, error) {
	form := mappers.AssessmentCreateForm{
		ID:       uuid.New(),
		OrgID:    user.Organization,
		Username: user.Username,
		Source:   service.SourceTypeRvtools,
	}

	for {
		part, err := multipartBody.NextPart()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return form, err
		}
		defer part.Close()

		switch part.FormName() {
		case "name":
			nameBytes, err := io.ReadAll(part)
			if err != nil {
				return form, err
			}
			form.Name = string(nameBytes)
		case "file":
			buff := bytes.NewBuffer([]byte{})
			n, err := io.Copy(buff, part)
			if err != nil {
				return form, err
			}
			if n == 0 {
				return form, fmt.Errorf("rvtools file body is empty")
			}
			// Store the entire part as RVToolsFile for processing
			form.RVToolsFile = buff
		case "labels":
			// Handle labels if provided in multipart form
			// This is optional based on the API spec
		}
	}

	// For RVTools, we'll set an empty inventory initially
	// The service layer should process the RVToolsFile to populate inventory
	form.Inventory = v1alpha1.Inventory{}

	return form, nil
}
