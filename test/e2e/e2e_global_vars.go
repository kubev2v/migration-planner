package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var TestOptions = struct {
	DisconnectedEnvironment bool
}{}

const (
	DefaultOrganization = "admin"
	DefaultUsername     = "admin"
	DefaultEmailDomain  = "example.com"
	VmName              = "coreos-vm"
	Vsphere1Port        = "8989"
	Vsphere2Port        = "8990"
)

var (
	DefaultAgentTestID = "1"
	DefaultBasePath    = "/tmp/untarova/"
	DefaultVmdkName    = filepath.Join(DefaultBasePath, "persistence-disk.vmdk")
	DefaultOvaPath     = filepath.Join(Home, "myimage.ova")
	DefaultServiceUrl  = fmt.Sprintf("http://%s:7443/api/migration-assessment", SystemIP)
	Home               = os.Getenv("HOME")
	PrivateKeyPath     = filepath.Join(os.Getenv("E2E_PRIVATE_KEY_FOLDER_PATH"), "private-key")
	SystemIP           = os.Getenv("PLANNER_IP")
	TestsExecutionTime = make(map[string]time.Duration)
)
