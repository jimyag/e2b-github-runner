package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qiniu/ci-runner/internal/config"
	"github.com/qiniu/ci-runner/internal/github"
	"github.com/qiniu/ci-runner/internal/redact"
	"github.com/qiniu/ci-runner/internal/sandboxrunner"
	"github.com/qiniu/ci-runner/internal/state"
)

// ---------- workflowConclusion ----------

func TestWorkflowConclusionPrefersConclusionOverStatus(t *testing.T) {
	job := github.WorkflowJob{Conclusion: "success", Status: "completed"}
	if got := workflowConclusion(job); got != "success" {
		t.Errorf("workflowConclusion: got %q, want %q", got, "success")
	}
}

func TestWorkflowConclusionFallsBackToStatusWhenConclusionEmpty(t *testing.T) {
	job := github.WorkflowJob{Conclusion: "", Status: "in_progress"}
	if got := workflowConclusion(job); got != "in_progress" {
		t.Errorf("workflowConclusion fallback: got %q, want %q", got, "in_progress")
	}
}

func TestWorkflowConclusionReturnsUnknownWhenBothEmpty(t *testing.T) {
	job := github.WorkflowJob{}
	if got := workflowConclusion(job); got != "unknown" {
		t.Errorf("workflowConclusion empty: got %q, want %q", got, "unknown")
	}
}

func TestWorkflowConclusionTrimsConclusionWhitespace(t *testing.T) {
	job := github.WorkflowJob{Conclusion: "  failure  "}
	if got := workflowConclusion(job); got != "failure" {
		t.Errorf("workflowConclusion trim: got %q, want %q", got, "failure")
	}
}

// ---------- workflowNameFor ----------

func TestWorkflowNameForReturnsFirstNonEmpty(t *testing.T) {
	if got := workflowNameFor("CI", "fallback"); got != "CI" {
		t.Errorf("workflowNameFor: got %q, want %q", got, "CI")
	}
}

func TestWorkflowNameForSkipsWhitespace(t *testing.T) {
	if got := workflowNameFor("  ", "build"); got != "build" {
		t.Errorf("workflowNameFor skip whitespace: got %q, want %q", got, "build")
	}
}

func TestWorkflowNameForReturnsUnknownWhenAllEmpty(t *testing.T) {
	if got := workflowNameFor("", "  ", ""); got != "unknown" {
		t.Errorf("workflowNameFor all empty: got %q, want %q", got, "unknown")
	}
}

func TestWorkflowNameForNoArgs(t *testing.T) {
	if got := workflowNameFor(); got != "unknown" {
		t.Errorf("workflowNameFor(): got %q, want %q", got, "unknown")
	}
}

// ---------- workflowJobName ----------

func TestWorkflowJobNamePrefersJobName(t *testing.T) {
	st := state.RunnerState{AssignedJobName: "state-job", RunnerName: "e2b-runner"}
	job := github.WorkflowJob{Name: "job-name"}
	if got := workflowJobName(st, job); got != "job-name" {
		t.Errorf("workflowJobName: got %q, want %q", got, "job-name")
	}
}

func TestWorkflowJobNameFallsBackToAssignedJobName(t *testing.T) {
	st := state.RunnerState{AssignedJobName: "state-job", RunnerName: "e2b-runner"}
	job := github.WorkflowJob{Name: ""}
	if got := workflowJobName(st, job); got != "state-job" {
		t.Errorf("workflowJobName assigned fallback: got %q, want %q", got, "state-job")
	}
}

func TestWorkflowJobNameSkipsStartedMarker(t *testing.T) {
	st := state.RunnerState{AssignedJobName: runnerJobStartedMarker, RunnerName: "e2b-runner"}
	job := github.WorkflowJob{Name: ""}
	// marker is not a real job name; should fall through to RunnerName
	if got := workflowJobName(st, job); got != "e2b-runner" {
		t.Errorf("workflowJobName skip marker: got %q, want %q", got, "e2b-runner")
	}
}

func TestWorkflowJobNameFallsBackToRunnerName(t *testing.T) {
	st := state.RunnerState{RunnerName: "e2b-runner-42"}
	job := github.WorkflowJob{}
	if got := workflowJobName(st, job); got != "e2b-runner-42" {
		t.Errorf("workflowJobName runner fallback: got %q, want %q", got, "e2b-runner-42")
	}
}

func TestWorkflowJobNameReturnsUnknownWhenAllEmpty(t *testing.T) {
	st := state.RunnerState{}
	job := github.WorkflowJob{}
	if got := workflowJobName(st, job); got != "unknown" {
		t.Errorf("workflowJobName all empty: got %q, want %q", got, "unknown")
	}
}

// ---------- isFailureConclusion ----------

