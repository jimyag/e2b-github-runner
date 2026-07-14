package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/qiniu/ci-runner/internal/config"
	"github.com/qiniu/ci-runner/internal/github"
	"github.com/qiniu/ci-runner/internal/sandboxrunner"
	"github.com/qiniu/ci-runner/internal/state"
	"golang.org/x/sync/singleflight"
)

type Server struct {
	cfg         config.Config
	store       state.Store
	gh          *github.Client
	sandbox     sandboxrunner.Service
	sandboxHTTP *http.Client
	logger      *slog.Logger
	mux         *http.ServeMux
	slots       chan struct{}
	oauth       *http.Client
	diagnostics *http.Client
	terminals   *terminalHub

	admissionMu sync.Mutex
	locks       [64]sync.Mutex
	queueNotify chan struct{}
	startOnce   sync.Once
	workerID    string
	loopCtx     context.Context
	loopCancel  context.CancelFunc
	loopWG      sync.WaitGroup

	pullTitleMu    sync.Mutex
	pullTitleCache map[string]cachedPullTitle
	pullTitleGroup singleflight.Group
}

type cachedPullTitle struct {
	title     string
	errorText string
	expiresAt time.Time
}

type manualCreateRequest struct {
	ID                 string   `json:"id"`
	RepositoryFullName string   `json:"repository_full_name"`
	ProfileName        string   `json:"runner_spec_name"`
	Labels             []string `json:"labels"`
}

type upsertProfileRequest struct {
	Name             string   `json:"name"`
	Labels           []string `json:"labels"`
	TemplateID       string   `json:"template_id"`
	RunnerGroup      string   `json:"runner_group"`
	MaxConcurrency   int      `json:"max_concurrency"`
	MinIdle          *int     `json:"min_idle"`
	Priority         *int     `json:"priority"`
	Enabled          *bool    `json:"enabled"`
	DefaultAvailable *bool    `json:"default_available"`
}

type upsertRepositoryPolicyRequest struct {
	RepositoryFullName string `json:"repository_full_name"`
	ProfileName        string `json:"runner_spec_name"`
	RunnerGroupName    string `json:"runner_group_name"`
	Enabled            *bool  `json:"enabled"`
}

type upsertRunnerGroupRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	SpecNames   []string `json:"spec_names"`
	Enabled     *bool    `json:"enabled"`
}

type profileMatchRequest struct {
	RepositoryFullName string   `json:"repository_full_name"`
	Labels             []string `json:"labels"`
}

type adminSession struct {
	Provider  string `json:"provider,omitempty"`
	Subject   string `json:"subject"`
	Login     string `json:"login"`
	Role      string `json:"role"`
	AvatarURL string `json:"avatar_url,omitempty"`
	ExpiresAt int64  `json:"expires_at"`
}

const runnerJobStartedMarker = "__runner_job_started__"

const (
	defaultRunnerRequestListLimit = 100
	maxRunnerRequestListLimit     = 500
	oauthStateCookieName          = "runnerd_oauth_state"
	oauthReturnToCookieName       = "runnerd_oauth_return_to"
	githubAppSetupStateCookieName = "runnerd_github_app_setup_state"
	adminSessionCookieName        = "runnerd_admin_session"
)

var (
	githubOAuthAuthorizeURL = "https://github.com/login/oauth/authorize"
	githubOAuthTokenURL     = "https://github.com/login/oauth/access_token"
)

func New(cfg config.Config, store state.Store, gh *github.Client, sandbox sandboxrunner.Service, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		cfg:            cfg,
		store:          store,
		gh:             gh,
		sandbox:        sandbox,
		sandboxHTTP:    &http.Client{Timeout: 60 * time.Second},
		logger:         logger,
		mux:            http.NewServeMux(),
		slots:          make(chan struct{}, cfg.MaxConcurrentRunners),
		queueNotify:    make(chan struct{}, 1),
		oauth:          &http.Client{Timeout: 10 * time.Second},
		diagnostics:    &http.Client{Timeout: 5 * time.Second},
		terminals:      newTerminalHub(logger),
		pullTitleCache: map[string]cachedPullTitle{},
	}
	hostname, _ := os.Hostname()
	s.workerID = fmt.Sprintf("%s-%d", hostname, os.Getpid())
	s.loopCtx, s.loopCancel = context.WithCancel(context.Background())
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	lw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
	s.mux.ServeHTTP(lw, r)
	s.logger.Info(
		"http request",
		"method", r.Method,
		"path", r.URL.Path,
		"status", lw.status,
		"bytes", lw.bytes,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"remote_addr", r.RemoteAddr,
		"github_event", r.Header.Get("X-GitHub-Event"),
		"github_delivery", r.Header.Get("X-GitHub-Delivery"),
	)
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

