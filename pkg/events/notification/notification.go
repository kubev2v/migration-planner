package notification

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Notification matches the Console Notifications service message schema.
type Notification struct {
	ID          string            `json:"id"`
	Version     string            `json:"version"`
	Bundle      string            `json:"bundle"`
	Application string            `json:"application"`
	EventType   string            `json:"event_type"`
	Timestamp   string            `json:"timestamp"`
	OrgID       string            `json:"org_id"`
	Severity    string            `json:"severity"`
	Context     map[string]string `json:"context,omitempty"`
	Events      []Event           `json:"events"`
	Recipients  []Recipient       `json:"recipients,omitempty"`
}

type Event struct {
	Metadata map[string]string `json:"metadata"`
	Payload  any               `json:"payload"`
}

type Recipient struct {
	OnlyAdmins            bool     `json:"only_admins"`
	IgnoreUserPreferences bool     `json:"ignore_user_preferences"`
	Users                 []string `json:"users,omitempty"`
}

// New builds a Notification for eventType, stamping the fields fixed by the
// application's registration with the notification service (id, version,
// bundle, application, timestamp).
func New(eventType, orgID, severity string, context map[string]string, payload any, recipients ...Recipient) Notification {
	return Notification{
		ID:          uuid.New().String(),
		Version:     schemaVersion,
		Bundle:      bundle,
		Application: application,
		EventType:   eventType,
		Timestamp:   time.Now().UTC().Format("2006-01-02T15:04:05"),
		OrgID:       orgID,
		Severity:    severity,
		Context:     context,
		Events:      []Event{{Metadata: map[string]string{}, Payload: payload}},
		Recipients:  recipients,
	}
}

// Build constructs a Notification for eventType and marshals it to JSON,
// ready to be stored in the outbox and later dispatched by a Writer.
func Build(eventType, orgID, severity string, context map[string]string, payload any, recipients ...Recipient) ([]byte, error) {
	data, err := json.Marshal(New(eventType, orgID, severity, context, payload, recipients...))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal notification %s: %w", eventType, err)
	}
	return data, nil
}
