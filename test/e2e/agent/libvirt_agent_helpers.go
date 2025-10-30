package agent

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/utils"
	"github.com/libvirt/libvirt-go"
	"go.uber.org/zap"
)

// cleanupAgentFiles removes all temporary and generated files used during VM setup,
// including the OVA, ISO, VMDK, and QCOW files.
func (p *plannerAgentLibvirt) cleanupAgentFiles() error {
	if err := RemoveFile(DefaultOvaPath); err != nil {
		return fmt.Errorf("failed to remove OVA file: %w", err)
	}

	if err := RemoveFile(p.isoFilePath()); err != nil {
		return fmt.Errorf("failed to remove ISO file: %w", err)
	}

	if err := RemoveFile(DefaultVmdkName); err != nil {
		return fmt.Errorf("failed to remove Vmdk file: %w", err)
	}

	if err := RemoveFile(p.qcowDiskFilePath()); err != nil {
		return fmt.Errorf("failed to remove qcow disk file: %w", err)
	}

	return nil
}

// prepareImage handles the full preparation of the agent VM image:
// it downloads the OVA (from URL or image service), extracts the required ISO and VMDK files,
// and converts the VMDK to QCOW format.
func (p *plannerAgentLibvirt) prepareImage() error {
	// Create OVA:
	ovaFile, err := os.Create(DefaultOvaPath)
	if err != nil {
		return err
	}
	defer os.Remove(ovaFile.Name())

	if err = os.Mkdir(DefaultBasePath, os.ModePerm); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("creating default base path: %w", err)
		}
	}

	if err := p.downloadOvaFromUrl(ovaFile); err != nil {
		return fmt.Errorf("failed to download OVA image from url: %w", err)
	}
	zap.S().Infof("Successfully downloaded ova file: %s", DefaultOvaPath)

	if err := p.ovaValidateAndExtract(ovaFile); err != nil {
		return err
	}

	zap.S().Infof("Successfully extracted the Iso and Vmdk files from the OVA.")

	if err := ConvertVMDKtoQCOW2(DefaultVmdkName, p.qcowDiskFilePath()); err != nil {
		return fmt.Errorf("failed to convert vmdk to qcow: %w", err)
	}

	zap.S().Infof("Successfully converted the vmdk to qcow.")

	return nil
}

// downloadOvaFromUrl fetches the OVA file from a remote URL and writes it to the provided file
func (p *plannerAgentLibvirt) downloadOvaFromUrl(ovaFile *os.File) error {
	url, err := p.service.GetImageUrl(p.sourceID)
	if err != nil {
		return err
	}

	res, err := http.Get(url) // Download OVA from the extracted URL

	if err != nil {
		return fmt.Errorf("failed to download image: %v", err)
	}

	defer res.Body.Close()

	if _, err = io.Copy(ovaFile, res.Body); err != nil {
		return fmt.Errorf("failed to write to the file: %w", err)
	}

	return nil
}

// createVm defines and starts a VM based on its XML configuration.
// It reads the XML from a file and uses libvirt to create the domain
func (p *plannerAgentLibvirt) createVm() error {
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

// ovaValidateAndExtract validates the OVA as a tar archive and extracts the ISO and VMDK files
// from it using their known filenames inside the archive.
func (p *plannerAgentLibvirt) ovaValidateAndExtract(ovaFile *os.File) error {
	if err := ValidateTar(ovaFile); err != nil {
		return fmt.Errorf("failed to validate tar: %w", err)
	}

	// Untar ISO from OVA
	if err := Untar(ovaFile, p.isoFilePath(), "MigrationAssessment.iso"); err != nil {
		return fmt.Errorf("failed to uncompress the file: %w", err)
	}

	// Untar VMDK from OVA
	if err := Untar(ovaFile, DefaultVmdkName, "persistence-disk.vmdk"); err != nil {
		return fmt.Errorf("failed to uncompress the file: %w", err)
	}

	return nil
}

// getConfigXmlVMPath returns the path to the VM XML configuration file,
// which can differ based on the test ID.
func (p *plannerAgentLibvirt) getConfigXmlVMPath() string {
	if p.agentEndToEndTestID == DefaultAgentTestID {
		return "data/vm.xml"
	}
	return fmt.Sprintf("data/vm-%s.xml", p.agentEndToEndTestID)
}

// isoFilePath returns the expected file path of the agent ISO,
// which may be test-specific or default.
func (p *plannerAgentLibvirt) isoFilePath() string {
	if p.agentEndToEndTestID == DefaultAgentTestID {
		return filepath.Join(DefaultBasePath, "agent.iso")
	}
	fileName := fmt.Sprintf("agent-%s.iso", p.agentEndToEndTestID)
	return filepath.Join(DefaultBasePath, fileName)
}

// qcowDiskFilePath returns the expected path of the QCOW2 disk image,
// varying based on the test ID.
func (p *plannerAgentLibvirt) qcowDiskFilePath() string {
	if p.agentEndToEndTestID == DefaultAgentTestID {
		return filepath.Join(DefaultBasePath, "persistence-disk.qcow2")
	}
	fileName := fmt.Sprintf("persistence-disk-vm-%s.qcow2", p.agentEndToEndTestID)
	return filepath.Join(DefaultBasePath, fileName)
}

// WaitForDomainState polls the libvirt domain state until the desired state is reached
// or a timeout occurs. It checks the state once every second.
func WaitForDomainState(duration time.Duration, domain *libvirt.Domain, desiredState libvirt.DomainState) error {
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
