package applianceupdater

import (
	"io"
	"strings"
	"testing"
)

func TestBoundedOutputCopy(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int
	}{
		{"stdout", 1024 * 1024},
		{"stderr", 4096},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := &boundedOutput{Limit: tc.limit}
			input := strings.Repeat("x", tc.limit+8192)
			// Hide strings.Reader.WriteTo to exercise io.Copy's destination
			// ReaderFrom optimization, as subprocess pipe readers do.
			reader := struct{ io.Reader }{strings.NewReader(input)}
			n, err := io.Copy(output, reader)
			if err != nil || n != int64(len(input)) {
				t.Fatalf("output was not fully drained: %d %v", n, err)
			}
			if output.Len() != tc.limit || string(output.Bytes()) != input[:tc.limit] {
				t.Fatalf("retained output exceeded or differed from its bounded prefix: %d", output.Len())
			}
		})
	}
}
