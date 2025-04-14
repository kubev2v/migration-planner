package e2e_test

import (
	"archive/tar"
	"bytes"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/cli"
	"github.com/libvirt/libvirt-go"
	"go.uber.org/zap"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Create VM with the UUID of the source created
func CreateAgent(idForTest string, uuid uuid.UUID, vmName string) (PlannerAgent, error) {
	zap.S().Info("Creating agent...")
	agent, err := NewPlannerAgent(uuid, vmName, idForTest)
	if err != nil {
		return nil, err
	}
	err = agent.Run()
	if err != nil {
		return nil, err
	}
	zap.S().Info("Agent created successfully")
	return agent, nil
}

// store the ip case there is no error
func FindAgentIp(agent PlannerAgent, agentIP *string) error {
	zap.S().Info("Attempting to retrieve agent IP")
	ip, err := agent.GetIp()
	if err != nil {
		return err
	}
	*agentIP = ip
	return nil
}

func IsPlannerAgentRunning(agent PlannerAgent, agentIP string) bool {
	return agent.IsServiceRunning(agentIP, "planner-agent")
}

// helper function to check that source is up to date eventually
func AgentIsUpToDate(svc PlannerService, uuid uuid.UUID) bool {
	source, err := svc.GetSource(uuid)
	if err != nil {
		zap.S().Errorf("Error getting source.")
		return false
	}
	zap.S().Infof("agent status is: %s", string(source.Agent.Status))
	return source.Agent.Status == v1alpha1.AgentStatusUpToDate
}

// helper function for wait until the service return correct credential url for a source by UUID
func CredentialURL(svc PlannerService, uuid uuid.UUID) string {
	zap.S().Info("try to retrieve valid credentials url")
	s, err := svc.GetSource(uuid)
	if err != nil {
		return ""
	}
	if s.Agent == nil {
		return ""
	}
	if s.Agent.CredentialUrl != "N/A" && s.Agent.CredentialUrl != "" {
		return s.Agent.CredentialUrl
	}

	return ""
}

func ValidateTar(file *os.File) error {
	_, _ = file.Seek(0, 0)
	tarReader := tar.NewReader(file)
	containsOvf := false
	containsVmdk := false
	containsIso := false
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		switch header.Typeflag {
		case tar.TypeReg:
			if strings.HasSuffix(header.Name, ".vmdk") {
				containsVmdk = true
			}
			if strings.HasSuffix(header.Name, ".ovf") {
				// Validate OVF file
				ovfContent, err := io.ReadAll(tarReader)
				if err != nil {
					return fmt.Errorf("failed to read OVF file: %w", err)
				}

				// Basic validation: check if the content contains essential OVF elements
				if !strings.Contains(string(ovfContent), "<Envelope") ||
					!strings.Contains(string(ovfContent), "<VirtualSystem") {
					return fmt.Errorf("invalid OVF file: missing essential elements")
				}
				containsOvf = true
			}
			if strings.HasSuffix(header.Name, ".iso") {
				containsIso = true
			}
		}
	}
	if !containsOvf {
		return fmt.Errorf("error ova image don't contain file with ovf suffix")
	}
	if !containsVmdk {
		return fmt.Errorf("error ova image don't contain file with vmdk suffix")
	}
	if !containsIso {
		return fmt.Errorf("error ova image don't contain file with iso suffix")
	}

	return nil
}

func Untar(file *os.File, destFile string, fileName string) error {
	_, _ = file.Seek(0, 0)
	tarReader := tar.NewReader(file)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		switch header.Typeflag {
		case tar.TypeReg:
			if header.Name == fileName {
				outFile, err := os.Create(destFile)
				if err != nil {
					return fmt.Errorf("failed to create file: %w", err)
				}
				defer outFile.Close()

				if _, err := io.Copy(outFile, tarReader); err != nil {
					return fmt.Errorf("failed to write file: %w", err)
				}
				return nil
			}
		}
	}

	return fmt.Errorf("file %s not found", fileName)
}

