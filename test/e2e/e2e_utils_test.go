package e2e_test

import (
	"archive/tar"
	"bytes"
	"fmt"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/onsi/gomega"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Create a source in the DB using the API
func CreateSource(name string) *v1alpha1.Source {
	source, err := svc.CreateSource(name)
	Expect(err).To(BeNil())
	Expect(source).NotTo(BeNil())
	return source
}

// Create VM with the UUID of the source created
func CreateAgent(configPath string, idForTest string, uuid uuid.UUID, vmName string) (PlannerAgent, string) {
	agent, err := NewPlannerAgent(configPath, uuid, vmName, idForTest)
	Expect(err).To(BeNil(), "Failed to create PlannerAgent")
	err = agent.Run()
	Expect(err).To(BeNil(), "Failed to run PlannerAgent")
	var agentIP string
	Eventually(func() string {
		agentIP, err = agent.GetIp()
		if err != nil {
			return ""
		}
		return agentIP
	}, "3m").ShouldNot(BeEmpty())
	Expect(agentIP).ToNot(BeEmpty())
	Eventually(func() bool {
		return agent.IsServiceRunning(agentIP, "planner-agent")
	}, "3m").Should(BeTrue())
	return agent, agentIP
}

// Login to VSphere and put the credentials
func LoginToVsphere(username string, password string, expectedStatusCode int) {
	res, err := agent.Login(fmt.Sprintf("https://%s:8989/sdk", systemIP), username, password)
	Expect(err).To(BeNil())
	Expect(res.StatusCode).To(Equal(expectedStatusCode))
}

// check that source is up to date eventually
func WaitForAgentToBeUpToDate(uuid uuid.UUID) {
	Eventually(func() bool {
		source, err := svc.GetSource(uuid)
		if err != nil {
			return false
		}
		return source.Agent.Status == v1alpha1.AgentStatusUpToDate
	}, "3m").Should(BeTrue())
}

// Wait for the service to return correct credential url for a source by UUID
func WaitForValidCredentialURL(uuid uuid.UUID, agentIP string) {
	Eventually(func() string {
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
	}, "3m").Should(Equal(fmt.Sprintf("https://%s:3333", agentIP)))
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

func RunCommand(ip string, command string) (string, error) {
	sshCmd := exec.Command("sshpass", "-p", "123456", "ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", fmt.Sprintf("core@%s", ip), command)

	var stdout, stderr bytes.Buffer
	sshCmd.Stdout = &stdout
	sshCmd.Stderr = &stderr

	if err := sshCmd.Run(); err != nil {
		return stderr.String(), fmt.Errorf("command failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	return stdout.String(), nil
}
