package server

import (
	"io"
	"net/http"
	"strings"

	"github.com/qiniu/ci-runner/internal/redact"
	"github.com/qiniu/ci-runner/internal/state"
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
	states, err := s.store.ListStates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	failures := make([]state.RunnerState, 0, 5)
	for _, st := range states {
		if st.Status == state.StatusFailed {
			failures = append(failures, st)
			if len(failures) == 5 {
				break
			}
		}
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
	addresses, _ := discoverPprofArtifacts()
	if len(addresses) == 0 {
		writeError(w, http.StatusNotFound, "pprof endpoint not discovered")
		return
	}
	resp, err := s.diagnostics.Get(strings.TrimRight(addresses[0].Address, "/") + "/debug/vars")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadGateway, resp.Status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.Copy(w, resp.Body)
}
