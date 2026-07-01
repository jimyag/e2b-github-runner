package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qiniu/ci-runner/internal/config"
	"github.com/qiniu/ci-runner/internal/github"
	"github.com/qiniu/ci-runner/internal/sandboxrunner"
	"github.com/qiniu/ci-runner/internal/state"
)

type fakeSandbox struct {
	mu             sync.Mutex
	started        int
	stopped        int
	startBlock     chan struct{}
	stopErr        error
	templateErr    error
	commandContext context.Context
	repositoryURL  string
	runnerGroup    string
}

func (f *fakeSandbox) ValidateTemplate(ctx context.Context, templateID string) error {
	return f.templateErr
}

func (f *fakeSandbox) StartRunner(ctx context.Context, input sandboxrunner.StartInput) (sandboxrunner.StartResult, error) {
	f.mu.Lock()
	f.started++
	block := f.startBlock
	f.commandContext = input.CommandContext
	f.repositoryURL = input.RepositoryURL
	f.runnerGroup = input.RunnerGroup
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return sandboxrunner.StartResult{}, ctx.Err()
		}
	}
	input.OnStdout([]byte("started\n"))
	return sandboxrunner.StartResult{SandboxID: "sb-" + input.RequestID, PID: 42}, nil
}

func (f *fakeSandbox) StopRunner(ctx context.Context, sandboxID string, pid uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped++
	return f.stopErr
}

func (f *fakeSandbox) startedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.started
}

func (f *fakeSandbox) stoppedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopped
}

func (f *fakeSandbox) lastCommandContext() context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commandContext
}

func (f *fakeSandbox) lastRepositoryURL() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.repositoryURL
}

func (f *fakeSandbox) lastRunnerGroup() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runnerGroup
}

