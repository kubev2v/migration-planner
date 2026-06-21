package jobs

import (
	"regexp"
	"testing"
)

var riverQueueRegex = regexp.MustCompile(`^[a-z0-9]+([_-]?[a-z0-9]+)*$`)

func TestSanitizeQueueName(t *testing.T) {
	tests := []struct {
		hostname string
		want     string
	}{
		{"simple-pod", "rvtools-simple-pod"},
		{"pod.namespace.svc.cluster.local", "rvtools-pod-namespace-svc-cluster-local"},
		{"MyHost.Example.COM", "rvtools-myhost-example-com"},
		{"host_name", "rvtools-host-name"},
		{"host:8080", "rvtools-host-8080"},
		{"pod@node!#1", "rvtools-pod-node-1"},
		{".leading.dot", "rvtools-leading-dot"},
		{"trailing.", "rvtools-trailing"},
		{"host-.example.com", "rvtools-host-example-com"},
		{"...", "default"},
		{"@#$", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			got := sanitizeQueueName(tt.hostname)
			if got != tt.want {
				t.Errorf("sanitizeQueueName(%q) = %q, want %q", tt.hostname, got, tt.want)
			}
			if !riverQueueRegex.MatchString(got) {
				t.Errorf("sanitizeQueueName(%q) = %q, does not match River queue regex", tt.hostname, got)
			}
		})
	}
}
