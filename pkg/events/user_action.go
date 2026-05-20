package events

import "time"

type UserActionEventPayload struct {
	UserAction UserActionData `json:"user_action"`
}

type UserActionData struct {
	Username     string    `json:"username"`
	AssessmentID *string   `json:"assessment_id,omitempty"`
	SourceID     *string   `json:"source_id,omitempty"`
	PartnerID    *string   `json:"partner_id,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

func NewUserActionPayload(data UserActionData) UserActionEventPayload {
	return UserActionEventPayload{UserAction: data}
}
