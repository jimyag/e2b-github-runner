package server

import (
	"net/http"
	"strings"

	"github.com/qiniu/ci-runner/internal/sandboxrunner"
)

func sandboxRegionEndpoint(region string) (string, bool) {
	switch strings.TrimSpace(region) {
	case "cn-yangzhou-1":
		return "https://cn-yangzhou-1-sandbox.qiniuapi.com", true
	case "us-south-1":
		return "https://us-south-1-sandbox.qiniuapi.com", true
	default:
		return "", false
	}
}

func (s *Server) sandboxCatalogForUser(w http.ResponseWriter, r *http.Request) (sandboxrunner.Catalog, bool) {
	_, account, ok := s.requireUserSession(w, r)
	if !ok {
		return nil, false
	}
	scope, err := s.accountPreferenceScopeFromRequest(account.ID, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	endpoint, ok := sandboxRegionEndpoint(r.URL.Query().Get("region"))
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported sandbox region")
		return nil, false
	}
	_, snapshot, err := s.sandboxServiceForScopeWithDefaultContext(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "sandbox service is not configured")
		return nil, false
	}
	snapshot.APIURL = endpoint
	svc, err := s.sandboxServiceForConfig(snapshot)
	if err != nil {
		s.logger.Warn("initialize sandbox service for catalog", "error", err)
		writeError(w, http.StatusServiceUnavailable, "initialize sandbox service")
		return nil, false
	}
	catalog, ok := svc.(sandboxrunner.Catalog)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "sandbox catalog is unavailable")
		return nil, false
	}
	return catalog, true
}

func (s *Server) handleListSandboxTemplates(w http.ResponseWriter, r *http.Request) {
	catalog, ok := s.sandboxCatalogForUser(w, r)
	if !ok {
		return
	}
	items, err := catalog.ListTemplates(r.Context())
	if err != nil {
		s.logger.Warn("list sandbox templates", "error", err)
		writeError(w, http.StatusBadGateway, "list sandbox templates")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleListSandboxes(w http.ResponseWriter, r *http.Request) {
	catalog, ok := s.sandboxCatalogForUser(w, r)
	if !ok {
		return
	}
	items, err := catalog.ListRunnerSandboxes(r.Context())
	if err != nil {
		s.logger.Warn("list sandboxes", "error", err)
		writeError(w, http.StatusBadGateway, "list sandboxes")
		return
	}
	templateID := strings.TrimSpace(r.URL.Query().Get("template_id"))
	items = filterCatalogSandboxes(items, templateID)
	writeJSON(w, http.StatusOK, items)
}

func filterCatalogSandboxes(items []sandboxrunner.CatalogSandbox, templateID string) []sandboxrunner.CatalogSandbox {
	if templateID == "" {
		return items
	}
	filtered := make([]sandboxrunner.CatalogSandbox, 0, len(items))
	for _, item := range items {
		if item.TemplateID == templateID {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
