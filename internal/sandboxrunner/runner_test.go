package sandboxrunner

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	qnsandbox "github.com/qiniu/go-sdk/v7/sandbox"
)

func TestStartScriptEncodesRunnerArguments(t *testing.T) {
	input := StartInput{
		RequestID:         `req$(touch /tmp/request)`,
		RepositoryURL:     `https://github.com/o/r$(touch /tmp/pwn)`,
		RegistrationToken: `tok"$(touch /tmp/token)"`,
		RunnerName:        "runner`touch /tmp/name`",
		Labels:            []string{"self-hosted", `e2b$(touch /tmp/label)`},
		RunnerGroup:       `group$(touch /tmp/group)`,
	}
	sandboxID := `sandbox$(touch /tmp/sandbox)`
	script := startScript(input, sandboxID)

	for _, raw := range []string{
		input.RequestID,
		input.RepositoryURL,
		input.RegistrationToken,
		input.RunnerName,
		strings.Join(input.Labels, ","),
		input.RunnerGroup,
		sandboxID,
	} {
		if strings.Contains(script, raw) {
			t.Fatalf("script contains raw argument %q:\n%s", raw, script)
		}
		if !strings.Contains(script, base64.StdEncoding.EncodeToString([]byte(raw))) {
			t.Fatalf("script does not contain encoded argument %q:\n%s", raw, script)
		}
	}
}

func TestStartScriptRunsCleanupAfterRunnerExit(t *testing.T) {
	script := startScript(StartInput{
		RepositoryURL:     "https://github.com/o/r",
		RegistrationToken: "token",
		RunnerName:        "runner",
		Labels:            []string{"self-hosted", "e2b"},
	}, "sandbox-1")

	if strings.Contains(script, "exec ./run.sh") {
		t.Fatalf("script must not exec run.sh because that bypasses the EXIT cleanup trap:\n%s", script)
	}
	if !strings.Contains(script, "trap cleanup EXIT") || !strings.Contains(script, "./run.sh") {
		t.Fatalf("script does not preserve cleanup trap and runner execution:\n%s", script)
	}
}

func TestStartScriptReportsSandboxIDInJobStartedHook(t *testing.T) {
	script := startScript(StartInput{
		RequestID:         "78433740691",
		RepositoryURL:     "https://github.com/o/r",
		RegistrationToken: "token",
		RunnerName:        "e2b-78433740691",
		Labels:            []string{"self-hosted", "e2b"},
	}, "sb-78433740691")

	for _, want := range []string{
		"export ACTIONS_RUNNER_HOOK_JOB_STARTED=/tmp/runnerd-hooks/job-started.sh",
		"RUNNERD_JOB_STARTED",
		"::notice title=Qiniu sandbox::sandbox_id=${RUNNERD_SANDBOX_ID} runner_request_id=${RUNNERD_REQUEST_ID} runner_name=${RUNNERD_RUNNER_NAME}",
		"Qiniu sandbox id: ${RUNNERD_SANDBOX_ID}",
		"Runner request id: ${RUNNERD_REQUEST_ID}",
		"Runner name: ${RUNNERD_RUNNER_NAME}",
		base64.StdEncoding.EncodeToString([]byte("78433740691")),
		base64.StdEncoding.EncodeToString([]byte("sb-78433740691")),
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
}

func TestTemplateBuildUsableAcceptsRunnableStatuses(t *testing.T) {
	for _, status := range []qnsandbox.TemplateBuildStatus{
		qnsandbox.BuildStatusReady,
		qnsandbox.BuildStatusUploaded,
	} {
		if !templateBuildUsable(status) {
			t.Fatalf("expected status %q to be usable", status)
		}
	}

	for _, status := range []qnsandbox.TemplateBuildStatus{
		qnsandbox.BuildStatusBuilding,
		qnsandbox.BuildStatusWaiting,
		qnsandbox.BuildStatusError,
	} {
		if templateBuildUsable(status) {
			t.Fatalf("expected status %q to be unusable", status)
		}
	}
}

func TestRecoveredRunnerPIDRequiresRunnerTagAndExpectedPID(t *testing.T) {
	runnerTag := "github-runner"
	otherTag := "other"
	processes := []qnsandbox.ProcessInfo{
		{PID: 10, Tag: &otherTag},
		{PID: 20, Tag: &runnerTag},
	}
	if pid, ok := recoveredRunnerPID(processes, 20); !ok || pid != 20 {
		t.Fatalf("expected persisted runner PID, got pid=%d ok=%t", pid, ok)
	}
	if pid, ok := recoveredRunnerPID(processes, 0); !ok || pid != 20 {
		t.Fatalf("expected tagged runner PID, got pid=%d ok=%t", pid, ok)
	}
	if _, ok := recoveredRunnerPID(processes, 10); ok {
		t.Fatal("expected non-runner tag to be rejected")
	}
}

func TestSandboxTimeoutSecondsRoundsUpAndClamps(t *testing.T) {
	if got := sandboxTimeoutSeconds(1500 * time.Millisecond); got != 2 {
		t.Fatalf("expected timeout to round up, got %d", got)
	}
	if got := sandboxTimeoutSeconds(0); got != 1 {
		t.Fatalf("expected minimum timeout of one second, got %d", got)
	}
}
