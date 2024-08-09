package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
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

func StartREST(log *log.PrefixLogger, dataDir string) {
	server := &http.Server{Addr: "0.0.0.0:3333", Handler: createRESTService(log, dataDir)}
	serverCtx, serverStopCtx := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sig
		shutdownCtx, _ := context.WithTimeout(serverCtx, 30*time.Second) // nolint:govet

		go func() {
			<-shutdownCtx.Done()
			if shutdownCtx.Err() == context.DeadlineExceeded {
				log.Fatal("graceful shutdown timed out.. forcing exit.")
			}
		}()

		// Trigger graceful shutdown
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			log.Fatal(err)
		}
		serverStopCtx()
	}()

	go func() {
		// Run the server
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}

		// Wait for server context to be stopped
		<-serverCtx.Done()
	}()
}

func createRESTService(log *log.PrefixLogger, dataDir string) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)

	r.Get("/status", func(w http.ResponseWriter, r *http.Request) {
		statusHandler(dataDir, w, r)
	})
	r.Put("/credentials", func(w http.ResponseWriter, r *http.Request) {
		credentialHandler(log, dataDir, w, r)
	})

	return r
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
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
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
