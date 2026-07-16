package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/qiniu/ci-runner/internal/state"
)

type upsertGitHubInstallationRequest struct {
	InstallationID int64  `json:"installation_id"`
	SetupState     string `json:"setup_state"`
}

type accountPreferencesResponse struct {
	Sandbox accountSandboxPreference `json:"sandbox"`
}

type accountSandboxPreference struct {
	Mode                   string                         `json:"mode"`
	ResolvedSource         string                         `json:"resolved_source"`
	APIURL                 string                         `json:"api_url"`
	APIKey                 accountSandboxAPIKeyPreference `json:"api_key"`
	Inherited              bool                           `json:"inherited"`
	SourceAccountID        int64                          `json:"source_account_id,omitempty"`
	SourceAccountLogin     string                         `json:"source_account_login,omitempty"`
	SourceIsCurrentAccount bool                           `json:"source_is_current_account,omitempty"`
	SourceAvailable        bool                           `json:"source_available"`
}

type accountSandboxAPIKeyPreference struct {
	Configured bool   `json:"configured"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type accountSandboxServicePreferenceValue struct {
	Mode            string `json:"mode,omitempty"`
	APIURL          string `json:"api_url,omitempty"`
	SourceAccountID int64  `json:"source_account_id,omitempty"`
}

type upsertSandboxConfigRequest struct {
	APIURL                 string `json:"api_url"`
	APIKey                 string `json:"api_key"`
	Mode                   string `json:"mode"`
	ReplaceInheritedSource bool   `json:"replace_inherited_source"`
}

type accountPreferenceScope struct {
	Type string
	ID   int64
}

const (
	accountPreferenceNamespaceSandbox  = "sandbox"
	accountPreferenceKeySandboxService = "service"
)

func (s *Server) handleGitHubAppInstallRedirect(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireUserSession(w, r); !ok {
		return
	}
	if strings.TrimSpace(s.cfg.GitHubAppSlug) == "" {
		writeError(w, http.StatusBadRequest, "github app slug is not configured")
		return
	}
	setupState := newID()
	http.SetCookie(w, &http.Cookie{
		Name:     githubAppSetupStateCookieName,
		Value:    setupState,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsSecure(r),
		MaxAge:   int((10 * time.Minute).Seconds()),
	})
	http.Redirect(w, r, s.githubAppInstallationURL(setupState), http.StatusFound)
}

func (s *Server) handleGitHubAppSetupRedirect(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireUserSession(w, r); !ok {
		return
	}
	values := url.Values{}
	for _, key := range []string{"installation_id", "setup_action", "state"} {
		if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
			values.Set(key, value)
		}
	}
	installationID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("installation_id")), 10, 64)
	if err == nil && installationID > 0 {
		if !s.validGitHubAppSetupState(r, r.URL.Query().Get("state")) {
			values.Set("setup_error", "invalid_state")
		}
	}
	target := "/account/repositories"
	if encoded := values.Encode(); encoded != "" {
		target += "?" + encoded
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) handleUserGitHubApp(w http.ResponseWriter, r *http.Request) {
	_, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	installations, err := s.store.ListGitHubInstallations(account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_slug":      strings.TrimSpace(s.cfg.GitHubAppSlug),
		"install_url":   s.githubAppInstallURL(),
		"setup_url":     "/github-app/setup",
		"installations": installations,
	})
}

func (s *Server) handleUserSaveGitHubInstallation(w http.ResponseWriter, r *http.Request) {
	session, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	var input upsertGitHubInstallationRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid github installation payload")
		return
	}
	if input.InstallationID <= 0 {
		writeError(w, http.StatusBadRequest, "installation_id is required")
		return
	}
	if !s.validGitHubAppSetupState(r, input.SetupState) {
		clearCookie(w, githubAppSetupStateCookieName, "/", requestIsSecure(r))
		writeError(w, http.StatusForbidden, "invalid github app setup state")
		return
	}
	installation, err := s.syncGitHubInstallation(r.Context(), account.ID, input.InstallationID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.invalidateUserRepositoryAccess(account.ID)
	clearCookie(w, githubAppSetupStateCookieName, "/", requestIsSecure(r))
	s.recordAudit("github:"+session.Subject, "github_app.configure", "github_installation", strconv.FormatInt(installation.InstallationID, 10), map[string]any{
		"account_login": installation.AccountLogin,
	})
	writeJSON(w, http.StatusOK, installation)
}

func (s *Server) handleUserDeleteGitHubInstallation(w http.ResponseWriter, r *http.Request) {
	session, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid github installation id")
		return
	}
	if err := s.store.DeleteGitHubInstallation(account.ID, id); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "github installation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.invalidateUserRepositoryAccess(account.ID)
	s.recordAudit("github:"+session.Subject, "github_app.delete", "github_installation", strconv.FormatInt(id, 10), nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleUserSyncGitHubInstallations(w http.ResponseWriter, r *http.Request) {
	session, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	if s.gh == nil {
		writeError(w, http.StatusInternalServerError, "github client is not configured")
		return
	}
	token, err := s.githubUserAccessToken(account.ID)
	if err != nil {
		if errors.Is(err, errGitHubUserAccessTokenRequired) {
			writeErrorCode(w, http.StatusBadRequest, "REAUTH_REQUIRED", "sign in with GitHub again before syncing installations")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	remoteInstallations, err := s.gh.ListUserInstallations(r.Context(), token)
	if err != nil {
		s.writeUserRepositoryAuthorizationError(w, err)
		return
	}
	existingInstallations, err := s.store.ListGitHubInstallations(account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer s.invalidateUserRepositoryAccess(account.ID)
	remoteIDs := make(map[int64]struct{}, len(remoteInstallations))
	synced := make([]state.GitHubInstallation, 0, len(remoteInstallations))
	for _, remote := range remoteInstallations {
		remoteIDs[remote.ID] = struct{}{}
		installation, err := s.store.UpsertGitHubInstallation(state.GitHubInstallation{
			AccountID:       account.ID,
			InstallationID:  remote.ID,
			GitHubAccountID: remote.AccountID,
			AccountType:     remote.AccountType,
			AccountLogin:    remote.AccountLogin,
			AccountName:     remote.AccountName,
			AccountAvatar:   remote.AccountAvatar,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		synced = append(synced, installation)
	}
	removed := 0
	for _, existing := range existingInstallations {
		if _, ok := remoteIDs[existing.InstallationID]; ok {
			continue
		}
		if err := s.store.DeleteGitHubInstallation(account.ID, existing.ID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		removed++
	}
	s.recordAudit("github:"+session.Subject, "github_app.sync", "account", strconv.FormatInt(account.ID, 10), map[string]any{
		"count":   len(synced),
		"removed": removed,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"installations": synced,
	})
}

func (s *Server) handleUserListGitHubInstallationRepositories(w http.ResponseWriter, r *http.Request) {
	_, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid github installation id")
		return
	}
	installation, ok, err := s.githubInstallationForAccount(account.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "github installation not found")
		return
	}
	if s.gh == nil {
		writeError(w, http.StatusInternalServerError, "github client is not configured")
		return
	}
	token, err := s.githubUserAccessToken(account.ID)
	if err != nil {
		s.writeUserRepositoryAuthorizationError(w, err)
		return
	}
	repositories, err := s.gh.ListUserInstallationRepositories(r.Context(), token, installation.InstallationID)
	if err != nil {
		s.writeUserRepositoryAuthorizationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"installation_id": installation.InstallationID,
		"repositories":    repositories,
	})
}

func (s *Server) handleUserListRunners(w http.ResponseWriter, r *http.Request) {
	_, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	access, err := s.userAuthorizedRepositoryAccess(r.Context(), account.ID)
	if err != nil {
		s.writeUserRepositoryAuthorizationError(w, err)
		return
	}
	states, err := s.store.ListStatesForGitHubInstallationRepositories(access, 500)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, states)
}

func (s *Server) handleUserPreferences(w http.ResponseWriter, r *http.Request) {
	_, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	scope, err := s.accountPreferenceScopeFromRequest(account.ID, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	response, err := s.accountPreferencesResponse(scope, account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleUserSaveSandboxConfig(w http.ResponseWriter, r *http.Request) {
	session, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	scope, err := s.accountPreferenceScopeFromRequest(account.ID, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var input upsertSandboxConfigRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid sandbox config payload")
		return
	}
	mode := normalizeSandboxPreferenceMode(input.Mode, scope)
	apiKey := strings.TrimSpace(input.APIKey)
	var value accountSandboxServicePreferenceValue
	var secret *state.AccountSecret
	if mode == sandboxPreferenceModeInherit {
		if scope.Type != state.AccountScopeTypeGitHubInstall {
			writeError(w, http.StatusBadRequest, "inherit mode is only supported for GitHub installation preferences")
			return
		}
		sourceAccountID, err := s.inheritedSandboxSourceAccountID(scope, account.ID, input.ReplaceInheritedSource)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if sourceAccountID == account.ID {
			configured, err := s.sandboxSourceAccountConfigured(account.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if !configured {
				writeError(w, http.StatusBadRequest, "configure your account Sandbox service first")
				return
			}
		}
		value = accountSandboxServicePreferenceValue{Mode: sandboxPreferenceModeInherit, SourceAccountID: sourceAccountID}
	} else {
		apiURL, err := normalizeHTTPURL(input.APIURL)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		_, currentKeyErr := s.store.GetAccountSecret(scope.Type, scope.ID, state.AccountSecretTypeSandboxAPIKey)
		if apiKey == "" && errors.Is(currentKeyErr, state.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "api_key is required")
			return
		}
		if currentKeyErr != nil && !errors.Is(currentKeyErr, state.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, currentKeyErr.Error())
			return
		}
		value = accountSandboxServicePreferenceValue{APIURL: apiURL}
		if apiKey != "" {
			encryptedAPIKey, err := encryptSecret(apiKey, s.cfg.AuthEncryptionKey)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			secret = &state.AccountSecret{
				ScopeType:      scope.Type,
				ScopeID:        scope.ID,
				KeyType:        state.AccountSecretTypeSandboxAPIKey,
				EncryptedValue: encryptedAPIKey,
			}
		}
	}
	valueJSON, err := json.Marshal(value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	preference := state.AccountPreference{
		ScopeType: scope.Type,
		ScopeID:   scope.ID,
		Namespace: accountPreferenceNamespaceSandbox,
		Key:       accountPreferenceKeySandboxService,
		ValueJSON: string(valueJSON),
	}
	if mode == sandboxPreferenceModeInherit {
		_, err = s.store.UpsertAccountPreferenceAndDeleteSecret(preference, state.AccountSecret{
			ScopeType: scope.Type,
			ScopeID:   scope.ID,
			KeyType:   state.AccountSecretTypeSandboxAPIKey,
		})
	} else {
		_, _, err = s.store.UpsertAccountPreferenceAndSecret(preference, secret)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	auditPayload := map[string]any{
		"mode":           mode,
		"api_url":        value.APIURL,
		"api_key_update": apiKey != "",
	}
	if mode == sandboxPreferenceModeInherit {
		auditPayload["source_account_id"] = value.SourceAccountID
	}
	s.recordAudit("github:"+session.Subject, "sandbox.configure", scope.Type, strconv.FormatInt(scope.ID, 10), auditPayload)
	response, err := s.accountPreferencesResponse(scope, account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleUserDeleteSandboxAPIKey(w http.ResponseWriter, r *http.Request) {
	session, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	scope, err := s.accountPreferenceScopeFromRequest(account.ID, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.DeleteAccountSecret(scope.Type, scope.ID, state.AccountSecretTypeSandboxAPIKey); err != nil && !errors.Is(err, state.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordAudit("github:"+session.Subject, "sandbox_api_key.delete", scope.Type, strconv.FormatInt(scope.ID, 10), nil)
	response, err := s.accountPreferencesResponse(scope, account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) accountPreferencesResponse(scope accountPreferenceScope, viewerAccountID int64) (accountPreferencesResponse, error) {
	var response accountPreferencesResponse
	preference, err := s.store.GetAccountPreference(scope.Type, scope.ID, accountPreferenceNamespaceSandbox, accountPreferenceKeySandboxService)
	if err != nil {
		if !errors.Is(err, state.ErrNotFound) {
			return accountPreferencesResponse{}, err
		}
		response.Sandbox.Mode = sandboxPreferenceModeCustom
		return s.fillSandboxResolvedSource(response, scope)
	} else {
		var value accountSandboxServicePreferenceValue
		if err := json.Unmarshal([]byte(preference.ValueJSON), &value); err != nil {
			return accountPreferencesResponse{}, err
		}
		mode := normalizeSandboxPreferenceMode(value.Mode, scope)
		response.Sandbox.Mode = mode
		if mode == sandboxPreferenceModeInherit {
			response.Sandbox.Inherited = true
			response.Sandbox.SourceAccountID = value.SourceAccountID
			response.Sandbox.SourceIsCurrentAccount = value.SourceAccountID == viewerAccountID
			identity, err := s.store.GetOAuthIdentityForAccount(value.SourceAccountID, "github")
			if err != nil && !errors.Is(err, state.ErrNotFound) {
				return accountPreferencesResponse{}, err
			}
			response.Sandbox.SourceAccountLogin = identity.OAuthLogin
			available, err := s.githubInstallationLinkedToAccount(value.SourceAccountID, scope.ID)
			if err != nil {
				return accountPreferencesResponse{}, err
			}
			response.Sandbox.SourceAvailable = available
			if !available {
				return s.fillSandboxResolvedSource(response, scope)
			}
			response, err = s.fillSandboxResponseFromScope(response, accountPreferenceScope{Type: state.AccountScopeTypeAccount, ID: value.SourceAccountID})
			if err != nil {
				return accountPreferencesResponse{}, err
			}
			return s.fillSandboxResolvedSource(response, scope)
		}
		response.Sandbox.APIURL = value.APIURL
	}
	response, err = s.fillSandboxResponseFromScope(response, scope)
	if err != nil {
		return accountPreferencesResponse{}, err
	}
	return s.fillSandboxResolvedSource(response, scope)
}

func (s *Server) fillSandboxResolvedSource(response accountPreferencesResponse, scope accountPreferenceScope) (accountPreferencesResponse, error) {
	_, snapshot, err := s.sandboxServiceForScopeWithDefault(scope)
	if err != nil {
		if errors.Is(err, errSandboxServiceNotConfigured) {
			response.Sandbox.ResolvedSource = "none"
			return response, nil
		}
		return accountPreferencesResponse{}, err
	}
	switch snapshot.Source {
	case sandboxConfigSourceInheritedAccount:
		response.Sandbox.ResolvedSource = "inherited"
	case sandboxConfigSourceAdminDefault:
		response.Sandbox.ResolvedSource = sandboxConfigSourceAdminDefault
	default:
		response.Sandbox.ResolvedSource = "custom"
	}
	return response, nil
}

func (s *Server) inheritedSandboxSourceAccountID(scope accountPreferenceScope, currentAccountID int64, replace bool) (int64, error) {
	if replace {
		return currentAccountID, nil
	}
	preference, err := s.store.GetAccountPreference(scope.Type, scope.ID, accountPreferenceNamespaceSandbox, accountPreferenceKeySandboxService)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return currentAccountID, nil
		}
		return 0, err
	}
	var value accountSandboxServicePreferenceValue
	if err := json.Unmarshal([]byte(preference.ValueJSON), &value); err != nil {
		return 0, err
	}
	if normalizeSandboxPreferenceMode(value.Mode, scope) == sandboxPreferenceModeInherit && value.SourceAccountID > 0 {
		return value.SourceAccountID, nil
	}
	return currentAccountID, nil
}

func (s *Server) sandboxSourceAccountConfigured(accountID int64) (bool, error) {
	preference, err := s.store.GetAccountPreference(state.AccountScopeTypeAccount, accountID, accountPreferenceNamespaceSandbox, accountPreferenceKeySandboxService)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	var value accountSandboxServicePreferenceValue
	if err := json.Unmarshal([]byte(preference.ValueJSON), &value); err != nil {
		return false, err
	}
	if strings.TrimSpace(value.APIURL) == "" {
		return false, nil
	}
	if _, err := s.store.GetAccountSecret(state.AccountScopeTypeAccount, accountID, state.AccountSecretTypeSandboxAPIKey); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Server) fillSandboxResponseFromScope(response accountPreferencesResponse, scope accountPreferenceScope) (accountPreferencesResponse, error) {
	if strings.TrimSpace(response.Sandbox.APIURL) == "" {
		preference, err := s.store.GetAccountPreference(scope.Type, scope.ID, accountPreferenceNamespaceSandbox, accountPreferenceKeySandboxService)
		if err != nil {
			if errors.Is(err, state.ErrNotFound) {
				return response, nil
			}
			return accountPreferencesResponse{}, err
		}
		var value accountSandboxServicePreferenceValue
		if err := json.Unmarshal([]byte(preference.ValueJSON), &value); err != nil {
			return accountPreferencesResponse{}, err
		}
		response.Sandbox.APIURL = value.APIURL
	}
	key, err := s.store.GetAccountSecret(scope.Type, scope.ID, state.AccountSecretTypeSandboxAPIKey)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return response, nil
		}
		return accountPreferencesResponse{}, err
	}
	response.Sandbox.APIKey.Configured = true
	response.Sandbox.APIKey.UpdatedAt = key.UpdatedAt.Format(time.RFC3339)
	return response, nil
}

const (
	sandboxPreferenceModeCustom  = "custom"
	sandboxPreferenceModeInherit = "inherit"
)

func normalizeSandboxPreferenceMode(mode string, scope accountPreferenceScope) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == sandboxPreferenceModeInherit && scope.Type == state.AccountScopeTypeGitHubInstall {
		return sandboxPreferenceModeInherit
	}
	return sandboxPreferenceModeCustom
}

func (s *Server) accountPreferenceScopeFromRequest(accountID int64, r *http.Request) (accountPreferenceScope, error) {
	installationIDText := strings.TrimSpace(r.URL.Query().Get("installation_id"))
	if installationIDText == "" {
		return accountPreferenceScope{Type: state.AccountScopeTypeAccount, ID: accountID}, nil
	}
	installationID, err := strconv.ParseInt(installationIDText, 10, 64)
	if err != nil || installationID <= 0 {
		return accountPreferenceScope{}, errors.New("invalid installation_id")
	}
	installation, ok, err := s.githubInstallationForAccount(accountID, installationID)
	if err != nil {
		return accountPreferenceScope{}, err
	} else if !ok {
		return accountPreferenceScope{}, errors.New("github installation not found")
	}
	return accountPreferenceScope{Type: state.AccountScopeTypeGitHubInstall, ID: installation.InstallationID}, nil
}

func normalizeHTTPURL(rawURL string) (string, error) {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return "", errors.New("api_url is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("api_url must be an absolute URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("api_url must use http or https")
	}
	return parsed.String(), nil
}

func (s *Server) githubAppInstallURL() string {
	slug := strings.TrimSpace(s.cfg.GitHubAppSlug)
	if slug == "" {
		return ""
	}
	return "/github-app/install"
}

func (s *Server) githubAppInstallationURL(setupState string) string {
	slug := strings.TrimSpace(s.cfg.GitHubAppSlug)
	if slug == "" {
		return ""
	}
	target := "https://github.com/apps/" + url.PathEscape(slug) + "/installations/new"
	if strings.TrimSpace(setupState) == "" {
		return target
	}
	values := url.Values{"state": {strings.TrimSpace(setupState)}}
	return target + "?" + values.Encode()
}

func (s *Server) githubInstallationForAccount(accountID, id int64) (state.GitHubInstallation, bool, error) {
	installations, err := s.store.ListGitHubInstallations(accountID)
	if err != nil {
		return state.GitHubInstallation{}, false, err
	}
	for _, installation := range installations {
		if installation.ID == id {
			return installation, true, nil
		}
	}
	return state.GitHubInstallation{}, false, nil
}

func (s *Server) syncGitHubInstallation(ctx context.Context, accountID, installationID int64) (state.GitHubInstallation, error) {
	if s.gh == nil {
		return state.GitHubInstallation{}, errors.New("github client is not configured")
	}
	installation, err := s.gh.GetInstallation(ctx, installationID)
	if err != nil {
		return state.GitHubInstallation{}, err
	}
	return s.store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:       accountID,
		InstallationID:  installation.ID,
		GitHubAccountID: installation.AccountID,
		AccountType:     installation.AccountType,
		AccountLogin:    installation.AccountLogin,
		AccountName:     installation.AccountName,
		AccountAvatar:   installation.AccountAvatar,
	})
}

func (s *Server) validGitHubAppSetupState(r *http.Request, setupState string) bool {
	setupState = strings.TrimSpace(setupState)
	cookie, err := r.Cookie(githubAppSetupStateCookieName)
	if err != nil || setupState == "" || strings.TrimSpace(cookie.Value) == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(setupState), []byte(cookie.Value)) == 1
}
