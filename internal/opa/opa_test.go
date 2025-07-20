package opa

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test Manager functionality
func TestNewManager(t *testing.T) {
	policiesDir := "./test-policies"
	manager := NewManager(policiesDir)

	require.NotNil(t, manager)
	require.NotNil(t, manager.config)
	assert.Equal(t, policiesDir, manager.config.PoliciesDir)
	assert.Equal(t, "127.0.0.1", manager.config.Host)
	assert.Equal(t, "8181", manager.config.Port)
	assert.False(t, manager.IsRunning())
}

func TestManagerInitializeWithValidDirectory(t *testing.T) {
	// Create temporary directory with .rego files
	tempDir := t.TempDir()
	policiesDir := filepath.Join(tempDir, "policies")
	err := os.MkdirAll(policiesDir, 0755)
	require.NoError(t, err)

	// Create a test .rego file
	testFile := filepath.Join(policiesDir, "test.rego")
	err = os.WriteFile(testFile, []byte("package test\ndefault allow = false"), 0644)
	require.NoError(t, err)

	manager := NewManager(policiesDir)
	err = manager.Initialize()
	require.NoError(t, err)
	assert.True(t, manager.IsRunning())

	manager.Shutdown()
	assert.False(t, manager.IsRunning())
}

func TestManagerInitializeWithInvalidDirectory(t *testing.T) {
	manager := NewManager("/non/existent/directory")
	err := manager.Initialize()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "policies directory does not exist")
	assert.False(t, manager.IsRunning())
}

func TestManagerShutdown(t *testing.T) {
	manager := NewManager("./test")

	// Should not panic when shutting down uninitialized manager
	assert.NotPanics(t, func() {
		manager.Shutdown()
	})
	assert.False(t, manager.IsRunning())
}

// Test Server functionality
func TestNewServer(t *testing.T) {
	config := &Config{
		Host:           "127.0.0.1",
		Port:           "8181",
		PoliciesDir:    "./test",
		StartupTimeout: 30 * time.Second,
	}

	server := NewServer(config)

	require.NotNil(t, server)
	assert.Equal(t, config, server.config)
	assert.NotNil(t, server.ctx)
	assert.NotNil(t, server.cancel)
	assert.NotNil(t, server.done)
}

func TestServerShutdown(t *testing.T) {
	config := &Config{
		Host:           "127.0.0.1",
		Port:           "8181",
		PoliciesDir:    "./test",
		StartupTimeout: 30 * time.Second,
	}

	server := NewServer(config)

	// Should not panic when shutting down uninitialized server
	assert.NotPanics(t, func() {
		server.Shutdown()
	})
}

func TestIsPoliciesDirectory(t *testing.T) {
	// Test with non-existent directory
	assert.False(t, isPoliciesDirectory("/non/existent/path"))

	// Test with directory without .rego files
	tempDir := t.TempDir()
	assert.False(t, isPoliciesDirectory(tempDir))

	// Test with directory containing .rego files
	testFile := filepath.Join(tempDir, "test.rego")
	err := os.WriteFile(testFile, []byte("package test"), 0644)
	require.NoError(t, err)
	assert.True(t, isPoliciesDirectory(tempDir))
}
