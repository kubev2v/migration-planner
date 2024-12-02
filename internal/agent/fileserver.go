package agent

import (
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi"
	"github.com/kubev2v/migration-planner/internal/agent/fileio"
	"go.uber.org/zap"
)

func RegisterFileServer(router *chi.Mux, wwwDir string) {
	fs := http.FileServer(http.Dir(wwwDir))

	router.Get("/login", handleGetLogin(wwwDir))
	router.Method("GET", "/*", fs)
}

func handleGetLogin(wwwDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathToIndeHtml := filepath.Join(wwwDir, "index.html")
		file, err := fileio.NewReader().ReadFile(pathToIndeHtml)
		if err != nil {
			zap.S().Named("handler_get_login").Warnf("Failed reading %s", pathToIndeHtml)
		}

		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", strconv.Itoa(len(file)))
		if _, err := w.Write(file); err != nil {
			zap.S().Named("handler_get_login").Warnf("Failed writing the content of %s", pathToIndeHtml)
		}
	}
}
