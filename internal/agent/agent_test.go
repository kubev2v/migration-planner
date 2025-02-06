package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	agentapi "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/agent"
	"github.com/kubev2v/migration-planner/internal/agent/client"
	"github.com/kubev2v/migration-planner/internal/agent/config"
	"github.com/kubev2v/migration-planner/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Agent", func() {
	var (
		// test http server used to get the cred url
		testHttpServer  *httptest.Server
		agentTmpFolder  string
		agentID         uuid.UUID
		endpointsCalled map[string]any
	)

	BeforeEach(func() {
		agentID, _ = uuid.NewUUID()
		endpointsCalled = make(map[string]any)

		testHttpServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.String(), "/api/v1/agents") {
				// save the response
				body, err := io.ReadAll(r.Body)
				Expect(err).To(BeNil())

				status := agentapi.AgentStatusUpdate{}

				err = json.Unmarshal(body, &status)
				Expect(err).To(BeNil())
				endpointsCalled[r.URL.String()] = status
				w.WriteHeader(http.StatusOK)
				return
			}

			endpointsCalled[r.URL.String()] = true
			w.WriteHeader(http.StatusOK)
		}))
		var err error
		agentTmpFolder, err = os.MkdirTemp("", "agent-data-folder")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		testHttpServer.Close()
		os.RemoveAll(agentTmpFolder)
	})

	Context("Agent", func() {
		It("agents starts successfully -- status waiting-for-credentials", func() {
			updateInterval, _ := time.ParseDuration("5s")
			config := config.Config{
				PlannerService:      config.PlannerService{Config: *client.NewDefault()},
				DataDir:             agentTmpFolder,
				PersistentDataDir:   agentTmpFolder,
				ConfigDir:           agentTmpFolder,
				UpdateInterval:      util.Duration{Duration: updateInterval},
				HealthCheckInterval: 10,
			}
			config.PlannerService.Service.Server = testHttpServer.URL

			jwt := ""
			a := agent.New(agentID, jwt, &config)
			ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
			go func() {
				err := a.Run(ctx)
				Expect(err).To(BeNil())
			}()

			<-time.After(30 * time.Second)
			cancel()

			select {
			case <-ctx.Done():
				Expect(ctx.Err().Error()).To(Equal("context canceled"))
			case <-time.After(20 * time.Second):
				Fail("agent did not returned when context was canceled")
			}

			// We should have calles to /health and /agents endpoint
			status, found := endpointsCalled[fmt.Sprintf("/api/v1/agents/%s/status", agentID)]
			Expect(found).To(BeTrue())
			Expect(status.(agentapi.AgentStatusUpdate).CredentialUrl).NotTo(BeEmpty())
			Expect(status.(agentapi.AgentStatusUpdate).Status).To(Equal("waiting-for-credentials"))

			_, found = endpointsCalled["/health"]
			Expect(found).To(BeTrue())
		})
	})

})
