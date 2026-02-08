package agent

import (
	"errors"
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
	files := []struct {
		path string
		name string
	}{
		{p.ovaFilePath(), "OVA file"},
		{p.isoFilePath(), "ISO file"},
		{p.vmdkDiskFilePath(), "Vmdk file"},
		{p.qcowDiskFilePath(), "qcow disk file"},
	}

	var errs []error

	for _, f := range files {
		if err := RemoveFile(f.path); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove %s: %w", f.name, err))
		}
	}

	return errors.Join(errs...)
}

// prepareImage handles the full preparation of the agent VM image:
// it downloads the OVA (from URL or image service), extracts the required ISO and VMDK files,
// and converts the VMDK to QCOW format.
func (p *plannerAgentLibvirt) prepareImage() error {
	// Create OVA
	ovaFilePath := p.ovaFilePath()
	ovaFile, err := os.Create(ovaFilePath)
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
	zap.S().Infof("Successfully downloaded ova file: %s", ovaFilePath)

	if err := p.ovaValidateAndExtract(ovaFile); err != nil {
		return err
	}

	zap.S().Infof("Successfully extracted the Iso and Vmdk files from the OVA.")

	if err := ConvertVMDKtoQCOW2(p.vmdkDiskFilePath(), p.qcowDiskFilePath()); err != nil {
		return fmt.Errorf("failed to convert vmdk to qcow: %w", err)
	}

	zap.S().Infof("Successfully converted the vmdk to qcow.")

	return nil
}

// downloadOvaFromUrl fetches the OVA file from a remote URL and writes it to the provided file
func (p *plannerAgentLibvirt) downloadOvaFromUrl(ovaFile *os.File) error {
	res, err := http.Get(p.url) // Download OVA from the given URL

	if err != nil {
		return fmt.Errorf("failed to download image: %v", err)
	}

	defer res.Body.Close()

	if _, err = io.Copy(ovaFile, res.Body); err != nil {
		return fmt.Errorf("failed to write to the file: %w", err)
	}

	return nil
}

// createVm defines and starts a VM by generating its XML configuration
// and using libvirt to create the domain.
func (p *plannerAgentLibvirt) createVm() error {
	// Generate VM XML definition
	vmXMLBytes, err := GenerateDomainXML(p.name, p.isoFilePath(), p.qcowDiskFilePath())
	if err != nil {
		return fmt.Errorf("failed to generate VM XML: %v", err)
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
	if err := Untar(ovaFile, p.vmdkDiskFilePath(), "persistence-disk.vmdk"); err != nil {
		return fmt.Errorf("failed to uncompress the file: %w", err)
	}

	return nil
}

// ovaFilePath returns the expected file path of the agent OVA,
func (p *plannerAgentLibvirt) ovaFilePath() string {
	return filepath.Join(Home, fmt.Sprintf("%s.ova", p.name))
}

// isoFilePath returns the expected file path of the agent ISO,
func (p *plannerAgentLibvirt) isoFilePath() string {
	return filepath.Join(DefaultBasePath, fmt.Sprintf("%s.iso", p.name))
}

// vmdkDiskFilePath returns the expected path of the VMDK disk image,
func (p *plannerAgentLibvirt) vmdkDiskFilePath() string {
	return filepath.Join(DefaultBasePath, fmt.Sprintf("%s.vmdk", p.name))
}

// qcowDiskFilePath returns the expected path of the QCOW2 disk image,
func (p *plannerAgentLibvirt) qcowDiskFilePath() string {
	return filepath.Join(DefaultBasePath, fmt.Sprintf("%s.qcow2", p.name))
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
