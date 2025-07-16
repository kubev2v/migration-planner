package model

import (
	"testing"
)

func TestParseDomainName(t *testing.T) {
	ptr := func(s string) *string {
		return &s
	}

	tests := []struct {
		source     Source
		domainName string
		wantErr    bool
	}{
		{
			source: Source{
				Username:    "doejoe@redhat.com",
				EmailDomain: ptr("redhat.com"),
			},
			domainName: "redhat.com",
			wantErr:    false,
		},
		{
			source: Source{
				Username:    "",
				EmailDomain: ptr("redhat.com"),
			},
			domainName: "redhat.com",
			wantErr:    false,
		},
		{
			source: Source{
				Username:    "doejoe",
				EmailDomain: ptr("redhat.com"),
			},
			domainName: "redhat.com",
			wantErr:    false,
		},
		{
			source: Source{
				Username: "",
			},
			domainName: "",
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.source.Username, func(t *testing.T) {
			got, err := getDomainName(tt.source)
			if (err != nil) != tt.wantErr {
				t.Errorf("getDomainName: error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.domainName {
				t.Errorf("getDomainName: got = %v, want %v", got, tt.domainName)
			}
		})
	}
}
