package model

import "github.com/google/uuid"

type RequestStatus string

const (
	RequestStatusPending  RequestStatus = "pending"
	RequestStatusAccepted RequestStatus = "accepted"
	RequestStatusRejected RequestStatus = "rejected"
)

// PartnerCustomer represents a partner request.
// DB constraints:
//   - uq_partner_customer_active_username: unique(username) WHERE request_status IN ('pending','accepted') — one active request per user
//   - idx_partners_customers_partner_id: index on partner_id for partner-side queries
type PartnerCustomer struct {
	ID            uuid.UUID     `gorm:"primaryKey;column:id;type:VARCHAR(255);"`
	Username      string        `gorm:"not null;type:VARCHAR(100)"`
	PartnerID     string        `gorm:"not null;type:VARCHAR(100)"`
	RequestStatus RequestStatus `gorm:"not null;type:request_status;default:'pending'"`
	Name          string        `gorm:"not null;type:VARCHAR(100)"`
	ContactName   string        `gorm:"not null;type:VARCHAR(100)"`
	ContactPhone  string        `gorm:"not null;type:VARCHAR(100)"`
	Email         string        `gorm:"not null;type:VARCHAR(100)"`
	Location      string        `gorm:"not null;type:VARCHAR(100)"`
	Reason        *string       `gorm:"type:VARCHAR(255)"`
}

type Request struct {
	Status RequestStatus
	Reason string
}

func (PartnerCustomer) TableName() string { return "partners_customers" }

type PartnerCustomerList []PartnerCustomer
