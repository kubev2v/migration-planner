package standalone

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kubev2v/migration-planner/internal/agent"
	"github.com/kubev2v/migration-planner/internal/agent/config"
	"github.com/kubev2v/migration-planner/internal/agent/fileio"
	"github.com/kubev2v/migration-planner/internal/agent/service"
	utils "github.com/kubev2v/migration-planner/internal/util"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	defaultCollectionTimeout = 5 * time.Minute
)

type CollectOptions struct {
	opaPoliciesFolderPath string
	dataDir               string
	credentialsDir        string
	credentialsFilePath   string
	inventoryFilePath     string
	username              string
	url                   string
	password              string
	collectionTimeout     time.Duration
}

func NewCollectOptions() *CollectOptions {
	return &CollectOptions{
		opaPoliciesFolderPath: utils.GetEnv("OPA_POLICY_FOLDER_PATH", "/usr/share/opa/policies"),
	}
}

func NewCmdCollect() *cobra.Command {
	o := NewCollectOptions()
	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Gather vCenter inventory",
		Example: "planner collect " +
			"--data-dir ~/Downloads " +
			"--credentials-dir /tmp",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.Run(cmd.Context(), args)
		},
		SilenceUsage: true,
	}
	o.Bind(cmd.Flags())
	return cmd
}

func (o *CollectOptions) Bind(fs *pflag.FlagSet) {
	pwd, err := os.Getwd()
	if err != nil {
		pwd = "."
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = utils.GetEnv("HOME", "~")
	}
	home = filepath.Join(home, "Downloads")

	fs.StringVar(&o.dataDir, "data-dir", home, "directory where the agent will write its data (e.g., inventory.json)")
	fs.StringVar(&o.credentialsDir, "credentials-dir", pwd, "directory where credentials.json is stored")
	fs.StringVarP(&o.username, "username", "u", "", "vsphere username")
	fs.StringVarP(&o.password, "password", "p", "", "vsphere password")
	fs.StringVar(&o.url, "url", "", "vsphere url")
	fs.DurationVar(&o.collectionTimeout, "timeout", defaultCollectionTimeout, "collection timeout")
}

func (o *CollectOptions) Run(ctx context.Context, args []string) error {

	o.init()

	if err := o.validateCredential(ctx); err != nil {
		return err
	}

	log.Printf("▶️  Launching OPA server from policies at %s", o.opaPoliciesFolderPath)

	opaCmd, err := backgroundStartOPA(o.opaPoliciesFolderPath)
	if err != nil {
		return fmt.Errorf("error running opa server: %w", err)
	}
	defer func() {
		_ = opaCmd.Process.Kill()
		_ = opaCmd.Wait()
	}()

	log.Printf("🏃  OPA server running (pid=%d)", opaCmd.Process.Pid)

	log.Printf("🔍 Starting vCenter inventory collection; dataDir=%q, creds=%q", o.dataDir, o.credentialsFilePath)

	if err := o.collect(ctx, o.collectionTimeout); err != nil {
		return fmt.Errorf("error generating the inventory.json: %w", err)
	}

	log.Printf("✅ Finished inventory collection; wrote %s", o.inventoryFilePath)

	return nil
}

func (o *CollectOptions) init() {

	o.inventoryFilePath = filepath.Join(o.dataDir, config.InventoryFile)
	o.credentialsFilePath = filepath.Join(o.credentialsDir, config.CredentialsFile)

}

func (o *CollectOptions) collect(ctx context.Context, timeout time.Duration) error {

	if _, err := os.Stat(o.inventoryFilePath); err == nil {
		if err := os.Remove(o.inventoryFilePath); err != nil {
			return err
		}
	}

	collector := service.NewCollector(o.dataDir, o.credentialsDir)
	collector.Collect(ctx)

	log.Printf("⏳ Waiting up to %s for %s to appear", timeout, o.inventoryFilePath)

	if err := waitForFile(o.inventoryFilePath, timeout); err != nil {
		return err
	}

	return nil
}

func (o *CollectOptions) saveCredential() error {
	if len(o.url) == 0 || len(o.username) == 0 || len(o.password) == 0 {
		return fmt.Errorf("error. Must pass url, username, and password")
	}

	if !strings.HasSuffix(o.url, "sdk") {
		o.url += "/sdk"
	}

	credentials := &config.Credentials{
		URL:      o.url,
		Username: o.username,
		Password: o.password,
	}

	buf, err := json.Marshal(credentials)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}
	writer := fileio.NewWriter()

	if err := writer.WriteFile(o.credentialsFilePath, buf); err != nil {
		return fmt.Errorf("failed saving credentials: %w", err)
	}

	return nil
}

func (o *CollectOptions) validateCredential(ctx context.Context) error {
	if o.username != "" {
		if err := o.saveCredential(); err != nil {
			return err
		}
	}

	if _, err := os.Stat(o.credentialsFilePath); err != nil {
		return fmt.Errorf("finding credentials file: %w", err)
	}

	reader := fileio.NewReader()
	buf, err := reader.ReadFile(o.credentialsFilePath)
	if err != nil {
		return fmt.Errorf("reading credentials file: %w", err)
	}
	credentials := &config.Credentials{}
	if err := json.Unmarshal(buf, credentials); err != nil {
		return fmt.Errorf("unmarshalling credentials: %w", err)
	}

	if _, err := agent.TestVmwareConnection(ctx, credentials); err != nil {
		return fmt.Errorf("connecting to vsphere: %w", err)
	}

	return nil
}

func backgroundStartOPA(policyDir string) (*exec.Cmd, error) {
	if _, err := os.Stat(policyDir); err != nil {
		return nil, fmt.Errorf("cannot find policies in %s: %w", policyDir, err)
	}

	if _, err := exec.LookPath("opa"); err != nil {
		return nil, fmt.Errorf("opa binary not found in PATH: %w", err)
	}

	cmd := exec.Command("opa", "run", policyDir, "--server")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start opa: %w", err)
	}

	return cmd, nil
}

func waitForFile(filename string, timeout time.Duration) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timed out waiting for %s after %s", filename, timeout)

		case <-ticker.C:
			if _, err := os.Stat(filename); err == nil {
				return nil
			}
		}
	}
}
