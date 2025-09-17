package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/kubev2v/migration-planner/internal/agent/common"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/agent"
	"github.com/kubev2v/migration-planner/internal/agent/config"
	"github.com/kubev2v/migration-planner/pkg/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	agentFilename = "agent_id"
	jwtFilename   = "jwt.json"
)

func main() {
	command := NewAgentCommand()
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}

type agentCmd struct {
	config      *config.Config
	configFile  string
	credentials config.Credentials
}

func NewAgentCommand() *agentCmd {

	a := &agentCmd{
		config: config.NewDefault(),
	}

	a.bind()

	if err := a.config.ParseConfigFile(a.configFile); err != nil {
		if err := a.config.CreateDefaultDirs(); err != nil {
			panic(fmt.Sprintf("Config file not found. Error creating default directories: %v", err))
		}
	}
	if err := a.config.Validate(); err != nil {
		panic(fmt.Sprintf("Error validating config: %v", err))
	}

	return a
}

func (a *agentCmd) bind() {
	flag.StringVar(&a.configFile, "config", config.DefaultConfigFile, "Path to the agent's configuration file.")
	flag.StringVar(&a.credentials.Username, "vsphere-username", "", "vSphere username for connecting to the vCenter API")
	flag.StringVar(&a.credentials.Password, "vsphere-password", "", "vSphere password for connecting to the vCenter API")
	flag.StringVar(&a.credentials.URL, "vsphere-url", "", "vSphere server URL (e.g. https://vcenter.example.com/sdk)")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Println("This program starts an agent with the specified configuration. Below are the available flags:")
		flag.PrintDefaults()
	}

	flag.Parse()
}

func (a *agentCmd) Execute() error {
	logLvl, err := zap.ParseAtomicLevel(a.config.LogLevel)
	if err != nil {
		logLvl = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}

	logger := log.InitLog(logLvl)
	defer func() { _ = logger.Sync() }()

	undo := zap.ReplaceGlobals(logger)
	defer undo()

	agentID, err := a.readFileFromPersistent(agentFilename)
	if err != nil {
		zap.S().Warnf("failed to retreive agent_id: %v", err)
	}

	// Try to read jwt from file.
	// We're assuming the jwt is valid.
	// The agent will not try to validate the jwt. The backend is responsible for validating the token.
	jwt, err := a.readFileFromVolatile(jwtFilename)
	if err != nil {
		zap.S().Warnf("failed to read jwt: %v", err)
	}

	ctx := context.WithValue(context.Background(), common.CmdCredentialsKey, a.credentials)

	agentInstance := agent.New(uuid.MustParse(agentID), jwt, a.config)
	if err := agentInstance.Run(ctx); err != nil {
		zap.S().Fatalf("running device agent: %v", err)
	}
	return nil
}

func (a *agentCmd) readFile(baseDir, filename, fallbackValue string) (string, error) {
	filePath := path.Join(baseDir, filename)
	if _, err := os.Stat(filePath); err == nil {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fallbackValue, err
		}
		return string(bytes.TrimSpace(content)), nil
	}

	return fallbackValue, fmt.Errorf("file not found: %s", filePath)
}

func (a *agentCmd) readFileFromVolatile(filename string) (string, error) {
	return a.readFile(a.config.DataDir, filename, "")
}

func (a *agentCmd) readFileFromPersistent(filename string) (string, error) {
	return a.readFile(a.config.PersistentDataDir, filename, uuid.Nil.String())
}
