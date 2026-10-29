package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyOSMTVStatus(t *testing.T) {
	for _, tt := range []struct{ name, status string }{
		{"Red Hat Enterprise Linux 7 (64-bit)", MTVStatusGreen},
		{"Red Hat Enterprise Linux 8 (64-bit)", MTVStatusGreen},
		{"Red Hat Enterprise Linux 9 (64-bit)", MTVStatusGreen},
		{"Red Hat Enterprise Linux 10 (64-bit)", MTVStatusGreen},
		{"Microsoft Windows 10 (64-bit)", MTVStatusBlue},
		{"Microsoft Windows 11 (64-bit)", MTVStatusBlue},
		{"Microsoft Windows Server 2016 (64-bit)", MTVStatusBlue},
		{"Microsoft Windows Server 2019 (64-bit)", MTVStatusBlue},
		{"Microsoft Windows Server 2022 (64-bit)", MTVStatusBlue},
		{"Microsoft Windows Server 2025 (64-bit)", MTVStatusBlue},
		{"Red Hat Enterprise Linux 6 (64-bit)", MTVStatusBlue},
		{"Microsoft Windows 7 (64-bit)", ""},
		{"Microsoft Windows XP (32-bit)", ""},
		{"Microsoft Windows Server 2003 (32-bit)", ""},
		{"Microsoft Windows Server 2008 (32-bit)", ""},
		{"Microsoft Windows Server 2008 (64-bit)", ""},
		{"Microsoft Windows Server 2008 R2 (64-bit)", ""},
		{"Microsoft Windows Server 2012 (64-bit)", ""},
		{"Microsoft Windows Server 2012 R2 (64-bit)", MTVStatusBlue},
		{"  microsoft windows 11 (64-bit)  ", MTVStatusBlue},
		{"Red Hat Enterprise Linux 9 (ARM64)", ""},
		{"Red Hat Enterprise Linux 9 (s390x)", ""},
		{"Red Hat Enterprise Linux 9 ARM (64-bit)", ""},
		{"Red Hat Enterprise Linux 9", MTVStatusGreen},
		{"Microsoft Windows Server 2016 or later (64-bit)", MTVStatusBlue},
		{"Microsoft Windows Server 2022 x64", MTVStatusBlue},
		{"Microsoft Windows 10 (32-bit)", ""},
		{"Microsoft Windows XP (64-bit)", ""},
		{"Ubuntu Linux 24.04 (64-bit)", ""},
		{"Oracle Linux 10 (64-bit)", ""},
		{"Unknown OS", ""},
		{"", ""},
	} {
		t.Run(tt.name, func(t *testing.T) { assert.Equal(t, tt.status, ClassifyOSMTVStatus(tt.name)) })
	}
}
