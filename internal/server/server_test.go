package server

import (
	"archive/zip"
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
	"errors"
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
	terminals      int
	startBlock     chan struct{}
	stopErr        error
	commandContext context.Context
	repositoryURL  string
	runnerGroup    string
	terminal       *fakeTerminalSession
}

type blockingSandboxDefaultStore struct {
	state.Store
	getStarted  chan struct{}
	continueGet chan struct{}
	blockOnce   sync.Once
}

func (s *blockingSandboxDefaultStore) GetSandboxServiceDefault() (state.SandboxServiceDefault, error) {
	defaultConfig, err := s.Store.GetSandboxServiceDefault()
	s.blockOnce.Do(func() {
		close(s.getStarted)
		<-s.continueGet
	})
	return defaultConfig, err
}

func (f *fakeSandbox) ValidateTemplate(ctx context.Context, templateID string) error {
	return nil
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

func (f *fakeSandbox) StartTerminal(ctx context.Context, sandboxID string, size sandboxrunner.PtySize, onData func([]byte)) (sandboxrunner.TerminalSession, error) {
	f.mu.Lock()
	f.terminals++
	terminal := &fakeTerminalSession{pid: 99}
	f.terminal = terminal
	f.mu.Unlock()
	if onData != nil {
		onData([]byte("terminal ready\n"))
	}
	return terminal, nil
}

type fakeTerminalSession struct {
	mu      sync.Mutex
	pid     uint32
	input   string
	closed  bool
	resized sandboxrunner.PtySize
}

func (f *fakeTerminalSession) PID() uint32 {
	return f.pid
}

func (f *fakeTerminalSession) SendInput(ctx context.Context, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.input += string(data)
	return nil
}

func (f *fakeTerminalSession) Resize(ctx context.Context, size sandboxrunner.PtySize) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resized = size
	return nil
}

func (f *fakeTerminalSession) Close(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeTerminalSession) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func TestTerminalHubClosesIdleSessionAfterDisconnect(t *testing.T) {
	hub := newTerminalHub(nil)
	terminal := &fakeTerminalSession{pid: 99}
	session := hub.Add("request-1", "sandbox-1", terminal)
	_, _, cancel := session.Subscribe()

	hub.CloseSessionWhenIdle(session.id, 10*time.Millisecond)
	time.Sleep(25 * time.Millisecond)
	if terminal.isClosed() {
		t.Fatal("expected terminal to stay open while a watcher is subscribed")
	}
	if _, ok := hub.Get(session.id); !ok {
		t.Fatal("expected terminal session to stay in the hub while a watcher is subscribed")
	}

	cancel()
	hub.CloseSessionWhenIdle(session.id, 0)
	deadline := time.Now().Add(time.Second)
	for !terminal.isClosed() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !terminal.isClosed() {
		t.Fatal("expected idle terminal session to close")
	}
	if _, ok := hub.Get(session.id); ok {
		t.Fatal("expected idle terminal session to be removed from the hub")
	}
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
		case r.Method == http.MethodGet && r.URL.Path == "/user/installations/987/repositories":
			if r.Header.Get("Authorization") != "Bearer user-token" {
				t.Fatalf("expected user token authorization, got %q", r.Header.Get("Authorization"))
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
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "user-token")
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

func TestUserRunnerAuthorizationUsesRepositoryIntersection(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user/installations/987/repositories" {
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer user-token" {
			t.Fatalf("expected user token authorization, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"repositories":[{"full_name":"o/visible"}]}`))
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "user-token")
	for _, request := range []state.RunnerRequest{
		{
			ID:                   "visible-job",
			Source:               "test",
			GitHubInstallationID: 987,
			RepositoryFullName:   "o/visible",
			Labels:               []string{"self-hosted"},
		},
		{
			ID:                   "hidden-job",
			Source:               "test",
			GitHubInstallationID: 987,
			RepositoryFullName:   "o/hidden",
			Labels:               []string{"self-hosted"},
		},
		{
			ID:                   "cross-installation-job",
			Source:               "test",
			GitHubInstallationID: 456,
			RepositoryFullName:   "o/visible",
			Labels:               []string{"self-hosted"},
		},
	} {
		_, runnerState, err := store.CreateRequest(request, nil)
		if err != nil {
			t.Fatal(err)
		}
		if request.ID == "hidden-job" {
			runnerState.Status = state.StatusRunning
			runnerState.SandboxID = "hidden-sandbox"
			runnerState.WorkflowRunID = 42
			runnerState.HeadBranch = "secret"
			runnerState.HeadSHA = "def"
			runnerState.PullRequestNumber = 7
		} else {
			runnerState.Status = state.StatusCompleted
		}
		if err := store.WriteState(runnerState); err != nil {
			t.Fatal(err)
		}
	}
	store.AppendLog("hidden-job", "control.log", []byte("secret runner output\n"))

	req := httptest.NewRequest(http.MethodGet, "/user/runner_requests", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected runner list status: %d body=%s", rec.Code, rec.Body.String())
	}
	var runners []state.RunnerState
	if err := json.Unmarshal(rec.Body.Bytes(), &runners); err != nil {
		t.Fatal(err)
	}
	if len(runners) != 1 || runners[0].ID != "visible-job" {
		t.Fatalf("expected only repository-authorized job, got %#v", runners)
	}
	if rec.Header().Get("X-Total-Count") != "1" || rec.Header().Get("X-Limit") != "100" || rec.Header().Get("X-Offset") != "0" {
		t.Fatalf("unexpected default user runner pagination headers: %#v", rec.Header())
	}

	deniedRequests := []struct {
		method string
		target string
		body   io.Reader
	}{
		{method: http.MethodGet, target: "/user/runner_requests/hidden-job"},
		{method: http.MethodGet, target: "/user/runner_requests/hidden-job/group"},
		{method: http.MethodGet, target: "/user/runner_requests/hidden-job/siblings"},
		{method: http.MethodGet, target: "/user/runner_requests/hidden-job/logs/control.log"},
		{method: http.MethodGet, target: "/user/runner_requests/hidden-job/github-log"},
		{method: http.MethodPost, target: "/user/runner_requests/hidden-job/terminal", body: strings.NewReader(`{"cols":120,"rows":32}`)},
		{method: http.MethodGet, target: "/user/github/pulls/o/hidden/7/jobs"},
		{method: http.MethodGet, target: "/user/github/runs/o/hidden/42/jobs"},
		{method: http.MethodGet, target: "/user/github/branches/o/hidden/def/jobs?branch=secret"},
		{method: http.MethodGet, target: "/user/runner_requests/cross-installation-job"},
	}
	for _, denied := range deniedRequests {
		req = httptest.NewRequest(denied.method, denied.target, denied.body)
		req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected %s %s to be not found, got %d body=%s", denied.method, denied.target, rec.Code, rec.Body.String())
		}
	}
	if fake.terminals != 0 {
		t.Fatalf("expected unauthorized terminal request not to start a terminal, got %d", fake.terminals)
	}
}

func TestUserRunnerListPagination(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user/installations/987/repositories" {
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"repositories":[{"full_name":"o/visible"}]}`))
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "user-token")
	now := time.Now().UTC()
	for index, id := range []string{"oldest", "middle", "newest"} {
		if _, _, err := store.CreateRequest(state.RunnerRequest{
			ID:                   id,
			Source:               "test",
			GitHubInstallationID: 987,
			RepositoryFullName:   "o/visible",
			CreatedAt:            now.Add(time.Duration(index) * time.Minute),
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/user/runner_requests?limit=1&offset=1", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected paginated runner list status: %d body=%s", rec.Code, rec.Body.String())
	}
	var runners []state.RunnerState
	if err := json.Unmarshal(rec.Body.Bytes(), &runners); err != nil {
		t.Fatal(err)
	}
	if len(runners) != 1 || runners[0].ID != "middle" {
		t.Fatalf("unexpected paginated user runners: %#v", runners)
	}
	if rec.Header().Get("X-Total-Count") != "3" || rec.Header().Get("X-Limit") != "1" || rec.Header().Get("X-Offset") != "1" {
		t.Fatalf("unexpected pagination headers: %#v", rec.Header())
	}
	link := rec.Header().Get("Link")
	if !strings.Contains(link, "</user/runner_requests?limit=1&offset=0>; rel=\"prev\"") ||
		!strings.Contains(link, "</user/runner_requests?limit=1&offset=2>; rel=\"next\"") {
		t.Fatalf("unexpected pagination link header: %q", link)
	}

	outOfWindowReq := httptest.NewRequest(http.MethodGet, "/user/runner_requests?limit=500&offset=1", nil)
	outOfWindowReq.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	outOfWindowRec := httptest.NewRecorder()
	srv.ServeHTTP(outOfWindowRec, outOfWindowReq)
	if outOfWindowRec.Code != http.StatusBadRequest || !strings.Contains(outOfWindowRec.Body.String(), "history window") {
		t.Fatalf("expected bounded history-window error, got status=%d body=%s", outOfWindowRec.Code, outOfWindowRec.Body.String())
	}
}

func TestPaginationHeadersStopAtHistoryWindow(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/user/runner_requests?limit=500&offset=0", nil)
	rec := httptest.NewRecorder()
	writePaginationHeadersWithinWindow(rec, req, 501, 500, 0, 500)
	if rec.Header().Get("X-Total-Count") != "501" {
		t.Fatalf("unexpected total count: %q", rec.Header().Get("X-Total-Count"))
	}
	if strings.Contains(rec.Header().Get("Link"), "rel=\"next\"") {
		t.Fatalf("history-window response exposed an unusable next link: %q", rec.Header().Get("Link"))
	}
}

func TestUserRunnerAuthorizationRequiresGitHubOAuthToken(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "https://github.example", &fakeSandbox{})
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CreateRequest(state.RunnerRequest{
		ID:                   "private-job",
		Source:               "test",
		GitHubInstallationID: 987,
		RepositoryFullName:   "o/private",
	}, nil); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/user/runner_requests", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"REAUTH_REQUIRED"`) {
		t.Fatalf("expected missing token to require reauthentication, got %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "private-job") || strings.Contains(rec.Body.String(), "o/private") {
		t.Fatalf("missing-token response leaked runner data: %s", rec.Body.String())
	}
}

func TestUserRunnerAuthorizationRequiresReauthenticationForUnreadableGitHubOAuthToken(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "https://github.example", &fakeSandbox{})
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountSecret(state.AccountSecret{
		ScopeType:      state.AccountScopeTypeAccount,
		ScopeID:        account.ID,
		KeyType:        state.AccountSecretTypeGitHubOAuthToken,
		EncryptedValue: "v1:unreadable",
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/user/runner_requests", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"REAUTH_REQUIRED"`) {
		t.Fatalf("expected unreadable token to require reauthentication, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUserRunnerAuthorizationRejectsInvalidGitHubOAuthToken(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user/installations/987/repositories" {
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "expired-user-token")
	if _, _, err := store.CreateRequest(state.RunnerRequest{
		ID:                   "private-job",
		Source:               "test",
		GitHubInstallationID: 987,
		RepositoryFullName:   "o/private",
	}, nil); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/user/runner_requests", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"REAUTH_REQUIRED"`) {
		t.Fatalf("expected rejected token to require reauthentication, got %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "private-job") || strings.Contains(rec.Body.String(), "o/private") {
		t.Fatalf("rejected-token response leaked runner data: %s", rec.Body.String())
	}
}

func TestUserRunnerAuthorizationDoesNotReauthenticateOnGitHubRateLimit(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user/installations/987/repositories" {
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Retry-After", "60")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "user-token")
	if _, _, err := store.CreateRequest(state.RunnerRequest{
		ID:                   "private-job",
		Source:               "test",
		GitHubInstallationID: 987,
		RepositoryFullName:   "o/private",
	}, nil); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/user/runner_requests", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected rate limit to be reported as unavailable, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("expected retry-after response header, got %q", rec.Header().Get("Retry-After"))
	}
	if strings.Contains(rec.Body.String(), `"code":"REAUTH_REQUIRED"`) {
		t.Fatalf("rate limit must not trigger GitHub reauthentication: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "private-job") || strings.Contains(rec.Body.String(), "o/private") {
		t.Fatalf("rate-limit response leaked runner data: %s", rec.Body.String())
	}
}

func TestUserRunnerAuthorizationSkipsInaccessibleInstallation(t *testing.T) {
	for _, inaccessibleStatus := range []int{http.StatusNotFound, http.StatusForbidden} {
		t.Run(http.StatusText(inaccessibleStatus), func(t *testing.T) {
			ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/user/installations/987/repositories":
					w.WriteHeader(inaccessibleStatus)
					w.Write([]byte(`{"message":"installation is not accessible"}`))
				case "/user/installations/654/repositories":
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"repositories":[{"full_name":"other/visible"}]}`))
				default:
					t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
				}
			}))
			defer ghServer.Close()

			store := state.New(t.TempDir())
			srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
			account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
			if err != nil {
				t.Fatal(err)
			}
			for _, installation := range []state.GitHubInstallation{
				{AccountID: account.ID, InstallationID: 987, AccountLogin: "o"},
				{AccountID: account.ID, InstallationID: 654, AccountLogin: "other"},
			} {
				if _, err := store.UpsertGitHubInstallation(installation); err != nil {
					t.Fatal(err)
				}
			}
			saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "user-token")
			for _, request := range []state.RunnerRequest{
				{
					ID:                   "inaccessible-job",
					Source:               "test",
					GitHubInstallationID: 987,
					RepositoryFullName:   "o/private",
				},
				{
					ID:                   "accessible-job",
					Source:               "test",
					GitHubInstallationID: 654,
					RepositoryFullName:   "other/visible",
				},
			} {
				if _, _, err := store.CreateRequest(request, nil); err != nil {
					t.Fatal(err)
				}
			}

			req := httptest.NewRequest(http.MethodGet, "/user/runner_requests", nil)
			req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected inaccessible installation to be skipped, got %d body=%s", rec.Code, rec.Body.String())
			}
			var runners []state.RunnerState
			if err := json.Unmarshal(rec.Body.Bytes(), &runners); err != nil {
				t.Fatal(err)
			}
			if len(runners) != 1 || runners[0].ID != "accessible-job" {
				t.Fatalf("expected only job from accessible installation, got %#v", runners)
			}
		})
	}
}

func TestUserRunnerAuthorizationCachesNoInstallations(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "https://github.example", &fakeSandbox{})
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}

	access, err := srv.userAuthorizedRepositoryAccess(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(access) != 0 {
		t.Fatalf("expected no repository access, got %#v", access)
	}
	cached, ok := srv.cachedUserRepositoryAccess(account.ID)
	if !ok || len(cached) != 0 {
		t.Fatalf("expected empty repository access to be cached, cached=%#v ok=%v", cached, ok)
	}
}

func TestUserRunnerAuthorizationCachesRepositoryAccess(t *testing.T) {
	var permissionRequests int
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user/installations/987/repositories" {
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
		permissionRequests++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"repositories":[{"full_name":"o/r"}]}`))
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "user-token")
	if _, _, err := store.CreateRequest(state.RunnerRequest{
		ID:                   "cached-job",
		Source:               "test",
		GitHubInstallationID: 987,
		RepositoryFullName:   "o/r",
	}, nil); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "/user/runner_requests/cached-job", nil)
		req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected cached job status: %d body=%s", rec.Code, rec.Body.String())
		}
	}
	if permissionRequests != 1 {
		t.Fatalf("expected one GitHub permission request, got %d", permissionRequests)
	}
}

