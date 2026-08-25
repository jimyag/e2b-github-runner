package server

import (
	"expvar"
	"net/http"

	"github.com/qiniu/ci-runner/internal/redact"
)

func (s *Server) handleDiagnosticsPprof(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	type artifact struct {
		Address     string `json:"address"`
		AddressFile string `json:"address_file"`
		DumpScript  string `json:"dump_script"`
	}
	addresses, scripts := discoverPprofArtifacts()
	failures, err := s.store.ListRecentFailedStates(5)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]artifact, 0, len(addresses))
	for i := range addresses {
		item := artifact{Address: addresses[i].Address, AddressFile: addresses[i].Path}
		if i < len(scripts) {
			item.DumpScript = scripts[i]
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pprof": out,
		"state": map[string]any{
			"backend":  s.cfg.StateBackend,
			"database": redact.DatabaseDSN(s.cfg.StateDatabaseDSN.Value()),
		},
		"github": map[string]any{
			"auth_mode":       s.cfg.GitHubAuthMode(),
			"installation_id": s.cfg.GitHubAppInstallationID,
			"api_base_url":    s.cfg.GitHubAPIBaseURL,
		},
		"recent_failures": failures,
	})
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func (s *Server) handleDiagnosticsVars(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	expvar.Handler().ServeHTTP(w, r)
}
