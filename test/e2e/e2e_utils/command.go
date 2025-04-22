package e2e_utils

import (
	"bytes"
	"fmt"
	"os/exec"
)

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
	sshCmd := exec.Command("sshpass", "-p", "123456", "ssh", "-o", "StrictHostKeyChecking=no", "-o",
		"UserKnownHostsFile=/dev/null", fmt.Sprintf("core@%s", ip), command)

	var stdout, stderr bytes.Buffer
	sshCmd.Stdout = &stdout
	sshCmd.Stderr = &stderr

	if err := sshCmd.Run(); err != nil {
		return stderr.String(), fmt.Errorf("command failed: %v\nstdout: %s\nstderr: %s", err,
			stdout.String(), stderr.String())
	}

	return stdout.String(), nil
}
