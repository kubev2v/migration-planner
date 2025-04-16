package e2e_agent

import (
	"encoding/json"
	"fmt"
	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_utils"
	"github.com/libvirt/libvirt-go"
	"go.uber.org/zap"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

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

	user, err := DefaultUserAuth()
	if err != nil {
		return err
	}

	var res *http.Response

	if TestOptions.DownloadImageByUrl {
		url, err := p.getDownloadURL(user.Token.Raw)
		if err != nil {
			return err
		}

		res, err = http.Get(url) // Download OVA from the extracted URL
		if err != nil {
			return err
		}
	} else {
		getImagePath := p.sourceID.String() + "/" + "image"
		res, err = p.serviceApi.GetRequest(getImagePath, user.Token.Raw)

		if err != nil {
			return fmt.Errorf("failed to get source image: %w", err)
		}
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download image: %s", res.Status)
	}

	if _, err = io.Copy(ovaFile, res.Body); err != nil {
		return fmt.Errorf("failed to write to the file: %w", err)
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

func (p *plannerAgentLibvirt) getDownloadURL(jwtToken string) (string, error) {
	getImageUrlPath := p.sourceID.String() + "/" + "image-url"
	res, err := p.serviceApi.GetRequest(getImageUrlPath, jwtToken)
	if err != nil {
		return "", fmt.Errorf("failed to get source url: %w", err)
	}
	defer res.Body.Close()

	var result struct {
		ExpiresAt string `json:"expires_at"`
		URL       string `json:"url"`
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to decod JSON: %w", err)
	}

	return result.URL, nil
}

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

func (p *plannerAgentLibvirt) getConfigXmlVMPath() string {
	if p.agentEndToEndTestID == DefaultAgentTestID {
		return "data/vm.xml"
	}
	return fmt.Sprintf("data/vm-%s.xml", p.agentEndToEndTestID)
}

func (p *plannerAgentLibvirt) isoFilePath() string {
	if p.agentEndToEndTestID == DefaultAgentTestID {
		return filepath.Join(DefaultBasePath, "agent.iso")
	}
	fileName := fmt.Sprintf("agent-%s.iso", p.agentEndToEndTestID)
	return filepath.Join(DefaultBasePath, fileName)
}

func (p *plannerAgentLibvirt) qcowDiskFilePath() string {
	if p.agentEndToEndTestID == DefaultAgentTestID {
		return filepath.Join(DefaultBasePath, "persistence-disk.qcow2")
	}
	fileName := fmt.Sprintf("persistence-disk-vm-%s.qcow2", p.agentEndToEndTestID)
	return filepath.Join(DefaultBasePath, fileName)
}

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
