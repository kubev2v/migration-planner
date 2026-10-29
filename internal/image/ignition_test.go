package image

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	"github.com/google/uuid"
)

func TestIgnitionTemplateAgentUpdate(t *testing.T) {
	b := NewImageBuilder(uuid.New())
	b.Template = "../../" + defaultTemplate

	// Template and Butane translation must both succeed.
	if _, err := b.generateIgnition(); err != nil {
		t.Fatal(err)
	}

	var rendered bytes.Buffer
	if err := template.Must(template.ParseFiles(b.Template)).Execute(&rendered, IgnitionData{}); err != nil {
		t.Fatal(err)
	}
	out := rendered.String()
	for _, want := range []string{
		"podman auto-update --format '{{.Updated}}'", // Go template escaping kept podman's format
		`"${UPDATE_DIR}/status.json"`,
		"HealthCmd=curl -fsk https://localhost:3333/api/v2/version",
		"Notify=healthy",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered ignition is missing %q", want)
		}
	}
	if strings.Contains(out, "podman-auto-update.timer") {
		t.Error("the daily podman auto-update timer must not be enabled")
	}
}
