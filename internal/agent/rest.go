package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	liberr "github.com/konveyor/forklift-controller/pkg/lib/error"
	"github.com/kubev2v/migration-planner/internal/agent/fileio"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
)

const (
	// name of the file which stores the source credentials
	CredentialsFile = "credentials.json"
)

func RegisterApi(router *chi.Mux, log *log.PrefixLogger, statusUpdater *StatusUpdater, dataDir string) {
	router.Get("/api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		_ = render.Render(w, r, VersionReply{Version: version})
	})
	router.Get("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		status, statusInfo, _ := statusUpdater.CalculateStatus()
		_ = render.Render(w, r, StatusReply{Status: string(status), StatusInfo: statusInfo})
	})
	router.Put("/api/v1/credentials", func(w http.ResponseWriter, r *http.Request) {
		credentialHandler(log, dataDir, w, r)
	})
}

type StatusReply struct {
	Status     string `json:"status"`
	StatusInfo string `json:"statusInfo"`
}

type VersionReply struct {
	Version string `json:"version"`
}

func (s StatusReply) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func (v VersionReply) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

type Credentials struct {
	URL                  string `json:"url"`
	Username             string `json:"username"`
	Password             string `json:"password"`
	IsDataSharingAllowed bool   `json:"isDataSharingAllowed"`
}

func credentialHandler(log *log.PrefixLogger, dataDir string, w http.ResponseWriter, r *http.Request) {
	credentials := &Credentials{}

	err := json.NewDecoder(r.Body).Decode(credentials)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(credentials.URL) == 0 || len(credentials.Username) == 0 || len(credentials.Password) == 0 {
		http.Error(w, "Must pass url, username, and password", http.StatusBadRequest)
		return
	}

	log.Info("received credentials")
	status, err := testVmwareConnection(r.Context(), log, credentials)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}

	credPath := filepath.Join(dataDir, CredentialsFile)
	buf, _ := json.Marshal(credentials)
	writer := fileio.NewWriter()

	err = writer.WriteFile(credPath, buf)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed saving credentials: %v", err), http.StatusInternalServerError)
		return
	}

	log.Info("successfully wrote credentials to file")
	w.WriteHeader(http.StatusNoContent)
}

func parseUrl(credentials *Credentials) (*url.URL, error) {
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

func testVmwareConnection(requestCtx context.Context, log *log.PrefixLogger, credentials *Credentials) (status int, err error) {
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
	log.Info("logging into vmware using received credentials")
	err = client.Login(ctx, u.User)
	if err != nil {
		err = liberr.Wrap(err)
		if strings.Contains(err.Error(), "incorrect") && strings.Contains(err.Error(), "password") {
			return http.StatusUnauthorized, err
		}
		return http.StatusBadRequest, err
	}

	_ = client.Logout(ctx)
	client.CloseIdleConnections()

	return http.StatusAccepted, nil
}