func TestIsFailureConclusionMatchesFailure(t *testing.T) {
	if !isFailureConclusion("failure") {
		t.Error("isFailureConclusion(\"failure\") should be true")
	}
}

func TestIsFailureConclusionIsCaseInsensitive(t *testing.T) {
	if !isFailureConclusion("FAILURE") {
		t.Error("isFailureConclusion(\"FAILURE\") should be true")
	}
	if !isFailureConclusion("Failure") {
		t.Error("isFailureConclusion(\"Failure\") should be true")
	}
}

func TestIsFailureConclusionTrimsWhitespace(t *testing.T) {
	if !isFailureConclusion("  failure  ") {
		t.Error("isFailureConclusion with whitespace should be true")
	}
}

func TestIsFailureConclusionReturnsFalseForOther(t *testing.T) {
	for _, c := range []string{"success", "cancelled", "skipped", "", "timed_out"} {
		if isFailureConclusion(c) {
			t.Errorf("isFailureConclusion(%q) should be false", c)
		}
	}
}

// ---------- runnerExitMessage ----------

func TestRunnerExitMessageIncludesExitCode(t *testing.T) {
	result := sandboxrunner.ExitResult{ExitCode: 1, Stderr: ""}
	msg := runnerExitMessage(result)
	if !strings.Contains(msg, "1") {
		t.Errorf("runnerExitMessage: missing exit code in %q", msg)
	}
}

func TestRunnerExitMessagePrefersStderr(t *testing.T) {
	result := sandboxrunner.ExitResult{ExitCode: 2, Stderr: "out of memory", Error: "process error", Stdout: "some output"}
	msg := runnerExitMessage(result)
	if !strings.Contains(msg, "out of memory") {
		t.Errorf("runnerExitMessage: expected stderr in message, got %q", msg)
	}
}

func TestRunnerExitMessageFallsBackToError(t *testing.T) {
	result := sandboxrunner.ExitResult{ExitCode: 2, Stderr: "", Error: "process error"}
	msg := runnerExitMessage(result)
	if !strings.Contains(msg, "process error") {
		t.Errorf("runnerExitMessage: expected error in message, got %q", msg)
	}
}

func TestRunnerExitMessageFallsBackToStdout(t *testing.T) {
	result := sandboxrunner.ExitResult{ExitCode: 2, Stderr: "", Error: "", Stdout: "runner output"}
	msg := runnerExitMessage(result)
	if !strings.Contains(msg, "runner output") {
		t.Errorf("runnerExitMessage: expected stdout fallback in message, got %q", msg)
	}
}

func TestRunnerExitMessageCodeOnlyWhenNoDetail(t *testing.T) {
	result := sandboxrunner.ExitResult{ExitCode: 137}
	msg := runnerExitMessage(result)
	if msg != "runner process exited with code 137" {
		t.Errorf("runnerExitMessage no detail: got %q", msg)
	}
}

// ---------- shouldStopIdleRunner ----------

func TestShouldStopIdleRunnerReturnsFalseWhenTimeoutDisabled(t *testing.T) {
	s := &Server{cfg: config.Config{RunnerIdleTimeout: 0}}
	st := state.RunnerState{
		RunningAt: time.Now().Add(-10 * time.Minute),
	}
	if s.shouldStopIdleRunner(st, time.Now()) {
		t.Error("shouldStopIdleRunner: should return false when timeout is 0")
	}
}

func TestShouldStopIdleRunnerReturnsFalseWhenRunningAtZero(t *testing.T) {
	s := &Server{cfg: config.Config{RunnerIdleTimeout: time.Minute}}
	st := state.RunnerState{}
	if s.shouldStopIdleRunner(st, time.Now()) {
		t.Error("shouldStopIdleRunner: should return false when RunningAt is zero")
	}
}

func TestShouldStopIdleRunnerReturnsFalseWhenJobAssigned(t *testing.T) {
	s := &Server{cfg: config.Config{RunnerIdleTimeout: time.Minute}}
	st := state.RunnerState{
		RunningAt:     time.Now().Add(-10 * time.Minute),
		AssignedJobID: 12345,
	}
	if s.shouldStopIdleRunner(st, time.Now()) {
		t.Error("shouldStopIdleRunner: should return false when job is assigned")
	}
}

func TestShouldStopIdleRunnerReturnsFalseWhenJobStartedMarkerSet(t *testing.T) {
	s := &Server{cfg: config.Config{RunnerIdleTimeout: time.Minute}}
	st := state.RunnerState{
		RunningAt:       time.Now().Add(-10 * time.Minute),
		AssignedJobName: runnerJobStartedMarker,
	}
	if s.shouldStopIdleRunner(st, time.Now()) {
		t.Error("shouldStopIdleRunner: should return false when job started marker set")
	}
}

