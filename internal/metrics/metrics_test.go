package metrics

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/qiniu/ci-runner/internal/state"
)

func TestRefreshTracksBusyAndIdleRunners(t *testing.T) {
	profile := "metrics-busy-idle"
	Refresh([]state.RunnerProfile{{Name: profile, MaxConcurrency: 3, Enabled: true}}, []state.RunnerState{
		{ProfileName: profile, Status: state.StatusRunning, AssignedJobName: "build"},
		{ProfileName: profile, Status: state.StatusRunning},
		{ProfileName: profile, Status: state.StatusQueued},
	})

	if !strings.Contains(profileBusy.String(), `"`+profile+`": 1`) {
		t.Fatalf("busy metric = %s", profileBusy.String())
	}
	if !strings.Contains(profileIdle.String(), `"`+profile+`": 1`) {
		t.Fatalf("idle metric = %s", profileIdle.String())
	}
	if !strings.Contains(profilePending.String(), `"`+profile+`": 1`) {
		t.Fatalf("pending metric = %s", profilePending.String())
	}
}

func TestRecordExpandedMetrics(t *testing.T) {
	RecordRunnerRegistered("metrics-expanded", "success")
	RecordRunnerCleanup("metrics-expanded", "removed")
	RecordRunnerRequest("octo/repo", "metrics-expanded", "github_webhook", "created")
	RecordGitHubAPI("metrics_api", "success", 5*time.Millisecond)
	RecordHTTPRequest("GET", "GET /user/runner_requests", http.StatusOK, 7*time.Millisecond)
	RecordWorkflowFailure("octo/repo", "ci", "test", "metrics-expanded", "runner_exit", "1")
	RecordWorkflowQueueDuration("octo/repo", "ci", "test", "metrics-expanded", 10*time.Millisecond)
	RecordWorkflowRunDuration("octo/repo", "ci", "test", "metrics-expanded", "success", 20*time.Millisecond)

	assertExpvarContains(t, runnerRegistered.String(), `metrics-expanded|success`)
	assertExpvarContains(t, runnerCleanup.String(), `metrics-expanded|removed`)
	assertExpvarContains(t, runnerRequestsTotal.String(), `octo/repo|metrics-expanded|github_webhook|created`)
	assertExpvarContains(t, githubAPIOperations.String(), `metrics_api|success`)
	assertExpvarContains(t, httpRequestsTotal.String(), `GET|GET /user/runner_requests|200`)
	assertExpvarContains(t, httpRequestDuration.String(), `GET|GET /user/runner_requests|200`)
	assertExpvarContains(t, workflowFailures.String(), `octo/repo|ci|test|metrics-expanded|runner_exit|1`)
	assertExpvarContains(t, workflowQueueCount.String(), `octo/repo|ci|test|metrics-expanded`)
	assertExpvarContains(t, workflowRunCount.String(), `octo/repo|ci|test|metrics-expanded|success`)
}

func assertExpvarContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("metric %q does not contain %q", got, want)
	}
}