func TestUserRunnerAuthorizationRefreshesValidCacheInBackground(t *testing.T) {
	var (
		requestMu       sync.Mutex
		permissionCalls int
	)
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user/installations/987/repositories" {
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
		requestMu.Lock()
		permissionCalls++
		call := permissionCalls
		requestMu.Unlock()
		if call == 2 {
			close(refreshStarted)
			select {
			case <-releaseRefresh:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			w.Write([]byte(`{"repositories":[{"full_name":"o/old"}]}`))
			return
		}
		w.Write([]byte(`{"repositories":[{"full_name":"o/new"}]}`))
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	defer srv.Close()
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "user-token")

	access, err := srv.userAuthorizedRepositoryAccess(t.Context(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRepositoryAccess(access, 987, "o/old") {
		t.Fatalf("unexpected initial access: %#v", access)
	}
	srv.userRepositoryAccessMu.Lock()
	cached := srv.userRepositoryAccessCache[account.ID]
	cached.refreshAfter = time.Now().Add(-time.Second)
	cached.expiresAt = time.Now().Add(time.Minute)
	srv.userRepositoryAccessCache[account.ID] = cached
	srv.userRepositoryAccessMu.Unlock()

	for range 2 {
		access, err = srv.userAuthorizedRepositoryAccess(t.Context(), account.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !hasRepositoryAccess(access, 987, "o/old") {
			t.Fatalf("valid cache must be returned while refresh runs: %#v", access)
		}
	}
	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("background permission refresh did not start")
	}
	requestMu.Lock()
	callsWhileBlocked := permissionCalls
	requestMu.Unlock()
	if callsWhileBlocked != 2 {
		t.Fatalf("expected one background refresh, got %d total permission calls", callsWhileBlocked)
	}
	close(releaseRefresh)

	deadline := time.Now().Add(time.Second)
	for {
		access, ok := srv.cachedUserRepositoryAccess(account.ID)
		if ok && hasRepositoryAccess(access, 987, "o/new") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background refresh did not replace cached access: %#v", access)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestUserRunnerAuthorizationBackgroundRefreshInvalidatesRejectedCredentials(t *testing.T) {
	var calls int
	rejected := make(chan struct{})
	var rejectOnce sync.Once
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"repositories":[{"full_name":"o/r"}]}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Bad credentials"}`))
		rejectOnce.Do(func() { close(rejected) })
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	defer srv.Close()
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "user-token")
	if _, err := srv.userAuthorizedRepositoryAccess(t.Context(), account.ID); err != nil {
		t.Fatal(err)
	}
	srv.userRepositoryAccessMu.Lock()
	cached := srv.userRepositoryAccessCache[account.ID]
	cached.refreshAfter = time.Now().Add(-time.Second)
	srv.userRepositoryAccessCache[account.ID] = cached
	srv.userRepositoryAccessMu.Unlock()

	access, err := srv.userAuthorizedRepositoryAccess(t.Context(), account.ID)
	if err != nil || !hasRepositoryAccess(access, 987, "o/r") {
		t.Fatalf("valid cache should be returned before rejection completes: access=%#v err=%v", access, err)
	}
	select {
	case <-rejected:
	case <-time.After(time.Second):
		t.Fatal("background rejected-credential refresh did not finish")
	}
	deadline := time.Now().Add(time.Second)
	for {
		if _, ok := srv.cachedUserRepositoryAccess(account.ID); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("rejected credentials did not invalidate repository access cache")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestUserRunnerAuthorizationInvalidationSupersedesInFlightRefresh(t *testing.T) {
	var (
		requestMu sync.Mutex
		calls     int
		release   sync.Once
	)
	refreshStarted := make(chan struct{})
	supersedingRefreshStarted := make(chan struct{})
	releaseStaleRefresh := make(chan struct{})
	releaseRefresh := func() { release.Do(func() { close(releaseStaleRefresh) }) }
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user/installations/987/repositories" {
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
		requestMu.Lock()
		calls++
		call := calls
		requestMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch call {
		case 1:
			w.Write([]byte(`{"repositories":[{"full_name":"o/old"}]}`))
		case 2:
			close(refreshStarted)
			<-releaseStaleRefresh
			w.Write([]byte(`{"repositories":[{"full_name":"o/stale"}]}`))
		case 3:
			close(supersedingRefreshStarted)
			w.Write([]byte(`{"repositories":[{"full_name":"o/new"}]}`))
		default:
			t.Fatalf("unexpected github permission request %d", call)
		}
	}))
	defer ghServer.Close()
	defer releaseRefresh()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	defer srv.Close()
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "user-token")
	if _, err := srv.userAuthorizedRepositoryAccess(t.Context(), account.ID); err != nil {
		t.Fatal(err)
	}
	staleResult := make(chan error, 1)
	go func() {
		_, err := srv.refreshUserRepositoryAccess(t.Context(), account.ID)
		staleResult <- err
	}()
	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("stale background refresh did not start")
	}

	srv.invalidateUserRepositoryAccess(account.ID)
	type accessResult struct {
		access []state.GitHubInstallationRepositoryAccess
		err    error
	}
	result := make(chan accessResult, 1)
	go func() {
		access, err := srv.userAuthorizedRepositoryAccess(t.Context(), account.ID)
		result <- accessResult{access: access, err: err}
	}()
	select {
	case <-supersedingRefreshStarted:
	case <-time.After(time.Second):
		t.Fatal("cache invalidation did not start a superseding permission refresh")
	}
	completed := <-result
	if completed.err != nil || !hasRepositoryAccess(completed.access, 987, "o/new") {
		t.Fatalf("unexpected superseding access: access=%#v err=%v", completed.access, completed.err)
	}
	releaseRefresh()

	deadline := time.Now().Add(time.Second)
	for {
		access, ok := srv.cachedUserRepositoryAccess(account.ID)
		if ok && hasRepositoryAccess(access, 987, "o/new") && !hasRepositoryAccess(access, 987, "o/stale") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stale refresh replaced superseding cache entry: %#v", access)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestUserRunnerAuthorizationRejectedStaleRefreshDoesNotInvalidateNewEpoch(t *testing.T) {
	var (
		requestMu sync.Mutex
		calls     int
		release   sync.Once
	)
	refreshStarted := make(chan struct{})
	supersedingRefreshStarted := make(chan struct{})
	releaseStaleRefresh := make(chan struct{})
	releaseRefresh := func() { release.Do(func() { close(releaseStaleRefresh) }) }
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user/installations/987/repositories" {
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
		requestMu.Lock()
		calls++
		call := calls
		requestMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch call {
		case 1:
			w.Write([]byte(`{"repositories":[{"full_name":"o/old"}]}`))
		case 2:
			close(refreshStarted)
			<-releaseStaleRefresh
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Bad credentials"}`))
		case 3:
			close(supersedingRefreshStarted)
			w.Write([]byte(`{"repositories":[{"full_name":"o/new"}]}`))
		default:
			t.Fatalf("unexpected github permission request %d", call)
		}
	}))
	defer ghServer.Close()
	defer releaseRefresh()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "user-token")
	if _, err := srv.userAuthorizedRepositoryAccess(t.Context(), account.ID); err != nil {
		t.Fatal(err)
	}
	staleResult := make(chan error, 1)
	go func() {
		_, err := srv.refreshUserRepositoryAccess(t.Context(), account.ID)
		staleResult <- err
	}()
	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("stale background refresh did not start")
	}

	srv.invalidateUserRepositoryAccess(account.ID)
	type accessResult struct {
		access []state.GitHubInstallationRepositoryAccess
		err    error
	}
	result := make(chan accessResult, 1)
	go func() {
		access, err := srv.userAuthorizedRepositoryAccess(t.Context(), account.ID)
		result <- accessResult{access: access, err: err}
	}()
	select {
	case <-supersedingRefreshStarted:
	case <-time.After(time.Second):
		t.Fatal("cache invalidation did not start a superseding permission refresh")
	}
	completed := <-result
	if completed.err != nil || !hasRepositoryAccess(completed.access, 987, "o/new") {
		t.Fatalf("unexpected superseding access: access=%#v err=%v", completed.access, completed.err)
	}
	releaseRefresh()
	if err := <-staleResult; err == nil {
		t.Fatal("expected stale refresh rejection")
	}
	access, ok := srv.cachedUserRepositoryAccess(account.ID)
	if !ok || !hasRepositoryAccess(access, 987, "o/new") {
		t.Fatalf("rejected stale refresh invalidated the new epoch cache: %#v", access)
	}
}

func TestUserRunnerAuthorizationSharedRefreshSurvivesCallerCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user/installations/987/repositories" {
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
		close(requestStarted)
		select {
		case <-releaseRequest:
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"repositories":[{"full_name":"o/visible"}]}`))
		case <-r.Context().Done():
			return
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	defer srv.Close()
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "user-token")

	firstCtx, cancelFirst := context.WithCancel(t.Context())
	firstResult := make(chan error, 1)
	go func() {
		_, err := srv.userAuthorizedRepositoryAccess(firstCtx, account.ID)
		firstResult <- err
	}()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("shared permission refresh did not start")
	}
	type accessResult struct {
		access []state.GitHubInstallationRepositoryAccess
		err    error
	}
	secondResult := make(chan accessResult, 1)
	go func() {
		access, err := srv.userAuthorizedRepositoryAccess(t.Context(), account.ID)
		secondResult <- accessResult{access: access, err: err}
	}()
	time.Sleep(20 * time.Millisecond)
	cancelFirst()
	if err := <-firstResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("first caller error = %v, want context canceled", err)
	}
	time.Sleep(20 * time.Millisecond)
	close(releaseRequest)
	completed := <-secondResult
	if completed.err != nil || !hasRepositoryAccess(completed.access, 987, "o/visible") {
		t.Fatalf("shared refresh was canceled with first caller: access=%#v err=%v", completed.access, completed.err)
	}
}

func TestUserRunnerAuthorizationCacheIsBounded(t *testing.T) {
	srv := &Server{userRepositoryAccessCache: map[int64]cachedUserRepositoryAccess{}}
	for accountID := int64(1); accountID <= 513; accountID++ {
		srv.cacheUserRepositoryAccess(accountID, []state.GitHubInstallationRepositoryAccess{
			{InstallationID: accountID, Repositories: []string{"o/r"}},
		})
	}
	if got := len(srv.userRepositoryAccessCache); got > 512 {
		t.Fatalf("expected bounded repository access cache, got %d entries", got)
	}
}

func TestEvictOneUserRepositoryAccess(t *testing.T) {
	cache := map[int64]cachedUserRepositoryAccess{
		1: {},
		2: {},
	}
	evictOneUserRepositoryAccess(cache)
	if len(cache) != 1 {
		t.Fatalf("expected exactly one cache entry to be evicted, got %d", len(cache))
	}
}

func TestUserSyncGitHubInstallationsFromOAuthToken(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user/installations":
			if r.Header.Get("Authorization") != "Bearer user-token" {
				t.Fatalf("expected oauth token authorization, got %q", r.Header.Get("Authorization"))
			}
			w.Write([]byte(`{"installations":[{"id":987,"account":{"login":"octo-org","name":"Octo Org","avatar_url":"https://avatars.example/o.png"}}]}`))
		default:
			t.Fatalf("unexpected github sync request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	encryptedToken, err := encryptSecret("user-token", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountSecret(state.AccountSecret{
		ScopeType:      state.AccountScopeTypeAccount,
		ScopeID:        account.ID,
		KeyType:        state.AccountSecretTypeGitHubOAuthToken,
		EncryptedValue: encryptedToken,
	}); err != nil {
		t.Fatal(err)
	}
	stale, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 654,
		AccountLogin:   "stale-org",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stale.ID <= 0 {
		t.Fatalf("expected stale installation id, got %#v", stale)
	}

	req := httptest.NewRequest(http.MethodPost, "/user/github-app/installations/sync", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected sync status: %d body=%s", rec.Code, rec.Body.String())
	}
	installations, err := store.ListGitHubInstallations(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 1 || installations[0].InstallationID != 987 || installations[0].AccountLogin != "octo-org" {
		t.Fatalf("unexpected synced installations: %#v", installations)
	}
}

func TestUserSyncGitHubInstallationsRequiresOAuthTokenCode(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "https://github.example", &fakeSandbox{})
	if _, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/user/github-app/installations/sync", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected sync status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"REAUTH_REQUIRED"`) {
		t.Fatalf("expected reauth code response, got %s", rec.Body.String())
	}
}

