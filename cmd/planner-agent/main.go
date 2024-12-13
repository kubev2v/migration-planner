package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/agent"
	"github.com/kubev2v/migration-planner/internal/agent/config"
	"github.com/kubev2v/migration-planner/pkg/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	agentFilename = "agent_id"
)

func main() {
	command := NewAgentCommand()
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}

type agentCmd struct {
	config     *config.Config
	configFile string
}

func NewAgentCommand() *agentCmd {
	logger := log.InitLog(zap.NewAtomicLevelAt(zapcore.InfoLevel))
	defer func() { _ = logger.Sync() }()

	undo := zap.ReplaceGlobals(logger)
	defer undo()

	a := &agentCmd{
		config: config.NewDefault(),
	}

	flag.StringVar(&a.configFile, "config", config.DefaultConfigFile, "Path to the agent's configuration file.")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Println("This program starts an agent with the specified configuration. Below are the available flags:")
		flag.PrintDefaults()
	}

	flag.Parse()

	if err := a.config.ParseConfigFile(a.configFile); err != nil {
		zap.S().Fatalf("Error parsing config: %v", err)
	}
	if err := a.config.Validate(); err != nil {
		zap.S().Fatalf("Error validating config: %v", err)
	}

	return a
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

	agentID, err := a.getAgentID()
	if err != nil {
		zap.S().Fatalf("failed to retreive agent_id: %v", err)
	}

	agentInstance := agent.New(uuid.MustParse(agentID), a.config)
	if err := agentInstance.Run(context.Background()); err != nil {
		zap.S().Fatalf("running device agent: %v", err)
	}
	return nil
}

func (a *agentCmd) getAgentID() (string, error) {
	// look for it in data dir
	dataDirPath := path.Join(a.config.DataDir, agentFilename)
	if _, err := os.Stat(dataDirPath); err == nil {
		content, err := os.ReadFile(dataDirPath)
		if err != nil {
			return "", err
		}
		return string(bytes.TrimSpace(content)), nil
	}

	return "", errors.New("agent_id not found")
}
