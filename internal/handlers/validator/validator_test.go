package validator

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	agentApi "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
)

const (
	validED25519SShKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILzKjzTWXASLbI+QKX8V7w+93JuHUoQRAOIQcgQibd3K test@ŧest"
	validRSASSHKey     = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCk83ddeteALlqCbO43E3ardbavFPboYIoFnlQZ3zVi+ls96c1x3P9DDWkNhuOgpQurull2y55Wm7HWLLK5hlk49s6tUuBDftH3XXfGMAmncBH9apGHxl0O+k/X1MrfhoEXHmmEwXTv+X6vC3BsZiazSOkKbIozHgnD7y1z83wuYWbbW0NYvgwhaoOtkWteKSJWwPxNaTwGCpj+RQ6xWygt5EbMSf7U3Ih2P1hcsa615zD5P2GSLxtLwWnHgWCylT/krdyIYlR1pqW9e/Iv2MKlGX6W1DSUxUz5BNxzCA8O53C0NSCeDFAhn9T8VE9U/RkGDtXBFJ8JVcmtM6S9buq5HZ12+0E0VCGFdmnvNT8XxdYrN0ff8f3DQI7ERgHEKQiqjrSPDv2+OMdv3nr3n5+tOBvQEn6aYDbnybILyrUP76UvLvjfgDTnnRxlkpw2Y43EtgtdeIUUo/VBSE9qfzRa21Pz3gBh6ZJE9xF+u6DstgvFigNJ7nMJoSktH5mzuBM= test@test"
	validEDSASshKey    = "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBC5VX/vbJWiGNGOzLNJNg1JlUgakBlyFEnG1JV43wWarrGxej9+9Ob7qeeoiQanA/FCXLvsI+/etNCBltmeI92c= test@test"
	invalidKey         = "ssh-rsa SOMEINVALIDKEY"
	validCert          = `
-----BEGIN CERTIFICATE-----
MIIDiTCCAnGgAwIBAgIUBvDjZ2irE/zWyKQxxRnPi3Ap5OowDQYJKoZIhvcNAQEL
BQAwVDELMAkGA1UEBhMCVVMxFDASBgNVBAcMC0dvdGhhbSBDaXR5MRowGAYDVQQK
DBFXYXluZSBFbnRlcnByaXNlczETMBEGA1UEAwwKYmF0bWFuLmNvbTAeFw0yNTA2
MTIxMjU1MzNaFw0yNjAzMDQxMjU1MzNaMFQxCzAJBgNVBAYTAlVTMRQwEgYDVQQH
DAtHb3RoYW0gQ2l0eTEaMBgGA1UECgwRV2F5bmUgRW50ZXJwcmlzZXMxEzARBgNV
BAMMCmJhdG1hbi5jb20wggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDk
w2USSDum5VXDR7uq3S7y2P+DJYB4k2cNcCJKkE0vSj6IlJ886YnohONGIPrZx2pa
5xZOR8yDaGzRTfRy5qt4X3RvctzEkDnXVSQFCOG1HoDmZ9EX7q9e0DlMX6tVV6Dm
Dv19C3zryHwA5zsG2xSVMJLLNlNbDmr+mgzNy9ot98MTRs8CszD/X0M7FkaYrmCo
9hUCm6ItU1R3rLLd60s2izso69zyjmW5ao8JuG9zfTKaL8Nvrt43xLLcMaR5iUTx
Fq29xHFk7YwmYPyH6lQQggQUGO7TMAidKGa9lSSmwKoB3HzSc+LP2ie34nY0wzyV
cXeKOYDbWQTZ2xMHDm/9AgMBAAGjUzBRMB0GA1UdDgQWBBShEoxZ2LSSMJaRFt9O
xZibvI5c+jAfBgNVHSMEGDAWgBShEoxZ2LSSMJaRFt9OxZibvI5c+jAPBgNVHRMB
Af8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQDNBS+evQj+4L7wNE/JReJ+MVen
ag8/v9/cBs0Yh+Rahglgp7iqzZDyrBwSUSRZ+BIVouHH8SuX2QPmW17Xy/IhNW6u
L0qx03is4pz+xjrXXIpKe+xlJGqYm/0DRLdDLBPSiMEtGm7sFQL8kBW5S6/1xg/B
4lX/tP70LbXygP+rkDjzVTRY3IVModi+fhKXxB3rWBH88IJTDYjQ0MmfXveLeTcK
TZUUZpsP4or19B48WSqiV/eMdCB/PxnFZYT1SyFLlDBiXolb+30HbGeeaF0bEg+u
1+6zqGMDx++ViZJ2IRU+rLETtnOwS3yV5dUCIRN7jZN1iLIjgcgs5XLKY1Ft
-----END CERTIFICATE-----
`
)