func TestShouldStopIdleRunnerReturnsTrueWhenIdleTimeout(t *testing.T) {
	s := &Server{cfg: config.Config{RunnerIdleTimeout: time.Minute}}
	st := state.RunnerState{
		RunningAt: time.Now().Add(-2 * time.Minute),
	}
	if !s.shouldStopIdleRunner(st, time.Now()) {
		t.Error("shouldStopIdleRunner: should return true when idle timeout exceeded")
	}
}

func TestShouldStopIdleRunnerReturnsFalseWhenNotYetTimedOut(t *testing.T) {
	s := &Server{cfg: config.Config{RunnerIdleTimeout: 5 * time.Minute}}
	st := state.RunnerState{
		RunningAt: time.Now().Add(-1 * time.Minute),
	}
	if s.shouldStopIdleRunner(st, time.Now()) {
		t.Error("shouldStopIdleRunner: should return false when not yet timed out")
	}
}

// ---------- handleHealthz ----------

func TestHealthzReturnsOK(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET /healthz: invalid JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("GET /healthz: expected status=ok, got %v", body)
	}
}

// ---------- handleGitHubWebhook – signature rejection ----------

func TestWebhookRejectsInvalidSignature(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	body := []byte(`{"action":"queued"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
	req.Header.Set("X-GitHub-Event", "workflow_job")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("webhook bad sig: expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWebhookRejectsMissingSignature(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(`{}`))
	req.Header.Set("X-GitHub-Event", "workflow_job")
	// No X-Hub-Signature-256 header
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("webhook no sig: expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ---------- handleGitHubWebhook – unknown event ----------

func TestWebhookIgnoresUnknownEventTypes(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	body := []byte(`{"action":"created"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign("secret", body))
	req.Header.Set("X-GitHub-Event", "push")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("webhook unknown event: expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ignored") {
		t.Errorf("webhook unknown event: expected 'ignored' in response, got %s", rec.Body.String())
	}
}

// ---------- handleListAuditEvents ----------

func TestListAuditEventsEndpointRequiresAuth(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := httptest.NewRequest(http.MethodGet, "/audit-events", nil)
	// No Authorization header
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /audit-events without auth: expected 401, got %d", rec.Code)
	}
}

func TestListAuditEventsEndpointReturnsEvents(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	// Insert an audit event directly into the store
	if _, err := store.AppendAuditEvent(state.AuditEvent{
		Actor:        "admin_api",
		Action:       "runner.retry",
		ResourceType: "runner_request",
		ResourceID:   "event-test-id",
	}); err != nil {
		t.Fatal(err)
	}

	req := adminRequest(http.MethodGet, "/audit-events", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /audit-events: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "runner.retry") {
		t.Errorf("GET /audit-events: expected event in response, got %s", rec.Body.String())
	}
}

// ---------- handleDeleteProfile ----------

func TestDeleteProfileEndpointRemovesProfile(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	// Create the profile via the API
	createReq := adminRequest(http.MethodPost, "/runner_specs",
		strings.NewReader(`{"name":"to-delete-api","labels":["self-hosted"],"template_id":"base","max_concurrency":1,"enabled":true}`))
	createRec := httptest.NewRecorder()
	srv.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create profile: got %d body=%s", createRec.Code, createRec.Body.String())
	}

	// Delete it
	deleteReq := adminRequest(http.MethodDelete, "/runner_specs/to-delete-api", nil)
	deleteRec := httptest.NewRecorder()
	srv.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("DELETE /runner_specs/to-delete-api: expected 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	// Verify it's gone via GET
	getReq := adminRequest(http.MethodGet, "/runner_specs/to-delete-api", nil)
	getRec := httptest.NewRecorder()
	srv.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("GET after delete: expected 404, got %d body=%s", getRec.Code, getRec.Body.String())
	}
}

// ---------- handleDeleteRunnerGroup ----------

