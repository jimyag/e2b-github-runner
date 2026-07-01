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
	s.recordAudit("github:"+session.Subject, "github_app.delete", "github_installation", strconv.FormatInt(id, 10), nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	repositories, err := s.gh.ListInstallationRepositories(r.Context(), installation.InstallationID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
	installations, err := s.store.ListGitHubInstallations(account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	states, err := s.store.ListStatesForGitHubInstallations(installationIDsForUserInstallations(installations), 500)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, states)
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
		AccountID:      accountID,
		InstallationID: installation.ID,
		AccountLogin:   installation.AccountLogin,
		AccountName:    installation.AccountName,
		AccountAvatar:  installation.AccountAvatar,
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

func installationIDsForUserInstallations(installations []state.GitHubInstallation) []int64 {
	var ids []int64
	for _, installation := range installations {
		ids = append(ids, installation.InstallationID)
	}
	return uniquePositiveInt64s(ids)
}

func uniquePositiveInt64s(values []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
