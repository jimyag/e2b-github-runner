package server

import (
	"net/http"

	"github.com/qiniu/ci-runner/internal/runnercatalog"
)

func (s *Server) handlePublicRunnerTemplates(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, runnercatalog.PublicTemplates())
}
