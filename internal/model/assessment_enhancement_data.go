package model

import api "github.com/kubev2v/migration-planner/api/v1alpha1"

// AssessmentEnhancementData is the service-layer representation of enhancement
// data. Its fields mirror api.EnhancementData.
type AssessmentEnhancementData struct {
	ActiveEnvironments  *api.ActiveEnvironmentsInput  `json:"activeEnvironments,omitempty"`
	AriaAutomation      *api.AriaAutomationInput      `json:"ariaAutomation,omitempty"`
	AriaOps             *api.AriaOpsInput             `json:"ariaOps,omitempty"`
	AriaSecure          *api.AriaSecureInput          `json:"ariaSecure,omitempty"`
	CustomerDetails     *api.CustomerDetailsInput     `json:"customerDetails,omitempty"`
	DeployedEnvironment *api.DeployedEnvironmentInput `json:"deployedEnvironment,omitempty"`
	Nsx                 *api.NsxInput                 `json:"nsx,omitempty"`
	VmwareSubscription  *api.VMwareSubscriptionInput  `json:"vmwareSubscription,omitempty"`
	VmwareVersionCounts *api.VMwareVersionCountsInput `json:"vmwareVersionCounts,omitempty"`
	VsphereCore         *api.VsphereCoreInput         `json:"vsphereCore,omitempty"`
}
