package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCpuTierKey(t *testing.T) {
	tests := []struct {
		name     string
		cpuCount int
		want     string
	}{
		{
			name:     "Zero CPUs",
			cpuCount: 0,
			want:     "0-4",
		},
		{
			name:     "1 CPU",
			cpuCount: 1,
			want:     "0-4",
		},
		{
			name:     "4 CPUs (upper boundary of first tier)",
			cpuCount: 4,
			want:     "0-4",
		},
		{
			name:     "5 CPUs (lower boundary of second tier)",
			cpuCount: 5,
			want:     "5-8",
		},
		{
			name:     "8 CPUs (upper boundary of second tier)",
			cpuCount: 8,
			want:     "5-8",
		},
		{
			name:     "9 CPUs (lower boundary of third tier)",
			cpuCount: 9,
			want:     "9-16",
		},
		{
			name:     "16 CPUs (upper boundary of third tier)",
			cpuCount: 16,
			want:     "9-16",
		},
		{
			name:     "17 CPUs (lower boundary of fourth tier)",
			cpuCount: 17,
			want:     "17-32",
		},
		{
			name:     "32 CPUs (upper boundary of fourth tier)",
			cpuCount: 32,
			want:     "17-32",
		},
		{
			name:     "33 CPUs (highest tier)",
			cpuCount: 33,
			want:     "32+",
		},
		{
			name:     "64 CPUs (highest tier)",
			cpuCount: 64,
			want:     "32+",
		},
		{
			name:     "128 CPUs (highest tier)",
			cpuCount: 128,
			want:     "32+",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cpuTierKey(tt.cpuCount)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMemoryTierKey(t *testing.T) {
	tests := []struct {
		name     string
		memoryGB int
		want     string
	}{
		{
			name:     "Zero GB",
			memoryGB: 0,
			want:     "0-4",
		},
		{
			name:     "1 GB",
			memoryGB: 1,
			want:     "0-4",
		},
		{
			name:     "4 GB (upper boundary of first tier)",
			memoryGB: 4,
			want:     "0-4",
		},
		{
			name:     "5 GB (lower boundary of second tier)",
			memoryGB: 5,
			want:     "5-16",
		},
		{
			name:     "16 GB (upper boundary of second tier)",
			memoryGB: 16,
			want:     "5-16",
		},
		{
			name:     "17 GB (lower boundary of third tier)",
			memoryGB: 17,
			want:     "17-32",
		},
		{
			name:     "32 GB (upper boundary of third tier)",
			memoryGB: 32,
			want:     "17-32",
		},
		{
			name:     "33 GB (lower boundary of fourth tier)",
			memoryGB: 33,
			want:     "33-64",
		},
		{
			name:     "64 GB (upper boundary of fourth tier)",
			memoryGB: 64,
			want:     "33-64",
		},
		{
			name:     "65 GB (lower boundary of fifth tier)",
			memoryGB: 65,
			want:     "65-128",
		},
		{
			name:     "128 GB (upper boundary of fifth tier)",
			memoryGB: 128,
			want:     "65-128",
		},
		{
			name:     "129 GB (lower boundary of sixth tier)",
			memoryGB: 129,
			want:     "129-256",
		},
		{
			name:     "256 GB (upper boundary of sixth tier)",
			memoryGB: 256,
			want:     "129-256",
		},
		{
			name:     "257 GB (highest tier)",
			memoryGB: 257,
			want:     "256+",
		},
		{
			name:     "512 GB (highest tier)",
			memoryGB: 512,
			want:     "256+",
		},
		{
			name:     "1024 GB (highest tier)",
			memoryGB: 1024,
			want:     "256+",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := memoryTierKey(tt.memoryGB)
			assert.Equal(t, tt.want, got)
		})
	}
}