func TestManualCreateAndDeleteRunner(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/actions/runners/registration-token":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"token":"runner-token","expires_at":"2026-05-18T10:00:00Z"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/actions/runners":
			w.Write([]byte(`{"runners":[{"id":99,"name":"e2b-manual-1","status":"online"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/repos/o/r/actions/runners/99":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)

	body := bytes.NewBufferString(`{"id":"manual-1","repository_full_name":"o/r","runner_spec_name":"default"}`)
	req := adminRequest(http.MethodPost, "/runner_requests", body)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	waitForState(t, store, "manual-1", state.StatusRunning)
	if fake.startedCount() != 1 {
		t.Fatalf("expected one sandbox start, got %d", fake.startedCount())
	}
	commandCtx := fake.lastCommandContext()
	if commandCtx == nil {
		t.Fatal("expected sandbox start to receive a long-lived command context")
	}
	select {
	case <-commandCtx.Done():
		t.Fatalf("command context was canceled immediately after runner create: %v", commandCtx.Err())
	default:
	}

	req = adminRequest(http.MethodDelete, "/runner_requests/manual-1", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected delete status: %d body=%s", rec.Code, rec.Body.String())
	}
	if fake.stoppedCount() != 1 {
		t.Fatalf("expected one sandbox stop, got %d", fake.stoppedCount())
	}

	req = adminRequest(http.MethodDelete, "/runner_requests/manual-1", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected second delete status: %d body=%s", rec.Code, rec.Body.String())
	}
	if fake.stoppedCount() != 1 {
		t.Fatalf("expected second delete to be idempotent, got %d stops", fake.stoppedCount())
	}
}

func TestOrgRunnerPassesMatchedRunnerGroupToSandbox(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/orgs/o/actions/runners/registration-token":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"token":"runner-token","expires_at":"2026-05-18T10:00:00Z"}`))
		default:
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)
	srv.gh = github.NewClient(ghServer.URL, http.DefaultClient)
	if _, err := store.UpsertProfile(state.RunnerProfile{
		Name:           "default",
		Labels:         []string{"self-hosted", "e2b"},
		TemplateID:     "base",
		RunnerGroup:    "default",
		MaxConcurrency: 10,
		Enabled:        true,
	}); err != nil {
		t.Fatal(err)
	}

	req := adminRequest(http.MethodPost, "/runner_requests", bytes.NewBufferString(`{"id":"org-1","repository_full_name":"o/r","runner_spec_name":"default"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	waitForState(t, store, "org-1", state.StatusRunning)
	if fake.lastRunnerGroup() != "default" {
		t.Fatalf("expected runner group to be passed to sandbox, got %q", fake.lastRunnerGroup())
	}
}

func TestRunnerExitMessageIncludesSDKError(t *testing.T) {
	got := runnerExitMessage(sandboxrunner.ExitResult{ExitCode: -1, Error: "stream closed before end event"})
	if !strings.Contains(got, "stream closed before end event") {
		t.Fatalf("expected SDK error in exit message, got %q", got)
	}
}

func TestManagementEndpointsRequireAdminAuth(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := httptest.NewRequest(http.MethodGet, "/runner_requests", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing auth to be rejected, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/runner_requests", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong auth to be rejected, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/runner_requests", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected non-admin session to be rejected, got %d", rec.Code)
	}

	if _, _, err := store.UpsertAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "hubot-id", OAuthLogin: "hubot"}, "admin"); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/runner_requests", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected promoted user session to authorize admin API, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodGet, "/runner_requests", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected authorized request, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUserGitHubAppConfigurationAndRunnerList(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations/987":
			if r.Header.Get("Authorization") == "" {
				t.Fatal("expected app JWT authorization for installation lookup")
			}
			w.Write([]byte(`{"id":987,"account":{"login":"o","name":"Octo Org","avatar_url":"https://avatars.example/o.png"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/987/access_tokens":
			if r.Header.Get("Authorization") == "" {
				t.Fatal("expected app JWT authorization for installation token")
			}
			w.Write([]byte(`{"token":"installation-token","expires_at":"2099-01-01T00:00:00Z"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/installation/repositories":
			if r.Header.Get("Authorization") != "token installation-token" {
				t.Fatalf("expected installation token authorization, got %q", r.Header.Get("Authorization"))
			}
			w.Write([]byte(`{"repositories":[{"full_name":"o/r"},{"full_name":"o/another"}]}`))
		default:
			t.Fatalf("unexpected github app request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	srv.cfg.GitHubAppSlug = "runnerd-test"
	appClient, err := github.NewAppClient(ghServer.URL, github.AppAuth{
		AppID:          123,
		PrivateKeyFile: testServerPrivateKeyFile(t),
	}, ghServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	srv.gh = appClient
	if _, _, err := store.CreateRequest(state.RunnerRequest{
		ID:                   "visible",
		Source:               "test",
		GitHubInstallationID: 987,
		RepositoryFullName:   "o/r",
		Labels:               []string{"self-hosted"},
		RunnerName:           "e2b-visible",
	}, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CreateRequest(state.RunnerRequest{
		ID:                   "hidden",
		Source:               "test",
		GitHubInstallationID: 456,
		RepositoryFullName:   "other/r",
		Labels:               []string{"self-hosted"},
		RunnerName:           "e2b-hidden",
	}, nil); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/user/github-app", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected app status: %d body=%s", rec.Code, rec.Body.String())
	}
	var appConfig struct {
		InstallURL string `json:"install_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &appConfig); err != nil {
		t.Fatal(err)
	}
	if appConfig.InstallURL != "/github-app/install" {
		t.Fatalf("unexpected install url: %q", appConfig.InstallURL)
	}

	req = httptest.NewRequest(http.MethodGet, "/github-app/install", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("unexpected install redirect status: %d body=%s", rec.Code, rec.Body.String())
	}
	installLocation := rec.Header().Get("Location")
	if !strings.HasPrefix(installLocation, "https://github.com/apps/runnerd-test/installations/new?") {
		t.Fatalf("unexpected github app install redirect: %q", installLocation)
	}
	parsedInstallLocation, err := url.Parse(installLocation)
	if err != nil {
		t.Fatal(err)
	}
	setupState := parsedInstallLocation.Query().Get("state")
	if setupState == "" {
		t.Fatalf("expected github app install state in %q", installLocation)
	}
	var setupStateCookie *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == githubAppSetupStateCookieName {
			setupStateCookie = cookie
		}
	}
	if setupStateCookie == nil || setupStateCookie.Value != setupState {
		t.Fatalf("expected github app setup state cookie, got %#v state=%q", setupStateCookie, setupState)
	}

	req = httptest.NewRequest(http.MethodGet, "/github-app/setup?installation_id=987&setup_action=install&state="+url.QueryEscape(setupState), nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	req.AddCookie(setupStateCookie)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("unexpected setup status: %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/account/repositories?installation_id=987&setup_action=install&state="+url.QueryEscape(setupState) {
		t.Fatalf("unexpected setup redirect: %q", rec.Header().Get("Location"))
	}

	req = httptest.NewRequest(http.MethodPost, "/user/github-app/installations", strings.NewReader(fmt.Sprintf(`{"installation_id":987,"setup_state":%q}`, setupState)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	req.AddCookie(setupStateCookie)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected installation save status: %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/user/github-app/installations", strings.NewReader(`{"installation_id":987,"setup_state":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	req.AddCookie(setupStateCookie)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected invalid setup state to be rejected, got %d body=%s", rec.Code, rec.Body.String())
	}
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	installations, err := store.ListGitHubInstallations(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 1 {
		t.Fatalf("expected one installation, got %#v", installations)
	}
	if installations[0].AccountName != "Octo Org" || installations[0].AccountAvatar != "https://avatars.example/o.png" {
		t.Fatalf("unexpected installation account metadata: %#v", installations[0])
	}
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/user/github-app/installations/%d/repositories", installations[0].ID), nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected installation repositories status: %d body=%s", rec.Code, rec.Body.String())
	}
	var installationRepositories struct {
		Repositories []string `json:"repositories"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &installationRepositories); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(installationRepositories.Repositories, []string{"o/r", "o/another"}) {
		t.Fatalf("unexpected installation repositories: %#v", installationRepositories.Repositories)
	}
	req = httptest.NewRequest(http.MethodGet, "/user/runner_requests", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected runner list status: %d body=%s", rec.Code, rec.Body.String())
	}
	var runners []state.RunnerState
	if err := json.Unmarshal(rec.Body.Bytes(), &runners); err != nil {
		t.Fatal(err)
	}
	if len(runners) != 1 || runners[0].ID != "visible" {
		t.Fatalf("unexpected user runners: %#v", runners)
	}
}

func TestGitHubOAuthLoginCreatesAdminSession(t *testing.T) {
	var gotTokenAuth string
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login/oauth/access_token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("client_id") != "Iv1.test" || r.Form.Get("client_secret") != "client-secret" || r.Form.Get("code") != "oauth-code" {
				t.Fatalf("unexpected oauth token form: %#v", r.Form)
			}
			w.Write([]byte(`{"access_token":"user-token"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			gotTokenAuth = r.Header.Get("Authorization")
			w.Write([]byte(`{"id":12345,"login":"octocat","avatar_url":"https://avatars.example/octocat.png"}`))
		default:
			t.Fatalf("unexpected github oauth request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	oldAuthorizeURL := githubOAuthAuthorizeURL
	oldTokenURL := githubOAuthTokenURL
	githubOAuthAuthorizeURL = "https://github.example/login/oauth/authorize"
	githubOAuthTokenURL = ghServer.URL + "/login/oauth/access_token"
	defer func() {
		githubOAuthAuthorizeURL = oldAuthorizeURL
		githubOAuthTokenURL = oldTokenURL
	}()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	srv.cfg.GitHubOAuthClientID = "Iv1.test"
	srv.cfg.GitHubOAuthClientSecret = "client-secret"
	srv.cfg.AuthSessionSecret = "session-secret"
	srv.cfg.AuthSessionTTL = time.Hour

	req := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"oauth_enabled":true`) {
		t.Fatalf("expected oauth-enabled session response, status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/auth/github/login", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected login redirect, got %d body=%s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if !strings.Contains(location, "client_id=Iv1.test") || !strings.Contains(location, "state=") {
		t.Fatalf("unexpected login redirect location: %s", location)
	}
	var stateCookie *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == oauthStateCookieName {
			stateCookie = cookie
			break
		}
	}
	if stateCookie == nil || stateCookie.Value == "" {
		t.Fatal("expected oauth state cookie")
	}

	req = httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=oauth-code&state="+stateCookie.Value, nil)
	req.AddCookie(stateCookie)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected callback redirect, got %d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("expected admin callback to redirect to /, got %q", loc)
	}
	if gotTokenAuth != "Bearer user-token" {
		t.Fatalf("expected user fetch bearer token, got %q", gotTokenAuth)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == adminSessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("expected admin session cookie")
	}
	session, err := srv.decodeAdminSession(sessionCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if session.Role != "admin" {
		t.Fatalf("expected admin session role, got %q", session.Role)
	}

	req = httptest.NewRequest(http.MethodGet, "/runner_requests", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected oauth session to authorize admin API, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"role":"admin"`) {
		t.Fatalf("expected session role response, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGitHubOAuthLoginCreatesNonAdminAccount(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login/oauth/access_token":
			w.Write([]byte(`{"access_token":"user-token"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			w.Write([]byte(`{"id":67890,"login":"newbie","avatar_url":"https://avatars.example/newbie.png"}`))
		default:
			t.Fatalf("unexpected github oauth request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	oldAuthorizeURL := githubOAuthAuthorizeURL
	oldTokenURL := githubOAuthTokenURL
	githubOAuthAuthorizeURL = "https://github.example/login/oauth/authorize"
	githubOAuthTokenURL = ghServer.URL + "/login/oauth/access_token"
	defer func() {
		githubOAuthAuthorizeURL = oldAuthorizeURL
		githubOAuthTokenURL = oldTokenURL
	}()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	srv.cfg.GitHubOAuthClientID = "Iv1.test"
	srv.cfg.GitHubOAuthClientSecret = "client-secret"
	srv.cfg.AuthSessionSecret = "session-secret"
	srv.cfg.AuthSessionTTL = time.Hour

	req := httptest.NewRequest(http.MethodGet, "/auth/github/login", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected login redirect, got %d body=%s", rec.Code, rec.Body.String())
	}
	var stateCookie *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == oauthStateCookieName {
			stateCookie = cookie
			break
		}
	}
	if stateCookie == nil || stateCookie.Value == "" {
		t.Fatal("expected oauth state cookie")
	}

	req = httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=oauth-code&state="+stateCookie.Value, nil)
	req.AddCookie(stateCookie)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected callback redirect, got %d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("expected user callback to redirect to /, got %q", loc)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == adminSessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("expected session cookie")
	}
	session, err := srv.decodeAdminSession(sessionCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if session.Role != "user" {
		t.Fatalf("expected default account role, got %q", session.Role)
	}
	account, _, err := store.GetAccountByOAuthIdentity("github", "67890")
	if err != nil {
		t.Fatal(err)
	}
	if account.Role != "user" {
		t.Fatalf("expected stored account role, got %#v", account)
	}

	req = httptest.NewRequest(http.MethodGet, "/runner_requests", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected non-admin oauth session to be rejected, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"role":"user"`) {
		t.Fatalf("expected user session response, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGitHubOAuthNonJSONErrorsIncludeStatus(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	srv.cfg.GitHubOAuthClientID = "Iv1.test"
	srv.cfg.GitHubOAuthClientSecret = "client-secret"
	srv.cfg.AuthSessionSecret = "session-secret"

	oldTokenURL := githubOAuthTokenURL
	githubOAuthTokenURL = ghServer.URL + "/login/oauth/access_token"
	defer func() {
		githubOAuthTokenURL = oldTokenURL
	}()

	if _, err := srv.exchangeGitHubOAuthCode(context.Background(), "code", ""); err == nil || !strings.Contains(err.Error(), "github oauth token status 502") {
		t.Fatalf("expected token status error, got %v", err)
	}
	if _, err := srv.fetchGitHubOAuthUser(context.Background(), "token"); err == nil || !strings.Contains(err.Error(), "github user status 502") {
		t.Fatalf("expected user status error, got %v", err)
	}
}

func TestListRunnerRequestsIsPaginated(t *testing.T) {
	store := state.New(t.TempDir())
	for i := 0; i < 105; i++ {
		if _, _, err := store.CreateRequest(state.RunnerRequest{
			ID:         fmt.Sprintf("runner-%03d", i),
			Source:     "test",
			Labels:     []string{"self-hosted"},
			RunnerName: fmt.Sprintf("e2b-runner-%03d", i),
			CreatedAt:  time.Unix(int64(i), 0).UTC(),
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := adminRequest(http.MethodGet, "/runner_requests", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var states []state.RunnerState
	if err := json.NewDecoder(rec.Body).Decode(&states); err != nil {
		t.Fatal(err)
	}
	if len(states) != 100 {
		t.Fatalf("default page len = %d, want 100", len(states))
	}
	if rec.Header().Get("X-Total-Count") != "105" {
		t.Fatalf("X-Total-Count = %q, want 105", rec.Header().Get("X-Total-Count"))
	}
	if rec.Header().Get("X-Limit") != "100" || rec.Header().Get("X-Offset") != "0" {
		t.Fatalf("unexpected pagination headers: limit=%q offset=%q", rec.Header().Get("X-Limit"), rec.Header().Get("X-Offset"))
	}
	if !strings.Contains(rec.Header().Get("Link"), `rel="next"`) {
		t.Fatalf("expected next link, got %q", rec.Header().Get("Link"))
	}

	req = adminRequest(http.MethodGet, "/runner_requests?limit=10&offset=100", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	states = nil
	if err := json.NewDecoder(rec.Body).Decode(&states); err != nil {
		t.Fatal(err)
	}
	if len(states) != 5 {
		t.Fatalf("second page len = %d, want 5", len(states))
	}
	if !strings.Contains(rec.Header().Get("Link"), `rel="prev"`) {
		t.Fatalf("expected prev link, got %q", rec.Header().Get("Link"))
	}
}

func TestAdminUIIsServedWithoutAPIAccess(t *testing.T) {
	if uiAssetsDevelopment {
		t.Skip("embedded UI is served only in production asset mode")
	}

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected admin ui, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `<div id="root">`) || !strings.Contains(rec.Body.String(), `/assets/`) {
		t.Fatalf("admin ui did not contain expected vite shell")
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/runner_specs", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected admin route fallback, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `<div id="root">`) {
		t.Fatalf("admin route fallback did not serve vite shell")
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected root user ui, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `<div id="root">`) {
		t.Fatalf("root user ui did not serve vite shell")
	}

	for _, path := range []string{"/repositories", "/account/repositories", "/account/preferences", "/organizations/qiniu/repositories", "/organizations/qiniu/preferences", "/settings", "/accounts"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected user route fallback for %s, got %d", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `<div id="root">`) {
			t.Fatalf("user route fallback for %s did not serve vite shell", path)
		}
	}
}

func TestLoggingResponseWriterSupportsResponseControllerHijack(t *testing.T) {
	base := newHijackableResponseWriter()
	t.Cleanup(base.close)
	wrapped := &loggingResponseWriter{ResponseWriter: base, status: http.StatusOK}

	conn, _, err := http.NewResponseController(wrapped).Hijack()
	if err != nil {
		t.Fatalf("expected logging response writer to delegate hijack: %v", err)
	}
	_ = conn.Close()
	if !base.hijacked {
		t.Fatal("expected underlying response writer to be hijacked")
	}
}

func TestLoggingResponseWriterSupportsResponseControllerFlush(t *testing.T) {
	base := newHijackableResponseWriter()
	t.Cleanup(base.close)
	wrapped := &loggingResponseWriter{ResponseWriter: base, status: http.StatusOK}

	if err := http.NewResponseController(wrapped).Flush(); err != nil {
		t.Fatalf("expected logging response writer to delegate flush: %v", err)
	}
	if !base.flushed {
		t.Fatal("expected underlying response writer to be flushed")
	}
}

func TestRunnerLogsRequireAdminAuth(t *testing.T) {
	store := state.New(t.TempDir())
	if _, _, err := store.CreateRequest(state.RunnerRequest{
		ID:         "with-log",
		Source:     "test",
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-with-log",
	}, nil); err != nil {
		t.Fatal(err)
	}
	store.AppendLog("with-log", "control.log", []byte("created\n"))
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := httptest.NewRequest(http.MethodGet, "/runner_requests/with-log/logs/control.log", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected log endpoint to require auth, got %d", rec.Code)
	}

	req = adminRequest(http.MethodGet, "/runner_requests/with-log/logs/control.log", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected log response, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "created\n" {
		t.Fatalf("unexpected log body: %q", rec.Body.String())
	}

	req = adminRequest(http.MethodGet, "/runner_requests/with-log/logs/request.json", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected unsupported log to be rejected, got %d", rec.Code)
	}

	req = adminRequest(http.MethodGet, "/runner_requests/with-log/logs/stderr.log", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected missing valid log to be returned as empty, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "" {
		t.Fatalf("expected empty missing log response, got %q", rec.Body.String())
	}
}

func TestWebhookQueuedIsIdempotent(t *testing.T) {
	ghServer := httptest.NewServer(githubRunnerAPI(t))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)

	payload := []byte(`{"action":"queued","repository":{"full_name":"o/r"},"workflow_job":{"id":1001,"labels":["self-hosted","e2b"]}}`)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
		req.Header.Set("X-GitHub-Event", "workflow_job")
		req.Header.Set("X-Hub-Signature-256", sign("secret", payload))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted && rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
	}
	waitForState(t, store, "1001", state.StatusRunning)
	if fake.startedCount() != 1 {
		t.Fatalf("expected one sandbox start, got %d", fake.startedCount())
	}
}

func TestWorkflowRunInProgressBackfillsQueuedJobs(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/actions/runs/123/jobs":
			w.Write([]byte(`{"jobs":[{"id":1001,"name":"test","status":"queued","labels":["self-hosted","e2b"]},{"id":1002,"name":"done","status":"completed","labels":["self-hosted","e2b"]}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/actions/jobs/1001":
			w.Write([]byte(`{"id":1001,"name":"test","status":"queued","labels":["self-hosted","e2b"]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/actions/runners/registration-token":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"token":"runner-token","expires_at":"2026-05-18T10:00:00Z"}`))
		default:
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)

	payload := []byte(`{"action":"in_progress","repository":{"full_name":"o/r"},"workflow_run":{"id":123,"name":"ci"}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "workflow_run")
	req.Header.Set("X-Hub-Signature-256", sign("secret", payload))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	waitForState(t, store, "1001", state.StatusRunning)
	if fake.startedCount() != 1 {
		t.Fatalf("expected one sandbox start, got %d", fake.startedCount())
	}
	if _, err := store.ReadState("1002"); err == nil {
		t.Fatal("completed workflow_run job should not create a runner")
	}
}

func TestWorkflowRunCompletedIsLoggedAndIgnored(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	payload := []byte(`{"action":"completed","repository":{"full_name":"o/r"},"workflow_run":{"id":123,"name":"ci"}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "workflow_run")
	req.Header.Set("X-Hub-Signature-256", sign("secret", payload))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["status"] != "ignored" {
		t.Fatalf("expected ignored response, got %#v", response)
	}
}

func TestWebhookQueuedSkipsSandboxWhenGitHubJobNoLongerQueued(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/actions/jobs/1001":
			w.Write([]byte(`{"id":1001,"name":"test","status":"completed","conclusion":"cancelled","labels":["self-hosted","e2b"]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/actions/runners/registration-token":
			t.Fatalf("registration token should not be requested for a completed job")
		default:
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)

	payload := []byte(`{"action":"queued","repository":{"full_name":"o/r"},"workflow_job":{"id":1001,"labels":["self-hosted","e2b"]}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", sign("secret", payload))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected queued status: %d body=%s", rec.Code, rec.Body.String())
	}
	waitForState(t, store, "1001", state.StatusCompleted)
	if fake.startedCount() != 0 {
		t.Fatalf("expected no sandbox start, got %d", fake.startedCount())
	}
}

func TestWebhookQueuedUsesEventRepositoryForRepoRunner(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/other/repo/actions/jobs/1001":
			w.Write([]byte(`{"id":1001,"name":"test","status":"queued","labels":["self-hosted","e2b"]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/other/repo/actions/runners/registration-token":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"token":"runner-token","expires_at":"2026-05-18T10:00:00Z"}`))
		default:
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	if _, err := store.UpsertRepositoryPolicy(state.RepositoryPolicy{
		RepositoryFullName: "other/repo",
		ProfileName:        "default",
		Enabled:            true,
	}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)

	payload := []byte(`{"action":"queued","repository":{"full_name":"other/repo"},"workflow_job":{"id":1001,"labels":["self-hosted","e2b"]}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", sign("secret", payload))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected queued status: %d body=%s", rec.Code, rec.Body.String())
	}
	waitForState(t, store, "1001", state.StatusRunning)
	if got := fake.lastRepositoryURL(); got != "https://github.com/other/repo" {
		t.Fatalf("expected event repository URL, got %q", got)
	}
}

func TestWebhookQueuedRejectsRepositoriesOutsideAllowlist(t *testing.T) {
	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, "http://example.test", fake)
	srv.cfg.GitHubAllowedRepositories = []string{"allowed/*"}

	payload := []byte(`{"action":"queued","repository":{"full_name":"blocked/repo"},"workflow_job":{"id":1001,"labels":["self-hosted","e2b"]}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", sign("secret", payload))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected queued status: %d body=%s", rec.Code, rec.Body.String())
	}
	got, err := store.ReadState("1001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusFailed || got.FailureReason != "repository_not_allowed" {
		t.Fatalf("expected allowlist admission failure, got %#v", got)
	}
	if fake.startedCount() != 0 {
		t.Fatalf("expected no sandbox start, got %d", fake.startedCount())
	}
}

func TestWebhookCompletedStopsActualRunnerAndRecordsJob(t *testing.T) {
	ghServer := httptest.NewServer(githubRunnerAPI(t))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)

	queued := []byte(`{"action":"queued","repository":{"full_name":"o/r"},"workflow_job":{"id":1001,"labels":["self-hosted","e2b"]}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(queued))
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", sign("secret", queued))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected queued status: %d body=%s", rec.Code, rec.Body.String())
	}
	waitForState(t, store, "1001", state.StatusRunning)

	completed := []byte(`{"action":"completed","repository":{"full_name":"o/r"},"workflow_job":{"id":1001,"name":"staticcheck","runner_name":"e2b-1001","labels":["self-hosted","e2b"]}}`)
	req = httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(completed))
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", sign("secret", completed))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected completed status: %d body=%s", rec.Code, rec.Body.String())
	}

	st, err := store.ReadState("1001")
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != state.StatusCompleted {
		t.Fatalf("expected completed state, got %s", st.Status)
	}
	if st.AssignedJobID != 1001 || st.AssignedJobName != "staticcheck" {
		t.Fatalf("expected assigned job to be recorded, got id=%d name=%q", st.AssignedJobID, st.AssignedJobName)
	}
	if fake.stoppedCount() != 1 {
		t.Fatalf("expected one sandbox stop, got %d", fake.stoppedCount())
	}
}

func TestWorkflowJobMismatchRequeuesOriginalJob(t *testing.T) {
	ghServer := httptest.NewServer(githubRunnerAPI(t))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)
	srv.Close()

	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "1001",
		Source:             "test",
		JobID:              1001,
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerName:         "e2b-1001",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = state.StatusRunning
	st.SandboxID = "sb-1001"
	st.ProcessPID = 42
	st.RunningAt = time.Now().UTC()
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	got, recorded, err := srv.stopRunner(t.Context(), "1001", github.WorkflowJob{ID: 2002, Name: "prepare", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if recorded {
		t.Fatal("expected mismatched job to suppress workflow completion metrics")
	}
	if got.Status != state.StatusQueued {
		t.Fatalf("expected original job to be requeued, got %s", got.Status)
	}
	if got.AssignedJobID != 0 || got.AssignedJobName != "" {
		t.Fatalf("expected assigned job to be cleared, got id=%d name=%q", got.AssignedJobID, got.AssignedJobName)
	}
	if fake.stoppedCount() != 1 {
		t.Fatalf("expected stolen runner sandbox to be stopped, got %d", fake.stoppedCount())
	}
}

func TestWebhookInProgressRecordsAssignedJob(t *testing.T) {
	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, "http://example.test", fake)

	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "1001",
		Source:             "test",
		JobID:              2002,
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerName:         "e2b-1001",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = state.StatusRunning
	st.RunningAt = time.Now().UTC()
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	payload := []byte(`{"action":"in_progress","repository":{"full_name":"o/r"},"workflow_job":{"id":2002,"name":"staticcheck","runner_name":"e2b-1001","workflow_name":"ci","labels":["self-hosted","e2b"]}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", sign("secret", payload))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected in_progress status: %d body=%s", rec.Code, rec.Body.String())
	}

	st, err = store.ReadState("1001")
	if err != nil {
		t.Fatal(err)
	}
	if st.AssignedJobID != 2002 || st.AssignedJobName != "staticcheck" {
		t.Fatalf("expected assigned job from in_progress event, got id=%d name=%q", st.AssignedJobID, st.AssignedJobName)
	}
}

func TestWebhookCompletedIsIdempotent(t *testing.T) {
	ghServer := httptest.NewServer(githubRunnerAPI(t))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)

	queued := []byte(`{"action":"queued","repository":{"full_name":"o/r"},"workflow_job":{"id":1001,"labels":["self-hosted","e2b"]}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(queued))
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", sign("secret", queued))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected queued status: %d body=%s", rec.Code, rec.Body.String())
	}
	waitForState(t, store, "1001", state.StatusRunning)

	completed := []byte(`{"action":"completed","repository":{"full_name":"o/r"},"workflow_job":{"id":1001,"name":"staticcheck","runner_name":"e2b-1001","labels":["self-hosted","e2b"]}}`)
	for i := 0; i < 2; i++ {
		req = httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(completed))
		req.Header.Set("X-GitHub-Event", "workflow_job")
		req.Header.Set("X-Hub-Signature-256", sign("secret", completed))
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("unexpected completed status: %d body=%s", rec.Code, rec.Body.String())
		}
	}

	st, err := store.ReadState("1001")
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != state.StatusCompleted {
		t.Fatalf("expected completed state, got %s", st.Status)
	}
	if st.AssignedJobID != 1001 || st.AssignedJobName != "staticcheck" {
		t.Fatalf("expected assigned job to be recorded, got id=%d name=%q", st.AssignedJobID, st.AssignedJobName)
	}
	if fake.stoppedCount() != 1 {
		t.Fatalf("expected duplicate completed event to stop once, got %d", fake.stoppedCount())
	}
}

func TestWebhookCompletedWithoutRunnerNameStopsByWorkflowJobID(t *testing.T) {
	ghServer := httptest.NewServer(githubRunnerAPI(t))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)

	queued := []byte(`{"action":"queued","repository":{"full_name":"o/r"},"workflow_job":{"id":1001,"labels":["self-hosted","e2b"]}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(queued))
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", sign("secret", queued))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected queued status: %d body=%s", rec.Code, rec.Body.String())
	}
	waitForState(t, store, "1001", state.StatusRunning)

	completed := []byte(`{"action":"completed","repository":{"full_name":"o/r"},"workflow_job":{"id":1001,"name":"staticcheck","runner_name":"","labels":["self-hosted","e2b"]}}`)
	req = httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(completed))
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", sign("secret", completed))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected completed status: %d body=%s", rec.Code, rec.Body.String())
	}

	st, err := store.ReadState("1001")
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != state.StatusCompleted {
		t.Fatalf("expected completed state, got %s", st.Status)
	}
	if fake.stoppedCount() != 1 {
		t.Fatalf("expected one sandbox stop, got %d", fake.stoppedCount())
	}
}

func TestWebhookCompletedWithoutLocalRunnerIsIgnored(t *testing.T) {
	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, "http://example.test", fake)

	completed := []byte(`{"action":"completed","repository":{"full_name":"o/r"},"workflow_job":{"id":1001,"name":"staticcheck","runner_name":"","labels":["self-hosted","e2b"]}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(completed))
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", sign("secret", completed))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected completed status: %d body=%s", rec.Code, rec.Body.String())
	}
	if fake.stoppedCount() != 0 {
		t.Fatalf("expected no sandbox stop, got %d", fake.stoppedCount())
	}
}

func TestStopDuringCreateCleansStartedSandbox(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"token":"runner-token","expires_at":"2026-05-18T10:00:00Z"}`))
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	block := make(chan struct{})
	fake := &fakeSandbox{startBlock: block}
	srv := newTestServer(t, store, ghServer.URL, fake)

	req := adminRequest(http.MethodPost, "/runner_requests", bytes.NewBufferString(`{"id":"manual-creating","repository_full_name":"o/r","runner_spec_name":"default"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected create status: %d body=%s", rec.Code, rec.Body.String())
	}
	waitForSandboxStarts(t, fake, 1)

	req = adminRequest(http.MethodDelete, "/runner_requests/manual-creating", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected delete status: %d body=%s", rec.Code, rec.Body.String())
	}
	if fake.stoppedCount() != 0 {
		t.Fatalf("sandbox should not stop before creation returns, got %d stops", fake.stoppedCount())
	}

	close(block)
	waitForStops(t, fake, 1)
	st, err := store.ReadState("manual-creating")
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != state.StatusCompleted {
		t.Fatalf("expected completed state, got %s", st.Status)
	}
}

func TestRecoverStopsActiveRunnerState(t *testing.T) {
	store := state.New(t.TempDir())
	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:         "recover-1",
		Source:     "test",
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-recover-1",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = state.StatusRunning
	st.SandboxID = "sb-recover-1"
	st.ProcessPID = 42
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, "http://example.test", fake)
	if err := srv.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := store.ReadState("recover-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusFailed {
		t.Fatalf("expected recovered state to be failed, got %s", got.Status)
	}
	if got.FailureStage != "recovery" || got.FailureReason != "interrupted_runner" {
		t.Fatalf("unexpected recovery failure metadata: %#v", got)
	}
	if fake.stoppedCount() != 1 {
		t.Fatalf("expected recovery to stop sandbox once, got %d", fake.stoppedCount())
	}
}

func TestRecoverSchedulesRetryWhenGitHubRunnerBusy(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/actions/runners":
			w.Write([]byte(`{"runners":[{"id":99,"name":"e2b-recover-busy","status":"online","busy":true}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/repos/o/r/actions/runners/99":
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(`{"message":"Bad request - Runner e2b-recover-busy is currently running a job and cannot be deleted.","status":"422"}`))
		default:
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "recover-busy",
		Source:             "test",
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted"},
		ProfileName:        "default",
		RunnerName:         "e2b-recover-busy",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = state.StatusRunning
	st.SandboxID = "sb-recover-busy"
	st.ProcessPID = 42
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	if err := srv.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := store.ReadState("recover-busy")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusStopping || got.FailureStage != "cleanup" || got.FailureReason != "github_runner_busy" || got.NextRetryAt.IsZero() {
		t.Fatalf("expected busy runner cleanup retry, got %#v", got)
	}
}

func TestStopRunnerSchedulesGitHubCleanupRetry(t *testing.T) {
	var listCalls int
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/actions/runners":
			listCalls++
			if listCalls == 1 {
				http.Error(w, "temporary github failure", http.StatusInternalServerError)
				return
			}
			w.Write([]byte(`{"runners":[{"id":99,"name":"e2b-cleanup-retry","status":"offline"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/repos/o/r/actions/runners/99":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)
	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "cleanup-retry",
		Source:             "test",
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerName:         "e2b-cleanup-retry",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = state.StatusRunning
	st.SandboxID = "sb-cleanup-retry"
	st.ProcessPID = 42
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	got, stopped, err := srv.stopRunner(t.Context(), "cleanup-retry", github.WorkflowJob{})
	if err != nil {
		t.Fatalf("stopRunner should schedule retry instead of failing immediately: %v", err)
	}
	if stopped {
		t.Fatal("stopRunner reported stopped even though GitHub cleanup retry was scheduled")
	}
	if got.Status != state.StatusStopping || got.FailureStage != "cleanup" || !got.LastErrorRetryable || got.NextRetryAt.IsZero() {
		t.Fatalf("expected stopping cleanup retry state, got %#v", got)
	}

	got.NextRetryAt = time.Now().UTC().Add(-time.Second)
	if err := store.WriteState(got); err != nil {
		t.Fatal(err)
	}
	got, stopped, err = srv.stopRunner(t.Context(), "cleanup-retry", github.WorkflowJob{})
	if err != nil {
		t.Fatalf("stopRunner retry should complete after GitHub cleanup succeeds: %v", err)
	}
	if !stopped || got.Status != state.StatusCompleted {
		t.Fatalf("expected cleanup retry to complete runner, stopped=%v state=%#v", stopped, got)
	}
	if got.FailureStage != "" || got.LastErrorMessage != "" || !got.NextRetryAt.IsZero() {
		t.Fatalf("expected cleanup retry metadata to be cleared after success, got %#v", got)
	}
}

func TestStopRunnerPreservesFailureAfterCleanupRetrySuccess(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/actions/runners":
			w.Write([]byte(`{"runners":[{"id":99,"name":"e2b-failed-cleanup","status":"offline"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/repos/o/r/actions/runners/99":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)
	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "failed-cleanup",
		Source:             "test",
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerName:         "e2b-failed-cleanup",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = state.StatusStopping
	st.SandboxID = "sb-failed-cleanup"
	st.ProcessPID = 42
	st.FailureStage = "sandbox_timeout"
	st.FailureReason = "timeout"
	st.Error = "runner exceeded sandbox timeout"
	st.NextRetryAt = time.Now().UTC().Add(-time.Second)
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	got, stopped, err := srv.stopRunner(t.Context(), "failed-cleanup", github.WorkflowJob{})
	if err != nil {
		t.Fatalf("stopRunner should finish cleanup for failed runner: %v", err)
	}
	if !stopped || got.Status != state.StatusFailed {
		t.Fatalf("expected failed terminal state after cleanup, stopped=%v state=%#v", stopped, got)
	}
	if got.FailureStage != "sandbox_timeout" || got.FailureReason != "timeout" {
		t.Fatalf("expected original failure metadata to be preserved, got %#v", got)
	}
}

func TestSweeperMarksTimedOutRunningRunnerFailed(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && (r.URL.Path == "/repos/o/r/actions/runners" || r.URL.Path == "/orgs/o/actions/runners") {
			w.Write([]byte(`{"runners":[]}`))
			return
		}
		t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)
	srv.cfg.SandboxTimeout = time.Nanosecond

	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "timed-out",
		Source:             "test",
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerName:         "e2b-timed-out",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = state.StatusRunning
	st.SandboxID = "sb-timed-out"
	st.ProcessPID = 42
	st.RunningAt = time.Now().UTC().Add(-time.Hour)
	st.AssignedJobName = runnerJobStartedMarker
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	srv.sweepOnce(t.Context())

	got, err := store.ReadState("timed-out")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusFailed {
		t.Fatalf("expected timed-out runner to be failed, got %#v", got)
	}
	if got.FailureStage != "sandbox_timeout" || got.FailureReason != "timeout" {
		t.Fatalf("expected sandbox timeout failure, got stage=%q reason=%q", got.FailureStage, got.FailureReason)
	}
	if fake.stoppedCount() != 1 {
		t.Fatalf("expected sandbox stop, got %d", fake.stoppedCount())
	}
}

func TestConcurrencyLimitDefersStartWithoutDroppingRequest(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"token":"runner-token","expires_at":"2026-05-18T10:00:00Z"}`))
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServerWithLimit(t, store, ghServer.URL, fake, 1)
	_, existing, err := store.CreateRequest(state.RunnerRequest{
		ID:         "existing-active",
		Source:     "test",
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-existing-active",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	existing.Status = state.StatusRunning
	existing.SandboxID = "sb-existing-active"
	if err := store.WriteState(existing); err != nil {
		t.Fatal(err)
	}

	req := adminRequest(http.MethodPost, "/runner_requests", bytes.NewBufferString(`{"id":"manual-2","repository_full_name":"o/r","runner_spec_name":"default"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected request to be queued, got %d body=%s", rec.Code, rec.Body.String())
	}
	got, err := store.ReadState("manual-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusQueued {
		t.Fatalf("expected queued request while global limit is full, got %#v", got)
	}
	if fake.startedCount() != 0 {
		t.Fatalf("expected no sandbox start while global limit is full, got %d", fake.startedCount())
	}
}

func TestWorkflowJobQueuedAtCapacityIsRecorded(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/actions/runners/registration-token" {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"token":"runner-token","expires_at":"2026-05-18T10:00:00Z"}`))
			return
		}
		t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServerWithLimit(t, store, ghServer.URL, fake, 1)
	_, existing, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "existing-active",
		Source:             "test",
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerName:         "e2b-existing-active",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	existing.Status = state.StatusRunning
	existing.SandboxID = "sb-existing-active"
	if err := store.WriteState(existing); err != nil {
		t.Fatal(err)
	}

	payload := []byte(`{"action":"queued","repository":{"full_name":"o/r"},"workflow_job":{"id":1001,"labels":["self-hosted","e2b"]}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", sign("secret", payload))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected queued webhook to be accepted, got %d body=%s", rec.Code, rec.Body.String())
	}
	got, err := store.ReadState("1001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusQueued {
		t.Fatalf("expected webhook job to be recorded as queued, got %#v", got)
	}
	if fake.startedCount() != 0 {
		t.Fatalf("expected no sandbox start while global limit is full, got %d", fake.startedCount())
	}
}

func TestProfileAndRepositoryPolicyEndpoints(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	profileBody := bytes.NewBufferString(`{"name":"large","labels":["self-hosted","e2b","large"],"template_id":"large","runner_group":"large","max_concurrency":5,"min_idle":1,"priority":10,"enabled":true}`)
	req := adminRequest(http.MethodPost, "/runner_specs", profileBody)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected create profile status: %d body=%s", rec.Code, rec.Body.String())
	}

	policyBody := bytes.NewBufferString(`{"repository_full_name":"o/heavy","runner_spec_name":"large","enabled":true}`)
	req = adminRequest(http.MethodPost, "/runner_policies", policyBody)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected create policy status: %d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodPost, "/runner_specs/match", bytes.NewBufferString(`{"repository_full_name":"o/heavy","labels":["self-hosted","e2b","large"]}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected match status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"large"`) {
		t.Fatalf("expected large profile in match response, got %s", rec.Body.String())
	}
}

func TestCreateProfileRejectsInvalidTemplate(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{templateErr: sandboxrunner.ErrTemplateNotFound})

	profileBody := bytes.NewBufferString(`{"name":"large","labels":["self-hosted","e2b","large"],"template_id":"missing-template","max_concurrency":5,"enabled":true}`)
	req := adminRequest(http.MethodPost, "/runner_specs", profileBody)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := store.GetProfile("large"); err == nil {
		t.Fatal("profile with invalid template should not be persisted")
	}
}

func TestPatchProfileRejectsInvalidTemplate(t *testing.T) {
	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, "http://example.test", fake)

	profileBody := bytes.NewBufferString(`{"name":"large","labels":["self-hosted","e2b","large"],"template_id":"valid-template","max_concurrency":5,"enabled":true}`)
	req := adminRequest(http.MethodPost, "/runner_specs", profileBody)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected create profile status: %d body=%s", rec.Code, rec.Body.String())
	}

	fake.templateErr = sandboxrunner.ErrTemplateNotReady
	req = adminRequest(http.MethodPatch, "/runner_specs/large", bytes.NewBufferString(`{"template_id":"not-ready-template"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d body=%s", rec.Code, rec.Body.String())
	}
	profile, err := store.GetProfile("large")
	if err != nil {
		t.Fatal(err)
	}
	if profile.TemplateID != "valid-template" {
		t.Fatalf("invalid template update should not be persisted: %#v", profile)
	}
}

func TestPatchProfilePreservesAndAllowsZeroSchedulingFields(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	profileBody := bytes.NewBufferString(`{"name":"large","labels":["self-hosted","e2b","large"],"template_id":"valid-template","max_concurrency":5,"min_idle":2,"priority":10,"enabled":true}`)
	req := adminRequest(http.MethodPost, "/runner_specs", profileBody)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected create profile status: %d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodPatch, "/runner_specs/large", bytes.NewBufferString(`{"runner_group":"gpu"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected patch profile status: %d body=%s", rec.Code, rec.Body.String())
	}
	profile, err := store.GetProfile("large")
	if err != nil {
		t.Fatal(err)
	}
	if profile.MinIdle != 2 || profile.Priority != 10 {
		t.Fatalf("partial patch reset scheduling fields: %#v", profile)
	}

	req = adminRequest(http.MethodPatch, "/runner_specs/large", bytes.NewBufferString(`{"min_idle":0,"priority":0}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected zero patch profile status: %d body=%s", rec.Code, rec.Body.String())
	}
	profile, err = store.GetProfile("large")
	if err != nil {
		t.Fatal(err)
	}
	if profile.MinIdle != 0 || profile.Priority != 0 {
		t.Fatalf("explicit zero scheduling fields were not applied: %#v", profile)
	}
}

func TestRunnerGroupAndRepositoryPolicyEndpoints(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	profileBody := bytes.NewBufferString(`{"name":"large","labels":["self-hosted","e2b","large"],"template_id":"large","max_concurrency":5,"enabled":true}`)
	req := adminRequest(http.MethodPost, "/runner_specs", profileBody)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected create profile status: %d body=%s", rec.Code, rec.Body.String())
	}

	groupBody := bytes.NewBufferString(`{"name":"ubuntu-2404","spec_names":["large"],"enabled":true}`)
	req = adminRequest(http.MethodPost, "/runner_groups", groupBody)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected create group status: %d body=%s", rec.Code, rec.Body.String())
	}

	policyBody := bytes.NewBufferString(`{"repository_full_name":"o/heavy","runner_group_name":"ubuntu-2404","enabled":true}`)
	req = adminRequest(http.MethodPost, "/runner_policies", policyBody)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected create group policy status: %d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodPost, "/runner_specs/match", bytes.NewBufferString(`{"repository_full_name":"o/heavy","labels":["self-hosted","e2b","large"]}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected match status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"large"`) {
		t.Fatalf("expected large profile through group policy, got %s", rec.Body.String())
	}
}

func TestManualExplicitProfileMustRespectRepositoryPolicy(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	profileBody := bytes.NewBufferString(`{"name":"large","labels":["self-hosted","e2b","large"],"template_id":"large","runner_group":"large","max_concurrency":5,"enabled":true}`)
	req := adminRequest(http.MethodPost, "/runner_specs", profileBody)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected create profile status: %d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodPost, "/runner_requests", bytes.NewBufferString(`{"id":"manual-large","repository_full_name":"o/blocked","runner_spec_name":"large"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected policy rejection, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRetryRunnerRequeuesFailedRequestAndWritesAuditEvent(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "retry-me",
		Source:             "test",
		RepositoryFullName: "o/r",
		RequestedLabels:    []string{"self-hosted", "e2b"},
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerGroup:        "default",
		RunnerName:         "e2b-retry-me",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = state.StatusFailed
	st.FailureStage = "sandbox_start"
	st.FailureReason = "backend_server_error"
	st.LastErrorCode = "backend_server_error"
	st.LastErrorMessage = "temporary failure"
	st.LastErrorRetryable = true
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	req := adminRequest(http.MethodPost, "/runner_requests/retry-me/retry", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected retry status: %d body=%s", rec.Code, rec.Body.String())
	}
	srv.Close()

	got, err := store.ReadState("retry-me")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusQueued || !got.NextRetryAt.IsZero() {
		t.Fatalf("expected queued retry state, got %#v", got)
	}
	events, err := store.ListAuditEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 || events[0].Action != "runner.retry" {
		t.Fatalf("expected retry audit event, got %#v", events)
	}
}

func TestRetryRunnerRejectsRunningRequest(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"token":"runner-token","expires_at":"2026-05-18T10:00:00Z"}`))
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServerWithLimit(t, store, ghServer.URL, &fakeSandbox{}, 10)

	req := adminRequest(http.MethodPost, "/runner_requests", bytes.NewBufferString(`{"id":"running-retry","repository_full_name":"o/r","runner_spec_name":"default"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected create status: %d body=%s", rec.Code, rec.Body.String())
	}
	waitForState(t, store, "running-retry", state.StatusRunning)

	req = adminRequest(http.MethodPost, "/runner_requests/running-retry/retry", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected retry conflict, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSweeperMarksTimedOutCreatesFailed(t *testing.T) {
	store := state.New(t.TempDir())
	if _, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "timed-out-create",
		Source:             "test",
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerGroup:        "default",
		RunnerName:         "e2b-timed-out-create",
	}, nil); err != nil {
		t.Fatal(err)
	} else {
		st.Status = state.StatusCreating
		st.CreatingAt = time.Now().Add(-5 * time.Second).UTC()
		if err := store.WriteState(st); err != nil {
			t.Fatal(err)
		}
	}
	srv := newTestServerWithLimit(t, store, "http://example.test", &fakeSandbox{}, 10)
	srv.cfg.SandboxCreateTimeout = time.Second
	srv.sweepOnce(t.Context())
	srv.Close()

	got, err := store.ReadState("timed-out-create")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusQueued || got.RetryCount != 1 || got.NextRetryAt.IsZero() || got.LastErrorRetryable != true {
		t.Fatalf("unexpected swept state: %#v", got)
	}
}

func TestSweeperRespectsStoppingRetryBackoff(t *testing.T) {
	store := state.New(t.TempDir())
	if _, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "stop-backoff",
		Source:             "test",
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerGroup:        "default",
		RunnerName:         "e2b-stop-backoff",
	}, nil); err != nil {
		t.Fatal(err)
	} else {
		st.Status = state.StatusStopping
		st.SandboxID = "sb-stop-backoff"
		st.ProcessPID = 42
		st.StoppingAt = time.Now().Add(-10 * time.Second).UTC()
		st.NextRetryAt = time.Now().Add(time.Minute).UTC()
		if err := store.WriteState(st); err != nil {
			t.Fatal(err)
		}
	}
	fake := &fakeSandbox{}
	srv := newTestServerWithLimit(t, store, "http://example.test", fake, 10)
	srv.cfg.SandboxStopTimeout = time.Second
	srv.sweepOnce(t.Context())
	if fake.stoppedCount() != 0 {
		t.Fatalf("expected sweeper to respect next_retry_at, got %d stops", fake.stoppedCount())
	}
}

func TestSweeperStopsIdleRunnerThatNeverAcceptedJob(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && (r.URL.Path == "/repos/o/r/actions/runners" || r.URL.Path == "/orgs/o/actions/runners") {
			w.Write([]byte(`{"runners":[]}`))
			return
		}
		t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	if _, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "idle-runner",
		Source:             "test",
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerGroup:        "default",
		RunnerName:         "e2b-idle-runner",
	}, nil); err != nil {
		t.Fatal(err)
	} else {
		st.Status = state.StatusRunning
		st.SandboxID = "sb-idle-runner"
		st.ProcessPID = 42
		st.RunningAt = time.Now().Add(-10 * time.Minute).UTC()
		if err := store.WriteState(st); err != nil {
			t.Fatal(err)
		}
	}
	fake := &fakeSandbox{}
	srv := newTestServerWithLimit(t, store, ghServer.URL, fake, 10)
	srv.cfg.RunnerIdleTimeout = time.Minute
	srv.sweepOnce(t.Context())
	if fake.stoppedCount() != 1 {
		t.Fatalf("expected sweeper to stop idle runner, got %d stops", fake.stoppedCount())
	}
	got, err := store.ReadState("idle-runner")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusCompleted {
		t.Fatalf("expected completed idle runner, got %s", got.Status)
	}
}

func TestSweeperKeepsRunnerAfterJobStarted(t *testing.T) {
	store := state.New(t.TempDir())
	if _, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "busy-runner",
		Source:             "test",
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerGroup:        "default",
		RunnerName:         "e2b-busy-runner",
	}, nil); err != nil {
		t.Fatal(err)
	} else {
		st.Status = state.StatusRunning
		st.SandboxID = "sb-busy-runner"
		st.ProcessPID = 42
		st.RunningAt = time.Now().Add(-10 * time.Minute).UTC()
		st.AssignedJobName = runnerJobStartedMarker
		if err := store.WriteState(st); err != nil {
			t.Fatal(err)
		}
	}
	fake := &fakeSandbox{}
	srv := newTestServerWithLimit(t, store, "http://example.test", fake, 10)
	srv.cfg.RunnerIdleTimeout = time.Minute
	srv.sweepOnce(t.Context())
	if fake.stoppedCount() != 0 {
		t.Fatalf("expected sweeper to keep runner with started job, got %d stops", fake.stoppedCount())
	}
}

func TestProfileConcurrencyLimitAppliesBeforeCreate(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"token":"runner-token","expires_at":"2026-05-18T10:00:00Z"}`))
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServerWithLimit(t, store, ghServer.URL, fake, 10)
	if _, err := store.UpsertProfile(state.RunnerProfile{
		Name:           "default",
		Labels:         []string{"self-hosted", "e2b"},
		TemplateID:     "base",
		RunnerGroup:    "default",
		MaxConcurrency: 1,
		Enabled:        true,
	}); err != nil {
		t.Fatal(err)
	}

	req := adminRequest(http.MethodPost, "/runner_requests", bytes.NewBufferString(`{"id":"profile-1","repository_full_name":"o/r","runner_spec_name":"default"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected first create status: %d body=%s", rec.Code, rec.Body.String())
	}
	waitForState(t, store, "profile-1", state.StatusRunning)

	req = adminRequest(http.MethodPost, "/runner_requests", bytes.NewBufferString(`{"id":"profile-2","repository_full_name":"o/r","runner_spec_name":"default"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected profile concurrency queueing, got %d body=%s", rec.Code, rec.Body.String())
	}
	got, err := store.ReadState("profile-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusQueued {
		t.Fatalf("expected second profile request to stay queued, got %#v", got)
	}
	if fake.startedCount() != 1 {
		t.Fatalf("expected only first profile runner to start, got %d starts", fake.startedCount())
	}
}

func newTestServer(t *testing.T, store state.Store, ghURL string, fake *fakeSandbox) *Server {
	t.Helper()
	return newTestServerWithLimit(t, store, ghURL, fake, 10)
}

func newTestServerWithLimit(t *testing.T, store state.Store, ghURL string, fake *fakeSandbox, limit int) *Server {
	t.Helper()
	cfg := config.Config{
		StateDir:                "./var/runner_requests",
		StateBackend:            "sqlite",
		StateDatabaseDSN:        "./var/runnerd.db",
		GitHubWebhookSecret:     "secret",
		GitHubOAuthClientID:     "Iv1.test",
		GitHubOAuthClientSecret: "oauth-secret",
		AuthSessionSecret:       "session-secret",
		AuthSessionTTL:          time.Hour,
		SandboxTimeout:          time.Hour,
		SandboxCreateTimeout:    time.Second,
		SandboxStopTimeout:      time.Second,
		RunnerIdleTimeout:       5 * time.Minute,
		WorkerLeaseTTL:          5 * time.Second,
		RetryBaseDelay:          50 * time.Millisecond,
		RetryMaxDelay:           time.Second,
		RetryMaxAttempts:        3,
		MaxConcurrentRunners:    limit,
		GitHubAPIBaseURL:        ghURL,
	}
	if _, err := store.UpsertProfile(state.RunnerProfile{
		Name:           "default",
		Labels:         []string{"self-hosted", "e2b"},
		TemplateID:     "base",
		MaxConcurrency: limit,
		Enabled:        true,
	}); err != nil {
		panic(err)
	}
	if _, err := store.UpsertRepositoryPolicy(state.RepositoryPolicy{
		RepositoryFullName: "o/r",
		ProfileName:        "default",
		Enabled:            true,
	}); err != nil {
		panic(err)
	}
	if _, _, err := store.UpsertAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "octocat"}, "admin"); err != nil {
		panic(err)
	}
	if _, _, err := store.UpsertAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "hubot-id", OAuthLogin: "hubot"}, "user"); err != nil {
		panic(err)
	}
	gh := github.NewClient(ghURL, http.DefaultClient)
	srv := New(cfg, store, gh, fake, nil)
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

func adminRequest(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	req.AddCookie(testSessionCookie("12345", "octocat", "admin"))
	return req
}

func testSessionCookie(subject, login, role string) *http.Cookie {
	payload, _ := json.Marshal(adminSession{
		Subject:   subject,
		Login:     login,
		Role:      role,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	payloadValue := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte("session-secret"))
	_, _ = mac.Write([]byte(payloadValue))
	return &http.Cookie{
		Name:  adminSessionCookieName,
		Value: payloadValue + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)),
		Path:  "/",
	}
}

func testServerPrivateKeyFile(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	path := filepath.Join(t.TempDir(), "github-app.pem")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func waitForState(t *testing.T, store state.Store, id, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st, err := store.ReadState(id)
		if err == nil && st.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	st, _ := store.ReadState(id)
	data, _ := json.Marshal(st)
	t.Fatalf("state %s did not become %s: %s", id, want, data)
}

func waitForSandboxStarts(t *testing.T, fake *fakeSandbox, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fake.startedCount() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("sandbox starts did not reach %d, got %d", want, fake.startedCount())
}

func waitForStops(t *testing.T, fake *fakeSandbox, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fake.stoppedCount() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("sandbox stops did not reach %d, got %d", want, fake.stoppedCount())
}

type hijackableResponseWriter struct {
	header   http.Header
	client   net.Conn
	server   net.Conn
	flushed  bool
	hijacked bool
}

func newHijackableResponseWriter() *hijackableResponseWriter {
	client, server := net.Pipe()
	return &hijackableResponseWriter{
		header: http.Header{},
		client: client,
		server: server,
	}
}

func (w *hijackableResponseWriter) Header() http.Header {
	return w.header
}

func (w *hijackableResponseWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

func (w *hijackableResponseWriter) WriteHeader(statusCode int) {}

func (w *hijackableResponseWriter) Flush() {
	w.flushed = true
}

func (w *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacked = true
	rw := bufio.NewReadWriter(bufio.NewReader(w.server), bufio.NewWriter(w.server))
	return w.server, rw, nil
}

func (w *hijackableResponseWriter) close() {
	_ = w.client.Close()
	_ = w.server.Close()
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func githubRunnerAPI(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/repos/o/r/actions/jobs/"):
			id := strings.TrimPrefix(r.URL.Path, "/repos/o/r/actions/jobs/")
			w.Write([]byte(`{"id":` + id + `,"name":"test","status":"queued","labels":["self-hosted","e2b"]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/actions/runners/registration-token":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"token":"runner-token","expires_at":"2026-05-18T10:00:00Z"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/actions/runners":
			w.Write([]byte(`{"runners":[]}`))
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/repos/o/r/actions/runners/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
	}
}