func (w *loggingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (s *Server) Close() {
	if s.terminals != nil {
		s.terminals.Close()
	}
	if s.loopCancel != nil {
		s.logger.Info("stopping background loops")
		s.loopCancel()
	}
	s.loopWG.Wait()
	s.logger.Info("background loops stopped")
}

func (s *Server) Start() {
	s.startBackgroundLoops()
}

func (s *Server) Recover(ctx context.Context) error {
	states, err := s.store.ListActiveStates()
	if err != nil {
		return err
	}
	s.logger.Info("recovering runner state", "count", len(states))
	for _, st := range states {
		if !isActiveStatus(st.Status) {
			continue
		}
		if err := s.recoverRunner(ctx, st.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /", s.handleRoot)
	s.mux.HandleFunc("GET /admin", s.handleAdminRedirect)
	s.mux.HandleFunc("GET /admin/", s.handleAdmin)
	s.mux.HandleFunc("GET /user", s.handleUserRedirect)
	s.mux.HandleFunc("GET /user/", s.handleUserRedirect)
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /auth/session", s.handleAuthSession)
	s.mux.HandleFunc("GET /auth/github/login", s.handleGitHubOAuthLogin)
	s.mux.HandleFunc("GET /auth/github/callback", s.handleGitHubOAuthCallback)
	s.mux.HandleFunc("POST /auth/logout", s.handleAuthLogout)
	s.mux.HandleFunc("GET /github-app/install", s.handleGitHubAppInstallRedirect)
	s.mux.HandleFunc("GET /github-app/setup", s.handleGitHubAppSetupRedirect)
	s.mux.HandleFunc("GET /user/github-app", s.handleUserGitHubApp)
	s.mux.HandleFunc("POST /user/github-app/installations", s.handleUserSaveGitHubInstallation)
	s.mux.HandleFunc("POST /user/github-app/installations/sync", s.handleUserSyncGitHubInstallations)
	s.mux.HandleFunc("GET /user/github-app/installations/{id}/repositories", s.handleUserListGitHubInstallationRepositories)
	s.mux.HandleFunc("DELETE /user/github-app/installations/{id}", s.handleUserDeleteGitHubInstallation)
	s.mux.HandleFunc("GET /user/runner_requests", s.handleUserListRunners)
	s.mux.HandleFunc("GET /user/runner_requests/{id}", s.handleUserGetRunner)
	s.mux.HandleFunc("GET /user/runner_requests/{id}/group", s.handleUserGetRunnerJobGroup)
	s.mux.HandleFunc("GET /user/github/pulls/{owner}/{repo}/{number}/jobs", s.handleUserGetGitHubPullJobGroup)
	s.mux.HandleFunc("GET /user/github/runs/{owner}/{repo}/{runID}/jobs", s.handleUserGetGitHubRunJobGroup)
	s.mux.HandleFunc("GET /user/github/branches/{owner}/{repo}/{sha}/jobs", s.handleUserGetGitHubBranchJobGroup)
	s.mux.HandleFunc("GET /user/github/branches/{owner}/{repo}/{branch}/{sha}/jobs", s.handleUserGetGitHubBranchJobGroup)
	s.mux.HandleFunc("GET /user/runner_requests/{id}/siblings", s.handleUserListRunnerSiblings)
	s.mux.HandleFunc("GET /user/runner_requests/{id}/logs/{name}", s.handleUserGetRunnerLog)
	s.mux.HandleFunc("GET /user/runner_requests/{id}/github-log", s.handleUserGetRunnerGitHubLog)
	s.mux.HandleFunc("POST /user/runner_requests/{id}/terminal", s.handleUserCreateRunnerTerminal)
	s.mux.HandleFunc("GET /user/runner_requests/{id}/terminal/{sessionID}/events", s.handleUserRunnerTerminalEvents)
	s.mux.HandleFunc("POST /user/runner_requests/{id}/terminal/{sessionID}/input", s.handleUserRunnerTerminalInput)
	s.mux.HandleFunc("POST /user/runner_requests/{id}/terminal/{sessionID}/resize", s.handleUserRunnerTerminalResize)
	s.mux.HandleFunc("DELETE /user/runner_requests/{id}/terminal/{sessionID}", s.handleUserCloseRunnerTerminal)
	s.mux.HandleFunc("GET /user/preferences", s.handleUserPreferences)
	s.mux.HandleFunc("PUT /user/preferences/sandbox", s.handleUserSaveSandboxConfig)
	s.mux.HandleFunc("DELETE /user/preferences/sandbox-api-key", s.handleUserDeleteSandboxAPIKey)
	s.mux.HandleFunc("GET /user/sandbox/templates", s.handleListSandboxTemplates)
	s.mux.HandleFunc("GET /user/sandbox/instances", s.handleListSandboxes)
	s.mux.HandleFunc("POST /webhooks/github", s.handleGitHubWebhook)
	s.mux.HandleFunc("POST /runner_requests", s.handleCreateRunner)
	s.mux.HandleFunc("GET /runner_requests", s.handleListRunners)
	s.mux.HandleFunc("GET /runner_requests/{id}", s.handleGetRunner)
	s.mux.HandleFunc("POST /runner_requests/{id}/retry", s.handleRetryRunner)
	s.mux.HandleFunc("GET /runner_requests/{id}/logs/{name}", s.handleGetRunnerLog)
	s.mux.HandleFunc("DELETE /runner_requests/{id}", s.handleDeleteRunner)
	s.mux.HandleFunc("GET /audit-events", s.handleListAuditEvents)
	s.mux.HandleFunc("GET /runner_specs", s.handleListProfiles)
	s.mux.HandleFunc("POST /runner_specs", s.handleCreateProfile)
	s.mux.HandleFunc("POST /runner_specs/match", s.handleMatchProfile)
	s.mux.HandleFunc("GET /runner_specs/{name}", s.handleGetProfile)
	s.mux.HandleFunc("PATCH /runner_specs/{name}", s.handlePatchProfile)
	s.mux.HandleFunc("DELETE /runner_specs/{name}", s.handleDeleteProfile)
	s.mux.HandleFunc("GET /runner_groups", s.handleListRunnerGroups)
	s.mux.HandleFunc("POST /runner_groups", s.handleCreateRunnerGroup)
	s.mux.HandleFunc("GET /runner_groups/{name}", s.handleGetRunnerGroup)
	s.mux.HandleFunc("PATCH /runner_groups/{name}", s.handlePatchRunnerGroup)
	s.mux.HandleFunc("DELETE /runner_groups/{name}", s.handleDeleteRunnerGroup)
	s.mux.HandleFunc("GET /runner_policies", s.handleListRepositoryPolicies)
	s.mux.HandleFunc("POST /runner_policies", s.handleCreateRepositoryPolicy)
	s.mux.HandleFunc("PATCH /runner_policies/{id}", s.handlePatchRepositoryPolicy)
	s.mux.HandleFunc("DELETE /runner_policies/{id}", s.handleDeleteRepositoryPolicy)
	s.mux.HandleFunc("GET /diagnostics/pprof", s.handleDiagnosticsPprof)
	s.mux.HandleFunc("GET /diagnostics/vars", s.handleDiagnosticsVars)
}

func (s *Server) handleAdminRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/admin/")
	s.handleUI(w, r, name)
}

func (s *Server) handleUserRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusMovedPermanently)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if isUserUIRoute(name) {
		name = "index.html"
	}
	s.handleUI(w, r, name)
}

func isUserUIRoute(name string) bool {
	if name == "accounts" || name == "repositories" || name == "settings" {
		return true
	}
	for _, prefix := range []string{
		"account/",
		"organizations/",
		"jobs/",
		"github/pulls/",
		"github/runs/",
		"github/branches/",
	} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request, name string) {
	if name == "" {
		name = "index.html"
	}
	name = path.Clean("/" + name)
	if strings.HasPrefix(name, "/..") {
		http.NotFound(w, r)
		return
	}
	if !s.serveUIAsset(w, r, name) {
		http.NotFound(w, r)
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
