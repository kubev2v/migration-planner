package e2e_utils

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"strings"
)

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

// RemoveFile Remove OS file if exist
func RemoveFile(fullPath string) error {
	if _, err := os.Stat(fullPath); err == nil {
		if err := os.Remove(fullPath); err != nil {
			return fmt.Errorf("failed to remove file: %v", err)
		}
	}
	return nil
}
