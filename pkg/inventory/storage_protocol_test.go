package inventory

import "testing"

func TestStorageProtocolFromDatastoreType(t *testing.T) {
	tests := []struct {
		name          string
		datastoreType string
		want          string
	}{
		{name: "VMFS", datastoreType: "VMFS", want: "Block"},
		{name: "NFS 3", datastoreType: "NFS", want: "File"},
		{name: "NFS 4.1", datastoreType: "NFS41", want: "File"},
		{name: "vSAN", datastoreType: "VSAN", want: "Object"},
		{name: "normalizes case and whitespace", datastoreType: "  vmfs  ", want: "Block"},
		{name: "unknown", datastoreType: "OTHER", want: ""},
		{name: "empty", datastoreType: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StorageProtocolFromDatastoreType(tt.datastoreType); got != tt.want {
				t.Fatalf("StorageProtocolFromDatastoreType(%q) = %q, want %q", tt.datastoreType, got, tt.want)
			}
		})
	}
}
