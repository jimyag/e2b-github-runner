package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qiniu/ci-runner/internal/github"
	"github.com/qiniu/ci-runner/internal/state"
)

type adminSandboxServiceDefaultResponse struct {
	Enabled           bool                                  `json:"enabled"`
	Configured        bool                                  `json:"configured"`
	AudienceMode      string                                `json:"audience_mode"`
	Audiences         []state.SandboxServiceDefaultAudience `json:"audiences"`
	AvailableAccounts []state.GitHubInstallationAccount     `json:"available_accounts"`
	APIURL            string                                `json:"api_url"`
	APIKey            adminSandboxAPIKeyPreference          `json:"api_key"`
}

type adminSandboxAPIKeyPreference struct {
	Configured bool   `json:"configured"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type upsertAdminSandboxServiceDefaultRequest struct {
	Enabled      bool   `json:"enabled"`
	AudienceMode string `json:"audience_mode"`
	APIURL       string `json:"api_url"`
	APIKey       string `json:"api_key"`
}

type addAdminSandboxServiceDefaultAudienceRequest struct {
	AccountLogin string `json:"account_login"`
}

func (s *Server) handleAdminGetSandboxServiceDefault(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	defaultConfig, err := s.store.GetSandboxServiceDefault()
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			s.writeAdminSandboxServiceDefaultResponse(w, state.SandboxServiceDefault{})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeAdminSandboxServiceDefaultResponse(w, defaultConfig)
}

func (s *Server) handleAdminSaveSandboxServiceDefault(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	session, _ := s.adminSessionFromRequest(r)
	var input upsertAdminSandboxServiceDefaultRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid sandbox service default payload")
		return
	}

	apiURL := strings.TrimSpace(input.APIURL)
	if apiURL != "" {
		normalized, err := normalizeHTTPURL(apiURL)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		apiURL = normalized
	} else if input.Enabled {
		writeError(w, http.StatusBadRequest, "api_url is required")
		return
	}

	current, err := s.store.GetSandboxServiceDefault()
	if err != nil && !errors.Is(err, state.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	audienceMode := strings.ToLower(strings.TrimSpace(input.AudienceMode))
	if audienceMode == "" {
		audienceMode = current.AudienceMode
		if audienceMode == "" {
			audienceMode = state.SandboxServiceDefaultAudienceModeAll
		}
	}
	if audienceMode != state.SandboxServiceDefaultAudienceModeAll && audienceMode != state.SandboxServiceDefaultAudienceModeSelected {
		writeError(w, http.StatusBadRequest, "audience_mode must be all or selected")
		return
	}
	defaultConfig := state.SandboxServiceDefault{
		Enabled:      input.Enabled,
		AudienceMode: audienceMode,
		APIURL:       apiURL,
	}
	hasAPIKey := strings.TrimSpace(current.APIKeyEncrypted) != ""
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey != "" {
		encryptedAPIKey, err := encryptSecret(apiKey, s.cfg.AuthEncryptionKey.Value())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		now := time.Now().UTC()
		defaultConfig.APIKeyEncrypted = encryptedAPIKey
		defaultConfig.APIKeyUpdatedAt = &now
		hasAPIKey = true
	}
	if input.Enabled && !hasAPIKey {
		writeError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	var saved state.SandboxServiceDefault
	if input.Enabled && apiKey == "" {
		saved, err = s.store.UpdateSandboxServiceDefaultPreservingAPIKey(defaultConfig)
	} else {
		saved, err = s.store.UpsertSandboxServiceDefault(defaultConfig)
	}
	if err != nil {
		if errors.Is(err, state.ErrSandboxServiceDefaultAPIKeyRequired) {
			writeError(w, http.StatusBadRequest, "api_key is required")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordAudit("github:"+session.Subject, "sandbox_default.configure", "sandbox_service_default", "global", map[string]any{
		"enabled":        saved.Enabled,
		"audience_mode":  saved.AudienceMode,
		"api_url":        saved.APIURL,
		"api_key_update": apiKey != "",
	})
	s.writeAdminSandboxServiceDefaultResponse(w, saved)
}

func (s *Server) handleAdminDeleteSandboxServiceDefaultAPIKey(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	session, _ := s.adminSessionFromRequest(r)
	if err := s.store.DeleteSandboxServiceDefaultAPIKey(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordAudit("github:"+session.Subject, "sandbox_default.api_key.delete", "sandbox_service_default", "global", nil)
	defaultConfig, err := s.store.GetSandboxServiceDefault()
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			s.writeAdminSandboxServiceDefaultResponse(w, state.SandboxServiceDefault{})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeAdminSandboxServiceDefaultResponse(w, defaultConfig)
}

func (s *Server) handleAdminAddSandboxServiceDefaultAudience(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	session, _ := s.adminSessionFromRequest(r)
	var input addAdminSandboxServiceDefaultAudienceRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid sandbox service audience payload")
		return
	}
	accountLogin := strings.TrimSpace(input.AccountLogin)
	accountLogin = strings.TrimLeft(accountLogin, "@")
	if accountLogin == "" {
		writeError(w, http.StatusBadRequest, "GitHub account login is required")
		return
	}
	if s.gh == nil {
		writeError(w, http.StatusServiceUnavailable, "GitHub account lookup is unavailable")
		return
	}
	account, err := s.gh.GetAccount(r.Context(), accountLogin)
	if err != nil {
		if errors.Is(err, github.ErrAccountNotFound) {
			writeError(w, http.StatusBadRequest, "GitHub account was not found")
			return
		}
		if errors.Is(err, github.ErrUnsupportedAccountType) {
			writeError(w, http.StatusBadRequest, "GitHub account must be a user or organization")
			return
		}
		writeError(w, http.StatusBadGateway, "resolve GitHub account: "+err.Error())
		return
	}
	audience, err := s.store.UpsertSandboxServiceDefaultAudience(state.SandboxServiceDefaultAudience{
		GitHubAccountID: account.ID,
		AccountType:     account.Type,
		AccountLogin:    account.Login,
		AccountName:     account.Name,
		AccountAvatar:   account.AvatarURL,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordAudit("github:"+session.Subject, "sandbox_default.audience.add", "sandbox_service_default_audience", strconv.FormatInt(audience.ID, 10), map[string]any{
		"github_account_id": audience.GitHubAccountID,
		"account_type":      audience.AccountType,
		"account_login":     audience.AccountLogin,
	})
	defaultConfig, err := s.store.GetSandboxServiceDefault()
	if err != nil && !errors.Is(err, state.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeAdminSandboxServiceDefaultResponse(w, defaultConfig)
}

func (s *Server) handleAdminDeleteSandboxServiceDefaultAudience(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	session, _ := s.adminSessionFromRequest(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "valid audience id is required")
		return
	}
	if err := s.store.DeleteSandboxServiceDefaultAudience(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordAudit("github:"+session.Subject, "sandbox_default.audience.delete", "sandbox_service_default_audience", strconv.FormatInt(id, 10), nil)
	defaultConfig, err := s.store.GetSandboxServiceDefault()
	if err != nil && !errors.Is(err, state.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeAdminSandboxServiceDefaultResponse(w, defaultConfig)
}

func adminSandboxServiceDefaultResponseFor(defaultConfig state.SandboxServiceDefault) adminSandboxServiceDefaultResponse {
	response := adminSandboxServiceDefaultResponse{
		Enabled:      defaultConfig.Enabled,
		Configured:   strings.TrimSpace(defaultConfig.APIURL) != "" && strings.TrimSpace(defaultConfig.APIKeyEncrypted) != "",
		AudienceMode: defaultConfig.AudienceMode,
		APIURL:       defaultConfig.APIURL,
	}
	if response.AudienceMode == "" {
		response.AudienceMode = state.SandboxServiceDefaultAudienceModeAll
	}
	response.APIKey.Configured = strings.TrimSpace(defaultConfig.APIKeyEncrypted) != ""
	if defaultConfig.APIKeyUpdatedAt != nil {
		response.APIKey.UpdatedAt = defaultConfig.APIKeyUpdatedAt.UTC().Format(time.RFC3339)
	}
	return response
}

func (s *Server) writeAdminSandboxServiceDefaultResponse(w http.ResponseWriter, defaultConfig state.SandboxServiceDefault) {
	response := adminSandboxServiceDefaultResponseFor(defaultConfig)
	audiences, err := s.store.ListSandboxServiceDefaultAudiences()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	accounts, err := s.store.ListGitHubInstallationAccounts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.Audiences = audiences
	response.AvailableAccounts = accounts
	writeJSON(w, http.StatusOK, response)
}
