package model

import (
	"time"

	"github.com/google/uuid"
)

type RequestStatus string

const (
	RequestStatusPending   RequestStatus = "pending"
	RequestStatusAccepted  RequestStatus = "accepted"
	RequestStatusRejected  RequestStatus = "rejected"
	RequestStatusCancelled RequestStatus = "cancelled"
)

// PartnerCustomer represents a partner request.
// DB constraints:
//   - uq_partner_customer_active_username: unique(username) WHERE request_status IN ('pending','accepted') — one active request per user
//   - idx_partners_customers_partner_id: index on partner_id for partner-side queries
type PartnerCustomer struct {
	ID            uuid.UUID     `gorm:"primaryKey;column:id;type:VARCHAR(255);"`
	Username      string        `gorm:"not null;type:VARCHAR(100)"`
	PartnerID     string        `gorm:"not null;type:VARCHAR(255)"`
	RequestStatus RequestStatus `gorm:"not null;type:request_status;default:'pending'"`
	Name          string        `gorm:"not null;type:VARCHAR(100)"`
	ContactName   string        `gorm:"not null;type:VARCHAR(100)"`
	ContactPhone  string        `gorm:"not null;type:VARCHAR(100)"`
	Email         string        `gorm:"not null;type:VARCHAR(100)"`
	Location      string        `gorm:"not null;type:VARCHAR(100)"`
	Reason        *string       `gorm:"type:VARCHAR(255)"`
	AcceptedAt    *time.Time    `gorm:"type:TIMESTAMPTZ"`
	TerminatedAt  *time.Time    `gorm:"type:TIMESTAMPTZ"`
	CreatedAt     time.Time     `gorm:"not null;default:now();type:TIMESTAMPTZ"`
	Partner       *Group        `gorm:"foreignKey:PartnerID;references:ID"`
}

type Request struct {
	Status RequestStatus
	Reason string
}

func (PartnerCustomer) TableName() string { return "partners_customers" }

type PartnerCustomerList []PartnerCustomer
