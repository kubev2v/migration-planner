package kafka

import (
	"encoding/json"
	"time"
)

type AssessmentEventPayload struct {
	Assessment AssessmentData `json:"assessment"`
}

type AssessmentData struct {
	ID            string          `json:"id"`
	Name          string          `json:"name,omitempty"`
	OrgID         string          `json:"org_id,omitempty"`
	OrgName       string          `json:"org_name,omitempty"`
	OrgType       string          `json:"org_type,omitempty"`
	Username      string          `json:"username,omitempty"`
	SourceType    string          `json:"source_type,omitempty"`
	PartnerID     *string         `json:"partner_id,omitempty"`
	PartnerName   *string         `json:"partner_name,omitempty"`
	Location      *string         `json:"location,omitempty"`
	Inventory     json.RawMessage `json:"inventory,omitempty"` // Deprecated
	MainInventory Inventory       `json:"main_inventory,omitzero"`
	CreatedAt     time.Time       `json:"created_at,omitempty"`
	UpdatedAt     *time.Time      `json:"updated_at,omitempty"`
	DeletedAt     *time.Time      `json:"deleted_at,omitempty"`
}

type Inventory struct {
	CreatedAt      time.Time     `json:"created_at,omitzero"`
	Data           InventoryData `json:"data"`
	Id             string        `json:"id"`
	VcenterVersion string        `json:"vcenter_version,omitempty"`
}

// InventoryData defines model for InventoryData.
type InventoryData struct {
	Infra Infra `json:"infra"`
	Vms   VMs   `json:"vms"`
}

type Infra struct {
	Datastores       []Datastore `json:"datastores,omitempty"`
	Hosts            []Host      `json:"hosts,omitempty"`
	Networks         []Network   `json:"networks,omitempty"`
	TotalClusters    int         `json:"totalClusters"`
	TotalDatacenters *int        `json:"totalDatacenters,omitempty"`
	TotalHosts       int         `json:"totalHosts,omitempty"`
}

type VMs struct {
	OsInfo                      map[string]OsInfo `json:"osInfo,omitempty"`
	Total                       int               `json:"total"`
	TotalMigratable             int               `json:"totalMigratable"`
	TotalMigratableWithWarnings *int              `json:"totalMigratableWithWarnings,omitempty"`
	TotalWithSharedDisks        *int              `json:"totalWithSharedDisks,omitempty"`
}

type Network struct {
	Dvswitch string `json:"dvswitch,omitempty"`
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	VlanId   string `json:"vlanId,omitempty"`
	VmsCount *int   `json:"vmsCount,omitempty"`
}

type Datastore struct {
	FreeCapacityGB  int     `json:"freeCapacityGB"`
	HostId          *string `json:"hostId"`
	Model           string  `json:"model,omitempty"`
	TotalCapacityGB int     `json:"totalCapacityGB,omitempty"`
	Type            string  `json:"type,omitempty"`
	Vendor          string  `json:"vendor,omitempty"`
}

type Host struct {
	Id     string `json:"id"`
	Model  string `json:"model,omitempty"`
	Vendor string `json:"vendor,omitempty"`
}

type OsInfo struct {
	Count int `json:"count"`
}

func NewAssessmentCreatedPayload(data AssessmentData) AssessmentEventPayload {
	return AssessmentEventPayload{Assessment: data}
}

func NewAssessmentDeletedPayload(assessmentID string, deletedAt time.Time) AssessmentEventPayload {
	return AssessmentEventPayload{Assessment: AssessmentData{ID: assessmentID, DeletedAt: &deletedAt}}
}
