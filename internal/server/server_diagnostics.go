package server

import (
	"encoding/json"
	"expvar"
	"net/http"
	"strconv"
	"time"

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

type catalogMigrationGate struct {
	Code   string `json:"code"`
	Passed bool   `json:"passed"`
}

type catalogMigrationCurrentProcess struct {
	StartedAt          time.Time        `json:"started_at"`
	CatalogMatchCounts map[string]int64 `json:"catalog_match_counts"`
}

type catalogMigrationReadinessResponse struct {
	state.CatalogMigrationReadiness
	AutomatedGatesPassed bool                           `json:"automated_gates_passed"`
	Gates                []catalogMigrationGate         `json:"gates"`
	ManualRequirements   []string                       `json:"manual_requirements"`
	CurrentProcess       catalogMigrationCurrentProcess `json:"current_process"`
}

func (s *Server) handleCatalogMigrationReadiness(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	hours := 72
	if raw := r.URL.Query().Get("window_hours"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 720 {
			writeError(w, http.StatusBadRequest, "window_hours must be an integer from 1 through 720")
			return
		}
		hours = value
	}
	end := time.Now().UTC()
	start := end.Add(-time.Duration(hours) * time.Hour)
	report, err := s.store.CatalogMigrationReadiness(start, end)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	gates := catalogMigrationGates(report)
	automatedGatesPassed := true
	for _, gate := range gates {
		automatedGatesPassed = automatedGatesPassed && gate.Passed
	}
	writeJSON(w, http.StatusOK, catalogMigrationReadinessResponse{
		CatalogMigrationReadiness: report,
		AutomatedGatesPassed:      automatedGatesPassed,
		Gates:                     gates,
		ManualRequirements: []string{
			"backup_restore_verified",
			"continuous_service_observation",
			"workflow_labels_unchanged",
		},
		CurrentProcess: catalogMigrationCurrentProcess{
			StartedAt:          s.startedAt,
			CatalogMatchCounts: currentCatalogMatchCounts(),
		},
	})
}

func catalogMigrationGates(report state.CatalogMigrationReadiness) []catalogMigrationGate {
	windowPassed := report.WindowEnd.Sub(report.WindowStart) >= 72*time.Hour
	catalogPassed := !report.CatalogChangesTruncated && len(report.CatalogChanges) == 0
	replay := report.Replay
	parityPassed := replay.RequestCount > 0 && !replay.Truncated && replay.ErrorRequests == 0 &&
		replay.LegacyOnlyRequests == 0 && replay.EnabledOnlyRequests == 0 &&
		replay.DifferentProfileRequests == 0 && replay.SameRequests == replay.RequestCount
	lifecyclePassed := len(report.Specs) > 0
	for _, spec := range report.Specs {
		lifecyclePassed = lifecyclePassed && spec.CleanupFinalizedRequests > 0
	}
	return []catalogMigrationGate{
		{Code: "window_at_least_72_hours", Passed: windowPassed},
		{Code: "catalog_unchanged", Passed: catalogPassed},
		{Code: "matcher_parity", Passed: parityPassed},
		{Code: "all_enabled_specs_full_lifecycle", Passed: lifecyclePassed},
	}
}

func currentCatalogMatchCounts() map[string]int64 {
	counts := map[string]int64{
		"same":              0,
		"legacy_only":       0,
		"enabled_only":      0,
		"different_profile": 0,
	}
	value := expvar.Get("e2b_runner_catalog_match_migration_total")
	if value == nil {
		return counts
	}
	var published map[string]int64
	if err := json.Unmarshal([]byte(value.String()), &published); err != nil {
		return counts
	}
	for key := range counts {
		counts[key] = published[key]
	}
	return counts
}
