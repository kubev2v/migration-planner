package model

import (
	"testing"
)

func TestParseDomainName(t *testing.T) {
	tests := []struct {
		username   string
		domainName string
		wantErr    bool
	}{
		{
			username:   "doejoe@redhat.com",
			domainName: "redhat.com",
			wantErr:    false,
		},
		{
			username:   "doejoe@another.redhat.com",
			domainName: "redhat.com",
			wantErr:    false,
		},
		{
			username:   "",
			domainName: "",
			wantErr:    true,
		},
		{
			username:   "doejoe@com",
			domainName: "com",
			wantErr:    false,
		},
		{
			username:   "doejoe",
			domainName: "",
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.username, func(t *testing.T) {
			got, err := getDomainName(tt.username)
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
