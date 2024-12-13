package client

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/middleware"
	"github.com/kubev2v/migration-planner/internal/api/client"
	"github.com/kubev2v/migration-planner/pkg/reqid"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/yaml"
)

const (
	// TestRootDirEnvKey is the environment variable key used to set the file system root when testing.
	TestRootDirEnvKey = "PLANNER_TEST_ROOT_DIR"
)

// Config holds the information needed to connect to a Planner API server
type Config struct {
	Service Service `json:"service"`

	// baseDir is used to resolve relative paths
	// If baseDir is empty, the current working directory is used.
	baseDir string `json:"-"`
	// TestRootDir is the root directory for test files.
	testRootDir string `json:"-"`
}

// Service contains information how to connect to and authenticate the Planner API server.
type Service struct {
	// Server is the URL of the Planner API server (the part before /api/v1/...).
	Server string `json:"server"`
	UI     string `json:"ui"`
}

func (c *Config) Equal(c2 *Config) bool {
	if c == c2 {
		return true
	}
	if c == nil || c2 == nil {
		return false
	}
	return c.Service.Equal(&c2.Service)
}

func (s *Service) Equal(s2 *Service) bool {
	if s == s2 {
		return true
	}
	if s == nil || s2 == nil {
		return false
	}
	return s.Server == s2.Server
}

func (c *Config) DeepCopy() *Config {
	if c == nil {
		return nil
	}
	return &Config{
		Service:     *c.Service.DeepCopy(),
		baseDir:     c.baseDir,
		testRootDir: c.testRootDir,
	}
}

func (s *Service) DeepCopy() *Service {
	if s == nil {
		return nil
	}
	s2 := *s
	return &s2
}

func (c *Config) SetBaseDir(baseDir string) {
	c.baseDir = baseDir
}

func NewDefault() *Config {
	c := &Config{}

	if value := os.Getenv(TestRootDirEnvKey); value != "" {
		c.testRootDir = filepath.Clean(value)
	}

	return c
}

// NewFromConfig returns a new Planner API client from the given config.
func NewFromConfig(config *Config) (*client.ClientWithResponses, error) {

	httpClient, err := NewHTTPClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("NewFromConfig: creating HTTP client %w", err)
	}
	ref := client.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		req.Header.Set(middleware.RequestIDHeader, reqid.GetReqID())
		return nil
	})
	return client.NewClientWithResponses(config.Service.Server, client.WithHTTPClient(httpClient), ref)
}

// NewHTTPClientFromConfig returns a new HTTP Client from the given config.
func NewHTTPClientFromConfig(config *Config) (*http.Client, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     false,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	return httpClient, nil
}

// DefaultPlannerClientConfigPath returns the default path to the Planner client config file.
func DefaultPlannerClientConfigPath() string {
	return filepath.Join(homedir.HomeDir(), ".planner", "client.yaml")
}

func ParseConfigFile(filename string) (*Config, error) {
	contents, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	config := NewDefault()
	if err := yaml.Unmarshal(contents, config); err != nil {
		return nil, fmt.Errorf("decoding config: %w", err)
	}
	config.SetBaseDir(filepath.Dir(filename))
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}

// NewFromConfigFile returns a new Planner API client using the config read from the given file.
func NewFromConfigFile(filename string) (*client.ClientWithResponses, error) {
	config, err := ParseConfigFile(filename)
	if err != nil {
		return nil, err
	}
	return NewFromConfig(config)
}

// WriteConfig writes a client config file using the given parameters.
func WriteConfig(filename string, server string) error {
	config := NewDefault()
	config.Service = Service{
		Server: server,
	}

	return config.Persist(filename)
}

func (c *Config) Persist(filename string) error {
	contents, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	directory := filename[:strings.LastIndex(filename, "/")]
	if err := os.MkdirAll(directory, 0700); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.WriteFile(filename, contents, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

func (c *Config) Validate() error {
	validationErrors := make([]error, 0)
	validationErrors = append(validationErrors, validateService(c.Service)...)
	if len(validationErrors) > 0 {
		return fmt.Errorf("invalid configuration: %v", utilerrors.NewAggregate(validationErrors).Error())
	}
	return nil
}

func validateService(service Service) []error {
	validationErrors := make([]error, 0)
	// Make sure the server is specified and well-formed
	if len(service.Server) == 0 {
		validationErrors = append(validationErrors, fmt.Errorf("no server found"))
	} else {
		u, err := url.Parse(service.Server)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("invalid server format %q: %w", service.Server, err))
		}
		if err == nil && len(u.Hostname()) == 0 {
			validationErrors = append(validationErrors, fmt.Errorf("invalid server format %q: no hostname", service.Server))
		}
	}
	return validationErrors
}
