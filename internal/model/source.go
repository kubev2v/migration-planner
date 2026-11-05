package model

import (
	"time"

	"github.com/google/uuid"
)

// Source is an inventory collection source, independent of persistence.
// Inventory is nil until an inventory has been received.
type Source struct {
	ID          uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Name        string
	VCenterID   string
	Username    string
	OrgID       string
	Version     uint
	Inventory   *Inventory
	OnPremises  bool
	UpdateType  string
	Agents      []Agent
	ImageInfra  ImageInfra
	Labels      []Label
	EmailDomain *string
}

type SourceList []Source

// SourceSubsetInventory is a named, structured subset collected for a source.
type SourceSubsetInventory struct {
	ID         uuid.UUID
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Name       string
	SourceID   uuid.UUID
	VCenterID  string
	VMsCount   int
	Version    uint
	Inventory  Inventory
	UpdateType string
}

type SourceSubsetInventoryList []SourceSubsetInventory

// Agent describes an agent associated with a source.
type Agent struct {
	ID         uuid.UUID
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Status     string
	StatusInfo string
	CredUrl    string
	Version    string
	SourceID   uuid.UUID
}

// ImageInfra holds the configuration used to build a source appliance.
type ImageInfra struct {
	ID               uint
	CreatedAt        time.Time
	UpdatedAt        time.Time
	SourceID         uuid.UUID
	HttpProxyUrl     string
	HttpsProxyUrl    string
	NoProxyDomains   string
	CertificateChain string
	SshPublicKey     string
	ImageTokenKey    string
	IpAddress        string
	SubnetMask       string
	DefaultGateway   string
	Dns              string
	AgentVersion     *string
	AgentToken       *string
}

// Label is a key/value label attached to a source.
type Label struct {
	Key      string
	Value    string
	SourceID string
}
