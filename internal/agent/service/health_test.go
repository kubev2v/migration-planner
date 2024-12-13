package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path"
	"time"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Health checker", func() {
	Context("create new heath chechker", func() {
		It("should be ok", func() {
			tmpDir, err := os.MkdirTemp("", "health")
			Expect(err).To(BeNil())

			healthChecker, err := NewHealthChecker(nil, tmpDir, 0)
			Expect(err).To(BeNil())
			Expect(healthChecker.logFilepath).To(Equal(path.Join(tmpDir, "health.log")))

			stat, err := os.Stat(healthChecker.logFilepath)
			Expect(err).To(BeNil())
			Expect(stat.IsDir()).To(BeFalse())
		})

		It("should fail -- log folder missing", func() {
			_, err := NewHealthChecker(nil, "some_unknown_folder", 0)
			Expect(err).NotTo(BeNil())
		})
	})

	Context("test the lifecycle of the health checker", func() {
		var (
			hc         *HealthChecker
			tmpDir     string
			testClient *agentTestClient
		)

		BeforeEach(func() {
			tmpDir, err := os.MkdirTemp("", "health")
			Expect(err).To(BeNil())

			testClient = &agentTestClient{}

			hc, err = NewHealthChecker(testClient, tmpDir, 2*time.Second)
			Expect(err).To(BeNil())
		})

		It("should close OK", func() {
			closeCh := make(chan chan any)
			hc.Start(context.TODO(), closeCh)
			<-time.After(2 * time.Second)

			c := make(chan any, 1)
			closeCh <- c

			ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
			defer cancel()

			cc := make(chan any)
			go func() {
				i := 0
				for range c {
					i++
				}
				Expect(i).To(Equal(1))
				cc <- struct{}{}
			}()
			select {
			case <-ctx.Done():
				Fail("Context expired before the test channel was closed. It means the health checker did not exit properly")
			case <-cc:
			}
		})

		It("should call health endpoint", func() {
			closeCh := make(chan chan any)
			hc.Start(context.TODO(), closeCh)
			<-time.After(5 * time.Second)

			c := make(chan any, 1)
			closeCh <- c

			ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
			defer cancel()

			cc := make(chan any)
			go func() {
				i := 0
				for range c {
					i++
				}
				Expect(i).To(Equal(1))
				cc <- struct{}{}
			}()
			select {
			case <-ctx.Done():
				Fail("Context expired before the test channel was closed. It means the health checker did not exit properly")
			case <-cc:
			}

			// in 5 seconds we should have at least 1 call to health endpoint
			Expect(testClient.HealthCallsCount).NotTo(Equal(0))
		})

		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})
	})

	Context("health checker log entries", func() {
		var (
			hc         *HealthChecker
			tmpDir     string
			testClient *agentTestClient
			err        error
		)

		BeforeEach(func() {
			tmpDir, err = os.MkdirTemp("", "health")
			Expect(err).To(BeNil())

			testClient = &agentTestClient{}

			hc, err = NewHealthChecker(testClient, tmpDir, 1*time.Second)
			Expect(err).To(BeNil())

		})

		It("should write OK -- only failures", func() {
			closeCh := make(chan chan any)
			testClient.ShouldReturnError = true
			hc.Start(context.TODO(), closeCh)

			<-time.After(5 * time.Second)

			c := make(chan any, 1)
			closeCh <- c
			<-c

			// In total we should find HealthCallsCount entries in the log file
			content, err := os.ReadFile(hc.logFilepath)
			Expect(err).To(BeNil())

			entries := bytes.Split(bytes.TrimSpace(content), []byte("\n"))
			Expect(len(entries)).To(Equal(testClient.HealthCallsCount))
		})

		It("should write OK -- failures and one OK line", func() {
			closeCh := make(chan chan any)
			testClient.ShouldReturnError = true
			hc.Start(context.TODO(), closeCh)

			<-time.After(2 * time.Second)
			testClient.ShouldReturnError = false
			<-time.After(2 * time.Second)

			c := make(chan any, 1)
			closeCh <- c
			<-c

			// We changed one time the state from unreachable to reachable so we should find 1 ok
			content, err := os.ReadFile(hc.logFilepath)
			Expect(err).To(BeNil())
			entries := bytes.Split(bytes.TrimSpace(content), []byte("\n"))

			countOK := 0
			for _, entry := range entries {
				if bytes.Index(entry, []byte("OK")) > 0 {
					countOK++
				}
			}
			Expect(countOK).To(Equal(1))
		})

		It("should write OK -- failures and 2 OK lines", func() {
			closeCh := make(chan chan any)
			testClient.ShouldReturnError = true
			hc.Start(context.TODO(), closeCh)

			<-time.After(2 * time.Second)
			testClient.ShouldReturnError = false
			<-time.After(2 * time.Second)
			testClient.ShouldReturnError = true
			<-time.After(2 * time.Second)
			testClient.ShouldReturnError = false
			<-time.After(2 * time.Second)

			c := make(chan any, 1)
			closeCh <- c
			<-c

			// We changed twice the state from unreachable to reachable so we should find 2 ok
			content, err := os.ReadFile(hc.logFilepath)
			Expect(err).To(BeNil())
			entries := bytes.Split(bytes.TrimSpace(content), []byte("\n"))

			countOK := 0
			for _, entry := range entries {
				if bytes.Index(entry, []byte("OK")) > 0 {
					countOK++
				}
			}
			Expect(countOK).To(Equal(2))
		})

		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})
	})
})

// Implement the planner interface
type agentTestClient struct {
	HealthCallsCount  int
	ShouldReturnError bool
}

func (c *agentTestClient) Health(ctx context.Context) error {
	c.HealthCallsCount++
	if c.ShouldReturnError {
		return errors.New("console unreachable")
	}
	return nil
}

func (c *agentTestClient) UpdateSourceStatus(ctx context.Context, id uuid.UUID, params api.SourceStatusUpdate) error {
	return nil
}

func (c *agentTestClient) UpdateAgentStatus(ctx context.Context, id uuid.UUID, params api.AgentStatusUpdate) error {
	return nil
}
