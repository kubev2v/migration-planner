package agent

import (
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi"
	"github.com/kubev2v/migration-planner/internal/agent/fileio"
	"github.com/kubev2v/migration-planner/pkg/log"
)

func RegisterFileServer(router *chi.Mux, log *log.PrefixLogger, wwwDir string) {
	fs := http.FileServer(http.Dir(wwwDir))

	router.Get("/login", handleGetLogin(log, wwwDir))
	router.Method("GET", "/*", fs)
}

func handleGetLogin(log *log.PrefixLogger, wwwDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathToIndeHtml := filepath.Join(wwwDir, "index.html")
		file, err := fileio.NewReader().ReadFile(pathToIndeHtml)
		if err != nil {
			log.Warnf("Failed reading %s", pathToIndeHtml)
		}

		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", strconv.Itoa(len(file)))
		if _, err := w.Write(file); err != nil {
			log.Warnf("Failed writing the content of %s", pathToIndeHtml)
		}
	}
}