func TestSourceCreateFormValidators(t *testing.T) {
	ptr := func(s string) *string { return &s }
	tests := []struct {
		name          string
		form          v1alpha1.SourceCreate
		shouldFail    bool
		message       string
		validationErr error
	}{
		{
			name: "validation ok -- no proxy,no sshkey, no certs",
			form: v1alpha1.SourceCreate{
				Name: "test",
			},
			shouldFail: false,
		},
		{
			name: "validation ko -- name contain illegal chars",
			form: v1alpha1.SourceCreate{
				Name: "test$$$",
			},
			message:    "source name contains invalid characters",
			shouldFail: true,
		},
		{
			name: "validation ko -- name has more chars than allowed",
			form: v1alpha1.SourceCreate{
				Name: string(make([]byte, 0, 101)),
			},
			message:    "source name has more chars than allowed",
			shouldFail: true,
		},
		{
			name: "validation ok -- with rsa sshKey",
			form: v1alpha1.SourceCreate{
				Name:         "test",
				SshPublicKey: ptr(validRSASSHKey),
			},
			shouldFail: false,
		},
		{
			name: "validation ok -- with ed25529 sshKey",
			form: v1alpha1.SourceCreate{
				Name:         "test",
				SshPublicKey: ptr(validED25519SShKey),
			},
			shouldFail: false,
		},
		{
			name: "validation ok -- with edsa sshKey",
			form: v1alpha1.SourceCreate{
				Name:         "test",
				SshPublicKey: ptr(validED25519SShKey),
			},
			shouldFail: false,
		},
		{
			name: "validation ko -- invalid key",
			form: v1alpha1.SourceCreate{
				Name:         "test",
				SshPublicKey: ptr(invalidKey),
			},
			message:    "invalid ssh key",
			shouldFail: true,
		},
		{
			name: "validation ok -- valid cert",
			form: v1alpha1.SourceCreate{
				Name:             "test",
				CertificateChain: ptr(validCert),
			},
			shouldFail: false,
		},
		{
			name: "validation ok -- invalid cert",
			form: v1alpha1.SourceCreate{
				Name:             "test",
				CertificateChain: ptr("some string"),
			},
			message:    "invalid certificate chain",
			shouldFail: true,
		},
		{
			name: "validation ok -- with proxy",
			form: v1alpha1.SourceCreate{
				Name: "test",
				Proxy: &v1alpha1.AgentProxy{
					HttpUrl:  ptr("http://example.com"),
					HttpsUrl: ptr("https://example.com"),
					NoProxy:  ptr("domain"),
				},
			},
			shouldFail: false,
		},
		{
			name: "validation ko -- invalid http proxy",
			form: v1alpha1.SourceCreate{
				Name: "test",
				Proxy: &v1alpha1.AgentProxy{
					HttpUrl:  ptr("proxy"),
					HttpsUrl: ptr("https://example.com"),
				},
			},
			message:    "http proxy url must be a valid url and not starting with https",
			shouldFail: true,
		},
		{
			name: "validation ko -- invalid https proxy",
			form: v1alpha1.SourceCreate{
				Name: "test",
				Proxy: &v1alpha1.AgentProxy{
					HttpUrl:  ptr("http://example.com"),
					HttpsUrl: ptr("invalid proxy"),
				},
			},
			shouldFail: true,
			message:    "https proxy url must be a valid url and it should start with https",
		},
		{
			name: "validation ko -- https proxy passed as http proxy",
			form: v1alpha1.SourceCreate{
				Name: "test",
				Proxy: &v1alpha1.AgentProxy{
					HttpUrl:  ptr("https://example.com"),
					HttpsUrl: ptr("https://example.com"),
				},
			},
			message:    "http proxy url must be a valid url and not starting with https",
			shouldFail: true,
		},
		{
			name: "validation ko -- http proxy passed as https proxy",
			form: v1alpha1.SourceCreate{
				Name: "test",
				Proxy: &v1alpha1.AgentProxy{
					HttpUrl:  ptr("http://example.com"),
					HttpsUrl: ptr("http://example.com"),
				},
			},
			message:    "https proxy url must be a valid url and it should start with https",
			shouldFail: true,
		},
	}

	v := NewValidator()
	sourceRules := NewSourceValidationRules()
	v.Register(sourceRules...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(tt.form)
			if (err != nil) != tt.shouldFail {
				t.Errorf("validation: error = %v, shouldValidate = %v", err, tt.shouldFail)
				return
			}
		})
	}
}

