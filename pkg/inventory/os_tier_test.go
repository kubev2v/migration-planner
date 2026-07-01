package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyOSTier(t *testing.T) {
	tests := []struct {
		osName   string
		wantTier string
	}{
		// Certified
		{"Red Hat Enterprise Linux 7 (64-bit)", TierCertified},
		{"Red Hat Enterprise Linux 8 (64-bit)", TierCertified},
		{"Red Hat Enterprise Linux 9 (64-bit)", TierCertified},
		{"Red Hat Enterprise Linux 10 (64-bit)", TierCertified},
		{"Microsoft Windows 10 (64-bit)", TierCertified},
		{"Microsoft Windows 11 (64-bit)", TierCertified},
		{"Microsoft Windows Server 2016 (64-bit)", TierCertified},
		{"Microsoft Windows Server 2019 (64-bit)", TierCertified},
		{"Microsoft Windows Server 2022 (64-bit)", TierCertified},
		{"Microsoft Windows Server 2025 (64-bit)", TierCertified},

		// Commercial Vendor Supported
		{"SUSE Linux Enterprise 15 (64-bit)", TierVendorSupported},
		{"Ubuntu Linux 22.04 (64-bit)", TierVendorSupported},
		{"Ubuntu Linux 24.04 (64-bit)", TierVendorSupported},
		{"Oracle Linux 8 (64-bit)", TierVendorSupported},
		{"Oracle Linux 9 (64-bit)", TierVendorSupported},

		// Community Supported
		{"CentOS Stream 9 (64-bit)", TierCommunitySupported},
		{"CentOS Stream 10 (64-bit)", TierCommunitySupported},
		{"Debian GNU/Linux 12 (64-bit)", TierCommunitySupported},
		{"Fedora Linux (64-bit)", TierCommunitySupported},
		{"openSUSE Leap 15.6 (64-bit)", TierCommunitySupported},
		{"openSUSE Tumbleweed (64-bit)", TierCommunitySupported},

		// Special Handling (fallback)
		{"Red Hat Enterprise Linux 6 (64-bit)", TierSpecialHandling},
		{"CentOS 7 (64-bit)", TierSpecialHandling},
		{"Microsoft Windows 7 (64-bit)", TierSpecialHandling},
		{"Microsoft Windows Server 2012 R2 (64-bit)", TierSpecialHandling},
		{"Oracle Solaris 11 (64-bit)", TierSpecialHandling},
		{"Unknown OS", TierSpecialHandling},
	}

	for _, tt := range tests {
		t.Run(tt.osName, func(t *testing.T) {
			assert.Equal(t, tt.wantTier, ClassifyOSTier(tt.osName))
		})
	}
}
