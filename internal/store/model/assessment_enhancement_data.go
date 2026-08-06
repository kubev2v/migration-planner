package model

import (
	"time"

	"github.com/google/uuid"
)

type AssessmentEnhancementData struct {
	AssessmentID uuid.UUID `gorm:"primaryKey;column:assessment_id;type:VARCHAR(255);"`

	// Deployed Environment
	DeployedEnvEnvironment *string `gorm:"column:deployed_env_environment;type:VARCHAR(50)"`

	// VMware Version Counts
	VMwareVerPerpetualLicensesCount *int `gorm:"column:vmware_ver_perpetual_licenses_count"`
	VMwareVerCoresLicensingCount    *int `gorm:"column:vmware_ver_cores_licensing_count"`
	VMwareVerEnvironmentCount       *int `gorm:"column:vmware_ver_environment_count"`
	VMwareVerOtherNodesCount        *int `gorm:"column:vmware_ver_other_nodes_count"`

	// Active Environments
	ActiveEnvEnvironments StringArray `gorm:"column:active_env_environments;type:text[]"`

	// VMware Subscription
	VMwareSubLevel StringArray `gorm:"column:vmware_sub_level;type:text[]"`

	// vSphere Core
	VsphereVmEncryptionEnabled *bool   `gorm:"column:vsphere_vm_encryption_enabled"`
	VsphereVmEncryptionPolicy  *string `gorm:"column:vsphere_vm_encryption_policy"`
	VsphereSrmEnabled          *bool   `gorm:"column:vsphere_srm_enabled"`

	// NSX / Aria
	NsxFeatures            StringArray `gorm:"column:nsx_features;type:text[]"`
	AriaOpsFeatures        StringArray `gorm:"column:aria_ops_features;type:text[]"`
	AriaAutomationFeatures StringArray `gorm:"column:aria_automation_features;type:text[]"`
	AriaSecureFeatures     StringArray `gorm:"column:aria_secure_features;type:text[]"`

	// Customer Details
	CustomerPhysicalLocationsCount *int    `gorm:"column:customer_physical_locations_count"`
	CustomerTargetHardware         *string `gorm:"column:customer_target_hardware"`

	UpdatedAt *time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`
}

func (AssessmentEnhancementData) TableName() string {
	return "assessment_enhancement_data"
}