func TestAgentUpdateFormValidators(t *testing.T) {
	tests := []struct {
		name          string
		form          agentApi.AgentStatusUpdate
		shouldFail    bool
		message       string
		validationErr error
	}{
		{
			name: "validation ok",
			form: agentApi.AgentStatusUpdate{
				SourceId:      uuid.New(),
				Status:        "waiting-for-credentials",
				StatusInfo:    "someinfo",
				CredentialUrl: "http://agent.com",
				Version:       "someversion",
			},
			shouldFail: false,
		},
		{
			name: "validation ko -- invalid status",
			form: agentApi.AgentStatusUpdate{
				SourceId:      uuid.New(),
				Status:        "invalid status",
				StatusInfo:    "someinfo",
				CredentialUrl: "http://agent.com",
				Version:       "someversion",
			},
			shouldFail: true,
		},
		{
			name: "validation ko - invalid credentials url",
			form: agentApi.AgentStatusUpdate{
				SourceId:      uuid.New(),
				Status:        "waiting-for-credentials",
				StatusInfo:    "someinfo",
				CredentialUrl: "invalid creds",
				Version:       "someversion",
			},
			shouldFail: true,
		},
		{
			name: "validation ko -- version has more than 20 characters",
			form: agentApi.AgentStatusUpdate{
				SourceId:      uuid.New(),
				Status:        "waiting-for-credentials",
				StatusInfo:    "someinfo",
				CredentialUrl: "http://agent.com",
				Version:       string(make([]byte, 0, 21)),
			},
			shouldFail: true,
		},
		{
			name: "validation ko -- statusinfo has more than 200 characters",
			form: agentApi.AgentStatusUpdate{
				SourceId:      uuid.New(),
				Status:        "waiting-for-credentials",
				StatusInfo:    string(make([]byte, 0, 201)),
				CredentialUrl: "http://agent.com",
				Version:       "someversion",
			},
			shouldFail: true,
		},
		{
			name: "validation ko -- source id is missing",
			form: agentApi.AgentStatusUpdate{
				Status:        "waiting-for-credentials",
				StatusInfo:    "someinfo",
				CredentialUrl: "http://agent.com",
				Version:       "someversion",
			},
			shouldFail: true,
		},
		{
			name: "validation ko -- status is missing",
			form: agentApi.AgentStatusUpdate{
				SourceId:      uuid.New(),
				StatusInfo:    "someinfo",
				CredentialUrl: "http://agent.com",
				Version:       "someversion",
			},
			shouldFail: true,
		},
		{
			name: "validation ko -- status info is missing",
			form: agentApi.AgentStatusUpdate{
				SourceId:      uuid.New(),
				Status:        "waiting-for-credentials",
				CredentialUrl: "http://agent.com",
				Version:       "someversion",
			},
			shouldFail: true,
		},
		{
			name: "validation ko -- credentials url is missing",
			form: agentApi.AgentStatusUpdate{
				SourceId:   uuid.New(),
				Status:     "waiting-for-credentials",
				StatusInfo: "some info",
				Version:    "someversion",
			},
			shouldFail: true,
		},
		{
			name: "validation ko -- version is missing",
			form: agentApi.AgentStatusUpdate{
				SourceId:      uuid.New(),
				Status:        "waiting-for-credentials",
				StatusInfo:    "some info",
				CredentialUrl: "http://agent.com",
			},
			shouldFail: true,
		},
	}

	v := NewValidator()
	v.Register(NewAgentValidationRules()...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(tt.form)
			if (err != nil) != tt.shouldFail {
				t.Errorf("validation: error = %v, shouldValidate = %v", err, tt.shouldFail)
				return
			}
		})
	}
}

func TestLabelValidator(t *testing.T) {
	tests := []struct {
		name       string
		label      string
		shouldPass bool
	}{
		// Valid labels
		{
			name:       "valid alphanumeric string",
			label:      "mylabel",
			shouldPass: true,
		},
		{
			name:       "valid label with numbers",
			label:      "label123",
			shouldPass: true,
		},
		{
			name:       "valid label with hyphens",
			label:      "my-label",
			shouldPass: true,
		},
		{
			name:       "valid label with underscores",
			label:      "my_label",
			shouldPass: true,
		},
		{
			name:       "valid label with dots",
			label:      "my.label",
			shouldPass: true,
		},
		{
			name:       "valid label with mixed special characters",
			label:      "my-label_123.test",
			shouldPass: true,
		},

		// Invalid labels
		{
			name:       "invalid empty string",
			label:      "",
			shouldPass: false,
		},
		{
			name:       "invalid starts with hyphen",
			label:      "-mylabel",
			shouldPass: false,
		},
		{
			name:       "invalid ends with hyphen",
			label:      "mylabel-",
			shouldPass: false,
		},
		{
			name:       "invalid contains space",
			label:      "my label",
			shouldPass: false,
		},
		{
			name:       "invalid contains special characters",
			label:      "my@label",
			shouldPass: false,
		},
		{
			name:       "invalid only special characters",
			label:      "---",
			shouldPass: false,
		},
	}

	v := NewValidator()
	sourceRules := NewSourceValidationRules()
	v.Register(sourceRules...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test struct with the label field
			testStruct := struct {
				Label string `validate:"label"`
			}{
				Label: tt.label,
			}

			err := v.Struct(testStruct)
			if (err == nil) != tt.shouldPass {
				t.Errorf("labelValidator(%q): expected pass=%v, got pass=%v, error=%v",
					tt.label, tt.shouldPass, err == nil, err)
			}
		})
	}
}
