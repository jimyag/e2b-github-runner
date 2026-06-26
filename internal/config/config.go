package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTPAddr                  string
	HTTPReadTimeout           time.Duration
	HTTPWriteTimeout          time.Duration
	HTTPIdleTimeout           time.Duration
	StateDir                  string
	StateBackend              string
	StateDatabaseURL          string
	E2BAPIKey                 string
	E2BAPIURL                 string
	GitHubAppID               int64
	GitHubAppInstallationID   int64
	GitHubAppPrivateKeyFile   string
	GitHubToken               string
	GitHubBasicAuthUsername   string
	GitHubBasicAuthPassword   string
	GitHubWebhookSecret       string
	GitHubAllowedRepositories []string
	AuthSessionSecret         string
	AuthSessionTTL            time.Duration
	GitHubOAuthClientID       string
	GitHubOAuthClientSecret   string
	GitHubOAuthRedirectURL    string
	SandboxTimeout            time.Duration
	SandboxAPITimeout         time.Duration
	SandboxCreateTimeout      time.Duration
	SandboxStopTimeout        time.Duration
	RecoveryTimeout           time.Duration
	WorkerLeaseTTL            time.Duration
	RunnerIdleTimeout         time.Duration
	RetryBaseDelay            time.Duration
	RetryMaxDelay             time.Duration
	RetryMaxAttempts          int
	MaxConcurrentRunners      int
	GitHubAPIBaseURL          string
	ConfigPath                string
}

type fileConfig struct {
	Server struct {
		HTTPAddr        string `yaml:"http_addr"`
		ReadTimeoutSec  int    `yaml:"read_timeout_seconds"`
		WriteTimeoutSec int    `yaml:"write_timeout_seconds"`
		IdleTimeoutSec  int    `yaml:"idle_timeout_seconds"`
	} `yaml:"server"`
	Database struct {
		Backend string `yaml:"backend"`
		URL     string `yaml:"url"`
	} `yaml:"database"`
	Auth struct {
		SessionSecret   string `yaml:"session_secret"`
		SessionTTLHours int    `yaml:"session_ttl_hours"`
	} `yaml:"auth"`
	E2B struct {
		APIKey           string `yaml:"api_key"`
		APIURL           string `yaml:"api_url"`
		TimeoutSec       int    `yaml:"timeout_seconds"`
		APITimeoutSec    int    `yaml:"api_timeout_seconds"`
		CreateTimeoutSec int    `yaml:"create_timeout_seconds"`
		StopTimeoutSec   int    `yaml:"stop_timeout_seconds"`
	} `yaml:"e2b"`
	GitHub struct {
		WebhookSecret       string   `yaml:"webhook_secret"`
		APIBaseURL          string   `yaml:"api_base_url"`
		AllowedRepositories []string `yaml:"allowed_repositories"`
		Owner               string   `yaml:"owner"`
		Repo                string   `yaml:"repo"`
		Token               string   `yaml:"token"`
		BasicAuth           struct {
			Username string `yaml:"username"`
			Password string `yaml:"password"`
		} `yaml:"basic_auth"`
		App struct {
			ID             int64  `yaml:"id"`
			InstallationID int64  `yaml:"installation_id"`
			PrivateKeyFile string `yaml:"private_key_file"`
		} `yaml:"app"`
		OAuth struct {
			ClientID     string `yaml:"client_id"`
			ClientSecret string `yaml:"client_secret"`
			RedirectURL  string `yaml:"redirect_url"`
		} `yaml:"oauth"`
	} `yaml:"github"`
	Worker struct {
		MaxConcurrentRunners int `yaml:"max_concurrent_runners"`
		RunnerIdleTimeoutSec int `yaml:"runner_idle_timeout_seconds"`
		RecoveryTimeoutSec   int `yaml:"recovery_timeout_seconds"`
		LeaseTTLSec          int `yaml:"lease_ttl_seconds"`
		RetryBaseDelaySec    int `yaml:"retry_base_delay_seconds"`
		RetryMaxDelaySec     int `yaml:"retry_max_delay_seconds"`
		RetryMaxAttempts     int `yaml:"retry_max_attempts"`
	} `yaml:"worker"`
}

const defaultGitHubAPIBaseURL = "https://api.github.com"

func Load(path string) (Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "runnerd.yaml"
	}
	return LoadFile(path)
}

func LoadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	var raw fileConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	if strings.TrimSpace(raw.GitHub.Owner) != "" || strings.TrimSpace(raw.GitHub.Repo) != "" {
		return Config{}, fmt.Errorf("invalid config github.owner/github.repo: no longer supported; pass repository_full_name on manual runner requests")
	}
	configDir := filepath.Dir(path)
	cfg := Config{
		HTTPAddr:                  defaultString(raw.Server.HTTPAddr, ":25500"),
		HTTPReadTimeout:           durationSeconds(raw.Server.ReadTimeoutSec, 15),
		HTTPWriteTimeout:          durationSeconds(raw.Server.WriteTimeoutSec, 60),
		HTTPIdleTimeout:           durationSeconds(raw.Server.IdleTimeoutSec, 120),
		StateBackend:              strings.ToLower(defaultString(raw.Database.Backend, "sqlite")),
		StateDatabaseURL:          raw.Database.URL,
		E2BAPIKey:                 raw.E2B.APIKey,
		E2BAPIURL:                 raw.E2B.APIURL,
		GitHubAppID:               raw.GitHub.App.ID,
		GitHubAppInstallationID:   raw.GitHub.App.InstallationID,
		GitHubAppPrivateKeyFile:   resolveConfigPath(configDir, raw.GitHub.App.PrivateKeyFile),
		GitHubToken:               raw.GitHub.Token,
		GitHubBasicAuthUsername:   raw.GitHub.BasicAuth.Username,
		GitHubBasicAuthPassword:   raw.GitHub.BasicAuth.Password,
		GitHubWebhookSecret:       raw.GitHub.WebhookSecret,
		GitHubAllowedRepositories: normalizePatterns(raw.GitHub.AllowedRepositories),
		AuthSessionSecret:         strings.TrimSpace(raw.Auth.SessionSecret),
		AuthSessionTTL:            durationHours(raw.Auth.SessionTTLHours, 12),
		GitHubOAuthClientID:       strings.TrimSpace(raw.GitHub.OAuth.ClientID),
		GitHubOAuthClientSecret:   strings.TrimSpace(raw.GitHub.OAuth.ClientSecret),
		GitHubOAuthRedirectURL:    strings.TrimSpace(raw.GitHub.OAuth.RedirectURL),
		SandboxTimeout:            durationSeconds(raw.E2B.TimeoutSec, 3600),
		SandboxAPITimeout:         durationSeconds(raw.E2B.APITimeoutSec, 60),
		SandboxCreateTimeout:      durationSeconds(raw.E2B.CreateTimeoutSec, 120),
		SandboxStopTimeout:        durationSeconds(raw.E2B.StopTimeoutSec, 30),
		RecoveryTimeout:           durationSeconds(raw.Worker.RecoveryTimeoutSec, 120),
		WorkerLeaseTTL:            durationSeconds(raw.Worker.LeaseTTLSec, 300),
		RunnerIdleTimeout:         durationSeconds(raw.Worker.RunnerIdleTimeoutSec, 300),
		RetryBaseDelay:            durationSeconds(raw.Worker.RetryBaseDelaySec, 15),
		RetryMaxDelay:             durationSeconds(raw.Worker.RetryMaxDelaySec, 300),
		RetryMaxAttempts:          defaultInt(raw.Worker.RetryMaxAttempts, 5),
		MaxConcurrentRunners:      defaultInt(raw.Worker.MaxConcurrentRunners, 100),
		GitHubAPIBaseURL:          defaultString(raw.GitHub.APIBaseURL, defaultGitHubAPIBaseURL),
		ConfigPath:                path,
	}
	if cfg.StateBackend == "sqlite" {
		if strings.TrimSpace(cfg.StateDatabaseURL) == "" {
			cfg.StateDatabaseURL = filepath.Join(configDir, "var", "runnerd.db")
		} else {
			cfg.StateDatabaseURL = resolveConfigPath(configDir, cfg.StateDatabaseURL)
		}
		cfg.StateDir = filepath.Dir(cfg.StateDatabaseURL)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) GitHubAuthMode() string {
	if strings.TrimSpace(c.GitHubToken) != "" {
		return "token"
	}
	if strings.TrimSpace(c.GitHubBasicAuthUsername) != "" || strings.TrimSpace(c.GitHubBasicAuthPassword) != "" {
		return "basic"
	}
	if c.GitHubAppID == 0 && c.GitHubAppInstallationID == 0 && strings.TrimSpace(c.GitHubAppPrivateKeyFile) == "" {
		return "none"
	}
	return "app"
}