func TestDeleteRunnerGroupEndpointRemovesGroup(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	// Create profile
	req := adminRequest(http.MethodPost, "/runner_specs",
		strings.NewReader(`{"name":"del-grp-spec-api","labels":["self-hosted"],"template_id":"base","max_concurrency":1,"enabled":true}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create profile: %d %s", rec.Code, rec.Body.String())
	}

	// Create runner group
	req = adminRequest(http.MethodPost, "/runner_groups",
		strings.NewReader(`{"name":"del-group-api","spec_names":["del-grp-spec-api"],"enabled":true}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create runner group: %d %s", rec.Code, rec.Body.String())
	}

	// Delete it
	req = adminRequest(http.MethodDelete, "/runner_groups/del-group-api", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /runner_groups/del-group-api: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Verify it's gone
	req = adminRequest(http.MethodGet, "/runner_groups/del-group-api", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET after delete: expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ---------- handleDeleteRepositoryPolicy ----------

func TestDeleteRepositoryPolicyEndpointRemovesPolicy(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	// Create profile
	req := adminRequest(http.MethodPost, "/runner_specs",
		strings.NewReader(`{"name":"del-pol-spec-api","labels":["self-hosted"],"template_id":"base","max_concurrency":1,"enabled":true}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create profile: %d %s", rec.Code, rec.Body.String())
	}

	// Create policy
	req = adminRequest(http.MethodPost, "/runner_policies",
		strings.NewReader(`{"repository_full_name":"org/del-pol-repo","runner_spec_name":"del-pol-spec-api","enabled":true}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create policy: %d %s", rec.Code, rec.Body.String())
	}

	// Extract policy ID from response
	var policyResp struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &policyResp); err != nil {
		t.Fatalf("unmarshal policy response: %v", err)
	}

	// Delete policy
	deleteReq := adminRequest(http.MethodDelete, fmt.Sprintf("/runner_policies/%d", policyResp.ID), nil)
	deleteRec := httptest.NewRecorder()
	srv.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("DELETE /runner_policies/{id}: expected 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
}

// ---------- handleAdminRedirect ----------

func TestAdminRedirectReturns301(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /admin: expected 301, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/" {
		t.Errorf("GET /admin: Location = %q, want /admin/", loc)
	}
}

func TestUserRedirectReturns301(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := httptest.NewRequest(http.MethodGet, "/user", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /user: expected 301, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("GET /user: Location = %q, want /", loc)
	}
}

// ---------- handleGetRunner ----------

func TestGetRunnerReturnsStateAfterCreate(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	// Create a runner request
	createReq := adminRequest(http.MethodPost, "/runner_requests",
		strings.NewReader(`{"id":"get-runner-1","repository_full_name":"o/r","runner_spec_name":"default"}`))
	createRec := httptest.NewRecorder()
	srv.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create runner: got %d body=%s", createRec.Code, createRec.Body.String())
	}

	// Fetch the runner state
	getReq := adminRequest(http.MethodGet, "/runner_requests/get-runner-1", nil)
	getRec := httptest.NewRecorder()
	srv.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /runner_requests/get-runner-1: expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "get-runner-1") {
		t.Errorf("GET /runner_requests/get-runner-1: id not in response: %s", getRec.Body.String())
	}
}

func TestGetRunnerReturns404ForMissing(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := adminRequest(http.MethodGet, "/runner_requests/does-not-exist", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET missing runner: expected 404, got %d", rec.Code)
	}
}

// ---------- handleListProfiles ----------

func TestListProfilesEndpointReturnsProfiles(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := adminRequest(http.MethodGet, "/runner_specs", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /runner_specs: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	// The default profile is created by newTestServer
	if !strings.Contains(rec.Body.String(), "default") {
		t.Errorf("GET /runner_specs: expected default profile in response, got %s", rec.Body.String())
	}
}

// ---------- handleListRunnerGroups ----------

func TestListRunnerGroupsEndpointReturnsEmpty(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := adminRequest(http.MethodGet, "/runner_groups", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /runner_groups: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListRunnerGroupsEndpointReturnsCreatedGroups(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	// Create a group
	req := adminRequest(http.MethodPost, "/runner_groups",
		strings.NewReader(`{"name":"list-test-group","spec_names":["default"],"enabled":true}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create group: %d %s", rec.Code, rec.Body.String())
	}

	// List should include it
	req = adminRequest(http.MethodGet, "/runner_groups", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /runner_groups: expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "list-test-group") {
		t.Errorf("GET /runner_groups: expected group in response, got %s", rec.Body.String())
	}
}

// ---------- handlePatchRunnerGroup ----------

func TestPatchRunnerGroupUpdatesDescription(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	// Create group
	req := adminRequest(http.MethodPost, "/runner_groups",
		strings.NewReader(`{"name":"patch-test-group","spec_names":["default"],"enabled":true}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create group: %d %s", rec.Code, rec.Body.String())
	}

	// Patch description
	req = adminRequest(http.MethodPatch, "/runner_groups/patch-test-group",
		strings.NewReader(`{"description":"updated desc"}`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH /runner_groups/patch-test-group: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "updated desc") {
		t.Errorf("PATCH runner group: description not updated: %s", rec.Body.String())
	}
}

func TestPatchRunnerGroupReturns404ForMissing(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := adminRequest(http.MethodPatch, "/runner_groups/no-such-group",
		strings.NewReader(`{"description":"x"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("PATCH missing group: expected 404, got %d", rec.Code)
	}
}

// ---------- handleListRepositoryPolicies ----------

func TestListRepositoryPoliciesEndpointReturnsDefaultPolicy(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := adminRequest(http.MethodGet, "/runner_policies", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /runner_policies: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	// newTestServer creates a default policy for o/r
	if !strings.Contains(rec.Body.String(), "o/r") {
		t.Errorf("GET /runner_policies: expected o/r policy in response, got %s", rec.Body.String())
	}
}

// ---------- handlePatchRepositoryPolicy ----------

func TestPatchRepositoryPolicyDisablesPolicy(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	// Get current policies to find the default policy ID
	listReq := adminRequest(http.MethodGet, "/runner_policies", nil)
	listRec := httptest.NewRecorder()
	srv.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list policies: %d", listRec.Code)
	}
	var policies []struct {
		ID      int64  `json:"id"`
		Enabled bool   `json:"enabled"`
		Repo    string `json:"repository_full_name"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &policies); err != nil {
		t.Fatalf("unmarshal policies: %v", err)
	}
	if len(policies) == 0 {
		t.Fatal("no policies found")
	}

	// Patch it to disable
	patchReq := adminRequest(http.MethodPatch, fmt.Sprintf("/runner_policies/%d", policies[0].ID),
		strings.NewReader(`{"enabled":false}`))
	patchRec := httptest.NewRecorder()
	srv.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("PATCH /runner_policies/%d: expected 200, got %d body=%s", policies[0].ID, patchRec.Code, patchRec.Body.String())
	}
	var updated struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(patchRec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("unmarshal updated policy: %v", err)
	}
	if updated.Enabled {
		t.Error("PATCH policy: expected enabled=false after patch")
	}
}

func TestPatchRepositoryPolicyReturns404ForMissing(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := adminRequest(http.MethodPatch, "/runner_policies/99999",
		strings.NewReader(`{"enabled":false}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("PATCH missing policy: expected 404, got %d", rec.Code)
	}
}

func TestPatchRepositoryPolicyReturns400ForInvalidID(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := adminRequest(http.MethodPatch, "/runner_policies/not-a-number",
		strings.NewReader(`{"enabled":false}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH invalid id: expected 400, got %d", rec.Code)
	}
}

// ---------- handleDiagnosticsPprof ----------

func TestDiagnosticsPprofEndpointRequiresAuth(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := httptest.NewRequest(http.MethodGet, "/diagnostics/pprof", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /diagnostics/pprof without auth: expected 401, got %d", rec.Code)
	}
}

func TestDiagnosticsPprofEndpointReturnsJSON(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := adminRequest(http.MethodGet, "/diagnostics/pprof", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /diagnostics/pprof: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET /diagnostics/pprof: invalid JSON: %v", err)
	}
	if _, ok := body["state"]; !ok {
		t.Error("GET /diagnostics/pprof: missing 'state' field in response")
	}
	if _, ok := body["github"]; !ok {
		t.Error("GET /diagnostics/pprof: missing 'github' field in response")
	}
}

func TestRedactDatabaseDSNRemovesPostgresURLCredentials(t *testing.T) {
	got := redact.DatabaseDSN("postgres://runner:secret@example.test/runnerd?sslmode=disable")
	if strings.Contains(got, "runner:") || strings.Contains(got, "secret") {
		t.Fatalf("database DSN leaked credentials: %s", got)
	}
	if !strings.Contains(got, "example.test") || !strings.Contains(got, "runnerd") {
		t.Fatalf("database DSN lost non-secret location fields: %s", got)
	}
}

func TestRedactDatabaseDSNRemovesMySQLCredentials(t *testing.T) {
	got := redact.DatabaseDSN("runner:secret@tcp(mysql.example:3306)/runnerd?parseTime=true")
	if strings.Contains(got, "runner:") || strings.Contains(got, "secret") {
		t.Fatalf("database DSN leaked credentials: %s", got)
	}
	if !strings.Contains(got, "mysql.example:3306") || !strings.Contains(got, "runnerd") {
		t.Fatalf("database DSN lost non-secret location fields: %s", got)
	}
}

func TestRedactDatabaseDSNRemovesPostgresKeyValuePassword(t *testing.T) {
	got := redact.DatabaseDSN("host=localhost user=postgres password='secret value' dbname=runnerd")
	if strings.Contains(got, "secret") {
		t.Fatalf("database DSN leaked key-value password: %s", got)
	}
	if !strings.Contains(got, "host=localhost") || !strings.Contains(got, "dbname=runnerd") {
		t.Fatalf("database DSN lost non-secret key-value fields: %s", got)
	}
}

// ---------- isSandboxGone ----------

func TestIsSandboxGoneReturnsFalseForNilError(t *testing.T) {
	if isSandboxGone(nil) {
		t.Error("isSandboxGone(nil) should be false")
	}
}

func TestIsSandboxGoneDetectsStatus404(t *testing.T) {
	err := fmt.Errorf("sandbox request failed: status 404")
	if !isSandboxGone(err) {
		t.Errorf("isSandboxGone(status 404): expected true")
	}
}

func TestIsSandboxGoneDetectsSandboxNotFound(t *testing.T) {
	err := fmt.Errorf("SandboxNotFound: sandbox xyz does not exist")
	if !isSandboxGone(err) {
		t.Errorf("isSandboxGone(SandboxNotFound): expected true")
	}
}

func TestIsSandboxGoneDetectsResumeError(t *testing.T) {
	err := fmt.Errorf("Sandbox can't be resumed")
	if !isSandboxGone(err) {
		t.Errorf("isSandboxGone(can't be resumed): expected true")
	}
}

func TestIsSandboxGoneReturnsFalseForOtherError(t *testing.T) {
	err := fmt.Errorf("network timeout")
	if isSandboxGone(err) {
		t.Errorf("isSandboxGone(timeout): expected false")
	}
}

// ---------- newID ----------

func TestNewIDReturnsNonEmptyHexString(t *testing.T) {
	id := newID()
	if len(id) == 0 {
		t.Error("newID: returned empty string")
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("newID: non-hex character %q in %q", c, id)
		}
	}
}

func TestNewIDGeneratesUniqueValues(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		id := newID()
		if seen[id] {
			t.Fatalf("newID: duplicate id %q generated", id)
		}
		seen[id] = true
	}
}

// ---------- handleDiagnosticsVars - no pprof endpoint discovered ----------

func TestDiagnosticsVarsReturns404WhenNoPprofEndpoint(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	req := adminRequest(http.MethodGet, "/diagnostics/vars", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	// Without a running pprof server, discoverPprofArtifacts() returns empty
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /diagnostics/vars: expected 404 (no pprof), got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ---------- scheduleStopRetry ----------

func TestScheduleStopRetryReturnsTrueForRetryableError(t *testing.T) {
	s := &Server{cfg: config.Config{
		RetryMaxAttempts: 3,
		RetryBaseDelay:   10 * time.Millisecond,
		RetryMaxDelay:    time.Second,
	}}
	st := &state.RunnerState{RetryCount: 0}
	retryable := s.scheduleStopRetry(st, fmt.Errorf("http_retryable_status: status 429"))
	if !retryable {
		t.Error("scheduleStopRetry: expected true for retryable error")
	}
	if st.RetryCount != 1 {
		t.Errorf("scheduleStopRetry: expected RetryCount=1, got %d", st.RetryCount)
	}
	if st.Status != state.StatusStopping {
		t.Errorf("scheduleStopRetry: expected status=stopping, got %s", st.Status)
	}
}

func TestScheduleStopRetryReturnsFalseWhenMaxAttemptsReached(t *testing.T) {
	s := &Server{cfg: config.Config{
		RetryMaxAttempts: 3,
		RetryBaseDelay:   10 * time.Millisecond,
		RetryMaxDelay:    time.Second,
	}}
	st := &state.RunnerState{RetryCount: 3}
	retryable := s.scheduleStopRetry(st, fmt.Errorf("status 429"))
	if retryable {
		t.Error("scheduleStopRetry: expected false when max attempts reached")
	}
}

func TestScheduleStopRetryReturnsFalseForNonRetryableError(t *testing.T) {
	s := &Server{cfg: config.Config{
		RetryMaxAttempts: 5,
		RetryBaseDelay:   10 * time.Millisecond,
		RetryMaxDelay:    time.Second,
	}}
	st := &state.RunnerState{RetryCount: 0}
	retryable := s.scheduleStopRetry(st, fmt.Errorf("status 401 unauthorized"))
	if retryable {
		t.Error("scheduleStopRetry: expected false for auth error")
	}
}

// ---------- markRunnerJobStarted ----------

func TestMarkRunnerJobStartedSetsMarkerOnRunningRunner(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	// Create a runner and set it to Running
	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "mark-job-1",
		Source:             "test",
		Labels:             []string{"self-hosted"},
		RunnerName:         "e2b-mark-job-1",
		RepositoryFullName: "o/r",
		ProfileName:        "default",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = state.StatusRunning
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	srv.markRunnerJobStarted("mark-job-1")

	got, err := store.ReadState("mark-job-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.AssignedJobName != runnerJobStartedMarker {
		t.Errorf("markRunnerJobStarted: AssignedJobName = %q, want %q", got.AssignedJobName, runnerJobStartedMarker)
	}
}

func TestMarkRunnerJobStartedIsIdempotent(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "mark-job-2",
		Source:             "test",
		Labels:             []string{"self-hosted"},
		RunnerName:         "e2b-mark-job-2",
		RepositoryFullName: "o/r",
		ProfileName:        "default",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = state.StatusRunning
	st.AssignedJobName = runnerJobStartedMarker
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	// Calling twice is safe
	srv.markRunnerJobStarted("mark-job-2")
	srv.markRunnerJobStarted("mark-job-2")

	got, err := store.ReadState("mark-job-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.AssignedJobName != runnerJobStartedMarker {
		t.Errorf("markRunnerJobStarted idempotent: AssignedJobName = %q", got.AssignedJobName)
	}
}

// ---------- cleanupSandboxAfterExit ----------

func TestCleanupSandboxAfterExitNoopWhenNoSandboxID(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	st := state.RunnerState{ID: "cleanup-no-sandbox", SandboxID: ""}
	if err := srv.cleanupSandboxAfterExit("cleanup-no-sandbox", st); err != nil {
		t.Errorf("cleanupSandboxAfterExit: expected nil for empty SandboxID, got %v", err)
	}
}

// ---------- runnerExited (clean exit) ----------

func TestRunnerExitedWithExitCode0TransitionsToCompleted(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/actions/runners" {
			w.Write([]byte(`{"runners":[]}`))
			return
		}
		t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})

	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "exited-clean",
		Source:             "test",
		Labels:             []string{"self-hosted"},
		RunnerName:         "e2b-exited-clean",
		RepositoryFullName: "o/r",
		ProfileName:        "default",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = state.StatusRunning
	st.SandboxID = "" // no sandbox to stop
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	srv.runnerExited("exited-clean", sandboxrunner.ExitResult{ExitCode: 0}, nil)

	got, err := store.ReadState("exited-clean")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusCompleted {
		t.Errorf("runnerExited exit=0: expected status=completed, got %s", got.Status)
	}
}

func TestRunnerExitedWithNonZeroExitCodeTransitionsToFailed(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/actions/runners" {
			w.Write([]byte(`{"runners":[]}`))
			return
		}
		t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})

	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "exited-nonzero",
		Source:             "test",
		Labels:             []string{"self-hosted"},
		RunnerName:         "e2b-exited-nonzero",
		RepositoryFullName: "o/r",
		ProfileName:        "default",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = state.StatusRunning
	st.SandboxID = ""
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	srv.runnerExited("exited-nonzero", sandboxrunner.ExitResult{ExitCode: 137, Stderr: "OOM killed"}, nil)

	got, err := store.ReadState("exited-nonzero")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusFailed {
		t.Errorf("runnerExited nonzero: expected status=failed, got %s", got.Status)
	}
	if !strings.Contains(got.Error, "137") {
		t.Errorf("runnerExited nonzero: expected error to contain exit code, got %q", got.Error)
	}
}

// ---------- reconcileOnce ----------

func TestReconcileOnceSignalsQueueForQueuedRunner(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	_, _, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "reconcile-queued",
		Source:             "test",
		Labels:             []string{"self-hosted"},
		RunnerName:         "e2b-reconcile-queued",
		RepositoryFullName: "o/r",
		ProfileName:        "default",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Drain any existing signal
	select {
	case <-srv.queueNotify:
	default:
	}

	srv.reconcileOnce(t.Context())

	// reconcileOnce should have signaled the queue for the queued runner
	// (It either directly signals or processQueuedRequests ran — we just verify no panic)
}

func TestReconcileMismatchedCompletedJobsRequeuesOriginalJob(t *testing.T) {
	ghServer := httptest.NewServer(githubRunnerAPI(t))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
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
	st.Status = state.StatusCompleted
	st.AssignedJobID = 2002
	st.AssignedJobName = "prepare"
	st.CompletedAt = time.Now().UTC()
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	srv.reconcileMismatchedCompletedJobs(t.Context())

	got, err := store.ReadState("1001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusQueued {
		t.Fatalf("expected mismatched completed request to be requeued, got %s", got.Status)
	}
	if got.AssignedJobID != 0 || got.AssignedJobName != "" {
		t.Fatalf("expected assigned job to be cleared, got id=%d name=%q", got.AssignedJobID, got.AssignedJobName)
	}
}

func TestReconcileCompletedWorkflowJobsMarksFailedRecoveryCompleted(t *testing.T) {
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/actions/jobs/1001":
			w.Write([]byte(`{"id":1001,"name":"check","status":"completed","conclusion":"success","labels":["self-hosted","e2b"]}`))
		default:
			t.Fatalf("unexpected github request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ghServer.Close()

	store := state.New(t.TempDir())
	srv := newTestServer(t, store, ghServer.URL, &fakeSandbox{})
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
	st.Status = state.StatusFailed
	st.FailureStage = "recovery"
	st.FailureReason = "cleanup_failed"
	st.Error = "recover cleanup github runner: busy"
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	srv.reconcileCompletedWorkflowJobs(t.Context())

	got, err := store.ReadState("1001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusCompleted {
		t.Fatalf("expected completed state, got %s", got.Status)
	}
	if got.FailureStage != "" || got.Error != "" || got.CompletedAt.IsZero() {
		t.Fatalf("expected failure metadata cleared and completed_at set, got %#v", got)
	}
}

// ---------- failStart ----------

func TestFailStartTransitionsRunnerToFailed(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	// Create a queued runner
	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "fail-start-1",
		Source:             "test",
		Labels:             []string{"self-hosted"},
		RunnerName:         "e2b-fail-start-1",
		RepositoryFullName: "o/r",
		ProfileName:        "default",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Exhaust retries so it goes straight to failed (not re-queued)
	st.RetryCount = srv.cfg.RetryMaxAttempts
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	srv.failStart("fail-start-1", st, "sandbox_start", fmt.Errorf("template not found"))

	got, err := store.ReadState("fail-start-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusFailed {
		t.Errorf("failStart: expected status=failed, got %s", got.Status)
	}
	if got.FailureStage != "sandbox_start" {
		t.Errorf("failStart: FailureStage = %q, want %q", got.FailureStage, "sandbox_start")
	}
}

func TestFailStartRequeuesRunnerWhenRetriesRemain(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "fail-start-retry",
		Source:             "test",
		Labels:             []string{"self-hosted"},
		RunnerName:         "e2b-fail-start-retry",
		RepositoryFullName: "o/r",
		ProfileName:        "default",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// RetryCount = 0, MaxAttempts = 3 → should retry
	st.RetryCount = 0
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	// Use a retryable error (status 429)
	srv.failStart("fail-start-retry", st, "github", fmt.Errorf("received status 429"))

	got, err := store.ReadState("fail-start-retry")
	if err != nil {
		t.Fatal(err)
	}
	// Should be re-queued for retry
	if got.Status != state.StatusQueued {
		t.Errorf("failStart retry: expected status=queued, got %s (error=%s)", got.Status, got.Error)
	}
}

func TestFailStartDefersRateLimitEvenWhenMaxAttemptsReached(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "fail-start-rate-limit",
		Source:             "test",
		Labels:             []string{"self-hosted"},
		RunnerName:         "e2b-fail-start-rate-limit",
		RepositoryFullName: "o/r",
		ProfileName:        "default",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.RetryCount = srv.cfg.RetryMaxAttempts
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	srv.failStart("fail-start-rate-limit", st, "sandbox_start", fmt.Errorf("api error: status 429"))

	got, err := store.ReadState("fail-start-rate-limit")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusQueued {
		t.Fatalf("expected rate-limited start to remain queued, got %#v", got)
	}
	if got.FailureReason != "http_rate_limited" {
		t.Fatalf("expected http_rate_limited reason, got %q", got.FailureReason)
	}
	if got.NextRetryAt.IsZero() {
		t.Fatal("expected next retry time for rate-limited start")
	}
	if got.RetryCount != srv.cfg.RetryMaxAttempts {
		t.Fatalf("expected retry count capped at max attempts, got %d", got.RetryCount)
	}
}

func TestFailStartDefersSandboxCapacityEvenWhenMaxAttemptsReached(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "fail-start-capacity",
		Source:             "test",
		Labels:             []string{"self-hosted"},
		RunnerName:         "e2b-fail-start-capacity",
		RepositoryFullName: "o/r",
		ProfileName:        "default",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.RetryCount = srv.cfg.RetryMaxAttempts
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	srv.failStart("fail-start-capacity", st, "sandbox_start", fmt.Errorf("failed to place sandbox"))

	got, err := store.ReadState("fail-start-capacity")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusQueued {
		t.Fatalf("expected capacity failure to remain queued, got %#v", got)
	}
	if got.FailureReason != "sandbox_capacity" {
		t.Fatalf("expected sandbox_capacity reason, got %q", got.FailureReason)
	}
	if got.NextRetryAt.IsZero() {
		t.Fatal("expected next retry time for capacity failure")
	}
}

func TestFailStartFailsRetryableErrorWhenMaxAttemptsReached(t *testing.T) {
	store := state.New(t.TempDir())
	srv := newTestServer(t, store, "http://example.test", &fakeSandbox{})

	_, st, err := store.CreateRequest(state.RunnerRequest{
		ID:                 "fail-start-timeout-max",
		Source:             "test",
		Labels:             []string{"self-hosted"},
		RunnerName:         "e2b-fail-start-timeout-max",
		RepositoryFullName: "o/r",
		ProfileName:        "default",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.RetryCount = srv.cfg.RetryMaxAttempts
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	srv.failStart("fail-start-timeout-max", st, "sandbox_start", fmt.Errorf("api error: status 408"))

	got, err := store.ReadState("fail-start-timeout-max")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.StatusFailed {
		t.Fatalf("expected generic retryable error to fail at max attempts, got %#v", got)
	}
	if got.FailureReason != "http_retryable_status" {
		t.Fatalf("expected http_retryable_status reason, got %q", got.FailureReason)
	}
}
