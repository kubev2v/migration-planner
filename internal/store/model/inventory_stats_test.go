package model

import (
	"testing"
)

func TestParseDomainName(t *testing.T) {
	tests := []struct {
		name       string
		assessment Assessment
		domainName string
		wantErr    bool
	}{
		{
			name: "valid email username",
			assessment: Assessment{
				Username: "doejoe@redhat.com",
			},
			domainName: "redhat.com",
			wantErr:    false,
		},
		{
			name: "email with subdomain",
			assessment: Assessment{
				Username: "user@mail.example.com",
			},
			domainName: "example.com",
			wantErr:    false,
		},
		{
			name: "invalid username without @",
			assessment: Assessment{
				Username: "doejoe",
			},
			domainName: "",
			wantErr:    true,
		},
		{
			name: "empty username",
			assessment: Assessment{
				Username: "",
			},
			domainName: "",
			wantErr:    true,
		},
		{
			name: "malformed email with @ but no domain",
			assessment: Assessment{
				Username: "doejoe@",
			},
			domainName: "",
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getDomainName(tt.assessment.Username)
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
