package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/jimmicro/pprof"
	"github.com/qiniu/ci-runner/internal/config"
	"github.com/qiniu/ci-runner/internal/github"
	"github.com/qiniu/ci-runner/internal/redact"
	"github.com/qiniu/ci-runner/internal/sandboxrunner"
	"github.com/qiniu/ci-runner/internal/server"
	"github.com/qiniu/ci-runner/internal/state"
)

func main() {
	configPath := flag.String("config", "runnerd.yaml", "path to runnerd config file")
	bootstrapAdmin := flag.String("bootstrap-admin", "", "bootstrap an admin user as provider:subject, for example github:12345678")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	store := state.NewWithOptions(state.Options{
		Backend:        cfg.StateBackend,
		DatabaseURL:    cfg.StateDatabaseURL,
		MigrateOnStart: true,
	})
	if err := store.Ensure(); err != nil {
		logger.Error("ensure state store", "error", err)
		os.Exit(1)
	}
	if err := bootstrapAdminUser(store, *bootstrapAdmin); err != nil {
		logger.Error("bootstrap admin user", "error", err)
		os.Exit(1)
	}
	githubHTTPClient := &http.Client{Timeout: 30 * time.Second}
	sandboxHTTPClient := &http.Client{}
	var gh *github.Client
	switch cfg.GitHubAuthMode() {
	case "app":
		gh, err = github.NewAppClient(cfg.GitHubAPIBaseURL, github.AppAuth{
			AppID:          cfg.GitHubAppID,
			InstallationID: cfg.GitHubAppInstallationID,
			PrivateKeyFile: cfg.GitHubAppPrivateKeyFile,
		}, githubHTTPClient)
		if err != nil {
			logger.Error("create github app client", "error", err)
			os.Exit(1)
		}
	case "token":
		gh = github.NewTokenClient(cfg.GitHubAPIBaseURL, cfg.GitHubToken, githubHTTPClient)
	case "basic":
		gh = github.NewBasicAuthClient(cfg.GitHubAPIBaseURL, cfg.GitHubBasicAuthUsername, cfg.GitHubBasicAuthPassword, githubHTTPClient)
	default:
		logger.Error("unsupported github auth mode", "mode", cfg.GitHubAuthMode())
		os.Exit(1)
	}
	sb, err := sandboxrunner.NewE2BService(cfg.E2BAPIKey, cfg.E2BAPIURL, sandboxHTTPClient)
	if err != nil {
		logger.Error("create sandbox client", "error", err)
		os.Exit(1)
	}
	handler := server.New(cfg, store, gh, sb, logger)
	go func() {
		recoveryCtx, cancel := context.WithTimeout(context.Background(), cfg.RecoveryTimeout)
		defer cancel()
		if err := handler.Recover(recoveryCtx); err != nil {
			logger.Error("recover runner state", "error", err)
		}
		handler.Start()
	}()
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
		MaxHeaderBytes:    1 << 20,
	}
	logger.Info("starting server", "addr", cfg.HTTPAddr, "state_backend", cfg.StateBackend, "state_database_url", redact.DatabaseURL(cfg.StateDatabaseURL))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func bootstrapAdminUser(store state.Store, identity string) error {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return nil
	}
	provider, subject, ok := strings.Cut(identity, ":")
	if !ok {
		provider = "github"
		subject = identity
	}
	provider = strings.TrimSpace(provider)
	subject = strings.TrimSpace(subject)
	if provider == "" || subject == "" {
		return fmt.Errorf("expected provider:subject")
	}
	_, err := store.UpsertUser(state.User{
		OAuthProvider: provider,
		OAuthSubject:  subject,
		OAuthLogin:    subject,
		Role:          "admin",
	})
	return err
}
