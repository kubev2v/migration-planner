package events

import "time"

type VisitorEventPayload struct {
	Visitor VisitorData `json:"visitor"`
}

type VisitorData struct {
	Username  string    `json:"username"`
	OrgID     string    `json:"org_id"`
	Timestamp time.Time `json:"timestamp"`
}

func NewVisitorPayload(username, orgID string) VisitorEventPayload {
	return VisitorEventPayload{
		Visitor: VisitorData{
			Username:  username,
			OrgID:     orgID,
			Timestamp: time.Now().UTC(),
		},
	}
}
