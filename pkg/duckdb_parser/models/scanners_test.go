package models

import (
	"reflect"
	"testing"
)

func TestToStringSlice(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  []string
	}{
		{
			name:  "string slice",
			value: []string{"naa.1", "naa.2"},
			want:  []string{"naa.1", "naa.2"},
		},
		{
			name:  "interface slice",
			value: []interface{}{"naa.1", nil, "naa.2"},
			want:  []string{"naa.1", "naa.2"},
		},
		{
			name:  "valid JSON",
			value: `["naa.1","naa.2"]`,
			want:  []string{"naa.1", "naa.2"},
		},
		{
			name:  "invalid JSON",
			value: `not-json`,
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toStringSlice(tt.value)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toStringSlice(%#v) = %#v, want %#v", tt.value, got, tt.want)
			}
		})
	}
}

func TestDisksScanStoresDatastoreBackingDeviceIDs(t *testing.T) {
	var disks Disks
	value := []interface{}{
		map[string]interface{}{
			"DatastoreBackingDeviceIDs": []interface{}{"naa.1", "naa.2"},
		},
	}

	if err := disks.Scan(value); err != nil {
		t.Fatalf("Disks.Scan() error = %v", err)
	}
	if len(disks) != 1 {
		t.Fatalf("Disks.Scan() returned %d disks, want 1", len(disks))
	}
	want := []string{"naa.1", "naa.2"}
	if !reflect.DeepEqual(disks[0].DatastoreBackingDeviceIDs, want) {
		t.Errorf("DatastoreBackingDeviceIDs = %#v, want %#v", disks[0].DatastoreBackingDeviceIDs, want)
	}
}
