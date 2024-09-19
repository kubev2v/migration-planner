package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"github.com/kubev2v/migration-planner/internal/agent/fileio"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/vmware/govmomi/session/cache"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
)

const (
	// name of the file which stores the source credentials
	CredentialsFile = "credentials.json"
)

func RegisterApi(router *chi.Mux, log *log.PrefixLogger, dataDir string) {
	router.Get("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		statusHandler(dataDir, w, r)
	})
	router.Put("/api/v1/credentials", func(w http.ResponseWriter, r *http.Request) {
		credentialHandler(log, dataDir, w, r)
	})
}

type StatusReply struct {
	Status     string
	StatusInfo string
}

func (s StatusReply) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func statusHandler(dataDir string, w http.ResponseWriter, r *http.Request) {
	status, statusInfo, _ := calculateStatus(dataDir)
	_ = render.Render(w, r, StatusReply{Status: string(status), StatusInfo: statusInfo})
}

type Credentials struct {
	URL                  string `json:"url"`
	Username             string `json:"username"`
	Password             string `json:"password"`
	IsDataSharingAllowed bool   `json:"isDataSharingAllowed"`
}

func credentialHandler(log *log.PrefixLogger, dataDir string, w http.ResponseWriter, r *http.Request) {
	var credentials Credentials

	err := json.NewDecoder(r.Body).Decode(&credentials)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(credentials.URL) == 0 || len(credentials.Username) == 0 || len(credentials.Password) == 0 {
		http.Error(w, "Must pass url, username, and password", http.StatusBadRequest)
		return
	}

	log.Info("received credentials")
	err = testVmwareConnection(log, credentials)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	w.WriteHeader(204)
}

func testVmwareConnection(log *log.PrefixLogger, credentials Credentials) error {
	u, err := soap.ParseURL(credentials.URL)
	if err != nil {
		return err
	}
	u.User = url.UserPassword(credentials.Username, credentials.Password)
	s := &cache.Session{URL: u, Reauth: true, Insecure: true}
	c := new(vim25.Client)
	log.Info("logging into vmware using received credentials")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = s.Login(ctx, c, nil)
	if err != nil {
		return err
	}
	return s.Logout(ctx, c)
}
