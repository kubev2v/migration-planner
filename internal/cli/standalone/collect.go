package standalone

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/agent"
	"github.com/kubev2v/migration-planner/internal/agent/collector"
	"github.com/kubev2v/migration-planner/internal/agent/config"
	"github.com/kubev2v/migration-planner/internal/agent/service"
	utils "github.com/kubev2v/migration-planner/internal/util"
	"github.com/open-policy-agent/opa/runtime"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

var defaultOpaAddress = ":8181"

type CollectOptions struct {
	opaPoliciesFolderPath string
	credentialsFilePath   string
	inventoryFilePath     string
	username              string
	url                   string
	password              string
	config                *config.Config
	server                *agent.Server
}

func NewCollectOptions() *CollectOptions {
	return &CollectOptions{
		opaPoliciesFolderPath: utils.GetEnv("OPA_POLICY_FOLDER_PATH", "/usr/share/opa/policies"),
		config: &config.Config{
			SourceID: uuid.Nil.String(),
		},
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
		home = utils.GetEnv("HOME", pwd)
	}

	fs.StringVar(&o.config.DataDir, "data-dir", home, "directory where the agent will write its data (e.g., inventory.json)")
	fs.StringVar(&o.config.PersistentDataDir, "credentials-dir", home, "directory where credentials.json is stored")
	fs.StringVar(&o.config.WwwDir, "ui-dir", config.DefaultWwwDir, "directory where the UI app stored")
	fs.StringVarP(&o.username, "username", "u", "", "vsphere username")
	fs.StringVarP(&o.password, "password", "p", "", "vsphere password")
	fs.StringVar(&o.url, "url", "", "vsphere url")
}

func (o *CollectOptions) Run(ctx context.Context, args []string) error {
	o.init()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	zap.S().Infof("Launching OPA server from policies at: %s", o.opaPoliciesFolderPath)

	if err := backgroundStartOPA(ctx, defaultOpaAddress, o.opaPoliciesFolderPath); err != nil {
		return err
	}

	defer func() {
		_ = os.Remove(o.credentialsFilePath)
		_ = os.Remove(o.inventoryFilePath)
	}()

	zap.S().Infof("Checking if credentials provided using CMD...")

	if err := o.useCmdCredentials(); err != nil {
		return err
	}

	collector := collector.NewCollector(o.config.DataDir, o.config.PersistentDataDir)
	collector.Collect(ctx)

	ctx, cancel := context.WithCancel(ctx)
	select {
	case <-sig:
	case <-ctx.Done():
	}

	o.stopServer()
	cancel()

	return nil
}

func (o *CollectOptions) init() {

	o.inventoryFilePath = filepath.Join(o.config.DataDir, config.InventoryFile)
	o.credentialsFilePath = filepath.Join(o.config.PersistentDataDir, config.CredentialsFile)

	// start server
	statusUpdater := service.NewStatusUpdater(uuid.MustParse(o.config.SourceID),
		uuid.Nil, "1.0", "http://127.0.0.1:3333", o.config, nil)
	o.server = agent.NewServer(agent.DefaultAgentPort, o.config, nil, nil)
	go o.server.Start(statusUpdater)

}

func (o *CollectOptions) useCmdCredentials() error {
	hasURL := len(o.url) > 0
	hasUser := len(o.username) > 0
	hasPass := len(o.password) > 0

	if hasURL || hasUser || hasPass {
		if !(hasURL && hasUser && hasPass) {
			return fmt.Errorf(
				"incomplete credentials. got url=%s, user=%s, pass=%s",
				o.url, o.username, o.password,
			)
		}
		zap.S().Infof("using credentials from CMD")
	} else {
		zap.S().Infof("no credentials provided via CMD, skipping")
		return nil
	}

	credentials := &config.Credentials{
		URL:      o.url,
		Username: o.username,
		Password: o.password,
	}

	jsonData, err := json.Marshal(credentials)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, "http://127.0.0.1:3333/api/v1/credentials", bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error getting response from local server: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to store credentials, unexpected status code: %d", res.StatusCode)
	}

	return nil
}

func backgroundStartOPA(ctx context.Context, addr, policyDir string) error {

	params := runtime.Params{
		Addrs: &[]string{addr},
		Paths: []string{policyDir},
	}

	rt, err := runtime.NewRuntime(ctx, params)
	if err != nil {
		return err
	}

	go rt.StartServer(ctx)

	return nil
}

func (o *CollectOptions) stopServer() {
	serverCh := make(chan any)
	o.server.Stop(serverCh)

	<-serverCh
	zap.S().Infof("server stopped")
}
