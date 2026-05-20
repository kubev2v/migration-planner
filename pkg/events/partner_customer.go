package events

import "time"

type PartnerCustomerEventPayload struct {
	PartnerCustomer PartnerCustomerData `json:"partner_customer"`
}

type PartnerCustomerData struct {
	ID               string     `json:"id"`
	CustomerUsername string     `json:"customer_username"`
	PartnerID        string     `json:"partner_id"`
	RequestStatus    string     `json:"request_status"`
	Location         string     `json:"location"`
	AcceptedAt       *time.Time `json:"accepted_at,omitempty"`
	TerminatedAt     *time.Time `json:"terminated_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

func NewPartnerCustomerPayload(data PartnerCustomerData) PartnerCustomerEventPayload {
	return PartnerCustomerEventPayload{PartnerCustomer: data}
}
