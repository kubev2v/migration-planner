package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var TestOptions = struct {
	DisconnectedEnvironment bool
	DownloadImageByUrl      bool
}{}

const (
	DefaultOrganization string = "admin"
	DefaultUsername     string = "admin"
	VmName              string = "coreos-vm"
	Vsphere1Port        string = "8989"
	Vsphere2Port        string = "8990"
)

var (
	DefaultAgentTestID string = "1"
	DefaultBasePath    string = "/tmp/untarova/"
	DefaultVmdkName    string = filepath.Join(DefaultBasePath, "persistence-disk.vmdk")
	DefaultOvaPath     string = filepath.Join(Home, "myimage.ova")
	DefaultServiceUrl  string = fmt.Sprintf("http://%s:3443", SystemIP)
	Home               string = os.Getenv("HOME")
	PrivateKeyPath     string = filepath.Join(os.Getenv("E2E_PRIVATE_KEY_FOLDER_PATH"), "private-key")
	SystemIP           string = os.Getenv("PLANNER_IP")
	TestsExecutionTime        = make(map[string]time.Duration)
)
