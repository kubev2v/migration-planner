package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	liberr "github.com/konveyor/forklift-controller/pkg/lib/error"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/agent/config"
	"github.com/kubev2v/migration-planner/internal/agent/fileio"
	"github.com/kubev2v/migration-planner/internal/agent/service"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	"go.uber.org/zap"
)

func RegisterApi(router *chi.Mux, statusUpdater *service.StatusUpdater, configuration *config.Config) {
	router.Get("/api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		_ = render.Render(w, r, VersionReply{Version: version})
	})
	router.Get("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		status, statusInfo, _ := statusUpdater.CalculateStatus()
		environmentStatus := true
		if statusUpdater.HealthChecker != nil && statusUpdater.HealthChecker.State() == service.HealthCheckStateConsoleUnreachable {
			environmentStatus = false
		}
		_ = render.Render(w, r, StatusReply{Status: string(status), StatusInfo: statusInfo, Connected: fmt.Sprintf("%t", environmentStatus)})
	})
	router.Get("/api/v1/url", func(w http.ResponseWriter, r *http.Request) {
		_ = render.Render(w, r, ServiceUIReply{URL: configuration.PlannerService.Service.UI})
	})
	router.Put("/api/v1/credentials", func(w http.ResponseWriter, r *http.Request) {
		credentialHandler(configuration.PersistentDataDir, w, r)
	})
	router.Get("/api/v1/inventory", func(w http.ResponseWriter, r *http.Request) {
		data, err := getInventory(configuration.DataDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if data == nil {
			http.Error(w, "Inventory not found", http.StatusNotFound)
			return
		}
		w.Header().Add("Content-Disposition", "attachment")
		_ = render.Render(w, r, InventoryReply{AgentID: statusUpdater.AgentID.String(), Inventory: data.Inventory})
	})
}

type StatusReply struct {
	Status     string `json:"status"`
	Connected  string `json:"connected"`
	StatusInfo string `json:"statusInfo"`
}

type VersionReply struct {
	Version string `json:"version"`
}

type ServiceUIReply struct {
	URL string `json:"url"`
}

type InventoryReply struct {
	AgentID   string        `json:"agentId"`
	Inventory api.Inventory `json:"inventory"`
}

func (s ServiceUIReply) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func (s StatusReply) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func (v VersionReply) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func (v InventoryReply) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func getInventory(dataDir string) (*service.InventoryData, error) {
	filename := filepath.Join(dataDir, config.InventoryFile)
	contents, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading inventory file: %v", err)
	}

	var inventory service.InventoryData
	err = json.Unmarshal(contents, &inventory)
	if err != nil {
		return nil, fmt.Errorf("error parsing inventory file: %v", err)
	}

	return &inventory, nil
}

func credentialHandler(dataDir string, w http.ResponseWriter, r *http.Request) {
	credentials := &config.Credentials{}

	err := json.NewDecoder(r.Body).Decode(credentials)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(credentials.URL) == 0 || len(credentials.Username) == 0 || len(credentials.Password) == 0 {
		http.Error(w, "Must pass url, username, and password", http.StatusBadRequest)
		return
	}

	zap.S().Named("rest").Info("received credentials")
	status, err := testVmwareConnection(r.Context(), credentials)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}

	credPath := filepath.Join(dataDir, config.CredentialsFile)
	buf, _ := json.Marshal(credentials)
	writer := fileio.NewWriter()

	err = writer.WriteFile(credPath, buf)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed saving credentials: %v", err), http.StatusInternalServerError)
		return
	}

	zap.S().Named("rest").Info("successfully wrote credentials to file")
	w.WriteHeader(http.StatusNoContent)
}

func parseUrl(credentials *config.Credentials) (*url.URL, error) {
	u, err := url.ParseRequestURI(credentials.URL)
	if err != nil {
		return nil, err
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/sdk"
	}
	credentials.URL = u.String()
	u.User = url.UserPassword(credentials.Username, credentials.Password)
	return u, nil
}

func testVmwareConnection(requestCtx context.Context, credentials *config.Credentials) (status int, err error) {
	u, err := parseUrl(credentials)
	if err != nil {
		return http.StatusUnprocessableEntity, liberr.Wrap(err)
	}

	ctx, cancel := context.WithTimeout(requestCtx, 10*time.Second)
	defer cancel()
	vimClient, err := vim25.NewClient(ctx, soap.NewClient(u, true))
	if err != nil {
		return http.StatusBadRequest, liberr.Wrap(err)
	}
	client := &govmomi.Client{
		SessionManager: session.NewManager(vimClient),
		Client:         vimClient,
	}
	zap.S().Named("rest").Info("logging into vmware using received credentials")
	err = client.Login(ctx, u.User)
	if err != nil {
		err = liberr.Wrap(err)
		// Cover both the different error messages returns from the production and test environments in case of incorrect credentials
		if strings.Contains(err.Error(), "Login failure") ||
			strings.Contains(err.Error(), "incorrect") && strings.Contains(err.Error(), "password") {
			return http.StatusUnauthorized, err
		}
		return http.StatusBadRequest, err
	}

	_ = client.Logout(ctx)
	client.CloseIdleConnections()

	return http.StatusAccepted, nil
}
