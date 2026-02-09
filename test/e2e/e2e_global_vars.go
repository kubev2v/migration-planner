package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultOrganization = "admin"
	DefaultUsername     = "admin"
	DefaultEmailDomain  = "example.com"
	Vsphere1Port        = "8989"
	Vsphere2Port        = "8990"
)

var (
	DefaultBasePath    = "/tmp/untarova/"
	DefaultServiceUrl  = fmt.Sprintf("http://%s:7443/api/migration-assessment", SystemIP)
	Home               = os.Getenv("HOME")
	PrivateKeyPath     = filepath.Join(os.Getenv("E2E_PRIVATE_KEY_FOLDER_PATH"), "private-key")
	SystemIP           = os.Getenv("PLANNER_IP")
	TestsExecutionTime = make(map[string]time.Duration)
)
