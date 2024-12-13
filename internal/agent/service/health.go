package service

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/kubev2v/migration-planner/internal/agent/client"
	"go.uber.org/zap"
)

type AgentHealthState int

const (
	HealthCheckStateConsoleUnreachable AgentHealthState = iota
	HealthCheckStateConsoleReachable
	logFilename    = "health.log"
	defaultTimeout = 5 //seconds
)

type HealthChecker struct {
	once          sync.Once
	lock          sync.Mutex
	state         AgentHealthState
	checkInterval time.Duration
	client        client.Planner
	logFilepath   string
	logFile       *os.File
}

func NewHealthChecker(client client.Planner, logFolder string, checkInterval time.Duration) (*HealthChecker, error) {
	logFile := path.Join(logFolder, logFilename)
	// check if we can write into the log file
	_, err := os.Stat(logFolder)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("log folder %s does not exists", logFolder)
		}
		return nil, fmt.Errorf("failed to stat the log file %s: %w", logFolder, err)
	}
	// At each start we want a clean file so try to remove it
	if err := os.Remove(logFile); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to delete the existing log file %w", err)
		}
	}
	// try to open it
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s for append %w", logFile, err)
	}
	return &HealthChecker{
		state:         HealthCheckStateConsoleUnreachable,
		checkInterval: checkInterval,
		client:        client,
		logFilepath:   logFile,
		logFile:       f,
	}, nil
}

// StartHealthCheck starts a periodic health check and log the result into a file.
// It is logging every failure but, to not polute the log file, it logs only one sucessfull check.
// For example:
// [2024-09-27T15:54:03+02:00] console.redhat.com is OK.
// [2024-09-27T15:54:09+02:00] console.redhat.com is unreachable.
// [2024-09-27T15:54:11+02:00] console.redhat.com is unreachable.
// [2024-09-27T15:54:13+02:00] console.redhat.com is unreachable.
// [2024-09-27T15:54:15+02:00] console.redhat.com is OK.
//
//
// client is the rest client used to send requests to console.redhat.com.
// logFile is the path of the log file.
// initialInterval represents the time after which the check is started.
// checkInterval represents the time to wait between checks.
// closeCh is the channel used to close the goroutine.
func (h *HealthChecker) Start(ctx context.Context, closeCh chan chan any) {
	h.do(ctx)

	h.once.Do(func() {
		go func() {
			t := time.NewTicker(h.checkInterval)
			defer t.Stop()
			for {
				select {
				case c := <-closeCh:
					if err := h.logFile.Sync(); err != nil {
						zap.S().Named("health").Errorf("failed to flush the log file %w", err)
					}
					if err := h.logFile.Close(); err != nil {
						zap.S().Named("health").Errorf("failed to close log file %s %w", h.logFilepath, err)
					}
					c <- struct{}{}
					close(c)
					return
				case <-t.C:
					h.do(ctx)
				}
			}
		}()
	})
}

func (h *HealthChecker) State() AgentHealthState {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.state
}

func (h *HealthChecker) do(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout*time.Second)
	defer cancel()

	err := h.client.Health(ctx)
	if err != nil {
		if _, err := h.logFile.Write([]byte(fmt.Sprintf("[%s] console.redhat.com is unreachable.\n", time.Now().Format(time.RFC3339)))); err != nil {
			zap.S().Named("health").Errorf("failed to write to log file %s %w", h.logFilepath, err)
		}
		h.lock.Lock()
		h.state = HealthCheckStateConsoleUnreachable
		h.lock.Unlock()
		return
	}
	// if state changed from unreachable to ok log the entry
	if h.state == HealthCheckStateConsoleUnreachable {
		if _, err := h.logFile.Write([]byte(fmt.Sprintf("[%s] console.redhat.com is OK.\n", time.Now().Format(time.RFC3339)))); err != nil {
			zap.S().Named("health").Errorf("failed to write to log file %s %w", h.logFilepath, err)
		}
	}
	h.lock.Lock()
	h.state = HealthCheckStateConsoleReachable
	h.lock.Unlock()
}
