package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/qiniu/ci-runner/internal/github"
	"github.com/qiniu/ci-runner/internal/state"
)

var errGitHubUserAccessTokenRequired = errors.New("github user access token is required")

const maxUserRepositoryAccessCacheItems = 512

func (s *Server) githubUserAccessToken(accountID int64) (string, error) {
	secret, err := s.store.GetAccountSecret(state.AccountScopeTypeAccount, accountID, state.AccountSecretTypeGitHubOAuthToken)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return "", errGitHubUserAccessTokenRequired
		}
		return "", err
	}
	token, err := decryptSecret(secret.EncryptedValue, s.cfg.AuthEncryptionKey.Value())
	if err != nil {
		return "", errGitHubUserAccessTokenRequired
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errGitHubUserAccessTokenRequired
	}
	return token, nil
}

func (s *Server) userAuthorizedRepositoryAccess(ctx context.Context, accountID int64) ([]state.GitHubInstallationRepositoryAccess, error) {
	if access, ok := s.cachedUserRepositoryAccess(accountID); ok {
		return access, nil
	}
	installations, err := s.store.ListGitHubInstallations(accountID)
	if err != nil {
		return nil, err
	}
	if len(installations) == 0 {
		access := []state.GitHubInstallationRepositoryAccess{}
		s.cacheUserRepositoryAccess(accountID, access)
		return access, nil
	}
	if s.gh == nil {
		return nil, errors.New("github client is not configured")
	}
	token, err := s.githubUserAccessToken(accountID)
	if err != nil {
		return nil, err
	}
	access := make([]state.GitHubInstallationRepositoryAccess, 0, len(installations))
	for _, installation := range installations {
		items, err := s.gh.ListUserInstallationRepositories(ctx, token, installation.InstallationID)
		if err != nil {
			if status, ok := github.ErrorStatus(err); ok &&
				(status == http.StatusNotFound || (status == http.StatusForbidden && !github.IsRateLimitError(err))) {
				continue
			}
			return nil, err
		}
		seen := map[string]bool{}
		repositories := make([]string, 0, len(items))
		for _, repository := range items {
			repository = strings.TrimSpace(repository)
			key := strings.ToLower(repository)
			if repository == "" || seen[key] {
				continue
			}
			seen[key] = true
			repositories = append(repositories, repository)
		}
		if len(repositories) > 0 {
			access = append(access, state.GitHubInstallationRepositoryAccess{
				InstallationID: installation.InstallationID,
				Repositories:   repositories,
			})
		}
	}
	s.cacheUserRepositoryAccess(accountID, access)
	return access, nil
}

func (s *Server) cachedUserRepositoryAccess(accountID int64) ([]state.GitHubInstallationRepositoryAccess, bool) {
	now := time.Now()
	s.userRepositoryAccessMu.Lock()
	cached, ok := s.userRepositoryAccessCache[accountID]
	if ok && !now.Before(cached.expiresAt) {
		delete(s.userRepositoryAccessCache, accountID)
		ok = false
	}
	s.userRepositoryAccessMu.Unlock()
	if !ok {
		return nil, false
	}
	return cloneUserRepositoryAccess(cached.access), true
}

func (s *Server) cacheUserRepositoryAccess(accountID int64, access []state.GitHubInstallationRepositoryAccess) {
	s.userRepositoryAccessMu.Lock()
	if s.userRepositoryAccessCache == nil {
		s.userRepositoryAccessCache = map[int64]cachedUserRepositoryAccess{}
	}
	now := time.Now()
	if _, exists := s.userRepositoryAccessCache[accountID]; !exists &&
		len(s.userRepositoryAccessCache) >= maxUserRepositoryAccessCacheItems {
		evictOneUserRepositoryAccess(s.userRepositoryAccessCache)
	}
	s.userRepositoryAccessCache[accountID] = cachedUserRepositoryAccess{
		access:    cloneUserRepositoryAccess(access),
		expiresAt: now.Add(30 * time.Second),
	}
	s.userRepositoryAccessMu.Unlock()
}

func evictOneUserRepositoryAccess(cache map[int64]cachedUserRepositoryAccess) {
	for accountID := range cache {
		delete(cache, accountID)
		return
	}
}

func (s *Server) invalidateUserRepositoryAccess(accountID int64) {
	s.userRepositoryAccessMu.Lock()
	delete(s.userRepositoryAccessCache, accountID)
	s.userRepositoryAccessMu.Unlock()
}

func cloneUserRepositoryAccess(access []state.GitHubInstallationRepositoryAccess) []state.GitHubInstallationRepositoryAccess {
	cloned := make([]state.GitHubInstallationRepositoryAccess, 0, len(access))
	for _, item := range access {
		cloned = append(cloned, state.GitHubInstallationRepositoryAccess{
			InstallationID: item.InstallationID,
			Repositories:   append([]string(nil), item.Repositories...),
		})
	}
	return cloned
}

func (s *Server) writeUserRepositoryAuthorizationError(w http.ResponseWriter, err error) {
	if errors.Is(err, errGitHubUserAccessTokenRequired) {
		writeErrorCode(w, http.StatusForbidden, "REAUTH_REQUIRED", "sign in with GitHub again to load repository access")
		return
	}
	if github.IsRateLimitError(err) {
		if retryAfter, ok := github.RateLimitRetryAfter(err); ok {
			w.Header().Set("Retry-After", retryAfter)
		}
		s.logger.Warn("github user repository access rate limited", "error", err)
		writeErrorCode(w, http.StatusServiceUnavailable, "GITHUB_RATE_LIMITED", "GitHub repository access is temporarily rate limited")
		return
	}
	if status, ok := github.ErrorStatus(err); ok {
		switch status {
		case http.StatusUnauthorized:
			writeErrorCode(w, http.StatusForbidden, "REAUTH_REQUIRED", "sign in with GitHub again to refresh repository access")
			return
		case http.StatusForbidden:
			writeErrorCode(w, http.StatusForbidden, "GITHUB_REPOSITORY_ACCESS_FORBIDDEN", "GitHub denied repository access")
			return
		case http.StatusNotFound:
			writeError(w, http.StatusNotFound, "github installation is not accessible")
			return
		}
	}
	s.logger.Warn("load github user repository access failed", "error", err)
	writeError(w, http.StatusBadGateway, "load github repository access")
}

func hasRepositoryAccess(access []state.GitHubInstallationRepositoryAccess, installationID int64, repository string) bool {
	repository = strings.TrimSpace(repository)
	for _, item := range access {
		if item.InstallationID != installationID {
			continue
		}
		for _, candidate := range item.Repositories {
			if strings.EqualFold(strings.TrimSpace(candidate), repository) {
				return true
			}
		}
	}
	return false
}
