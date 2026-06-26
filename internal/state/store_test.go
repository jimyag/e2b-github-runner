package state

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSQLiteStoreUsesWALAndBusyTimeout(t *testing.T) {
	store := NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseURL:    t.TempDir() + "/runnerd.db",
		MigrateOnStart: true,
	}).(*DBStore)
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	db, err := store.dbOrEnsure()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if got := sqlDB.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", got)
	}

	var journalMode string
	if err := db.Raw("PRAGMA journal_mode").Scan(&journalMode).Error; err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	var busyTimeout int
	if err := db.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 15000 {
		t.Fatalf("busy_timeout = %d, want 15000", busyTimeout)
	}
}

func TestCreateRequestIsIdempotent(t *testing.T) {
	store := New(t.TempDir())
	req := RunnerRequest{ID: "123", Source: "test", Labels: []string{"self-hosted"}, RunnerName: "e2b-123"}
	created, st, err := store.CreateRequest(req, []byte(`{"x":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if !created || st.Status != StatusQueued {
		t.Fatalf("unexpected first create: created=%v state=%#v", created, st)
	}

	created, st, err = store.CreateRequest(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("expected duplicate request to reuse existing row")
	}
	if st.ID != "123" {
		t.Fatalf("unexpected state id: %q", st.ID)
	}
}

func TestListMismatchedCompletedStates(t *testing.T) {
	store := New(t.TempDir())
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:         "mismatch",
		Source:     "test",
		JobID:      1001,
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-mismatch",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = StatusCompleted
	st.AssignedJobID = 2002
	st.AssignedJobName = "other"
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}
	_, matched, err := store.CreateRequest(RunnerRequest{
		ID:         "matched",
		Source:     "test",
		JobID:      3003,
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-matched",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	matched.Status = StatusCompleted
	matched.AssignedJobID = 3003
	if err := store.WriteState(matched); err != nil {
		t.Fatal(err)
	}

	states, err := store.ListMismatchedCompletedStates(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].ID != "mismatch" {
		t.Fatalf("expected only mismatched completed state, got %#v", states)
	}
}

func TestListFailedWorkflowJobStates(t *testing.T) {
	store := New(t.TempDir())
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:                 "failed-recovery",
		Source:             "test",
		JobID:              1001,
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted"},
		RunnerName:         "e2b-failed-recovery",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = StatusFailed
	st.FailureStage = "recovery"
	st.FailureReason = "cleanup_failed"
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}
	_, other, err := store.CreateRequest(RunnerRequest{
		ID:                 "failed-start",
		Source:             "test",
		JobID:              2002,
		RepositoryFullName: "o/r",
		Labels:             []string{"self-hosted"},
		RunnerName:         "e2b-failed-start",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	other.Status = StatusFailed
	other.FailureStage = "sandbox_start"
	if err := store.WriteState(other); err != nil {
		t.Fatal(err)
	}

	states, err := store.ListFailedWorkflowJobStates(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].ID != "failed-recovery" {
		t.Fatalf("expected only failed recovery workflow job state, got %#v", states)
	}
}

func TestUpsertUserMaintainsRoleByOAuthIdentity(t *testing.T) {
	store := New(t.TempDir())
	user, err := store.UpsertUser(User{OAuthProvider: "GitHub", OAuthSubject: "12345", OAuthLogin: "OctoCat", Role: "ADMIN"})
	if err != nil {
		t.Fatal(err)
	}
	if user.OAuthProvider != "github" || user.OAuthSubject != "12345" || user.OAuthLogin != "octocat" || user.Role != "admin" {
		t.Fatalf("unexpected created user: %#v", user)
	}

	updated, err := store.UpsertUser(User{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "renamed", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != user.ID || updated.OAuthLogin != "renamed" || updated.Role != "user" {
		t.Fatalf("unexpected updated user: %#v first=%#v", updated, user)
	}

	got, err := store.GetUserByOAuthIdentity("GITHUB", "12345")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != user.ID || got.Role != "user" {
		t.Fatalf("unexpected fetched user: %#v", got)
	}

	gitlab, err := store.UpsertUser(User{OAuthProvider: "gitlab", OAuthSubject: "12345", OAuthLogin: "octocat", Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if gitlab.ID == user.ID {
		t.Fatalf("expected provider to separate oauth identities: github=%#v gitlab=%#v", user, gitlab)
	}

	reusedLogin, err := store.UpsertUser(User{OAuthProvider: "github", OAuthSubject: "67890", OAuthLogin: "renamed", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	if reusedLogin.ID == user.ID {
		t.Fatalf("expected stable subject to separate reused login identities: first=%#v reused=%#v", user, reusedLogin)
	}
}

func TestEnsureUserCreatesDefaultWithoutOverwritingExistingRole(t *testing.T) {
	store := New(t.TempDir())
	created, err := store.EnsureUser(User{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "octocat", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Role != "user" {
		t.Fatalf("unexpected created role: %#v", created)
	}

	if _, err := store.UpsertUser(User{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "octocat", Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	ensured, err := store.EnsureUser(User{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "renamed", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	if ensured.ID != created.ID || ensured.OAuthLogin != "renamed" || ensured.Role != "admin" {
		t.Fatalf("ensure should preserve existing admin role, got %#v created=%#v", ensured, created)
	}
}

func TestUpsertUserRejectsInvalidRole(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.UpsertUser(User{OAuthProvider: "github", OAuthSubject: "12345", OAuthLogin: "octocat", Role: "owner"}); err == nil {
		t.Fatal("expected invalid role error")
	}
}

func TestWriteStateUsesVersionCAS(t *testing.T) {
	store := New(t.TempDir())
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:         "123",
		Source:     "test",
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-123",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	st.Status = StatusRunning
	st.ProcessPID = 42
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	st.Status = StatusFailed
	err = store.WriteState(st)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func TestListStatesAndActiveCount(t *testing.T) {
	store := New(t.TempDir())
	if _, _, err := store.CreateRequest(RunnerRequest{
		ID:         "active",
		Source:     "test",
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-active",
	}, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, st, err := store.CreateRequest(RunnerRequest{
		ID:         "done",
		Source:     "test",
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-done",
	}, nil); err != nil {
		t.Fatal(err)
	} else {
		st.Status = StatusCompleted
		if err := store.WriteState(st); err != nil {
			t.Fatal(err)
		}
	}

	states, err := store.ListStates()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 {
		t.Fatalf("expected 2 states, got %d", len(states))
	}
	if states[0].ID != "done" {
		t.Fatalf("expected newest state first, got %q", states[0].ID)
	}
	active, err := store.ActiveCount()
	if err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("expected 1 active runner, got %d", active)
	}
}

func TestListStatesPage(t *testing.T) {
	store := New(t.TempDir())
	for i := 0; i < 5; i++ {
		if _, _, err := store.CreateRequest(RunnerRequest{
			ID:         fmt.Sprintf("runner-%d", i),
			Source:     "test",
			Labels:     []string{"self-hosted"},
			RunnerName: fmt.Sprintf("e2b-runner-%d", i),
			CreatedAt:  time.Unix(int64(i), 0).UTC(),
		}, nil); err != nil {
			t.Fatal(err)
		}
	}

	paged, total, err := store.ListStatesPage(2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(paged) != 2 {
		t.Fatalf("len = %d, want 2", len(paged))
	}
	if paged[0].ID != "runner-3" || paged[1].ID != "runner-2" {
		t.Fatalf("unexpected page order: %#v", []string{paged[0].ID, paged[1].ID})
	}
}

func TestListActiveStatesExcludesTerminalStates(t *testing.T) {
	store := New(t.TempDir())
	for _, tc := range []struct {
		id     string
		status string
	}{
		{id: "queued", status: StatusQueued},
		{id: "running", status: StatusRunning},
		{id: "completed", status: StatusCompleted},
		{id: "failed", status: StatusFailed},
	} {
		if _, st, err := store.CreateRequest(RunnerRequest{
			ID:         tc.id,
			Source:     "test",
			Labels:     []string{"self-hosted"},
			RunnerName: "e2b-" + tc.id,
		}, nil); err != nil {
			t.Fatal(err)
		} else if tc.status != StatusQueued {
			st.Status = tc.status
			if err := store.WriteState(st); err != nil {
				t.Fatal(err)
			}
		}
	}

	states, err := store.ListActiveStates()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, st := range states {
		got[st.ID] = true
	}
	if !got["queued"] || !got["running"] {
		t.Fatalf("active states missing expected rows: %#v", got)
	}
	if got["completed"] || got["failed"] {
		t.Fatalf("active states included terminal rows: %#v", got)
	}
}

func TestReadLogCanReturnTail(t *testing.T) {
	store := New(t.TempDir())
	if _, _, err := store.CreateRequest(RunnerRequest{
		ID:         "123",
		Source:     "test",
		Labels:     []string{"self-hosted"},
		RunnerName: "e2b-123",
	}, nil); err != nil {
		t.Fatal(err)
	}
	store.AppendLog("123", "control.log", []byte("line-1\n"))
	store.AppendLog("123", "control.log", []byte("line-2\n"))
	data, err := store.ReadLog("123", "control.log", 7)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "line-2\n" {
		t.Fatalf("unexpected log tail: %q", string(data))
	}
}

func TestProfilesAndPoliciesCanBeMatched(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.UpsertProfile(RunnerProfile{
		Name:           "default",
		Labels:         []string{"self-hosted", "e2b"},
		TemplateID:     "base",
		MaxConcurrency: 10,
		Enabled:        true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertProfile(RunnerProfile{
		Name:           "large",
		Labels:         []string{"self-hosted", "e2b", "large"},
		TemplateID:     "large",
		MaxConcurrency: 2,
		Priority:       10,
		Enabled:        true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertRepositoryPolicy(RepositoryPolicy{
		RepositoryFullName: "owner/repo",
		ProfileName:        "default",
		Enabled:            true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertRepositoryPolicy(RepositoryPolicy{
		RepositoryFullName: "owner/repo",
		ProfileName:        "large",
		Enabled:            true,
	}); err != nil {
		t.Fatal(err)
	}

	match, err := store.MatchProfile("owner/repo", []string{"self-hosted", "e2b", "large"})
	if err != nil {
		t.Fatal(err)
	}
	if match.Profile == nil || match.Profile.Name != "large" {
		t.Fatalf("expected large profile match, got %#v", match.Profile)
	}
}

func TestMatchProfileReturnsReasonWhenPolicyMissing(t *testing.T) {
	store := New(t.TempDir())
	match, err := store.MatchProfile("owner/repo", []string{"self-hosted", "e2b"})
	if err != nil {
		t.Fatal(err)
	}
	if match.Profile != nil || match.Reason != "profile_not_allowed" {
		t.Fatalf("unexpected match result: %#v", match)
	}
}

func TestDefaultAvailableProfileMatchesWithoutRepositoryPolicy(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.UpsertProfile(RunnerProfile{
		Name:             "default",
		Labels:           []string{"self-hosted", "e2b"},
		TemplateID:       "base",
		MaxConcurrency:   10,
		Enabled:          true,
		DefaultAvailable: true,
	}); err != nil {
		t.Fatal(err)
	}
	match, err := store.MatchProfile("owner/any-repo", []string{"self-hosted", "e2b"})
	if err != nil {
		t.Fatal(err)
	}
	if match.Profile == nil || match.Profile.Name != "default" {
		t.Fatalf("expected default profile match, got %#v", match.Profile)
	}
}

func TestMatchProfileAllowsRunnerSpecWithAdditionalLabels(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.UpsertProfile(RunnerProfile{
		Name:             "default",
		Labels:           []string{"self-hosted", "e2b", "las-sandbox", "github-runner-ubuntu-24-04"},
		TemplateID:       "base",
		MaxConcurrency:   10,
		Enabled:          true,
		DefaultAvailable: true,
	}); err != nil {
		t.Fatal(err)
	}
	match, err := store.MatchProfile("owner/any-repo", []string{"github-runner-ubuntu-24-04"})
	if err != nil {
		t.Fatal(err)
	}
	if match.Profile == nil || match.Profile.Name != "default" {
		t.Fatalf("expected default profile match, got %#v", match.Profile)
	}
}

func TestRunnerGroupPolicyMatchesGroupSpecs(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.UpsertProfile(RunnerProfile{
		Name:           "small",
		Labels:         []string{"self-hosted", "e2b", "small"},
		TemplateID:     "small-template",
		MaxConcurrency: 10,
		Enabled:        true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertProfile(RunnerProfile{
		Name:           "large",
		Labels:         []string{"self-hosted", "e2b", "large"},
		TemplateID:     "large-template",
		MaxConcurrency: 5,
		Priority:       20,
		Enabled:        true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertRunnerGroup(RunnerGroup{
		Name:      "ubuntu-2404",
		SpecNames: []string{"small", "large"},
		Enabled:   true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertRepositoryPolicy(RepositoryPolicy{
		RepositoryFullName: "owner/repo",
		RunnerGroupName:    "ubuntu-2404",
		Enabled:            true,
	}); err != nil {
		t.Fatal(err)
	}

	match, err := store.MatchProfile("owner/repo", []string{"self-hosted", "e2b", "large"})
	if err != nil {
		t.Fatal(err)
	}
	if match.Profile == nil || match.Profile.Name != "large" {
		t.Fatalf("expected large profile through group policy, got %#v", match.Profile)
	}
}

func TestClaimNextRunnableHonorsLeaseAndRetryWindow(t *testing.T) {
	store := New(t.TempDir())
	now := time.Now().UTC()
	if _, st, err := store.CreateRequest(RunnerRequest{
		ID:              "retryable",
		Source:          "test",
		RequestedLabels: []string{"self-hosted"},
		Labels:          []string{"self-hosted"},
		RunnerName:      "e2b-retryable",
	}, nil); err != nil {
		t.Fatal(err)
	} else {
		st.NextRetryAt = now.Add(-time.Second)
		if err := store.WriteState(st); err != nil {
			t.Fatal(err)
		}
	}

	req, st, claimed, err := store.ClaimNextRunnable("worker-1", now, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed || req.ID != "retryable" || st.LeaseOwner != "worker-1" || st.LastAttemptAt.IsZero() {
		t.Fatalf("unexpected claim result: req=%#v state=%#v claimed=%v", req, st, claimed)
	}

	_, _, claimed, err = store.ClaimNextRunnable("worker-2", now, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("expected active lease to block second claim")
	}
}

func TestRetryRequestClearsFailureFields(t *testing.T) {
	store := New(t.TempDir())
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:              "retry-me",
		Source:          "test",
		RequestedLabels: []string{"self-hosted"},
		Labels:          []string{"self-hosted"},
		RunnerName:      "e2b-retry-me",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = StatusFailed
	st.FailureStage = "sandbox_start"
	st.FailureReason = "backend_server_error"
	st.LastErrorCode = "backend_server_error"
	st.LastErrorMessage = "temporary failure"
	st.LastErrorRetryable = true
	st.NextRetryAt = time.Now().Add(time.Minute).UTC()
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	retried, err := store.RetryRequest("retry-me", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != StatusQueued || retried.FailureStage != "" || retried.LastErrorCode != "" || !retried.NextRetryAt.IsZero() {
		t.Fatalf("unexpected retried state: %#v", retried)
	}
}

func TestRetryRequestRejectsActiveState(t *testing.T) {
	store := New(t.TempDir())
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:              "running",
		Source:          "test",
		RequestedLabels: []string{"self-hosted"},
		Labels:          []string{"self-hosted"},
		RunnerName:      "e2b-running",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Status = StatusRunning
	if err := store.WriteState(st); err != nil {
		t.Fatal(err)
	}

	if _, err := store.RetryRequest("running", time.Now().UTC()); !errors.Is(err, ErrRetryNotAllowed) {
		t.Fatalf("expected retry guard, got %v", err)
	}
}

func TestAuditEventsCanBeListed(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.AppendAuditEvent(AuditEvent{
		Actor:        "admin_api",
		Action:       "runner.retry",
		ResourceType: "runner_request",
		ResourceID:   "retry-me",
		PayloadJSON:  `{"status":"queued"}`,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListAuditEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Action != "runner.retry" {
		t.Fatalf("unexpected audit events: %#v", events)
	}
}

func TestRunnerStateDerivesGitHubJobLinkFromWorkflowJobPayload(t *testing.T) {
	store := New(t.TempDir())
	payload := []byte(`{
		"workflow_job": {
			"id": 77684492230,
			"run_id": 26392225417,
			"html_url": "https://github.com/qbox/las/actions/runs/26392225417/job/77684492230",
			"pull_requests": [{"number": 3335}]
		}
	}`)
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:                 "77684492230",
		Source:             "github",
		JobID:              77684492230,
		RepositoryFullName: "qbox/las",
		RequestedLabels:    []string{"github-runner-ubuntu-24-04"},
		Labels:             []string{"github-runner-ubuntu-24-04"},
		RunnerName:         "e2b-77684492230",
	}, payload)
	if err != nil {
		t.Fatal(err)
	}
	if st.WorkflowRunID != 26392225417 || st.WorkflowJobID != 77684492230 || st.PullRequestNumber != 3335 {
		t.Fatalf("unexpected github metadata: %#v", st)
	}
	wantURL := "https://github.com/qbox/las/actions/runs/26392225417/job/77684492230?pr=3335"
	if st.GitHubJobURL != wantURL {
		t.Fatalf("unexpected github job url: %q", st.GitHubJobURL)
	}
}

func TestRunnerStateBuildsGitHubJobLinkFromWorkflowRunPayload(t *testing.T) {
	store := New(t.TempDir())
	payload := []byte(`{
		"workflow_run": {
			"id": 26392225417,
			"pull_requests": [{"number": 3335}]
		}
	}`)
	_, st, err := store.CreateRequest(RunnerRequest{
		ID:                 "77684492230",
		Source:             "github_reconcile",
		JobID:              77684492230,
		RepositoryFullName: "qbox/las",
		RequestedLabels:    []string{"github-runner-ubuntu-24-04"},
		Labels:             []string{"github-runner-ubuntu-24-04"},
		RunnerName:         "e2b-77684492230",
	}, payload)
	if err != nil {
		t.Fatal(err)
	}
	wantURL := "https://github.com/qbox/las/actions/runs/26392225417/job/77684492230?pr=3335"
	if st.WorkflowRunID != 26392225417 || st.GitHubJobURL != wantURL {
		t.Fatalf("unexpected github metadata: %#v", st)
	}
}
