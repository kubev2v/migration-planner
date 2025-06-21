package validator

import (
	"testing"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
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
			if err != nil {
				messages := v.GetErrorMessage(err)
				found := false
				for _, m := range messages {
					if m == tt.message {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("validation message: got = %v, want = %s", messages, tt.message)
				}
			}
		})
	}
}
