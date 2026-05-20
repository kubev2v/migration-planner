package events

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/google/uuid"
)

const namespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

var eventSource = sync.OnceValue(func() string {
	if ns, err := os.ReadFile(namespacePath); err == nil && len(ns) > 0 {
		return string(ns)
	}
	return DefaultEventSource
})

func BuildCloudEvent(eventType string, payload any) ([]byte, error) {
	e := newCloudEvent(eventType, eventSource())

	if err := e.SetData(cloudevents.ApplicationJSON, payload); err != nil {
		return nil, fmt.Errorf("failed to set cloud event data: %w", err)
	}
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cloud event: %w", err)
	}
	return data, nil
}

func newCloudEvent(eventType, eventSource string) event.Event {
	e := cloudevents.NewEvent()
	e.SetID(uuid.New().String())
	e.SetSource(eventSource)
	e.SetType(eventType)
	e.SetTime(time.Now().UTC())
	return e
}