func TestUserSyncGitHubInstallationsRequiresReauthenticationForUnreadableOAuthToken(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "https://github.example", &fakeSandbox{})
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountSecret(state.AccountSecret{
		ScopeType:      state.AccountScopeTypeAccount,
		ScopeID:        account.ID,
		KeyType:        state.AccountSecretTypeGitHubOAuthToken,
		EncryptedValue: "v1:unreadable",
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/user/github-app/installations/sync", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected unreadable token to require reauthentication, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"REAUTH_REQUIRED"`) {
		t.Fatalf("expected reauth code response, got %s", rec.Body.String())
	}
}

func TestUserSyncGitHubInstallationsRejectsInvalidOAuthTokenCode(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user/installations" {
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "expired-user-token")

	req := httptest.NewRequest(http.MethodPost, "/user/github-app/installations/sync", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected rejected token to require reauthentication, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"REAUTH_REQUIRED"`) {
		t.Fatalf("expected reauth code response, got %s", rec.Body.String())
	}
}

func TestUserRunnerDetailLogAndTerminal(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user/installations/987/repositories" {
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer user-token" {
			t.Fatalf("expected user token authorization, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"repositories":[{"full_name":"o/r"}]}`))
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	account, _, err := store.UpsertAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "hubot-id", OAuthLogin: "hubot"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                   "job-detail-1",
		Source:               "github_webhook",
		GitHubInstallationID: 987,
		RepositoryFullName:   "o/r",
		RunnerName:           "e2b-job-detail-1",
		Labels:               []string{"self-hosted", "e2b"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = state.StatusRunning
	st.SandboxID = "sb-job-detail-1"
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}
	store.AppendLog(st.ID, "control.log", []byte("runner request created\n"))
	fake := &fakeSandbox{}
	srv := newTestServer(t, store, ghServer.URL, fake)
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "user-token")

	req := httptest.NewRequest(http.MethodGet, "/user/runner_requests/job-detail-1", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected user runner detail, got %d body=%s", rec.Code, rec.Body.String())
	}
	var got state.RunnerState
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != st.ID || got.SandboxID != st.SandboxID {
		t.Fatalf("unexpected runner detail: %#v", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/user/runner_requests/job-detail-1/logs/control.log", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "runner request created") {
		t.Fatalf("expected user runner log, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/user/runner_requests/job-detail-1/terminal", strings.NewReader(`{"cols":120,"rows":32}`))
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected terminal session, got %d body=%s", rec.Code, rec.Body.String())
	}
	var terminal struct {
		SessionID string `json:"session_id"`
		PID       uint32 `json:"pid"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.SessionID == "" || terminal.PID != 99 {
		t.Fatalf("unexpected terminal response: %#v", terminal)
	}
	if fake.terminals != 1 {
		t.Fatalf("expected one terminal start, got %d", fake.terminals)
	}

	latest, err := store.ReadState(st.ID)
	if err != nil {
		t.Fatal(err)
	}
	latest.Status = state.StatusCompleted
	if err := store.WriteState(latest); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/user/runner_requests/job-detail-1/terminal/"+terminal.SessionID+"/input", strings.NewReader(`{"data":"ls\n"}`))
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected inactive terminal to be rejected, got %d body=%s", rec.Code, rec.Body.String())
	}
	if fake.terminal == nil || !fake.terminal.isClosed() {
		t.Fatalf("expected inactive terminal session to be closed")
	}
}

func TestUserRunnerSiblings(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user/installations/987/repositories":
			if r.Header.Get("Authorization") != "Bearer user-token" {
				t.Fatalf("expected user token authorization, got %q", r.Header.Get("Authorization"))
			}
			w.Write([]byte(`{"repositories":[{"full_name":"o/r"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/1":
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/issues/1":
			w.Write([]byte(`{"number":1,"title":"Add runner smoke coverage"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/2":
			w.Write([]byte(`{"number":2,"title":"Other pull request"}`))
		default:
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	account, _, err := store.UpsertAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "hubot-id", OAuthLogin: "hubot"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	requests := []struct {
		id             string
		jobID          int64
		installationID int64
		repository     string
		runID          int64
		prNumber       int
	}{
		{id: "current", jobID: 1001, installationID: 987, repository: "o/r", runID: 28582191510, prNumber: 1},
		{id: "same-run", jobID: 1002, installationID: 987, repository: "o/r", runID: 28582191510, prNumber: 1},
		{id: "same-pr-other-run", jobID: 1003, installationID: 987, repository: "o/r", runID: 1, prNumber: 1},
		{id: "other-pr", jobID: 1004, installationID: 987, repository: "o/r", runID: 2, prNumber: 2},
		{id: "hidden", jobID: 1005, installationID: 456, repository: "o/r", runID: 28582191510, prNumber: 1},
	}
	for _, item := range requests {
		payload := []byte(fmt.Sprintf(`{"workflow_job":{"id":%d,"run_id":%d,"workflow_name":"CI","run_attempt":1,"pull_requests":[{"number":%d}]}}`, item.jobID, item.runID, item.prNumber))
		if _, _, err := store.CreateRequest(state.RunnerRequest{
			ID:                   item.id,
			Source:               "github_webhook",
			JobID:                item.jobID,
			GitHubInstallationID: item.installationID,
			RepositoryFullName:   item.repository,
			Labels:               []string{"self-hosted"},
		}, payload); err != nil {
			t.Fatal(err)
		}
	}
	branchPayload := []byte(`{"workflow_job":{"id":2001,"run_id":3,"workflow_name":"CI","run_attempt":1,"head_branch":"feature/job-detail-page","head_sha":"def","pull_requests":[]}}`)
	if _, _, err := store.CreateRequest(state.RunnerRequest{
		ID:                   "slash-branch",
		Source:               "github_webhook",
		JobID:                2001,
		GitHubInstallationID: 987,
		RepositoryFullName:   "o/r",
		Labels:               []string{"self-hosted"},
	}, branchPayload); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "user-token")

	req := httptest.NewRequest(http.MethodGet, "/user/runner_requests/current/siblings", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected runner siblings, got %d body=%s", rec.Code, rec.Body.String())
	}
	var got userRunnerSiblingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Group != "pull_request" {
		t.Fatalf("unexpected siblings group: %q", got.Group)
	}
	var ids []string
	for _, job := range got.Jobs {
		ids = append(ids, job.ID)
	}
	if !reflect.DeepEqual(ids, []string{"current", "same-run", "same-pr-other-run"}) {
		t.Fatalf("unexpected sibling ids: %#v", ids)
	}

	req = httptest.NewRequest(http.MethodGet, "/user/runner_requests/current/group", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected runner job group, got %d body=%s", rec.Code, rec.Body.String())
	}
	var group userRunnerJobGroupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &group); err != nil {
		t.Fatal(err)
	}
	if group.Key != "pr:o/r:1" || group.Group != "pull_request" {
		t.Fatalf("unexpected job group key=%q group=%q", group.Key, group.Group)
	}
	if group.PullRequestTitle != "Add runner smoke coverage" {
		t.Fatalf("unexpected pull request title: %q", group.PullRequestTitle)
	}
	if ids := runnerStateIDs(group.CurrentJobs); !reflect.DeepEqual(ids, []string{"same-run", "current"}) {
		t.Fatalf("unexpected current job ids: %#v", ids)
	}
	if ids := runnerStateIDs(group.PreviousJobs); !reflect.DeepEqual(ids, []string{"same-pr-other-run"}) {
		t.Fatalf("unexpected previous job ids: %#v", ids)
	}

	req = httptest.NewRequest(http.MethodGet, "/user/github/pulls/o/r/1/jobs", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected github pull job group, got %d body=%s", rec.Code, rec.Body.String())
	}
	var pullGroup userRunnerJobGroupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &pullGroup); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runnerStateIDs(pullGroup.Jobs), runnerStateIDs(group.Jobs)) {
		t.Fatalf("pull route jobs differ: route=%#v runner=%#v", runnerStateIDs(pullGroup.Jobs), runnerStateIDs(group.Jobs))
	}

	req = httptest.NewRequest(http.MethodGet, "/user/github/branches/o/r/def/jobs?branch=feature%2Fjob-detail-page", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected github slash branch job group, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func runnerStateIDs(states []state.RunnerState) []string {
	ids := make([]string, 0, len(states))
	for _, st := range states {
		ids = append(ids, st.ID)
	}
	return ids
}

func TestUserRunnerGitHubLog(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	file, err := zw.Create("0_Qiniu runner smoke.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("Run tests\nAll green\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user/installations/987/repositories":
			if r.Header.Get("Authorization") != "Bearer user-token" {
				t.Fatalf("expected user token authorization, got %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"repositories":[{"full_name":"o/r"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/actions/jobs/1001/logs":
			w.Header().Set("Content-Type", "application/zip")
			w.Write(archive.Bytes())
		default:
			t.Fatalf("unexpected github log request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	account, _, err := store.UpsertAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "hubot-id", OAuthLogin: "hubot"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "o",
	}); err != nil {
		t.Fatal(err)
	}
	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                   "github-log-1",
		Source:               "github_webhook",
		JobID:                1001,
		GitHubInstallationID: 987,
		RepositoryFullName:   "o/r",
		Labels:               []string{"self-hosted", "e2b"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = state.StatusCompleted
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
	saveTestGitHubOAuthToken(t, store, account.ID, srv.cfg.AuthEncryptionKey.Value(), "user-token")

	req := httptest.NewRequest(http.MethodGet, "/user/runner_requests/github-log-1/github-log", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected github log, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "===== 0_Qiniu runner smoke.txt =====") || !strings.Contains(body, "All green") {
		t.Fatalf("unexpected github log body: %s", body)
	}
}

func TestFormatGitHubActionsLogMarksTruncatedFiles(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	file, err := zw.Create("0_large.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(bytes.Repeat([]byte("x"), (8<<20)+1)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	text, err := formatGitHubActionsLog(archive.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "[runnerd] GitHub log file truncated after 8388608 bytes.") {
		t.Fatalf("expected truncation marker, got suffix %q", text[len(text)-128:])
	}
}

func TestFormatGitHubActionsLogCapsTotalOutput(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for i := 0; i < 3; i++ {
		file, err := zw.Create(fmt.Sprintf("%d_large.txt", i))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(bytes.Repeat([]byte("x"), 9<<20)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	text, err := formatGitHubActionsLog(archive.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "[runnerd] GitHub log output truncated after 16777216 bytes.") {
		t.Fatalf("expected total truncation marker, got suffix %q", text[len(text)-128:])
	}
	if strings.Contains(text, "===== 2_large.txt =====") {
		t.Fatal("expected formatter to stop before appending every log file")
	}
}

func TestFormatGitHubActionsLogCapsPlainTextOutput(t *testing.T) {
	text, err := formatGitHubActionsLog(bytes.Repeat([]byte("x"), maxGitHubLogOutputBytes+1))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "[runnerd] GitHub log output truncated after 16777216 bytes.") {
		t.Fatalf("expected total truncation marker, got suffix %q", text[len(text)-128:])
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

	req = httptest.NewRequest(http.MethodGet, "/auth/github/login?return_to=/account/repositories", nil)
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
	loginCookies := rec.Result().Cookies()
	if returnCookie := testCookie(loginCookies, oauthReturnToCookieName); returnCookie == nil || returnCookie.Value != "/account/repositories" {
		t.Fatalf("expected oauth return_to cookie, got %#v", returnCookie)
	}

	req = httptest.NewRequest(http.MethodGet, "/auth/github/login", nil)
	req.AddCookie(&http.Cookie{Name: oauthReturnToCookieName, Value: "/account/repositories"})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected plain login redirect, got %d body=%s", rec.Code, rec.Body.String())
	}
	clearedReturnCookie := testCookie(rec.Result().Cookies(), oauthReturnToCookieName)
	if clearedReturnCookie == nil || clearedReturnCookie.MaxAge >= 0 {
		t.Fatalf("expected plain login to clear stale return_to cookie, got %#v", clearedReturnCookie)
	}

	req = httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=oauth-code&state="+stateCookie.Value, nil)
	for _, cookie := range loginCookies {
		req.AddCookie(cookie)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected callback redirect, got %d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/account/repositories" {
		t.Fatalf("expected admin callback to redirect to return_to path, got %q", loc)
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
	account, _, err := store.GetAccountByOAuthIdentity("github", "12345")
	if err != nil {
		t.Fatal(err)
	}
	savedToken, err := store.GetAccountSecret(state.AccountScopeTypeAccount, account.ID, state.AccountSecretTypeGitHubOAuthToken)
	if err != nil {
		t.Fatal(err)
	}
	decryptedToken, err := decryptSecret(savedToken.EncryptedValue, srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if decryptedToken != "user-token" {
		t.Fatalf("unexpected saved github oauth token: %q", decryptedToken)
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

func TestSanitizeOAuthReturnToRejectsBackslashRedirects(t *testing.T) {
	for _, value := range []string{`/\example.com`, `/\\example.com`, `/account\repositories`} {
		if got := sanitizeOAuthReturnTo(value); got != "" {
			t.Fatalf("expected %q to be rejected, got %q", value, got)
		}
	}
	if got := sanitizeOAuthReturnTo("/account/repositories"); got != "/account/repositories" {
		t.Fatalf("expected account path to be preserved, got %q", got)
	}
}

func TestUserSandboxAPIKeyPreferencesAreEncrypted(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "", &fakeSandbox{})

	req := httptest.NewRequest(http.MethodGet, "/user/preferences", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected preferences status: %d body=%s", rec.Code, rec.Body.String())
	}
	var preferences struct {
		Sandbox struct {
			Mode            string `json:"mode"`
			APIURL          string `json:"api_url"`
			Inherited       bool   `json:"inherited"`
			SourceAccountID int64  `json:"source_account_id"`
			APIKey          struct {
				Configured bool   `json:"configured"`
				UpdatedAt  string `json:"updated_at"`
			} `json:"api_key"`
		} `json:"sandbox"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &preferences); err != nil {
		t.Fatal(err)
	}
	if preferences.Sandbox.Mode != "custom" || preferences.Sandbox.APIURL != "" || preferences.Sandbox.APIKey.Configured {
		t.Fatalf("expected sandbox api key to start unconfigured: %#v", preferences)
	}

	req = httptest.NewRequest(http.MethodPut, "/user/preferences/sandbox", strings.NewReader(`{"api_url":" https://sandbox.example.test ","api_key":"sandbox-secret-key"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected save status: %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sandbox-secret-key") {
		t.Fatalf("response leaked sandbox api key: %s", rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &preferences); err != nil {
		t.Fatal(err)
	}
	if preferences.Sandbox.Mode != "custom" || preferences.Sandbox.APIURL != "https://sandbox.example.test" || !preferences.Sandbox.APIKey.Configured || preferences.Sandbox.APIKey.UpdatedAt == "" {
		t.Fatalf("expected configured sandbox response: %#v", preferences)
	}

	account, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	installation, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "gitwikitree",
	})
	if err != nil {
		t.Fatal(err)
	}
	config, err := store.GetAccountPreference(state.AccountScopeTypeAccount, account.ID, "sandbox", "service")
	if err != nil {
		t.Fatal(err)
	}
	if config.ValueJSON != `{"api_url":"https://sandbox.example.test"}` {
		t.Fatalf("unexpected sandbox preference value: %q", config.ValueJSON)
	}
	saved, err := store.GetAccountSecret(state.AccountScopeTypeAccount, account.ID, state.AccountSecretTypeSandboxAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(saved.EncryptedValue, "sandbox-secret-key") {
		t.Fatalf("stored sandbox api key should be encrypted, got %q", saved.EncryptedValue)
	}
	decrypted, err := decryptSecret(saved.EncryptedValue, srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != "sandbox-secret-key" {
		t.Fatalf("unexpected decrypted sandbox api key: %q", decrypted)
	}

	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/user/preferences?installation_id=%d", installation.ID), nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected org preferences status: %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &preferences); err != nil {
		t.Fatal(err)
	}
	if preferences.Sandbox.Mode != "custom" || preferences.Sandbox.Inherited || preferences.Sandbox.SourceAccountID != 0 || preferences.Sandbox.APIURL != "" || preferences.Sandbox.APIKey.Configured {
		t.Fatalf("expected org sandbox preferences to start unconfigured: %#v", preferences)
	}

	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/user/preferences/sandbox?installation_id=%d", installation.ID), strings.NewReader(`{"api_url":"https://org-sandbox.example.test","api_key":"org-sandbox-secret-key"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected org custom save status: %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := store.GetAccountSecret(state.AccountScopeTypeGitHubInstall, installation.InstallationID, state.AccountSecretTypeSandboxAPIKey); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/user/preferences/sandbox?installation_id=%d", installation.ID), strings.NewReader(`{"mode":"inherit"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected org inherit save status: %d body=%s", rec.Code, rec.Body.String())
	}
	config, err = store.GetAccountPreference(state.AccountScopeTypeGitHubInstall, installation.InstallationID, "sandbox", "service")
	if err != nil {
		t.Fatal(err)
	}
	expectedOrgConfig := fmt.Sprintf(`{"mode":"inherit","source_account_id":%d}`, account.ID)
	if config.ValueJSON != expectedOrgConfig {
		t.Fatalf("unexpected org inherited sandbox preference value: %q", config.ValueJSON)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &preferences); err != nil {
		t.Fatal(err)
	}
	if preferences.Sandbox.Mode != "inherit" || !preferences.Sandbox.Inherited || preferences.Sandbox.APIURL != "https://sandbox.example.test" || !preferences.Sandbox.APIKey.Configured {
		t.Fatalf("expected saved org sandbox preferences to inherit account preferences: %#v", preferences)
	}
	if _, err := store.GetAccountSecret(state.AccountScopeTypeGitHubInstall, installation.InstallationID, state.AccountSecretTypeSandboxAPIKey); err != state.ErrNotFound {
		t.Fatalf("expected inherited org sandbox preference to delete scoped api key, got %v", err)
	}

	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/user/preferences/sandbox?installation_id=%d", installation.ID), strings.NewReader(`{"api_url":"https://org-sandbox-updated.example.test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected org custom save without new key to fail after inherit cleared scoped key, status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/user/preferences/sandbox", strings.NewReader(`{"api_url":"https://sandbox-updated.example.test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected url-only save status: %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sandbox-secret-key") {
		t.Fatalf("response leaked sandbox api key after url-only update: %s", rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &preferences); err != nil {
		t.Fatal(err)
	}
	if preferences.Sandbox.Mode != "custom" || preferences.Sandbox.APIURL != "https://sandbox-updated.example.test" || !preferences.Sandbox.APIKey.Configured {
		t.Fatalf("expected updated url with existing api key: %#v", preferences)
	}

	req = httptest.NewRequest(http.MethodDelete, "/user/preferences/sandbox-api-key", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected delete status: %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := store.GetAccountSecret(state.AccountScopeTypeAccount, account.ID, state.AccountSecretTypeSandboxAPIKey); err != state.ErrNotFound {
		t.Fatalf("expected sandbox api key to be deleted, got %v", err)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &preferences); err != nil {
		t.Fatal(err)
	}
	if preferences.Sandbox.Mode != "custom" || preferences.Sandbox.APIURL != "https://sandbox-updated.example.test" || preferences.Sandbox.APIKey.Configured {
		t.Fatalf("expected delete to preserve sandbox api url and clear key: %#v", preferences)
	}
}

func TestUserSandboxInheritanceRequiresExplicitSourceReplacement(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "", &fakeSandbox{})
	if _, _, err := store.UpsertAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "alice-id", OAuthLogin: "alice"}, "user"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.UpsertAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "bob-id", OAuthLogin: "bob"}, "user"); err != nil {
		t.Fatal(err)
	}

	type sandboxPreferences struct {
		Sandbox struct {
			Mode                   string `json:"mode"`
			APIURL                 string `json:"api_url"`
			Inherited              bool   `json:"inherited"`
			SourceAccountID        int64  `json:"source_account_id"`
			SourceAccountLogin     string `json:"source_account_login"`
			SourceIsCurrentAccount bool   `json:"source_is_current_account"`
			SourceAvailable        bool   `json:"source_available"`
			APIKey                 struct {
				Configured bool `json:"configured"`
			} `json:"api_key"`
		} `json:"sandbox"`
	}

	saveAccountDefault := func(subject, login, apiURL, apiKey string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "/user/preferences/sandbox", strings.NewReader(fmt.Sprintf(`{"api_url":%q,"api_key":%q}`, apiURL, apiKey)))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(testSessionCookie(subject, login, "user"))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("save %s account defaults: status=%d body=%s", login, rec.Code, rec.Body.String())
		}
	}

	saveAccountDefault("alice-id", "alice", "https://alice-sandbox.example.test", "alice-key")
	saveAccountDefault("bob-id", "bob", "https://bob-sandbox.example.test", "bob-key")
	alice, _, err := store.GetAccountByOAuthIdentity("github", "alice-id")
	if err != nil {
		t.Fatal(err)
	}
	bob, _, err := store.GetAccountByOAuthIdentity("github", "bob-id")
	if err != nil {
		t.Fatal(err)
	}
	aliceInstallation, err := store.UpsertGitHubInstallation(state.GitHubInstallation{AccountID: alice.ID, InstallationID: 987, AccountLogin: "example-org"})
	if err != nil {
		t.Fatal(err)
	}
	bobInstallation, err := store.UpsertGitHubInstallation(state.GitHubInstallation{AccountID: bob.ID, InstallationID: 987, AccountLogin: "example-org"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/user/preferences/sandbox?installation_id=%d", aliceInstallation.ID), strings.NewReader(`{"mode":"inherit"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testSessionCookie("alice-id", "alice", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("alice enables inheritance: status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/user/preferences?installation_id=%d", bobInstallation.ID), nil)
	req.AddCookie(testSessionCookie("bob-id", "bob", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bob reads inherited preferences: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var preferences sandboxPreferences
	if err := json.Unmarshal(rec.Body.Bytes(), &preferences); err != nil {
		t.Fatal(err)
	}
	if preferences.Sandbox.SourceAccountID != alice.ID || preferences.Sandbox.SourceAccountLogin != "alice" || preferences.Sandbox.SourceIsCurrentAccount || !preferences.Sandbox.SourceAvailable || preferences.Sandbox.APIURL != "https://alice-sandbox.example.test" {
		t.Fatalf("expected bob to see alice as the inherited source: %#v", preferences)
	}

	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/user/preferences/sandbox?installation_id=%d", bobInstallation.ID), strings.NewReader(`{"mode":"inherit"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testSessionCookie("bob-id", "bob", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bob preserves inherited source: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &preferences); err != nil {
		t.Fatal(err)
	}
	if preferences.Sandbox.SourceAccountID != alice.ID || preferences.Sandbox.SourceIsCurrentAccount {
		t.Fatalf("expected ordinary inherit save to preserve alice as source: %#v", preferences)
	}
	if err := store.DeleteGitHubInstallation(alice.ID, aliceInstallation.ID); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/user/preferences?installation_id=%d", bobInstallation.ID), nil)
	req.AddCookie(testSessionCookie("bob-id", "bob", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bob reads unavailable inherited preferences: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &preferences); err != nil {
		t.Fatal(err)
	}
	if !preferences.Sandbox.Inherited || preferences.Sandbox.SourceAvailable || preferences.Sandbox.SourceAccountID != alice.ID || preferences.Sandbox.SourceAccountLogin != "alice" || preferences.Sandbox.APIURL != "" || preferences.Sandbox.APIKey.Configured {
		t.Fatalf("expected unlinked inherited source to be unavailable: %#v", preferences)
	}

	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/user/preferences/sandbox?installation_id=%d", bobInstallation.ID), strings.NewReader(`{"mode":"inherit","replace_inherited_source":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testSessionCookie("bob-id", "bob", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bob replaces inherited source: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &preferences); err != nil {
		t.Fatal(err)
	}
	if preferences.Sandbox.SourceAccountID != bob.ID || preferences.Sandbox.SourceAccountLogin != "bob" || !preferences.Sandbox.SourceIsCurrentAccount || !preferences.Sandbox.SourceAvailable || preferences.Sandbox.APIURL != "https://bob-sandbox.example.test" {
		t.Fatalf("expected explicit replacement to use bob as source: %#v", preferences)
	}
}

func TestUserSandboxInheritanceRejectsUnconfiguredSourceAccount(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "", &fakeSandbox{})
	if _, _, err := store.UpsertAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "alice-id", OAuthLogin: "alice"}, "user"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/user/preferences", nil)
	req.AddCookie(testSessionCookie("alice-id", "alice", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create alice account: status=%d body=%s", rec.Code, rec.Body.String())
	}
	alice, _, err := store.GetAccountByOAuthIdentity("github", "alice-id")
	if err != nil {
		t.Fatal(err)
	}
	installation, err := store.UpsertGitHubInstallation(state.GitHubInstallation{AccountID: alice.ID, InstallationID: 987, AccountLogin: "example-org"})
	if err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/user/preferences/sandbox?installation_id=%d", installation.ID), strings.NewReader(`{"mode":"inherit"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testSessionCookie("alice-id", "alice", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "configure your account Sandbox service first") {
		t.Fatalf("expected unconfigured inherited source to be rejected: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := store.GetAccountPreference(state.AccountScopeTypeGitHubInstall, installation.InstallationID, "sandbox", "service"); err != state.ErrNotFound {
		t.Fatalf("expected rejected inheritance not to save an org preference, got %v", err)
	}
}

func TestAdminSandboxServiceDefaultRequiresAdmin(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "", &fakeSandbox{})

	req := httptest.NewRequest(http.MethodGet, "/admin/api/sandbox-service-default", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET sandbox default without auth: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminAccountsRequiresAdmin(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "", &fakeSandbox{})

	req := httptest.NewRequest(http.MethodGet, "/admin/api/accounts", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET accounts without auth: status=%d body=%s", rec.Code, rec.Body.String())
	}

	hubot, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/admin/api/accounts/%d/role", hubot.ID), strings.NewReader(`{"role":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("PATCH account role without auth: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminAccountsSearchFilterAndPagination(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "", &fakeSandbox{})
	alice, _, err := store.UpsertAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "alice-id", OAuthLogin: "alice"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LinkOAuthIdentityToAccount(alice.ID, state.OAuthIdentity{OAuthProvider: "gitlab", OAuthSubject: "alice-gl", OAuthLogin: "alice-lab"}); err != nil {
		t.Fatal(err)
	}
	current, _, err := store.GetAccountByOAuthIdentity("github", "12345")
	if err != nil {
		t.Fatal(err)
	}

	type accountsResponse struct {
		Accounts         []state.AccountListItem `json:"accounts"`
		CurrentAccountID int64                   `json:"current_account_id"`
		Stats            state.AccountStats      `json:"stats"`
		Total            int64                   `json:"total"`
		Limit            int                     `json:"limit"`
		Offset           int                     `json:"offset"`
	}
	req := adminRequest(http.MethodGet, "/admin/api/accounts?q=ALICE-LAB&role=user&limit=1&offset=0", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET filtered accounts: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body accountsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 || body.Limit != 1 || body.Offset != 0 || body.CurrentAccountID != current.ID || len(body.Accounts) != 1 || body.Accounts[0].ID != alice.ID || len(body.Accounts[0].OAuthIdentities) != 2 {
		t.Fatalf("unexpected filtered accounts response: %#v", body)
	}
	wantStats := state.AccountStats{TotalAccounts: 3, AdminAccounts: 1, UserAccounts: 2, OAuthIdentities: 4}
	if body.Stats != wantStats {
		t.Fatalf("unexpected unfiltered account stats: got=%#v want=%#v", body.Stats, wantStats)
	}
	if rec.Header().Get("X-Total-Count") != "1" || rec.Header().Get("X-Limit") != "1" || rec.Header().Get("X-Offset") != "0" {
		t.Fatalf("unexpected pagination headers: %#v", rec.Header())
	}

	req = adminRequest(http.MethodGet, "/admin/api/accounts?limit=1&offset=1", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET account page: status=%d body=%s", rec.Code, rec.Body.String())
	}
	body = accountsResponse{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 3 || len(body.Accounts) != 1 || rec.Header().Get("Link") == "" {
		t.Fatalf("unexpected account page response: body=%#v headers=%#v", body, rec.Header())
	}

	req = adminRequest(http.MethodGet, "/admin/api/accounts?role=owner", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "role must be admin or user") {
		t.Fatalf("GET invalid role filter: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminAccountRoleUpdateIsAudited(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "", &fakeSandbox{})
	hubot, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil {
		t.Fatal(err)
	}

	req := adminRequest(http.MethodPatch, fmt.Sprintf("/admin/api/accounts/%d/role", hubot.ID), strings.NewReader(`{"role":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH account role: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var updated state.Account
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != hubot.ID || updated.Role != "admin" {
		t.Fatalf("unexpected updated account: %#v", updated)
	}
	persisted, _, err := store.GetAccountByOAuthIdentity("github", "hubot-id")
	if err != nil || persisted.Role != "admin" {
		t.Fatalf("unexpected persisted account: account=%#v err=%v", persisted, err)
	}
	events, err := store.ListAuditEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Actor != "github:12345" || events[0].Action != "account.role.update" || events[0].ResourceType != "account" || events[0].ResourceID != fmt.Sprint(hubot.ID) || !strings.Contains(events[0].PayloadJSON, `"old_role":"user"`) || !strings.Contains(events[0].PayloadJSON, `"new_role":"admin"`) {
		t.Fatalf("unexpected role update audit event: %#v", events)
	}

	req = adminRequest(http.MethodPatch, fmt.Sprintf("/admin/api/accounts/%d/role", hubot.ID), strings.NewReader(`{"role":"owner"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "role must be admin or user") {
		t.Fatalf("PATCH invalid account role: status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodPatch, "/admin/api/accounts/999999/role", strings.NewReader(`{"role":"user"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("PATCH missing account role: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminAccountRoleUpdateRejectsCurrentAccount(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "", &fakeSandbox{})
	current, _, err := store.GetAccountByOAuthIdentity("github", "12345")
	if err != nil {
		t.Fatal(err)
	}

	req := adminRequest(http.MethodPatch, fmt.Sprintf("/admin/api/accounts/%d/role", current.ID), strings.NewReader(`{"role":"user"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "cannot change your own role") {
		t.Fatalf("PATCH current account role: status=%d body=%s", rec.Code, rec.Body.String())
	}
	persisted, _, err := store.GetAccountByOAuthIdentity("github", "12345")
	if err != nil || persisted.Role != "admin" {
		t.Fatalf("expected current account to stay admin: account=%#v err=%v", persisted, err)
	}
}

func TestAdminSandboxServiceDefaultLifecycle(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "", &fakeSandbox{})

	req := adminRequest(http.MethodGet, "/admin/api/sandbox-service-default", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"enabled":false`) || !strings.Contains(rec.Body.String(), `"configured":false`) {
		t.Fatalf("GET empty sandbox default: status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodPut, "/admin/api/sandbox-service-default", strings.NewReader(`{"enabled":true,"api_url":"https://sandbox.example.test"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "api_key is required") {
		t.Fatalf("enable incomplete sandbox default: status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodPut, "/admin/api/sandbox-service-default", strings.NewReader(`{"enabled":true,"api_url":"ftp://sandbox.example.test","api_key":"secret-value"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "api_url must use http or https") {
		t.Fatalf("save invalid sandbox default URL: status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodPut, "/admin/api/sandbox-service-default", strings.NewReader(`{"enabled":true,"api_url":"https://sandbox.example.test","api_key":"secret-value"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"configured":true`) || strings.Contains(rec.Body.String(), "secret-value") {
		t.Fatalf("save sandbox default: status=%d body=%s", rec.Code, rec.Body.String())
	}
	saved, err := store.GetSandboxServiceDefault()
	if err != nil {
		t.Fatal(err)
	}
	if saved.APIKeyEncrypted == "" || saved.APIKeyEncrypted == "secret-value" {
		t.Fatalf("expected encrypted API key at rest, got %q", saved.APIKeyEncrypted)
	}
	decrypted, err := decryptSecret(saved.APIKeyEncrypted, srv.cfg.AuthEncryptionKey.Value())
	if err != nil || decrypted != "secret-value" {
		t.Fatalf("decrypt saved API key: value=%q err=%v", decrypted, err)
	}

	req = adminRequest(http.MethodPut, "/admin/api/sandbox-service-default", strings.NewReader(`{"enabled":false,"api_url":"https://sandbox.example.test/v2"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"enabled":false`) || !strings.Contains(rec.Body.String(), `"configured":true`) {
		t.Fatalf("disable sandbox default: status=%d body=%s", rec.Code, rec.Body.String())
	}
	saved, err = store.GetSandboxServiceDefault()
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err = decryptSecret(saved.APIKeyEncrypted, srv.cfg.AuthEncryptionKey.Value())
	if err != nil || decrypted != "secret-value" {
		t.Fatalf("expected omitted API key to preserve saved key: value=%q err=%v", decrypted, err)
	}

	req = adminRequest(http.MethodDelete, "/admin/api/sandbox-service-default/api-key", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"configured":false`) {
		t.Fatalf("delete sandbox default key: status=%d body=%s", rec.Code, rec.Body.String())
	}
	req = adminRequest(http.MethodDelete, "/admin/api/sandbox-service-default/api-key", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete sandbox default key twice: status=%d body=%s", rec.Code, rec.Body.String())
	}

	events, err := store.ListAuditEvents(20)
	if err != nil {
		t.Fatal(err)
	}
	var configured, deleted bool
	for _, event := range events {
		if strings.Contains(event.PayloadJSON, "secret-value") || strings.Contains(event.PayloadJSON, saved.APIKeyEncrypted) {
			t.Fatalf("audit event leaked API key: %#v", event)
		}
		configured = configured || event.Action == "sandbox_default.configure"
		deleted = deleted || event.Action == "sandbox_default.api_key.delete"
	}
	if !configured || !deleted {
		t.Fatalf("expected configure and delete audit events, got %#v", events)
	}
}

func TestAdminSandboxServiceDefaultRejectsStaleSaveAfterAPIKeyDeletion(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "", &fakeSandbox{})
	encrypted, err := encryptSecret("admin-sandbox-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefault(state.SandboxServiceDefault{
		Enabled:         true,
		APIURL:          "https://sandbox.example.test",
		APIKeyEncrypted: encrypted,
	}); err != nil {
		t.Fatal(err)
	}

	blockingStore := &blockingSandboxDefaultStore{
		Store:       store,
		getStarted:  make(chan struct{}),
		continueGet: make(chan struct{}),
	}
	srv.store = blockingStore
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := adminRequest(http.MethodPut, "/admin/api/sandbox-service-default", strings.NewReader(`{"enabled":true,"api_url":"https://sandbox.example.test/v2"}`))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		response <- rec
	}()

	<-blockingStore.getStarted
	if err := store.DeleteSandboxServiceDefaultAPIKey(); err != nil {
		t.Fatal(err)
	}
	close(blockingStore.continueGet)
	rec := <-response
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "api_key is required") {
		t.Fatalf("stale save after key deletion: status=%d body=%s", rec.Code, rec.Body.String())
	}
	saved, err := store.GetSandboxServiceDefault()
	if err != nil {
		t.Fatal(err)
	}
	if saved.APIKeyEncrypted != "" {
		t.Fatalf("stale save restored deleted API key: %#v", saved)
	}
}

func TestAdminSandboxServiceDefaultAudienceLifecycle(t *testing.T) {
	store := state.New(t.TempDir())
	account, _, err := store.EnsureAccountForOAuthIdentity(state.OAuthIdentity{
		OAuthProvider: "github",
		OAuthSubject:  "100",
		OAuthLogin:    "admin",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:       account.ID,
		InstallationID:  987,
		GitHubAccountID: 9002,
		AccountType:     "organization",
		AccountLogin:    "synced-org",
		AccountName:     "Synced Org",
		AccountAvatar:   "https://avatars.example/s.png",
	}); err != nil {
		t.Fatal(err)
	}
	githubAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/users/OCTO-ORG", "/users/octo-org":
			w.Write([]byte(`{"id":9001,"login":"octo-org","type":"Organization","name":"Octo Org","avatar_url":"https://avatars.example/o.png"}`))
		case "/users/missing":
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer githubAPI.Close()
	srv := newTestServer(t, store, "", &fakeSandbox{})
	srv.gh = github.NewClient(githubAPI.URL, githubAPI.Client())

	type response struct {
		AudienceMode      string                                `json:"audience_mode"`
		Audiences         []state.SandboxServiceDefaultAudience `json:"audiences"`
		AvailableAccounts []state.GitHubInstallationAccount     `json:"available_accounts"`
	}
	decode := func(rec *httptest.ResponseRecorder) response {
		t.Helper()
		var body response
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode audience response: %v body=%s", err, rec.Body.String())
		}
		return body
	}

	req := adminRequest(http.MethodGet, "/admin/api/sandbox-service-default", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	body := decode(rec)
	if rec.Code != http.StatusOK || body.AudienceMode != "all" || len(body.AvailableAccounts) != 1 || body.AvailableAccounts[0].GitHubAccountID != 9002 {
		t.Fatalf("GET audience defaults: status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodPut, "/admin/api/sandbox-service-default", strings.NewReader(`{"enabled":false,"audience_mode":"unknown"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "audience_mode") {
		t.Fatalf("save invalid audience mode: status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodPut, "/admin/api/sandbox-service-default", strings.NewReader(`{"enabled":false,"audience_mode":"selected"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	body = decode(rec)
	if rec.Code != http.StatusOK || body.AudienceMode != "selected" || len(body.Audiences) != 0 {
		t.Fatalf("save selected audience mode: status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/admin/api/sandbox-service-default/audiences", strings.NewReader(`{"account_login":"octo-org"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("add audience without auth: status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodPost, "/admin/api/sandbox-service-default/audiences", strings.NewReader(`{"account_login":"missing"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "GitHub account was not found") {
		t.Fatalf("add unknown audience: status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodPost, "/admin/api/sandbox-service-default/audiences", strings.NewReader(`{"account_login":" @OCTO-ORG "}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	body = decode(rec)
	if rec.Code != http.StatusOK || len(body.Audiences) != 1 || body.Audiences[0].GitHubAccountID != 9001 || body.Audiences[0].AccountType != "organization" {
		t.Fatalf("add manually resolved audience: status=%d body=%s", rec.Code, rec.Body.String())
	}
	audienceID := body.Audiences[0].ID

	req = adminRequest(http.MethodPost, "/admin/api/sandbox-service-default/audiences", strings.NewReader(`{"account_login":"octo-org"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	body = decode(rec)
	if rec.Code != http.StatusOK || len(body.Audiences) != 1 || body.Audiences[0].ID != audienceID {
		t.Fatalf("add duplicate audience: status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodDelete, fmt.Sprintf("/admin/api/sandbox-service-default/audiences/%d", audienceID), nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	body = decode(rec)
	if rec.Code != http.StatusOK || len(body.Audiences) != 0 {
		t.Fatalf("delete audience: status=%d body=%s", rec.Code, rec.Body.String())
	}

	events, err := store.ListAuditEvents(20)
	if err != nil {
		t.Fatal(err)
	}
	var added, deleted bool
	for _, event := range events {
		added = added || event.Action == "sandbox_default.audience.add"
		deleted = deleted || event.Action == "sandbox_default.audience.delete"
	}
	if !added || !deleted {
		t.Fatalf("expected audience audit events, got %#v", events)
	}
}

func TestSandboxServiceUsesAdminDefault(t *testing.T) {
	store := state.New(t.TempDir())
	cfg := config.Config{
		AuthEncryptionKey:    "encryption-key",
		MaxConcurrentRunners: 10,
	}
	srv := New(cfg, store, github.NewClient("", http.DefaultClient), nil, nil)
	encrypted, err := encryptSecret("admin-sandbox-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefault(state.SandboxServiceDefault{
		Enabled:         true,
		APIURL:          "https://admin-sandbox.example.test",
		APIKeyEncrypted: encrypted,
	}); err != nil {
		t.Fatal(err)
	}

	svc, snapshot, err := srv.sandboxServiceAndConfigForRunnerRequest(state.RunnerRequest{ID: "req-1", GitHubInstallationID: 987})
	if err != nil || svc == nil {
		t.Fatalf("expected admin Sandbox default, service=%T err=%v", svc, err)
	}
	if snapshot.APIURL != "https://admin-sandbox.example.test" || snapshot.EncryptedAPIKey != encrypted || snapshot.Source != sandboxConfigSourceAdminDefault {
		t.Fatalf("unexpected admin default snapshot: %#v", snapshot)
	}
}

func TestSandboxServiceAllAdminDefaultDoesNotRequireInstallationScope(t *testing.T) {
	store := state.New(t.TempDir())
	srv := New(config.Config{AuthEncryptionKey: "encryption-key", MaxConcurrentRunners: 10}, store, github.NewClient("", http.DefaultClient), nil, nil)
	encrypted, err := encryptSecret("admin-sandbox-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefault(state.SandboxServiceDefault{
		Enabled:         true,
		AudienceMode:    state.SandboxServiceDefaultAudienceModeAll,
		APIURL:          "https://admin-sandbox.example.test",
		APIKeyEncrypted: encrypted,
	}); err != nil {
		t.Fatal(err)
	}

	svc, snapshot, err := srv.sandboxServiceAndConfigForRunnerRequest(state.RunnerRequest{
		ID:                 "manual-request",
		RepositoryFullName: "unsynced/example",
	})
	if err != nil || svc == nil || snapshot.Source != sandboxConfigSourceAdminDefault {
		t.Fatalf("expected all-account admin default without installation scope, service=%T snapshot=%#v err=%v", svc, snapshot, err)
	}
}

func TestSandboxServiceAdminDefaultAudienceAllowsSelectedInstallationOwner(t *testing.T) {
	store := state.New(t.TempDir())
	account, _, err := store.EnsureAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "alice"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:       account.ID,
		InstallationID:  987,
		GitHubAccountID: 9001,
		AccountType:     "organization",
		AccountLogin:    "octo-org",
	}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AuthEncryptionKey: "encryption-key", MaxConcurrentRunners: 10}
	srv := New(cfg, store, github.NewClient("", http.DefaultClient), nil, nil)
	encrypted, err := encryptSecret("admin-sandbox-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefault(state.SandboxServiceDefault{
		Enabled:         true,
		AudienceMode:    state.SandboxServiceDefaultAudienceModeSelected,
		APIURL:          "https://admin-sandbox.example.test",
		APIKeyEncrypted: encrypted,
	}); err != nil {
		t.Fatal(err)
	}
	audience, err := store.UpsertSandboxServiceDefaultAudience(state.SandboxServiceDefaultAudience{
		GitHubAccountID: 9001,
		AccountType:     "organization",
		AccountLogin:    "octo-org",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, snapshot, err := srv.sandboxServiceAndConfigForRunnerRequest(state.RunnerRequest{ID: "req-1", GitHubInstallationID: 987})
	if err != nil || snapshot.Source != sandboxConfigSourceAdminDefault {
		t.Fatalf("expected selected owner to use admin default, snapshot=%#v err=%v", snapshot, err)
	}
	if err := store.DeleteSandboxServiceDefaultAudience(audience.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := srv.sandboxServiceAndConfigForRunnerRequest(state.RunnerRequest{ID: "req-2", GitHubInstallationID: 987}); !errors.Is(err, errSandboxServiceNotConfigured) {
		t.Fatalf("expected removed owner to lose admin default, got %v", err)
	}
	if svc, restored, err := srv.sandboxServiceAndConfigForRunnerRequest(state.RunnerRequest{
		ID:                     "req-1",
		GitHubInstallationID:   987,
		SandboxAPIURL:          snapshot.APIURL,
		SandboxAPIKeyEncrypted: snapshot.EncryptedAPIKey,
		SandboxConfigSource:    snapshot.Source,
	}); err != nil || svc == nil || restored.Source != sandboxConfigSourceAdminDefault {
		t.Fatalf("expected saved snapshot after audience removal, service=%T snapshot=%#v err=%v", svc, restored, err)
	}
}

func TestSandboxServiceAdminDefaultAudienceResolvesAndCachesUnsynchronizedInstallationOwner(t *testing.T) {
	store := state.New(t.TempDir())
	var installationLookups int
	githubAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/app/installations/987" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		installationLookups++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":987,"account":{"id":9001,"login":"octo-org","type":"Organization","name":"Octo Org","avatar_url":"https://avatars.example/o.png"}}`))
	}))
	defer githubAPI.Close()
	gh, err := github.NewAppClient(githubAPI.URL, github.AppAuth{
		AppID:          123,
		PrivateKeyFile: testServerPrivateKeyFile(t),
	}, githubAPI.Client())
	if err != nil {
		t.Fatal(err)
	}
	srv := New(config.Config{AuthEncryptionKey: "encryption-key", MaxConcurrentRunners: 10}, store, gh, nil, nil)
	encrypted, err := encryptSecret("admin-sandbox-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefault(state.SandboxServiceDefault{
		Enabled:         true,
		AudienceMode:    state.SandboxServiceDefaultAudienceModeSelected,
		APIURL:          "https://admin-sandbox.example.test",
		APIKeyEncrypted: encrypted,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefaultAudience(state.SandboxServiceDefaultAudience{
		GitHubAccountID: 9001,
		AccountType:     "organization",
		AccountLogin:    "octo-org",
	}); err != nil {
		t.Fatal(err)
	}

	for _, requestID := range []string{"req-1", "req-2"} {
		_, snapshot, err := srv.sandboxServiceAndConfigForRunnerRequest(state.RunnerRequest{ID: requestID, GitHubInstallationID: 987})
		if err != nil || snapshot.Source != sandboxConfigSourceAdminDefault {
			t.Fatalf("expected unsynchronized owner to use admin default, request=%s snapshot=%#v err=%v", requestID, snapshot, err)
		}
	}
	if installationLookups != 1 {
		t.Fatalf("expected one GitHub installation lookup followed by a cache hit, got %d", installationLookups)
	}
	cached, err := store.GetGitHubInstallationOwner(987)
	if err != nil || cached.GitHubAccountID != 9001 || cached.AccountType != "organization" {
		t.Fatalf("unexpected cached installation owner: %#v err=%v", cached, err)
	}
}

func TestGitHubInstallationOwnerRejectsInvalidInstallationID(t *testing.T) {
	server := &Server{}
	if _, err := server.githubInstallationOwner(t.Context(), 0); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("githubInstallationOwner() error = %v, want ErrNotFound", err)
	}
}

func TestSandboxServiceAdminDefaultAudienceRejectsUnselectedOwner(t *testing.T) {
	store := state.New(t.TempDir())
	account, _, err := store.EnsureAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "alice"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:       account.ID,
		InstallationID:  987,
		GitHubAccountID: 9002,
		AccountType:     "organization",
		AccountLogin:    "other-org",
	}); err != nil {
		t.Fatal(err)
	}
	srv := New(config.Config{AuthEncryptionKey: "encryption-key", MaxConcurrentRunners: 10}, store, github.NewClient("", http.DefaultClient), nil, nil)
	encrypted, err := encryptSecret("admin-sandbox-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefault(state.SandboxServiceDefault{
		Enabled:         true,
		AudienceMode:    state.SandboxServiceDefaultAudienceModeSelected,
		APIURL:          "https://admin-sandbox.example.test",
		APIKeyEncrypted: encrypted,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefaultAudience(state.SandboxServiceDefaultAudience{
		GitHubAccountID: 9001,
		AccountType:     "organization",
		AccountLogin:    "octo-org",
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := srv.sandboxServiceAndConfigForRunnerRequest(state.RunnerRequest{ID: "req-1", GitHubInstallationID: 987}); !errors.Is(err, errSandboxServiceNotConfigured) {
		t.Fatalf("expected unselected owner to remain unconfigured, got %v", err)
	}
}

func TestSandboxServiceAdminDefaultAudienceAllowsSelectedPersonalAccount(t *testing.T) {
	store := state.New(t.TempDir())
	account, _, err := store.EnsureAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "alice"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(config.Config{AuthEncryptionKey: "encryption-key", MaxConcurrentRunners: 10}, store, github.NewClient("", http.DefaultClient), nil, nil)
	encrypted, err := encryptSecret("admin-sandbox-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefault(state.SandboxServiceDefault{
		Enabled:         true,
		AudienceMode:    state.SandboxServiceDefaultAudienceModeSelected,
		APIURL:          "https://admin-sandbox.example.test",
		APIKeyEncrypted: encrypted,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefaultAudience(state.SandboxServiceDefaultAudience{
		GitHubAccountID: 100,
		AccountType:     "user",
		AccountLogin:    "alice",
	}); err != nil {
		t.Fatal(err)
	}

	_, snapshot, err := srv.sandboxServiceForScopeWithDefault(accountPreferenceScope{Type: state.AccountScopeTypeAccount, ID: account.ID})
	if err != nil || snapshot.Source != sandboxConfigSourceAdminDefault {
		t.Fatalf("expected selected personal account to use admin default, snapshot=%#v err=%v", snapshot, err)
	}
}

func TestSandboxServiceForScopeUsesAdminDefault(t *testing.T) {
	store := state.New(t.TempDir())
	cfg := config.Config{
		AuthEncryptionKey:    "encryption-key",
		MaxConcurrentRunners: 10,
	}
	srv := New(cfg, store, github.NewClient("", http.DefaultClient), nil, nil)
	encrypted, err := encryptSecret("admin-sandbox-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefault(state.SandboxServiceDefault{
		Enabled:         true,
		APIURL:          "https://admin-sandbox.example.test",
		APIKeyEncrypted: encrypted,
	}); err != nil {
		t.Fatal(err)
	}

	svc, snapshot, err := srv.sandboxServiceForScopeWithDefault(accountPreferenceScope{
		Type: state.AccountScopeTypeGitHubInstall,
		ID:   987,
	})
	if err != nil || svc == nil || snapshot.Source != sandboxConfigSourceAdminDefault {
		t.Fatalf("expected scoped resolver to use admin default, service=%T snapshot=%#v err=%v", svc, snapshot, err)
	}
}

func TestUserPreferencesReportsAdminDefaultWithoutLeakingMetadata(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "", &fakeSandbox{})
	encrypted, err := encryptSecret("admin-sandbox-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefault(state.SandboxServiceDefault{
		Enabled:         true,
		APIURL:          "https://admin-sandbox.example.test",
		APIKeyEncrypted: encrypted,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/user/preferences", nil)
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"resolved_source":"admin_default"`) {
		t.Fatalf("GET preferences with admin default: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "admin-sandbox.example.test") || strings.Contains(rec.Body.String(), "admin-sandbox-key") || strings.Contains(rec.Body.String(), encrypted) {
		t.Fatalf("preferences leaked admin default metadata: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/user/preferences/sandbox", strings.NewReader(`{"api_url":"https://user-sandbox.example.test","api_key":"user-key"}`))
	req.AddCookie(testSessionCookie("hubot-id", "hubot", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"resolved_source":"custom"`) {
		t.Fatalf("PUT user Sandbox preferences: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUserPreferencesReportsSelectedAdminDefaultOnlyForEligibleAccount(t *testing.T) {
	store := state.New(t.TempDir())
	if _, _, err := store.EnsureAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "alice"}, "user"); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, store, "", &fakeSandbox{})
	encrypted, err := encryptSecret("admin-sandbox-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefault(state.SandboxServiceDefault{
		Enabled:         true,
		AudienceMode:    state.SandboxServiceDefaultAudienceModeSelected,
		APIURL:          "https://admin-sandbox.example.test",
		APIKeyEncrypted: encrypted,
	}); err != nil {
		t.Fatal(err)
	}
	audience, err := store.UpsertSandboxServiceDefaultAudience(state.SandboxServiceDefaultAudience{
		GitHubAccountID: 100,
		AccountType:     "user",
		AccountLogin:    "alice",
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/user/preferences", nil)
	req.AddCookie(testSessionCookie("100", "alice", "user"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"resolved_source":"admin_default"`) || strings.Contains(rec.Body.String(), `"audiences"`) {
		t.Fatalf("eligible preferences response: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := store.DeleteSandboxServiceDefaultAudience(audience.ID); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodGet, "/user/preferences", nil)
	req.AddCookie(testSessionCookie("100", "alice", "user"))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"resolved_source":"none"`) {
		t.Fatalf("ineligible preferences response: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSandboxServiceDoesNotUseDisabledAdminDefault(t *testing.T) {
	store := state.New(t.TempDir())
	cfg := config.Config{
		AuthEncryptionKey:    "encryption-key",
		MaxConcurrentRunners: 10,
	}
	srv := New(cfg, store, github.NewClient("", http.DefaultClient), nil, nil)
	encrypted, err := encryptSecret("admin-sandbox-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefault(state.SandboxServiceDefault{
		Enabled:         false,
		APIURL:          "https://admin-sandbox.example.test",
		APIKeyEncrypted: encrypted,
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := srv.sandboxServiceAndConfigForRunnerRequest(state.RunnerRequest{ID: "req-1", GitHubInstallationID: 987}); !errors.Is(err, errSandboxServiceNotConfigured) {
		t.Fatalf("expected disabled admin default to remain unavailable, got %v", err)
	}
}

func TestSandboxServiceDoesNotUseAdminDefaultForCorruptScope(t *testing.T) {
	store := state.New(t.TempDir())
	cfg := config.Config{
		AuthEncryptionKey:    "encryption-key",
		MaxConcurrentRunners: 10,
	}
	srv := New(cfg, store, github.NewClient("", http.DefaultClient), nil, nil)
	encrypted, err := encryptSecret("admin-sandbox-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefault(state.SandboxServiceDefault{
		Enabled:         true,
		AudienceMode:    state.SandboxServiceDefaultAudienceModeSelected,
		APIURL:          "https://admin-sandbox.example.test",
		APIKeyEncrypted: encrypted,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountPreference(state.AccountPreference{
		ScopeType: state.AccountScopeTypeGitHubInstall,
		ScopeID:   987,
		Namespace: accountPreferenceNamespaceSandbox,
		Key:       accountPreferenceKeySandboxService,
		ValueJSON: "{",
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := srv.sandboxServiceAndConfigForRunnerRequest(state.RunnerRequest{ID: "req-1", GitHubInstallationID: 987}); err == nil || errors.Is(err, errSandboxServiceNotConfigured) {
		t.Fatalf("expected corrupt scoped config error without fallback, got %v", err)
	}
}

func TestSandboxServicePreservesSnapshotSource(t *testing.T) {
	store := state.New(t.TempDir())
	cfg := config.Config{
		AuthEncryptionKey:    "encryption-key",
		MaxConcurrentRunners: 10,
	}
	srv := New(cfg, store, github.NewClient("", http.DefaultClient), nil, nil)
	encrypted, err := encryptSecret("snapshot-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}

	svc, snapshot, err := srv.sandboxServiceAndConfigForRunnerRequest(state.RunnerRequest{
		ID:                     "req-1",
		SandboxAPIURL:          "https://snapshot.example.test",
		SandboxAPIKeyEncrypted: encrypted,
		SandboxConfigSource:    sandboxConfigSourceAdminDefault,
	})
	if err != nil || svc == nil {
		t.Fatalf("expected request snapshot, service=%T err=%v", svc, err)
	}
	if snapshot.Source != sandboxConfigSourceAdminDefault {
		t.Fatalf("snapshot source = %q", snapshot.Source)
	}
}

func TestSandboxServiceUsesGitHubInstallationPreferences(t *testing.T) {
	store := state.New(t.TempDir())
	cfg := config.Config{
		AuthEncryptionKey:    "encryption-key",
		MaxConcurrentRunners: 10,
	}
	srv := New(cfg, store, github.NewClient("", http.DefaultClient), nil, nil)

	if _, err := srv.sandboxServiceForRunnerRequest(context.Background(), state.RunnerRequest{ID: "req-1", GitHubInstallationID: 987}); err == nil {
		t.Fatal("expected missing sandbox service config error")
	}
	adminEncrypted, err := encryptSecret("admin-sandbox-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertSandboxServiceDefault(state.SandboxServiceDefault{
		Enabled:         true,
		AudienceMode:    state.SandboxServiceDefaultAudienceModeSelected,
		APIURL:          "https://admin-sandbox.example.test",
		APIKeyEncrypted: adminEncrypted,
	}); err != nil {
		t.Fatal(err)
	}

	valueJSON, err := json.Marshal(accountSandboxServicePreferenceValue{APIURL: "https://sandbox.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountPreference(state.AccountPreference{
		ScopeType: state.AccountScopeTypeGitHubInstall,
		ScopeID:   987,
		Namespace: accountPreferenceNamespaceSandbox,
		Key:       accountPreferenceKeySandboxService,
		ValueJSON: string(valueJSON),
	}); err != nil {
		t.Fatal(err)
	}
	encrypted, err := encryptSecret("sandbox-secret-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountSecret(state.AccountSecret{
		ScopeType:      state.AccountScopeTypeGitHubInstall,
		ScopeID:        987,
		KeyType:        state.AccountSecretTypeSandboxAPIKey,
		EncryptedValue: encrypted,
	}); err != nil {
		t.Fatal(err)
	}

	if svc, err := srv.sandboxServiceForRunnerRequest(context.Background(), state.RunnerRequest{ID: "req-1", GitHubInstallationID: 987}); err != nil || svc == nil {
		t.Fatalf("expected sandbox service from installation preferences, service=%T err=%v", svc, err)
	}
	svc, snapshot, err := srv.sandboxServiceAndConfigForRunnerRequest(state.RunnerRequest{ID: "req-1", GitHubInstallationID: 987})
	if err != nil || svc == nil || snapshot.APIURL == "" || snapshot.EncryptedAPIKey == "" || snapshot.Source != sandboxConfigSourceInstallation {
		t.Fatalf("expected sandbox service snapshot, service=%T snapshot=%#v err=%v", svc, snapshot, err)
	}
	if err := store.DeleteAccountSecret(state.AccountScopeTypeGitHubInstall, 987, state.AccountSecretTypeSandboxAPIKey); err != nil {
		t.Fatal(err)
	}
	if svc, err := srv.sandboxServiceForRunnerRequest(context.Background(), state.RunnerRequest{
		ID:                     "req-1",
		GitHubInstallationID:   987,
		SandboxAPIURL:          snapshot.APIURL,
		SandboxAPIKeyEncrypted: snapshot.EncryptedAPIKey,
	}); err != nil || svc == nil {
		t.Fatalf("expected sandbox service from request snapshot after preference deletion, service=%T err=%v", svc, err)
	}
}

func TestSandboxServiceFallsBackToPersonalAccountPreferences(t *testing.T) {
	store := state.New(t.TempDir())
	cfg := config.Config{
		AuthEncryptionKey:    "encryption-key",
		MaxConcurrentRunners: 10,
	}
	srv := New(cfg, store, github.NewClient("", http.DefaultClient), nil, nil)
	account, _, err := store.EnsureAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "miclle"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "miclle",
	}); err != nil {
		t.Fatal(err)
	}
	valueJSON, err := json.Marshal(accountSandboxServicePreferenceValue{APIURL: "https://sandbox.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountPreference(state.AccountPreference{
		ScopeType: state.AccountScopeTypeAccount,
		ScopeID:   account.ID,
		Namespace: accountPreferenceNamespaceSandbox,
		Key:       accountPreferenceKeySandboxService,
		ValueJSON: string(valueJSON),
	}); err != nil {
		t.Fatal(err)
	}
	encrypted, err := encryptSecret("sandbox-secret-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountSecret(state.AccountSecret{
		ScopeType:      state.AccountScopeTypeAccount,
		ScopeID:        account.ID,
		KeyType:        state.AccountSecretTypeSandboxAPIKey,
		EncryptedValue: encrypted,
	}); err != nil {
		t.Fatal(err)
	}
	if svc, snapshot, err := srv.sandboxServiceAndConfigForRunnerRequest(state.RunnerRequest{ID: "req-1", GitHubInstallationID: 987}); err != nil || svc == nil || snapshot.Source != sandboxConfigSourceAccount {
		t.Fatalf("expected sandbox service from personal account preferences, service=%T snapshot=%#v err=%v", svc, snapshot, err)
	}
}

func TestSandboxServiceUsesInheritedAccountPreferencesForOrgInstallation(t *testing.T) {
	store := state.New(t.TempDir())
	cfg := config.Config{
		AuthEncryptionKey:    "encryption-key",
		MaxConcurrentRunners: 10,
	}
	srv := New(cfg, store, github.NewClient("", http.DefaultClient), nil, nil)
	account, _, err := store.EnsureAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "miclle"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	installation, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "gitwikitree",
	})
	if err != nil {
		t.Fatal(err)
	}
	valueJSON, err := json.Marshal(accountSandboxServicePreferenceValue{APIURL: "https://sandbox.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountPreference(state.AccountPreference{
		ScopeType: state.AccountScopeTypeAccount,
		ScopeID:   account.ID,
		Namespace: accountPreferenceNamespaceSandbox,
		Key:       accountPreferenceKeySandboxService,
		ValueJSON: string(valueJSON),
	}); err != nil {
		t.Fatal(err)
	}
	inheritedValueJSON, err := json.Marshal(accountSandboxServicePreferenceValue{
		Mode:            sandboxPreferenceModeInherit,
		SourceAccountID: account.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountPreference(state.AccountPreference{
		ScopeType: state.AccountScopeTypeGitHubInstall,
		ScopeID:   987,
		Namespace: accountPreferenceNamespaceSandbox,
		Key:       accountPreferenceKeySandboxService,
		ValueJSON: string(inheritedValueJSON),
	}); err != nil {
		t.Fatal(err)
	}
	encrypted, err := encryptSecret("sandbox-secret-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountSecret(state.AccountSecret{
		ScopeType:      state.AccountScopeTypeAccount,
		ScopeID:        account.ID,
		KeyType:        state.AccountSecretTypeSandboxAPIKey,
		EncryptedValue: encrypted,
	}); err != nil {
		t.Fatal(err)
	}
	if svc, snapshot, err := srv.sandboxServiceAndConfigForRunnerRequest(state.RunnerRequest{ID: "req-1", GitHubInstallationID: 987}); err != nil || svc == nil || snapshot.Source != sandboxConfigSourceInheritedAccount {
		t.Fatalf("expected sandbox service from inherited account preferences, service=%T snapshot=%#v err=%v", svc, snapshot, err)
	}
	if err := store.DeleteGitHubInstallation(account.ID, installation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.sandboxServiceForRunnerRequest(context.Background(), state.RunnerRequest{ID: "req-1", GitHubInstallationID: 987}); !errors.Is(err, errSandboxServiceNotConfigured) {
		t.Fatalf("expected inherited sandbox service to require current installation link, got %v", err)
	}
}

func TestSandboxServiceInfersInstallationFromRepositoryOwner(t *testing.T) {
	store := state.New(t.TempDir())
	cfg := config.Config{
		AuthEncryptionKey:    "encryption-key",
		MaxConcurrentRunners: 10,
	}
	srv := New(cfg, store, github.NewClient("", http.DefaultClient), nil, nil)
	account, _, err := store.EnsureAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "miclle"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "miclle",
	}); err != nil {
		t.Fatal(err)
	}
	valueJSON, err := json.Marshal(accountSandboxServicePreferenceValue{APIURL: "https://sandbox.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountPreference(state.AccountPreference{
		ScopeType: state.AccountScopeTypeGitHubInstall,
		ScopeID:   987,
		Namespace: accountPreferenceNamespaceSandbox,
		Key:       accountPreferenceKeySandboxService,
		ValueJSON: string(valueJSON),
	}); err != nil {
		t.Fatal(err)
	}
	encrypted, err := encryptSecret("sandbox-secret-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountSecret(state.AccountSecret{
		ScopeType:      state.AccountScopeTypeGitHubInstall,
		ScopeID:        987,
		KeyType:        state.AccountSecretTypeSandboxAPIKey,
		EncryptedValue: encrypted,
	}); err != nil {
		t.Fatal(err)
	}
	if svc, err := srv.sandboxServiceForRunnerRequest(context.Background(), state.RunnerRequest{
		ID:                 "req-1",
		RepositoryFullName: "miclle/example",
	}); err != nil || svc == nil {
		t.Fatalf("expected sandbox service by repository owner installation, service=%T err=%v", svc, err)
	}
}

func TestSandboxServiceDoesNotFallBackToAccountForOrgInstallation(t *testing.T) {
	store := state.New(t.TempDir())
	cfg := config.Config{
		AuthEncryptionKey:    "encryption-key",
		MaxConcurrentRunners: 10,
	}
	srv := New(cfg, store, github.NewClient("", http.DefaultClient), nil, nil)
	account, _, err := store.EnsureAccountForOAuthIdentity(state.OAuthIdentity{OAuthProvider: "github", OAuthSubject: "100", OAuthLogin: "miclle"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGitHubInstallation(state.GitHubInstallation{
		AccountID:      account.ID,
		InstallationID: 987,
		AccountLogin:   "gitwikitree",
	}); err != nil {
		t.Fatal(err)
	}
	valueJSON, err := json.Marshal(accountSandboxServicePreferenceValue{APIURL: "https://sandbox.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountPreference(state.AccountPreference{
		ScopeType: state.AccountScopeTypeAccount,
		ScopeID:   account.ID,
		Namespace: accountPreferenceNamespaceSandbox,
		Key:       accountPreferenceKeySandboxService,
		ValueJSON: string(valueJSON),
	}); err != nil {
		t.Fatal(err)
	}
	encrypted, err := encryptSecret("sandbox-secret-key", srv.cfg.AuthEncryptionKey.Value())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountSecret(state.AccountSecret{
		ScopeType:      state.AccountScopeTypeAccount,
		ScopeID:        account.ID,
		KeyType:        state.AccountSecretTypeSandboxAPIKey,
		EncryptedValue: encrypted,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.sandboxServiceForRunnerRequest(context.Background(), state.RunnerRequest{ID: "req-1", GitHubInstallationID: 987}); !errors.Is(err, errSandboxServiceNotConfigured) {
		t.Fatalf("expected org installation to require its own sandbox service config, got %v", err)
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

	for _, path := range []string{
		"/repositories",
		"/account/repositories",
		"/account/preferences",
		"/organizations/qiniu/repositories",
		"/organizations/qiniu/preferences",
		"/jobs/runner-123",
		"/github/pulls/qiniu/ci-runner/22/jobs",
		"/github/runs/qiniu/ci-runner/123/jobs",
		"/github/branches/qiniu/ci-runner/abc123/jobs?branch=feature%2Fjob-detail-page",
		"/github/branches/qiniu/ci-runner/main/abc123/jobs",
		"/settings",
		"/accounts",
	} {
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

func TestCreateProfileStoresTemplateWithoutSandboxValidation(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	profileBody := bytes.NewBufferString(`{"name":"large","labels":["self-hosted","e2b","large"],"template_id":"missing-template","max_concurrency":5,"enabled":true}`)
	req := adminRequest(http.MethodPost, "/runner_specs", profileBody)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected created, got %d body=%s", rec.Code, rec.Body.String())
	}
	profile, err := store.GetProfile("large")
	if err != nil {
		t.Fatal(err)
	}
	if profile.TemplateID != "missing-template" {
		t.Fatalf("template id should be persisted without sandbox validation: %#v", profile)
	}
}

func TestPatchProfileStoresTemplateWithoutSandboxValidation(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	profileBody := bytes.NewBufferString(`{"name":"large","labels":["self-hosted","e2b","large"],"template_id":"valid-template","max_concurrency":5,"enabled":true}`)
	req := adminRequest(http.MethodPost, "/runner_specs", profileBody)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected create profile status: %d body=%s", rec.Code, rec.Body.String())
	}

	req = adminRequest(http.MethodPatch, "/runner_specs/large", bytes.NewBufferString(`{"template_id":"not-ready-template"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d body=%s", rec.Code, rec.Body.String())
	}
	profile, err := store.GetProfile("large")
	if err != nil {
		t.Fatal(err)
	}
	if profile.TemplateID != "not-ready-template" {
		t.Fatalf("template id update should be persisted without sandbox validation: %#v", profile)
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

func TestSweeperKeepsIdleTimedOutRunnerWhenGitHubRunnerIsBusy(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/orgs/o/actions/runners" {
			w.Write([]byte(`{"runners":[{"id":99,"name":"e2b-busy-on-github","status":"online","busy":true}]}`))
			return
		}
		t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	if _, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "busy-on-github",
		Source:             "test",
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerGroup:        "default",
		RunnerName:         "e2b-busy-on-github",
	}, nil); err != nil {
		t.Fatal(err)
	} else {
		st.Status = state.StatusRunning
		st.SandboxID = "sb-busy-on-github"
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
	if fake.stoppedCount() != 0 {
		t.Fatalf("expected sweeper to keep github-busy runner, got %d stops", fake.stoppedCount())
	}
	got, err := store.ReadState("busy-on-github")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusRunning {
		t.Fatalf("expected runner to stay running, got %s", got.Status)
	}
	if got.AssignedJobName != runnerJobStartedMarker {
		t.Fatalf("expected github busy runner to be marked started, got assigned job name %q", got.AssignedJobName)
	}
}

func TestSweeperStillStopsSandboxTimedOutRunnerWhenGitHubBusyCheckFails(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/orgs/o/actions/runners" {
			http.Error(w, `{"message":"server error"}`, http.StatusInternalServerError)
			return
		}
		t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	if _, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "timeout-after-busy-check-error",
		Source:             "test",
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerGroup:        "default",
		RunnerName:         "e2b-timeout-after-busy-check-error",
	}, nil); err != nil {
		t.Fatal(err)
	} else {
		st.Status = state.StatusRunning
		st.SandboxID = "sb-timeout-after-busy-check-error"
		st.ProcessPID = 42
		st.RunningAt = time.Now().Add(-10 * time.Minute).UTC()
		if err := store.WriteState(st); err != nil {
			t.Fatal(err)
		}
	}
	fake := &fakeSandbox{}
	srv := newTestServerWithLimit(t, store, ghServer.URL, fake, 10)
	srv.cfg.RunnerIdleTimeout = time.Minute
	srv.cfg.SandboxTimeout = time.Minute
	srv.sweepOnce(t.Context())
	if fake.stoppedCount() != 1 {
		t.Fatalf("expected sweeper to stop sandbox-timed-out runner, got %d stops", fake.stoppedCount())
	}
	got, err := store.ReadState("timeout-after-busy-check-error")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusStopping || got.FailureStage != "sandbox_timeout" || got.FailureReason != "timeout" {
		t.Fatalf("expected sandbox timeout failure, got %#v", got)
	}
}

func TestSweeperHandlesSandboxTimeoutBeforeIdleBusyLookup(t *testing.T) {
	lookupStarted := make(chan struct{})
	releaseLookup := make(chan struct{})
	var closeLookupStarted sync.Once
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/orgs/o/actions/runners" {
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
		closeLookupStarted.Do(func() { close(lookupStarted) })
		select {
		case <-releaseLookup:
		case <-r.Context().Done():
			return
		}
		w.Write([]byte(`{"runners":[]}`))
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	if _, st, err := store.CreateRequest(state.RunnerRequest{
		ID:          "sandbox-timeout-before-idle",
		Source:      "test",
		Labels:      []string{"self-hosted", "e2b"},
		ProfileName: "default",
	}, nil); err != nil {
		t.Fatal(err)
	} else {
		st.Status = state.StatusRunning
		st.SandboxID = "sb-sandbox-timeout-before-idle"
		st.ProcessPID = 42
		st.RunningAt = time.Now().Add(-10 * time.Minute).UTC()
		if err := store.WriteState(st); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(time.Millisecond)
	if _, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "idle-before-timeout",
		Source:             "test",
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerGroup:        "default",
		RunnerName:         "e2b-idle-before-timeout",
	}, nil); err != nil {
		t.Fatal(err)
	} else {
		st.Status = state.StatusRunning
		st.SandboxID = "sb-idle-before-timeout"
		st.ProcessPID = 42
		st.RunningAt = time.Now().Add(-2 * time.Minute).UTC()
		if err := store.WriteState(st); err != nil {
			t.Fatal(err)
		}
	}

	fake := &fakeSandbox{}
	srv := newTestServerWithLimit(t, store, ghServer.URL, fake, 10)
	srv.cfg.RunnerIdleTimeout = time.Minute
	srv.cfg.SandboxTimeout = 5 * time.Minute
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.sweepOnce(ctx)
	}()
	select {
	case <-lookupStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for idle github runner lookup")
	}
	if fake.stoppedCount() != 1 {
		t.Fatalf("expected sandbox timeout to be handled before idle github lookup, got %d stops", fake.stoppedCount())
	}
	close(releaseLookup)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for sweeper")
	}
}

func TestSweeperStillStopsSandboxTimedOutRunnerWhenGitHubRunnerIsBusy(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/orgs/o/actions/runners":
			w.Write([]byte(`{"runners":[{"id":99,"name":"e2b-busy-timeout","status":"online","busy":true}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/orgs/o/actions/runners/99":
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(`{"message":"Bad request - Runner e2b-busy-timeout is currently running a job and cannot be deleted.","status":"422"}`))
		default:
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	if _, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "busy-timeout",
		Source:             "test",
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted", "e2b"},
		ProfileName:        "default",
		RunnerGroup:        "default",
		RunnerName:         "e2b-busy-timeout",
	}, nil); err != nil {
		t.Fatal(err)
	} else {
		st.Status = state.StatusRunning
		st.SandboxID = "sb-busy-timeout"
		st.ProcessPID = 42
		st.RunningAt = time.Now().Add(-10 * time.Minute).UTC()
		if err := store.WriteState(st); err != nil {
			t.Fatal(err)
		}
	}
	fake := &fakeSandbox{}
	srv := newTestServerWithLimit(t, store, ghServer.URL, fake, 10)
	srv.cfg.RunnerIdleTimeout = time.Minute
	srv.cfg.SandboxTimeout = time.Minute
	srv.sweepOnce(t.Context())
	if fake.stoppedCount() != 1 {
		t.Fatalf("expected sweeper to stop sandbox-timed-out busy runner, got %d stops", fake.stoppedCount())
	}
	got, err := store.ReadState("busy-timeout")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusStopping || got.FailureStage != "sandbox_timeout" || got.FailureReason != "timeout" {
		t.Fatalf("expected sandbox timeout cleanup retry, got %#v", got)
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
		AuthEncryptionKey:       "encryption-key",
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

func saveTestGitHubOAuthToken(t *testing.T, store state.Store, accountID int64, encryptionKey, token string) {
	t.Helper()
	encryptedToken, err := encryptSecret(token, encryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAccountSecret(state.AccountSecret{
		ScopeType:      state.AccountScopeTypeAccount,
		ScopeID:        accountID,
		KeyType:        state.AccountSecretTypeGitHubOAuthToken,
		EncryptedValue: encryptedToken,
	}); err != nil {
		t.Fatal(err)
	}
}

func testCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
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