func (p *plannerAgentLibvirt) CreateVm() error {
	// Read VM XML definition from file
	vmXMLBytes, err := os.ReadFile(p.getConfigXmlVMPath())
	if err != nil {
		return fmt.Errorf("failed to read VM XML file: %v", err)
	}
	domain, err := p.con.DomainDefineXML(string(vmXMLBytes))
	if err != nil {
		return fmt.Errorf("failed to define domain: %v", err)
	}
	defer func() {
		_ = domain.Free()
	}()

	// Start the domain
	if err := domain.Create(); err != nil {
		return fmt.Errorf("failed to create domain: %v", err)
	}
	return nil
}

func waitForDomainState(duration time.Duration, domain *libvirt.Domain, desiredState libvirt.DomainState) error {
	timeout := time.After(duration)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting for desired state")
		case <-ticker.C:
			state, _, err := domain.GetState()
			if err != nil {
				return fmt.Errorf("failed to get VM state: %w", err)
			}
			if state == desiredState {
				return nil
			}
		}
	}
}

// ConvertVMDKtoQCOW2 converts a VMDK file to QCOW2 using qemu-img
func ConvertVMDKtoQCOW2(src string, dst string) error {
	command := fmt.Sprintf("qemu-img convert -f vmdk -O qcow2 %s %s", src, dst)
	output, err := RunLocalCommand(command)
	if err != nil {
		return fmt.Errorf("conversion failed: %v\nOutput: %s", err, output)
	}
	return nil
}

// RunLocalCommand runs the given shell command locally and returns its combined output or error
func RunLocalCommand(command string) (string, error) {
	cmd := exec.Command("bash", "-c", command)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func RunSSHCommand(ip string, command string) (string, error) {
	sshCmd := exec.Command("sshpass", "-p", "123456", "ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", fmt.Sprintf("core@%s", ip), command)

	var stdout, stderr bytes.Buffer
	sshCmd.Stdout = &stdout
	sshCmd.Stderr = &stderr

	if err := sshCmd.Run(); err != nil {
		return stderr.String(), fmt.Errorf("command failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	return stdout.String(), nil
}

func getToken(username string, organization string) (string, error) {
	privateKeyString, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return "", fmt.Errorf("error, unable to read the private key: %v", err)
	}

	privateKey, err := cli.ParsePrivateKey(string(privateKeyString))
	if err != nil {
		return "", fmt.Errorf("error with parsing the private key: %v", err)
	}

	token, err := cli.GenerateToken(username, organization, privateKey)
	if err != nil {
		return "", fmt.Errorf("error, unable to generate token: %v", err)
	}

	return token, nil
}

// Remove OS file if exist
func RemoveFile(fullPath string) error {
	if _, err := os.Stat(fullPath); err == nil {
		if err := os.Remove(fullPath); err != nil {
			return fmt.Errorf("failed to remove file: %v", err)
		}
	}
	return nil
}

func defaultUserAuth() (*auth.User, error) {
	tokenVal, err := getToken(defaultUsername, defaultOrganization)
	if err != nil {
		return nil, fmt.Errorf("unable to create user: %v", err)
	}
	token := jwt.New(jwt.SigningMethodRS256)
	token.Raw = tokenVal
	return &auth.User{
		Username:     defaultUsername,
		Organization: defaultOrganization,
		Token:        token,
	}, nil
}

func logExecutionSummary() {
	zap.S().Infof("============Summarizing execution time============")

	type testResult struct {
		name     string
		duration time.Duration
	}

	var results []testResult

	for test, duration := range testsExecutionTime {
		results = append(results, testResult{name: test, duration: duration})
	}

	// Sort tests by duration descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].duration > results[j].duration
	})

	for _, result := range results {
		zap.S().Infof("[%s] finished after: %s", result.name, result.duration.Round(time.Second))
	}
}
