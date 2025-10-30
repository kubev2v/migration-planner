package agent

import (
	"context"
	"fmt"
	"github.com/containers/podman/v6/pkg/bindings"
	"github.com/containers/podman/v6/pkg/bindings/containers"
	"github.com/containers/podman/v6/pkg/specgen"
	"github.com/coreos/ignition/v2/config/util"
	"github.com/kubev2v/migration-planner/test/e2e"
	"github.com/opencontainers/runtime-spec/specs-go"
	"go.podman.io/common/libnetwork/types"
	"go.uber.org/zap"
	"os"
	"os/user"
)

type plannerAgentPodman struct {
	conn                   context.Context
	imagePath              string
	containerID            string
	localDataDir           string
	localPersistentDataDir string
	DestDataDir            string
	DestPersistentDataDir  string
	sourceID               string
}

func NewPlannerAgentPodman(sourceID string) (PlannerAgent, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}

	conn, err := bindings.NewConnection(context.Background(), fmt.Sprintf("unix:/run/user/%s/podman/podman.sock", u.Uid))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to podman socket: %w", err)
	}

	return &plannerAgentPodman{
		conn:                   conn,
		imagePath:              e2e.AgentImagePath,
		localDataDir:           e2e.AgentLocalDataDir,
		localPersistentDataDir: e2e.AgentLocalPersistentDataDir,
		DestDataDir:            e2e.AgentDestDataDir,
		DestPersistentDataDir:  e2e.AgentDestPersistentDataDir,
	}, nil
}

func (p *plannerAgentPodman) Run() error {
	spec, err := p.spec()
	if err != nil {
		return err
	}

	createResponse, err := containers.CreateWithSpec(p.conn, spec, nil)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}
	p.containerID = createResponse.ID

	if err := containers.Start(p.conn, p.containerID, nil); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	zap.S().Infof("Container %s started successfully\n", p.containerID)

	return nil
}

func (p *plannerAgentPodman) spec() (*specgen.SpecGenerator, error) {
	pwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %w", err)
	}
	agentConfigFilePath := fmt.Sprintf("%s/data/agent_podman_config.yaml", pwd)

	spec := specgen.NewSpecGenerator(p.imagePath, false)
	spec.Name = "planner-agent-e2e"
	spec.Command = []string{
		"/app/planner-agent",
		fmt.Sprintf("-config %s", agentConfigFilePath),
		"",
	}
	spec.PortMappings = []types.PortMapping{
		{
			ContainerPort: e2e.AgentPort,
			HostPort:      e2e.AgentPort,
		},
	}

	spec.Mounts = []specs.Mount{
		{
			Source:      p.localDataDir,
			Destination: p.DestDataDir,
			Type:        "bind",
			Options:     []string{"Z"},
		},
		{
			Source:      p.localPersistentDataDir,
			Destination: p.DestPersistentDataDir,
			Type:        "bind",
			Options:     []string{"Z"},
		},
	}

	return spec, nil
}

func (p *plannerAgentPodman) DumpLogs(_ string) {
	if p.containerID == "" {
		zap.S().Warn("No container ID found, cannot dump logs")
		return
	}

	stdoutChan := make(chan string)
	stderrChan := make(chan string)

	go func() {
		for msg := range stdoutChan {
			fmt.Print(msg)
		}
	}()
	go func() {
		for msg := range stderrChan {
			fmt.Print(msg)
		}
	}()

	err := containers.Logs(p.conn, p.containerID, &containers.LogOptions{
		Stdout:     util.BoolToPtr(true),
		Stderr:     util.BoolToPtr(true),
		Timestamps: util.BoolToPtr(true),
	}, stdoutChan, stderrChan)
	if err != nil {
		zap.S().Errorf("Failed to fetch logs for container %s: %v", p.containerID, err)
	}

	close(stdoutChan)
	close(stderrChan)
}

func (p *plannerAgentPodman) GetIp() (string, error) {
	return e2e.SystemIP, nil
}

func (p *plannerAgentPodman) IsServiceRunning(_ string, _ string) bool {
	return true
}

func (p *plannerAgentPodman) Restart() error {
	return nil
}

func (p *plannerAgentPodman) Remove() error {
	removeOptions := new(containers.RemoveOptions)
	removeOptions.Force = util.BoolToPtr(true)
	removeOptions.Volumes = util.BoolToPtr(true)

	if _, err := containers.Remove(p.conn, p.containerID, removeOptions); err != nil {
		return fmt.Errorf("failed to remove container %s: %w", p.containerID, err)
	}

	zap.S().Infof("Container %s removed successfully", p.containerID)

	return nil
}