func (c Config) validate() error {
	var missing []string
	for path, value := range map[string]string{
		"e2b.api_key":           c.E2BAPIKey,
		"e2b.api_url":           c.E2BAPIURL,
		"github.webhook_secret": c.GitHubWebhookSecret,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, path)
		}
	}
	authModes := 0
	if c.GitHubAppID != 0 || c.GitHubAppInstallationID != 0 || strings.TrimSpace(c.GitHubAppPrivateKeyFile) != "" {
		authModes++
		if c.GitHubAppID == 0 {
			missing = append(missing, "github.app.id")
		}
		if strings.TrimSpace(c.GitHubAppPrivateKeyFile) == "" {
			missing = append(missing, "github.app.private_key_file")
		}
	}
	if strings.TrimSpace(c.GitHubToken) != "" {
		authModes++
	}
	if strings.TrimSpace(c.GitHubBasicAuthUsername) != "" || strings.TrimSpace(c.GitHubBasicAuthPassword) != "" {
		authModes++
		if strings.TrimSpace(c.GitHubBasicAuthUsername) == "" {
			missing = append(missing, "github.basic_auth.username")
		}
		if strings.TrimSpace(c.GitHubBasicAuthPassword) == "" {
			missing = append(missing, "github.basic_auth.password")
		}
	}
	if authModes == 0 {
		missing = append(missing, "github.app or github.token or github.basic_auth")
	}
	if authModes > 1 {
		return fmt.Errorf("invalid config github auth: configure exactly one of app, token, or basic_auth")
	}
	if c.StateBackend != "sqlite" && c.StateBackend != "postgres" {
		return fmt.Errorf("invalid config database.backend: must be sqlite or postgres")
	}
	if strings.TrimRight(c.GitHubAPIBaseURL, "/") != defaultGitHubAPIBaseURL {
		return fmt.Errorf("invalid config github.api_base_url: only %s is supported", defaultGitHubAPIBaseURL)
	}
	if strings.TrimSpace(c.StateDatabaseURL) == "" {
		missing = append(missing, "database.url")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	var oauthMissing []string
	if strings.TrimSpace(c.GitHubOAuthClientID) == "" {
		oauthMissing = append(oauthMissing, "github.oauth.client_id")
	}
	if strings.TrimSpace(c.GitHubOAuthClientSecret) == "" {
		oauthMissing = append(oauthMissing, "github.oauth.client_secret")
	}
	if strings.TrimSpace(c.AuthSessionSecret) == "" {
		oauthMissing = append(oauthMissing, "auth.session_secret")
	}
	if len(oauthMissing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(oauthMissing, ", "))
	}
	if c.MaxConcurrentRunners < 1 {
		return fmt.Errorf("invalid config worker.max_concurrent_runners: must be >= 1")
	}
	if c.RetryMaxAttempts < 1 {
		return fmt.Errorf("invalid config worker.retry_max_attempts: must be >= 1")
	}
	return nil
}

func (c Config) GitHubOAuthEnabled() bool {
	return strings.TrimSpace(c.GitHubOAuthClientID) != "" &&
		strings.TrimSpace(c.GitHubOAuthClientSecret) != "" &&
		strings.TrimSpace(c.AuthSessionSecret) != ""
}

func (c Config) RepositoryAllowed(repository string) bool {
	repository = strings.TrimSpace(repository)
	if repository == "" {
		return false
	}
	if len(c.GitHubAllowedRepositories) == 0 {
		return true
	}
	for _, pattern := range c.GitHubAllowedRepositories {
		if repositoryMatches(pattern, repository) {
			return true
		}
	}
	return false
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func durationSeconds(value, fallback int) time.Duration {
	if value <= 0 {
		value = fallback
	}
	return time.Duration(value) * time.Second
}

func durationHours(value, fallback int) time.Duration {
	if value <= 0 {
		value = fallback
	}
	return time.Duration(value) * time.Hour
}

func normalizePatterns(values []string) []string {
	seen := map[string]bool{}
	var patterns []string
	for _, value := range values {
		pattern := strings.TrimSpace(value)
		if pattern == "" || seen[pattern] {
			continue
		}
		seen[pattern] = true
		patterns = append(patterns, pattern)
	}
	return patterns
}

func repositoryMatches(pattern, repository string) bool {
	pattern = strings.TrimSpace(pattern)
	repository = strings.TrimSpace(repository)
	if pattern == "" || repository == "" {
		return false
	}
	if pattern == repository {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		return strings.HasPrefix(repository, strings.TrimSuffix(pattern, "*"))
	}
	return false
}

func resolveConfigPath(configDir, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(configDir, value)
}
