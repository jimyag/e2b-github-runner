package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/qiniu/ci-runner/internal/sandboxrunner"
	"github.com/qiniu/ci-runner/internal/state"
)

var errSandboxServiceNotConfigured = errors.New("sandbox service not configured")

func (s *Server) sandboxServiceForRunnerRequest(_ context.Context, req state.RunnerRequest) (sandboxrunner.Service, error) {
	svc, _, err := s.sandboxServiceAndConfigForRunnerRequest(req)
	return svc, err
}

type sandboxServiceConfigSnapshot struct {
	APIURL          string
	EncryptedAPIKey string
}

func (s *Server) sandboxServiceAndConfigForRunnerRequest(req state.RunnerRequest) (sandboxrunner.Service, sandboxServiceConfigSnapshot, error) {
	if s.sandbox != nil {
		return s.sandbox, sandboxServiceConfigSnapshot{}, nil
	}
	if strings.TrimSpace(req.SandboxAPIURL) != "" && strings.TrimSpace(req.SandboxAPIKeyEncrypted) != "" {
		svc, err := s.sandboxServiceForConfig(sandboxServiceConfigSnapshot{
			APIURL:          req.SandboxAPIURL,
			EncryptedAPIKey: req.SandboxAPIKeyEncrypted,
		})
		return svc, sandboxServiceConfigSnapshot{
			APIURL:          strings.TrimSpace(req.SandboxAPIURL),
			EncryptedAPIKey: strings.TrimSpace(req.SandboxAPIKeyEncrypted),
		}, err
	}
	if req.GitHubInstallationID <= 0 {
		installationID, ok, err := s.githubInstallationScopeForRepository(req.RepositoryFullName)
		if err != nil {
			return nil, sandboxServiceConfigSnapshot{}, err
		}
		if ok {
			req.GitHubInstallationID = installationID
		}
	}
	scope, err := sandboxScopeForRunnerRequest(req)
	if err != nil {
		return nil, sandboxServiceConfigSnapshot{}, err
	}
	svc, snapshot, err := s.sandboxServiceForScope(scope)
	if err == nil {
		return svc, snapshot, nil
	}
	if !errors.Is(err, errSandboxServiceNotConfigured) {
		return nil, sandboxServiceConfigSnapshot{}, err
	}
	accountID, ok, lookupErr := s.store.AccountScopeForPersonalGitHubInstallation(req.GitHubInstallationID)
	if lookupErr != nil {
		return nil, sandboxServiceConfigSnapshot{}, lookupErr
	}
	if !ok {
		return nil, sandboxServiceConfigSnapshot{}, err
	}
	accountScope := accountPreferenceScope{Type: state.AccountScopeTypeAccount, ID: accountID}
	return s.sandboxServiceForScope(accountScope)
}

func (s *Server) githubInstallationScopeForRepository(repositoryFullName string) (int64, bool, error) {
	owner, _, ok := strings.Cut(strings.TrimSpace(repositoryFullName), "/")
	if !ok {
		return 0, false, nil
	}
	return s.store.GitHubInstallationScopeForAccountLogin(owner)
}

func (s *Server) sandboxServiceForScope(scope accountPreferenceScope) (sandboxrunner.Service, sandboxServiceConfigSnapshot, error) {
	preference, err := s.store.GetAccountPreference(scope.Type, scope.ID, accountPreferenceNamespaceSandbox, accountPreferenceKeySandboxService)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return nil, sandboxServiceConfigSnapshot{}, fmt.Errorf("sandbox service is not configured for %s:%d: %w", scope.Type, scope.ID, errSandboxServiceNotConfigured)
		}
		return nil, sandboxServiceConfigSnapshot{}, err
	}
	var value accountSandboxServicePreferenceValue
	if err := json.Unmarshal([]byte(preference.ValueJSON), &value); err != nil {
		return nil, sandboxServiceConfigSnapshot{}, err
	}
	if normalizeSandboxPreferenceMode(value.Mode, scope) == sandboxPreferenceModeInherit {
		if value.SourceAccountID <= 0 {
			return nil, sandboxServiceConfigSnapshot{}, fmt.Errorf("sandbox service inherited account is not configured for %s:%d: %w", scope.Type, scope.ID, errSandboxServiceNotConfigured)
		}
		ok, err := s.githubInstallationLinkedToAccount(value.SourceAccountID, scope.ID)
		if err != nil {
			return nil, sandboxServiceConfigSnapshot{}, err
		}
		if !ok {
			return nil, sandboxServiceConfigSnapshot{}, fmt.Errorf("sandbox service inherited account no longer has github installation %d: %w", scope.ID, errSandboxServiceNotConfigured)
		}
		return s.sandboxServiceForScope(accountPreferenceScope{Type: state.AccountScopeTypeAccount, ID: value.SourceAccountID})
	}
	apiURL := strings.TrimSpace(value.APIURL)
	if apiURL == "" {
		return nil, sandboxServiceConfigSnapshot{}, fmt.Errorf("sandbox service api_url is not configured for %s:%d", scope.Type, scope.ID)
	}
	secret, err := s.store.GetAccountSecret(scope.Type, scope.ID, state.AccountSecretTypeSandboxAPIKey)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return nil, sandboxServiceConfigSnapshot{}, fmt.Errorf("sandbox service api key is not configured for %s:%d: %w", scope.Type, scope.ID, errSandboxServiceNotConfigured)
		}
		return nil, sandboxServiceConfigSnapshot{}, err
	}
	snapshot := sandboxServiceConfigSnapshot{
		APIURL:          apiURL,
		EncryptedAPIKey: secret.EncryptedValue,
	}
	svc, err := s.sandboxServiceForConfig(snapshot)
	return svc, snapshot, err
}

func (s *Server) githubInstallationLinkedToAccount(accountID, installationID int64) (bool, error) {
	installations, err := s.store.ListGitHubInstallations(accountID)
	if err != nil {
		return false, err
	}
	for _, installation := range installations {
		if installation.InstallationID == installationID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) sandboxServiceForConfig(snapshot sandboxServiceConfigSnapshot) (sandboxrunner.Service, error) {
	apiKey, err := decryptSecret(snapshot.EncryptedAPIKey, s.cfg.AuthEncryptionKey)
	if err != nil {
		return nil, err
	}
	return sandboxrunner.NewE2BService(apiKey, strings.TrimSpace(snapshot.APIURL), s.sandboxHTTP)
}

func sandboxScopeForRunnerRequest(req state.RunnerRequest) (accountPreferenceScope, error) {
	if req.GitHubInstallationID <= 0 {
		return accountPreferenceScope{}, fmt.Errorf("runner request %s has no github installation scope", req.ID)
	}
	return accountPreferenceScope{
		Type: state.AccountScopeTypeGitHubInstall,
		ID:   req.GitHubInstallationID,
	}, nil
}
