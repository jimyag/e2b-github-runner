package sandboxrunner

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	qnsandbox "github.com/qiniu/go-sdk/v7/sandbox"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func writeExecutable(t *testing.T, name, contents string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
}

func runCommand(t *testing.T, command string, args []string, env ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(command, args...)
	cmd.Dir = repositoryRoot(t)
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

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
		`export ACTIONS_RUNNER_HOOK_JOB_STARTED="$hook_root/job-started.sh"`,
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

func TestStartScriptUsesHostedRunnerFilesystemContract(t *testing.T) {
	fixture := t.TempDir()
	actionsRunnerRoot := filepath.Join(fixture, "actions-runner")
	workdir := filepath.Join(fixture, "home", "runner", "work", "_runner")
	runnerJobWork := filepath.Join(fixture, "home", "runner", "work")
	runnerHome := filepath.Join(fixture, "home", "runner")
	hookRoot := filepath.Join(fixture, "hooks")
	logPath := filepath.Join(fixture, "runner.log")
	if err := os.MkdirAll(actionsRunnerRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(actionsRunnerRoot, "config.sh"), `#!/usr/bin/env bash
set -euo pipefail
printf 'config HOME=%s PWD=%s TOOL_CACHE=%s AGENT_TOOLS=%s\n' "$HOME" "$PWD" "$RUNNER_TOOL_CACHE" "$AGENT_TOOLSDIRECTORY" >>"$RUNNER_TEST_LOG"
`)
	writeExecutable(t, filepath.Join(actionsRunnerRoot, "run.sh"), `#!/usr/bin/env bash
set -euo pipefail
printf 'run HOME=%s PWD=%s RUNASROOT=%s\n' "$HOME" "$PWD" "${RUNNER_ALLOW_RUNASROOT:-}" >>"$RUNNER_TEST_LOG"
`)

	script := startScript(StartInput{
		RequestID:         "request-1",
		RepositoryURL:     "https://github.com/o/r",
		RegistrationToken: "token",
		RunnerName:        "runner",
		Labels:            []string{"self-hosted", "linux", "x64"},
	}, "sandbox-1")
	scriptPath := filepath.Join(fixture, "start-runner.sh")
	writeExecutable(t, scriptPath, script)

	output, err := runCommand(
		t,
		"bash",
		[]string{scriptPath},
		"ACTIONS_RUNNER_ROOT="+actionsRunnerRoot,
		"RUNNER_WORKDIR="+workdir,
		"RUNNER_JOB_WORK="+runnerJobWork,
		"RUNNER_HOME="+runnerHome,
		"RUNNER_HOOK_ROOT="+hookRoot,
		"RUNNER_TEST_LOG="+logPath,
		"ENSURE_DOCKER=/bin/true",
	)
	if err != nil {
		t.Fatalf("start script failed: %v\n%s", err, output)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logBytes)
	for _, want := range []string{
		"config HOME=" + runnerHome + " PWD=" + workdir,
		"TOOL_CACHE=/opt/hostedtoolcache AGENT_TOOLS=/opt/hostedtoolcache",
		"run HOME=" + runnerHome + " PWD=" + workdir + " RUNASROOT=",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("runner execution log missing %q:\n%s", want, log)
		}
	}
}

func TestRunnerTemplateMatrixGateAcceptsManagedCatalog(t *testing.T) {
	output, err := runCommand(t, "bash", []string{"scripts/check-runner-template-matrix.sh"})
	if err != nil {
		t.Fatalf("matrix gate failed: %v\n%s", err, output)
	}
}

func TestRunnerTemplateMatrixGateAcceptsLifecycleStatesAndRejectsUnknownState(t *testing.T) {
	root := repositoryRoot(t)
	readmeBytes, err := os.ReadFile(filepath.Join(root, "templates", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "templates-readme.md")
	verified := strings.ReplaceAll(string(readmeBytes), "| development |", "| verified |")
	if err := os.WriteFile(fixture, []byte(verified), 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := runCommand(
		t,
		"bash",
		[]string{"scripts/check-runner-template-matrix.sh"},
		"RUNNER_TEMPLATES_README="+fixture,
	)
	if err != nil {
		t.Fatalf("verified publication state must pass: %v\n%s", err, output)
	}

	invalid := strings.Replace(verified, "| verified |", "| released |", 1)
	if err := os.WriteFile(fixture, []byte(invalid), 0o644); err != nil {
		t.Fatal(err)
	}
	output, err = runCommand(
		t,
		"bash",
		[]string{"scripts/check-runner-template-matrix.sh"},
		"RUNNER_TEMPLATES_README="+fixture,
	)
	if err == nil || !strings.Contains(output, "unsupported publication state") {
		t.Fatalf("unsupported publication state must fail, got err=%v\n%s", err, output)
	}
}

func TestRunnerTemplateQshellBuildUsesTemporaryConfigAndRequiresReady(t *testing.T) {
	fixture := t.TempDir()
	templateDir := filepath.Join(fixture, "template")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(templateDir, "qshell.sandbox.toml")
	const trackedConfig = "name = \"fixture-template\"\n"
	if err := os.WriteFile(configPath, []byte(trackedConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	qshellPath := filepath.Join(fixture, "qshell")
	qshellLog := filepath.Join(fixture, "qshell.log")
	writeExecutable(t, qshellPath, `#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = version ]; then
  printf 'v2.19.10\n'
  exit 0
fi
printf 'pwd=%s args=%s\n' "$PWD" "$*" >>"$QSHELL_TEST_LOG"
if [ "$QSHELL_TEST_MODE" = ready ]; then
  printf 'Status:       ready\n'
else
  printf 'Error: fixture build failed\n' >&2
fi
`)
	commonEnv := []string{
		"QSHELL=" + qshellPath,
		"QSHELL_TEST_LOG=" + qshellLog,
		"QINIU_SANDBOX_API_URL=https://sandbox.example.test",
		"QINIU_API_KEY=test-api-key",
	}
	output, err := runCommand(
		t,
		"bash",
		[]string{"scripts/run-runner-template-operation.sh", "build", templateDir},
		append(commonEnv, "QSHELL_TEST_MODE=ready")...,
	)
	if err != nil {
		t.Fatalf("ready qshell build failed: %v\n%s", err, output)
	}
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(configBytes) != trackedConfig {
		t.Fatalf("tracked qshell config changed:\n%s", configBytes)
	}
	temporaryConfigs, err := filepath.Glob(filepath.Join(templateDir, ".qshell-sandbox.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporaryConfigs) != 0 {
		t.Fatalf("temporary qshell configs were not removed: %v", temporaryConfigs)
	}
	logBytes, err := os.ReadFile(qshellLog)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logBytes)
	if !strings.Contains(log, "pwd="+templateDir) ||
		!strings.Contains(log, "args=sandbox template build --wait --config .qshell-sandbox.") ||
		strings.Contains(log, "--config qshell.sandbox.toml") {
		t.Fatalf("build did not use a temporary config from the template directory:\n%s", log)
	}

	output, err = runCommand(
		t,
		"bash",
		[]string{"scripts/run-runner-template-operation.sh", "build", templateDir},
		append(commonEnv, "QSHELL_TEST_MODE=failed")...,
	)
	if err == nil || !strings.Contains(output, "did not report terminal Status: ready") {
		t.Fatalf("zero-exit qshell failure must fail closed, got err=%v\n%s", err, output)
	}
}

func TestRunnerTemplateQshellResolvesRelativeTemplateFromScriptCheckout(t *testing.T) {
	root := repositoryRoot(t)
	fixture := t.TempDir()
	qshellPath := filepath.Join(fixture, "qshell")
	qshellLog := filepath.Join(fixture, "qshell.log")
	writeExecutable(t, qshellPath, `#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = version ]; then
  printf 'v2.19.10\n'
  exit 0
fi
printf 'pwd=%s\n' "$PWD" >"$QSHELL_TEST_LOG"
printf 'Status:       ready\n'
`)
	scriptPath := filepath.Join(root, "scripts", "run-runner-template-operation.sh")
	cmd := exec.Command(
		"bash",
		scriptPath,
		"build",
		"templates/github-runner-ubuntu-slim",
	)
	cmd.Dir = fixture
	cmd.Env = append(
		os.Environ(),
		"QSHELL="+qshellPath,
		"QSHELL_TEST_LOG="+qshellLog,
		"QINIU_SANDBOX_API_URL=https://sandbox.example.test",
		"QINIU_API_KEY=test-api-key",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("relative template build failed: %v\n%s", err, output)
	}
	logBytes, err := os.ReadFile(qshellLog)
	if err != nil {
		t.Fatal(err)
	}
	want := "pwd=" + filepath.Join(root, "templates", "github-runner-ubuntu-slim")
	if !strings.Contains(string(logBytes), want) {
		t.Fatalf("relative template resolved outside the script checkout; want %q, got:\n%s", want, logBytes)
	}
}

func TestRunnerTemplateQshellPublicationRunsFromTemplateDirectory(t *testing.T) {
	fixture := t.TempDir()
	templateDir := filepath.Join(fixture, "template")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(templateDir, "qshell.sandbox.toml"),
		[]byte("name = \"fixture-template\"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	qshellPath := filepath.Join(fixture, "qshell")
	qshellLog := filepath.Join(fixture, "qshell.log")
	writeExecutable(t, qshellPath, `#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = version ]; then
  printf 'v2.19.10\n'
  exit 0
fi
printf 'pwd=%s args=%s\n' "$PWD" "$*" >>"$QSHELL_TEST_LOG"
case "$3" in
  publish) printf 'Template tmpl-fixture published\n' ;;
  unpublish) printf 'Template tmpl-fixture unpublished\n' ;;
  *) exit 64 ;;
esac
`)
	commonEnv := []string{
		"QSHELL=" + qshellPath,
		"QSHELL_TEST_LOG=" + qshellLog,
		"QINIU_SANDBOX_API_URL=https://sandbox.example.test",
		"QINIU_API_KEY=test-api-key",
	}
	for _, operation := range []string{"publish", "unpublish"} {
		output, err := runCommand(
			t,
			"bash",
			[]string{"scripts/run-runner-template-operation.sh", operation, templateDir},
			commonEnv...,
		)
		if err != nil {
			t.Fatalf("%s failed: %v\n%s", operation, err, output)
		}
	}
	logBytes, err := os.ReadFile(qshellLog)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logBytes)
	for _, operation := range []string{"publish", "unpublish"} {
		want := "pwd=" + templateDir + " args=sandbox template " + operation + " -y"
		if !strings.Contains(log, want) {
			t.Fatalf("missing qshell publication invocation %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "--config") {
		t.Fatalf("publish/unpublish must not use build-only --config:\n%s", log)
	}
}

func TestDefaultTemplateCatalogCheckRequiresUniqueRunnablePublicTemplates(t *testing.T) {
	templates := []map[string]any{
		{
			"templateID":  "tmpl-slim",
			"names":       []string{"qiniu/github-runner-ubuntu-slim"},
			"public":      true,
			"buildStatus": "ready",
		},
		{
			"templateID":  "tmpl-22",
			"names":       []string{"github-runner-ubuntu-22-04"},
			"public":      true,
			"buildStatus": "uploaded",
		},
		{
			"templateID":  "tmpl-24",
			"names":       []string{"github-runner-ubuntu-24-04"},
			"public":      true,
			"buildStatus": "ready",
		},
		{
			"templateID":  "tmpl-26",
			"names":       []string{"github-runner-ubuntu-26-04"},
			"public":      true,
			"buildStatus": "ready",
		},
	}
	initialResponseBody, err := json.Marshal(templates)
	if err != nil {
		t.Fatal(err)
	}
	var responseBody atomic.Value
	responseBody.Store(initialResponseBody)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/default-templates" {
			http.NotFound(response, request)
			return
		}
		if request.Header.Get("X-API-Key") != "test-api-key" {
			http.Error(response, "missing API key", http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write(responseBody.Load().([]byte))
	}))
	defer server.Close()

	commonEnv := []string{
		"QINIU_SANDBOX_API_URL=" + server.URL,
		"QINIU_API_KEY=test-api-key",
	}
	output, err := runCommand(
		t,
		"bash",
		[]string{"scripts/check-default-template-catalog.sh"},
		commonEnv...,
	)
	if err != nil {
		t.Fatalf("valid default-template catalog failed: %v\n%s", err, output)
	}
	for _, want := range []string{
		"github-runner-ubuntu-slim\ttmpl-slim\tready",
		"github-runner-ubuntu-22-04\ttmpl-22\tuploaded",
		"github-runner-ubuntu-24-04\ttmpl-24\tready",
		"github-runner-ubuntu-26-04\ttmpl-26\tready",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("catalog output missing %q:\n%s", want, output)
		}
	}

	templates = append(templates, map[string]any{
		"templateID":  "tmpl-24-duplicate",
		"names":       []string{"team/github-runner-ubuntu-24-04"},
		"public":      true,
		"buildStatus": "ready",
	})
	duplicateResponseBody, err := json.Marshal(templates)
	if err != nil {
		t.Fatal(err)
	}
	responseBody.Store(duplicateResponseBody)
	output, err = runCommand(
		t,
		"bash",
		[]string{"scripts/check-default-template-catalog.sh"},
		commonEnv...,
	)
	if err == nil || !strings.Contains(
		output,
		"expected exactly one default template name match for github-runner-ubuntu-24-04; found 2",
	) {
		t.Fatalf("duplicate catalog entry must fail, got err=%v\n%s", err, output)
	}

	privateDuplicateTemplates := append(
		templates[:len(templates)-1],
		map[string]any{
			"templateID":  "tmpl-24-private",
			"names":       []string{"team/github-runner-ubuntu-24-04"},
			"public":      false,
			"buildStatus": "ready",
		},
	)
	privateDuplicateResponseBody, err := json.Marshal(privateDuplicateTemplates)
	if err != nil {
		t.Fatal(err)
	}
	responseBody.Store(privateDuplicateResponseBody)
	output, err = runCommand(
		t,
		"bash",
		[]string{"scripts/check-default-template-catalog.sh"},
		commonEnv...,
	)
	if err == nil || !strings.Contains(
		output,
		"expected exactly one default template name match for github-runner-ubuntu-24-04; found 2",
	) {
		t.Fatalf("private duplicate catalog entry must fail, got err=%v\n%s", err, output)
	}
}

func TestCompatibilityGateRejectsMissingAndUnexplainedCoverage(t *testing.T) {
	fixture := t.TempDir()
	reportDir := filepath.Join(fixture, "reports")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := `# Fixture
- OS Version: 24.04

## Installed Software

### Tools
- Git 2.0
- Docker Client 28.0
`
	for _, name := range []string{
		"Ubuntu2204-Readme.md",
		"Ubuntu2404-Readme.md",
		"Ubuntu2604-Readme.md",
		"ubuntu-slim-Readme.md",
	} {
		if err := os.WriteFile(filepath.Join(reportDir, name), []byte(report), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lockPath := filepath.Join(fixture, "lock.json")
	lock := `{
  "repository": "actions/runner-images",
  "commit": "e986db797519f06a2e5e53701a715cfa4c1545e8",
  "reports": {
    "ubuntu-slim": "images/ubuntu-slim/ubuntu-slim-Readme.md",
    "ubuntu-22.04": "images/ubuntu/Ubuntu2204-Readme.md",
    "ubuntu-24.04": "images/ubuntu/Ubuntu2404-Readme.md",
    "ubuntu-26.04": "images/ubuntu/Ubuntu2604-Readme.md"
  }
}`
	if err := os.WriteFile(lockPath, []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(fixture, "compatibility.json")
	missingManifest := `{
  "upstream_commit": "e986db797519f06a2e5e53701a715cfa4c1545e8",
  "images": {
    "ubuntu-slim": {"entries": []},
    "ubuntu-22.04": {"entries": []},
    "ubuntu-24.04": {"entries": []},
    "ubuntu-26.04": {"entries": []}
  }
}`
	if err := os.WriteFile(manifestPath, []byte(missingManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	commonEnv := []string{
		"RUNNER_IMAGES_REPORT_DIR=" + reportDir,
		"RUNNER_IMAGES_LOCK=" + lockPath,
		"RUNNER_IMAGES_MANIFEST=" + manifestPath,
	}
	output, err := runCommand(t, "bash", []string{"scripts/check-runner-image-compatibility.sh"}, commonEnv...)
	if err == nil || !strings.Contains(output, "missing compatibility entry") {
		t.Fatalf("missing coverage must fail with a specific error, got err=%v\n%s", err, output)
	}

	entries := make([]map[string]string, 0, 12)
	for _, image := range []string{"ubuntu-slim", "ubuntu-22.04", "ubuntu-24.04", "ubuntu-26.04"} {
		for _, item := range []string{"Image metadata|OS Version", "Tools|Git", "Tools|Docker Client"} {
			parts := strings.Split(item, "|")
			entries = append(entries, map[string]string{
				"image":         image,
				"category":      parts[0],
				"upstream_name": parts[1],
				"status":        "excluded",
				"verification":  "false",
				"reason":        "",
			})
		}
	}
	images := map[string]map[string]any{}
	for _, image := range []string{"ubuntu-slim", "ubuntu-22.04", "ubuntu-24.04", "ubuntu-26.04"} {
		imageEntries := make([]map[string]string, 0, 3)
		for _, entry := range entries {
			if entry["image"] == image {
				delete(entry, "image")
				imageEntries = append(imageEntries, entry)
			}
		}
		images[image] = map[string]any{"entries": imageEntries}
	}
	unexplained, err := json.Marshal(map[string]any{
		"upstream_commit": "e986db797519f06a2e5e53701a715cfa4c1545e8",
		"images":          images,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, unexplained, 0o644); err != nil {
		t.Fatal(err)
	}
	output, err = runCommand(t, "bash", []string{"scripts/check-runner-image-compatibility.sh"}, commonEnv...)
	if err == nil || !strings.Contains(output, "Sandbox-specific reason") {
		t.Fatalf("unexplained exclusion must fail with a specific error, got err=%v\n%s", err, output)
	}
}

func TestDockerConformanceStopsOnFirstFailureAndRecordsJSON(t *testing.T) {
	fixture := t.TempDir()
	binDir := filepath.Join(fixture, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(binDir, "docker"), `#!/usr/bin/env bash
set -euo pipefail
if [ "$1" != run ]; then
  exit 64
fi
shift
while [ "$#" -gt 0 ] && [ "$1" != bash ]; do
  shift
done
shift
test "$1" = -lc
shift
exec bash -lc "$1"
`)
	manifestPath := filepath.Join(fixture, "compatibility.json")
	manifest := `{
  "images": {
    "ubuntu-24.04": {
      "entries": [
        {"category":"Tools","upstream_name":"first","status":"provided","verification":"printf first"},
        {"category":"Tools","upstream_name":"second","status":"provided","verification":"printf second >&2; exit 17"},
        {"category":"Tools","upstream_name":"third","status":"provided","verification":"printf third"}
      ]
    }
  }
}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(fixture, "result.json")
	output, err := runCommand(
		t,
		"bash",
		[]string{
			"scripts/run-runner-image-conformance.sh",
			"--image", "ubuntu-24.04",
			"--executor", "docker",
			"--target", "fixture-image",
			"--output", resultPath,
		},
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"RUNNER_IMAGES_MANIFEST="+manifestPath,
	)
	if err == nil {
		t.Fatalf("conformance must fail on the failing assertion:\n%s", output)
	}
	resultBytes, readErr := os.ReadFile(resultPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var result struct {
		Completed bool `json:"completed"`
		Passed    bool `json:"passed"`
		Results   []struct {
			Name       string `json:"name"`
			Stdout     string `json:"stdout"`
			Stderr     string `json:"stderr"`
			ExitStatus int    `json:"exit_status"`
		} `json:"results"`
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("invalid result JSON: %v\n%s", err, resultBytes)
	}
	if !result.Completed || result.Passed || len(result.Results) != 2 {
		t.Fatalf("result must contain the two executed assertions and fail: %#v", result)
	}
	if result.Results[0].Stdout != "first" || result.Results[1].Stderr != "second" || result.Results[1].ExitStatus != 17 {
		t.Fatalf("unexpected command evidence: %#v", result.Results)
	}
}

func TestSandboxConformanceKillsSandboxOnFailure(t *testing.T) {
	fixture := t.TempDir()
	binDir := filepath.Join(fixture, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	qshellLog := filepath.Join(fixture, "qshell.log")
	writeExecutable(t, filepath.Join(binDir, "qshell"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$QSHELL_TEST_LOG"
case "$1 $2" in
  "sandbox create")
    printf '{"sandbox_id":"sb-fixture"}\n'
    ;;
  "sandbox exec")
    shift 2
    test "$1" = sb-fixture
    shift
    test "$1" = -u
    test "$2" = runner
    shift 2
    test "$1" = --
    shift
    exec "$@"
    ;;
  "sandbox kill")
    test "$3" = sb-fixture
    printf 'Killed sandbox sb-fixture\n'
    ;;
  *)
    exit 64
    ;;
esac
`)
	manifestPath := filepath.Join(fixture, "compatibility.json")
	manifest := `{
  "images": {
    "ubuntu-slim": {
      "entries": [
        {"category":"Tools","upstream_name":"failure","status":"provided","verification":"exit 23"}
      ]
    }
  }
}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(fixture, "result.json")
	output, err := runCommand(
		t,
		"bash",
		[]string{
			"scripts/run-runner-image-conformance.sh",
			"--image", "ubuntu-slim",
			"--executor", "sandbox",
			"--target", "template-fixture",
			"--output", resultPath,
		},
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"RUNNER_IMAGES_MANIFEST="+manifestPath,
		"QSHELL_TEST_LOG="+qshellLog,
		"QINIU_SANDBOX_API_URL=https://sandbox.example.test",
		"QINIU_API_KEY=test-api-key",
	)
	if err == nil {
		t.Fatalf("sandbox conformance must fail on the assertion:\n%s", output)
	}
	logBytes, readErr := os.ReadFile(qshellLog)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(logBytes), "sandbox kill sb-fixture") {
		t.Fatalf("temporary sandbox was not killed:\n%s", logBytes)
	}
	if _, statErr := os.Stat(resultPath); errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("failure result JSON was not written")
	}
	resultBytes, readErr := os.ReadFile(resultPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var result struct {
		Completed bool `json:"completed"`
		Passed    bool `json:"passed"`
		Cleanup   struct {
			Attempted bool `json:"attempted"`
			Passed    bool `json:"passed"`
		} `json:"cleanup"`
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("invalid result JSON: %v\n%s", err, resultBytes)
	}
	if !result.Completed || result.Passed || !result.Cleanup.Attempted || !result.Cleanup.Passed {
		t.Fatalf("failed assertion must retain successful Sandbox cleanup evidence: %#v", result)
	}
}

func TestSandboxConformanceParsesOpaqueQshellSandboxID(t *testing.T) {
	fixture := t.TempDir()
	binDir := filepath.Join(fixture, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	qshellLog := filepath.Join(fixture, "qshell.log")
	writeExecutable(t, filepath.Join(binDir, "qshell"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$QSHELL_TEST_LOG"
case "$1 $2" in
  "sandbox create")
    printf 'Sandbox ID:   opaque-fixture-42\n'
    ;;
  "sandbox exec")
    if IFS= read -r unexpected_input; then
      printf 'unexpected stdin: %s\n' "$unexpected_input" >&2
      exit 88
    fi
    shift 2
    test "$1" = opaque-fixture-42
    shift
    test "$1" = -u
    test "$2" = runner
    shift 2
    test "$1" = --
    shift
    exec "$@"
    ;;
  "sandbox kill")
    test "$3" = opaque-fixture-42
    printf 'Killed sandbox opaque-fixture-42\n'
    ;;
  *)
    exit 64
    ;;
esac
`)
	manifestPath := filepath.Join(fixture, "compatibility.json")
	manifest := `{
  "images": {
    "ubuntu-slim": {
      "entries": [
        {"category":"Tools","upstream_name":"success","status":"provided","verification":"printf verified"}
      ]
    }
  }
}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(fixture, "result.json")
	output, err := runCommand(
		t,
		"bash",
		[]string{
			"scripts/run-runner-image-conformance.sh",
			"--image", "ubuntu-slim",
			"--executor", "sandbox",
			"--target", "template-fixture",
			"--output", resultPath,
		},
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"RUNNER_IMAGES_MANIFEST="+manifestPath,
		"QSHELL_TEST_LOG="+qshellLog,
		"QINIU_SANDBOX_API_URL=https://sandbox.example.test",
		"QINIU_API_KEY=test-api-key",
	)
	if err != nil {
		t.Fatalf("sandbox conformance rejected an opaque qshell Sandbox ID: %v\n%s", err, output)
	}
	logBytes, err := os.ReadFile(qshellLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"sandbox exec opaque-fixture-42 -u runner",
		"sandbox kill opaque-fixture-42",
	} {
		if !strings.Contains(string(logBytes), command) {
			t.Fatalf("qshell did not receive %q:\n%s", command, logBytes)
		}
	}
}

func TestSandboxConformanceRejectsQshellTransportErrorsWithZeroExitStatus(t *testing.T) {
	fixture := t.TempDir()
	binDir := filepath.Join(fixture, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	qshellLog := filepath.Join(fixture, "qshell.log")
	writeExecutable(t, filepath.Join(binDir, "qshell"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$QSHELL_TEST_LOG"
case "$1 $2" in
  "sandbox create")
    printf 'Sandbox ID:   sb-fixture\n'
    ;;
  "sandbox exec")
    printf 'Error: connect to sandbox sb-fixture failed: fixture transport failure\n' >&2
    exit 0
    ;;
  "sandbox kill")
    test "$3" = sb-fixture
    printf 'Killed sandbox sb-fixture\n'
    ;;
  *)
    exit 64
    ;;
esac
`)
	manifestPath := filepath.Join(fixture, "compatibility.json")
	manifest := `{
  "images": {
    "ubuntu-slim": {
      "entries": [
        {"category":"Tools","upstream_name":"transport","status":"provided","verification":"printf should-not-run"}
      ]
    }
  }
}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(fixture, "result.json")
	output, err := runCommand(
		t,
		"bash",
		[]string{
			"scripts/run-runner-image-conformance.sh",
			"--image", "ubuntu-slim",
			"--executor", "sandbox",
			"--target", "template-fixture",
			"--output", resultPath,
		},
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"RUNNER_IMAGES_MANIFEST="+manifestPath,
		"QSHELL_TEST_LOG="+qshellLog,
		"QINIU_SANDBOX_API_URL=https://sandbox.example.test",
		"QINIU_API_KEY=test-api-key",
	)
	if err == nil {
		t.Fatalf("sandbox conformance must reject a qshell transport error even when qshell exits zero:\n%s", output)
	}
	resultBytes, readErr := os.ReadFile(resultPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var result struct {
		Passed  bool `json:"passed"`
		Results []struct {
			Stdout     string `json:"stdout"`
			Stderr     string `json:"stderr"`
			ExitStatus int    `json:"exit_status"`
		} `json:"results"`
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("invalid result JSON: %v\n%s", err, resultBytes)
	}
	if result.Passed || len(result.Results) != 1 {
		t.Fatalf("transport failure must be retained as one failed result: %#v", result)
	}
	if result.Results[0].ExitStatus == 0 ||
		!strings.Contains(result.Results[0].Stderr, "fixture transport failure") ||
		strings.Contains(result.Results[0].Stdout, "should-not-run") {
		t.Fatalf("transport failure evidence was not preserved: %#v", result.Results[0])
	}
	logBytes, readErr := os.ReadFile(qshellLog)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(logBytes), "sandbox kill sb-fixture") {
		t.Fatalf("temporary sandbox was not killed after transport failure:\n%s", logBytes)
	}
}

func TestSandboxConformanceFailsWhenCleanupCannotBeConfirmed(t *testing.T) {
	fixture := t.TempDir()
	binDir := filepath.Join(fixture, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(binDir, "qshell"), `#!/usr/bin/env bash
set -euo pipefail
case "$1 $2" in
  "sandbox create")
    printf 'Sandbox ID:   sb-fixture\n'
    ;;
  "sandbox exec")
    shift 2
    test "$1" = sb-fixture
    shift
    test "$1" = -u
    test "$2" = runner
    shift 2
    test "$1" = --
    shift
    exec "$@"
    ;;
  "sandbox kill")
    printf 'Error: kill sandbox sb-fixture failed: fixture cleanup failure\n' >&2
    exit 0
    ;;
  *)
    exit 64
    ;;
esac
`)
	manifestPath := filepath.Join(fixture, "compatibility.json")
	manifest := `{
  "images": {
    "ubuntu-slim": {
      "entries": [
        {"category":"Tools","upstream_name":"success","status":"provided","verification":"printf verified"}
      ]
    }
  }
}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(fixture, "result.json")
	output, err := runCommand(
		t,
		"bash",
		[]string{
			"scripts/run-runner-image-conformance.sh",
			"--image", "ubuntu-slim",
			"--executor", "sandbox",
			"--target", "template-fixture",
			"--output", resultPath,
		},
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"RUNNER_IMAGES_MANIFEST="+manifestPath,
		"QINIU_SANDBOX_API_URL=https://sandbox.example.test",
		"QINIU_API_KEY=test-api-key",
	)
	if err == nil {
		t.Fatalf("sandbox conformance must fail when cleanup cannot be confirmed:\n%s", output)
	}
	resultBytes, readErr := os.ReadFile(resultPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var result struct {
		Passed  bool `json:"passed"`
		Cleanup struct {
			Attempted  bool   `json:"attempted"`
			Passed     bool   `json:"passed"`
			ExitStatus int    `json:"exit_status"`
			Stderr     string `json:"stderr"`
		} `json:"cleanup"`
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("invalid result JSON: %v\n%s", err, resultBytes)
	}
	if result.Passed ||
		!result.Cleanup.Attempted ||
		result.Cleanup.Passed ||
		result.Cleanup.ExitStatus == 0 ||
		!strings.Contains(result.Cleanup.Stderr, "fixture cleanup failure") {
		t.Fatalf("unconfirmed cleanup must be retained as failed release evidence: %#v", result)
	}
}

func TestTemplateReleaseSmokeRunsOnlyUsabilityContract(t *testing.T) {
	fixture := t.TempDir()
	binDir := filepath.Join(fixture, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(binDir, "qshell"), `#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = version ]; then
  printf 'v2.19.10\n'
  exit 0
fi
case "$1 $2" in
  "sandbox create")
    printf 'Sandbox ID:   sb-smoke\n'
    ;;
  "sandbox exec")
    printf '__QINIU_RUNNER_CONFORMANCE_REMOTE_STARTED__\n'
    ;;
  "sandbox kill")
    test "$3" = sb-smoke
    printf 'Killed sandbox sb-smoke\n'
    ;;
  *)
    exit 64
    ;;
esac
`)
	manifestPath := filepath.Join(fixture, "compatibility.json")
	manifest := `{
  "images": {
    "ubuntu-26.04": {
      "entries": [
        {
          "category": "Full inventory",
          "upstream_name": "expensive diagnostic",
          "status": "provided",
          "verification": "exit 99"
        }
      ]
    }
  }
}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(fixture, "smoke")
	output, err := runCommand(
		t,
		"bash",
		[]string{"scripts/smoke-runner-template.sh", "ubuntu-26.04", "template-fixture"},
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"RUNNER_IMAGES_MANIFEST="+manifestPath,
		"RUNNER_SMOKE_OUTPUT_DIR="+outputDir,
		"QINIU_SANDBOX_API_URL=https://sandbox.example.test",
		"QINIU_API_KEY=test-api-key",
	)
	if err != nil {
		t.Fatalf("release smoke failed: %v\n%s", err, output)
	}
	resultFiles, err := filepath.Glob(filepath.Join(outputDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(resultFiles) != 1 {
		t.Fatalf("release smoke result files = %#v, want one", resultFiles)
	}
	resultBytes, err := os.ReadFile(resultFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Passed         bool   `json:"passed"`
		SupportChannel string `json:"support_channel"`
		Results        []struct {
			Category string `json:"category"`
			Name     string `json:"name"`
			Command  string `json:"command"`
		} `json:"results"`
		Cleanup struct {
			Passed bool `json:"passed"`
		} `json:"cleanup"`
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("invalid release smoke JSON: %v\n%s", err, resultBytes)
	}
	if !result.Passed ||
		result.SupportChannel != "preview" ||
		!result.Cleanup.Passed ||
		len(result.Results) != 6 {
		t.Fatalf("release smoke result = %#v, want six passing usability checks and cleanup", result)
	}
	for _, check := range result.Results {
		if check.Category != "Release smoke" {
			t.Fatalf("release smoke unexpectedly ran full inventory check %#v", check)
		}
		if check.Name == "Docker daemon" && !strings.Contains(check.Command, "sudo -H -u runner") {
			t.Fatalf("Docker smoke does not reproduce the runnerd group context: %#v", check)
		}
	}
}

func TestEnsureDockerIsIdempotentAndKeepsSocketNonRootAccessible(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the non-root socket contract requires a non-root test process")
	}
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			fixture, err := os.MkdirTemp("", "runnerd-docker-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.RemoveAll(fixture); err != nil {
					t.Errorf("remove fixture: %v", err)
				}
			})
			binDir := filepath.Join(fixture, "bin")
			if err := os.MkdirAll(binDir, 0o755); err != nil {
				t.Fatal(err)
			}
			socketPath := filepath.Join(fixture, "docker.sock")
			listener, err := net.Listen("unix", socketPath)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = listener.Close()
			})
			if err := os.Chmod(socketPath, 0o600); err != nil {
				t.Fatal(err)
			}
			sudoLog := filepath.Join(fixture, "sudo.log")
			dockerdLog := filepath.Join(fixture, "dockerd.log")
			socketFixedMarker := filepath.Join(fixture, "docker-socket.fixed")
			writeExecutable(t, filepath.Join(binDir, "docker"), `#!/usr/bin/env bash
set -euo pipefail
test "$1" = info
test -f "$DOCKER_SOCKET_FIXED_MARKER"
`)
			writeExecutable(t, filepath.Join(binDir, "dockerd"), `#!/usr/bin/env bash
set -euo pipefail
printf 'unexpected dockerd start\n' >>"$DOCKERD_TEST_LOG"
: >"$DOCKER_SOCKET_FIXED_MARKER"
exit 1
`)
			writeExecutable(t, filepath.Join(binDir, "sudo"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$SUDO_TEST_LOG"
if [ "$1" = chmod ]; then
  : >"$DOCKER_SOCKET_FIXED_MARKER"
  exec "$@"
fi
if [ "$1" = chgrp ]; then
  exit 0
fi
exec "$@"
`)
			script := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"ensure-docker",
			)
			env := []string{
				"PATH=" + binDir + ":" + os.Getenv("PATH"),
				"DOCKER_BIN=" + filepath.Join(binDir, "docker"),
				"DOCKERD_BIN=" + filepath.Join(binDir, "dockerd"),
				"DOCKER_SOCKET=" + socketPath,
				"DOCKER_PID_FILE=" + filepath.Join(fixture, "docker.pid"),
				"DOCKER_LOG_FILE=" + dockerdLog,
				"DOCKERD_TEST_LOG=" + dockerdLog,
				"DOCKER_SOCKET_FIXED_MARKER=" + socketFixedMarker,
				"SUDO_TEST_LOG=" + sudoLog,
			}
			for run := 0; run < 2; run++ {
				output, err := runCommand(t, "bash", []string{script}, env...)
				if err != nil {
					t.Fatalf("ensure-docker run %d failed: %v\n%s", run+1, err, output)
				}
			}
			info, err := os.Stat(socketPath)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Perm(); got != 0o660 {
				t.Fatalf("socket mode = %04o, want 0660", got)
			}
			if contents, err := os.ReadFile(dockerdLog); err == nil && len(contents) > 0 {
				t.Fatalf("idempotent ready path started dockerd:\n%s", contents)
			}
			sudoBytes, err := os.ReadFile(sudoLog)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(sudoBytes), "chmod 0660 "+socketPath) != 2 {
				t.Fatalf("both runs must preserve group-only socket access:\n%s", sudoBytes)
			}
		})
	}
}

func TestEnsureDockerReplacesStalePIDOwnedByAnotherProcess(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the stale PID contract requires the non-root sudo path")
	}
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			fixture := t.TempDir()
			binDir := filepath.Join(fixture, "bin")
			if err := os.MkdirAll(binDir, 0o755); err != nil {
				t.Fatal(err)
			}
			readyPath := filepath.Join(fixture, "docker.ready")
			startMarker := filepath.Join(fixture, "dockerd.started")
			pidPath := filepath.Join(fixture, "docker.pid")
			socketPath := filepath.Join(fixture, "docker.sock")
			if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(socketPath, []byte("stale socket placeholder"), 0o644); err != nil {
				t.Fatal(err)
			}
			writeExecutable(t, filepath.Join(binDir, "docker"), `#!/usr/bin/env bash
set -euo pipefail
test "$1" = info
test -f "$DOCKER_READY_FILE"
`)
			writeExecutable(t, filepath.Join(binDir, "dockerd"), `#!/usr/bin/env bash
set -euo pipefail
pid_file=""
for argument in "$@"; do
  case "$argument" in
    --pidfile=*) pid_file="${argument#--pidfile=}" ;;
  esac
done
test -n "$pid_file"
printf '%s\n' "$$" >"$pid_file"
: >"$DOCKER_READY_FILE"
: >"$DOCKER_START_MARKER"
`)
			writeExecutable(t, filepath.Join(binDir, "sudo"), `#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = mkdir ]; then
  exit 0
fi
exec "$@"
`)
			script := filepath.Join(root, "templates", "github-runner-"+image, "scripts", "ensure-docker")
			output, err := runCommand(t, "bash", []string{script},
				"PATH="+binDir+":"+os.Getenv("PATH"),
				"DOCKER_BIN="+filepath.Join(binDir, "docker"),
				"DOCKERD_BIN="+filepath.Join(binDir, "dockerd"),
				"DOCKER_SOCKET="+socketPath,
				"DOCKER_PID_FILE="+pidPath,
				"DOCKER_LOG_FILE="+filepath.Join(fixture, "dockerd.log"),
				"DOCKER_READY_FILE="+readyPath,
				"DOCKER_START_MARKER="+startMarker,
			)
			if err != nil {
				t.Fatalf("ensure-docker did not replace the stale PID: %v\n%s", err, output)
			}
			if _, err := os.Stat(startMarker); err != nil {
				t.Fatalf("dockerd was not started after stale PID detection: %v", err)
			}
			if _, err := os.Stat(socketPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("stale socket was not removed: %v", err)
			}
		})
	}
}

func TestEnsureDockerRestartsUnreadyDockerdProcess(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the unready dockerd contract requires the non-root sudo path")
	}
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			fixture := t.TempDir()
			binDir := filepath.Join(fixture, "bin")
			if err := os.MkdirAll(binDir, 0o755); err != nil {
				t.Fatal(err)
			}

			staleDockerd := exec.Command("sleep", "60")
			if err := staleDockerd.Start(); err != nil {
				t.Fatal(err)
			}
			staleDockerdWait := make(chan error, 1)
			go func() {
				staleDockerdWait <- staleDockerd.Wait()
			}()
			staleDockerdExited := false
			t.Cleanup(func() {
				if staleDockerdExited {
					return
				}
				_ = staleDockerd.Process.Kill()
				<-staleDockerdWait
			})

			procDir := filepath.Join(fixture, "proc", strconv.Itoa(staleDockerd.Process.Pid))
			if err := os.MkdirAll(procDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(procDir, "comm"), []byte("dockerd\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			readyPath := filepath.Join(fixture, "docker.ready")
			startMarker := filepath.Join(fixture, "dockerd.started")
			pidPath := filepath.Join(fixture, "docker.pid")
			socketPath := filepath.Join(fixture, "docker.sock")
			if err := os.WriteFile(pidPath, []byte(strconv.Itoa(staleDockerd.Process.Pid)+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			writeExecutable(t, filepath.Join(binDir, "docker"), `#!/usr/bin/env bash
set -euo pipefail
test "$1" = info
test -f "$DOCKER_READY_FILE"
`)
			writeExecutable(t, filepath.Join(binDir, "dockerd"), `#!/usr/bin/env bash
set -euo pipefail
pid_file=""
for argument in "$@"; do
  case "$argument" in
    --pidfile=*) pid_file="${argument#--pidfile=}" ;;
  esac
done
test -n "$pid_file"
printf '%s\n' "$$" >"$pid_file"
: >"$DOCKER_READY_FILE"
: >"$DOCKER_START_MARKER"
`)
			writeExecutable(t, filepath.Join(binDir, "sudo"), `#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = mkdir ]; then
  exit 0
fi
exec "$@"
`)
			script := filepath.Join(root, "templates", "github-runner-"+image, "scripts", "ensure-docker")
			output, err := runCommand(t, "bash", []string{script},
				"PATH="+binDir+":"+os.Getenv("PATH"),
				"DOCKER_BIN="+filepath.Join(binDir, "docker"),
				"DOCKERD_BIN="+filepath.Join(binDir, "dockerd"),
				"DOCKER_SOCKET="+socketPath,
				"DOCKER_PID_FILE="+pidPath,
				"DOCKER_LOG_FILE="+filepath.Join(fixture, "dockerd.log"),
				"DOCKER_PROC_ROOT="+filepath.Join(fixture, "proc"),
				"DOCKER_READY_FILE="+readyPath,
				"DOCKER_START_MARKER="+startMarker,
			)
			if err != nil {
				t.Fatalf("ensure-docker did not restart the unready daemon: %v\n%s", err, output)
			}
			if _, err := os.Stat(startMarker); err != nil {
				t.Fatalf("dockerd was not restarted after the unready daemon was stopped: %v", err)
			}
			select {
			case err := <-staleDockerdWait:
				staleDockerdExited = true
				if err == nil {
					t.Fatal("unready dockerd exited successfully; want termination by ensure-docker")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("ensure-docker left the unready dockerd process running")
			}
		})
	}
}

func TestTemplateCurlAddsAuthenticationOnlyForGitHubAPI(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			fixture := t.TempDir()
			curlLog := filepath.Join(fixture, "curl.log")
			fakeCurl := filepath.Join(fixture, "curl")
			writeExecutable(t, fakeCurl, `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$CURL_TEST_LOG"
if [[ "$*" == *"https://api.github.com/"* ]]; then
  printf '[]\n'
fi
`)
			wrapper := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"curl",
			)
			env := []string{
				"RUNNER_TEMPLATE_CURL_BIN=" + fakeCurl,
				"GITHUB_TOKEN=fixture-token",
				"CURL_TEST_LOG=" + curlLog,
			}
			for _, url := range []string{
				"https://api.github.com/repos/actions/runner/releases",
				"https://download.docker.com/linux/ubuntu/gpg",
			} {
				output, err := runCommand(t, wrapper, []string{"-fsSL", url}, env...)
				if err != nil {
					t.Fatalf("curl wrapper failed for %s: %v\n%s", url, err, output)
				}
			}
			logBytes, err := os.ReadFile(curlLog)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(string(logBytes)), "\n")
			if len(lines) != 2 {
				t.Fatalf("curl invocation log = %q", logBytes)
			}
			if !strings.Contains(lines[0], "Authorization: Bearer fixture-token") {
				t.Fatalf("GitHub API request was not authenticated: %s", lines[0])
			}
			if strings.Contains(lines[1], "Authorization:") {
				t.Fatalf("non-GitHub request received the GitHub credential: %s", lines[1])
			}
			for lineNumber, line := range lines {
				for _, option := range []string{
					"--http1.1",
					"--connect-timeout 15",
					"--speed-limit 1024",
					"--speed-time 60",
					"--retry 5",
					"--retry-all-errors",
					"--retry-delay 2",
				} {
					if !strings.Contains(line, option) {
						t.Fatalf(
							"curl invocation %d missing resilient transport option %q: %s",
							lineNumber+1,
							option,
							line,
						)
					}
				}
			}
		})
	}
}

func TestTemplateCurlRetriesTruncatedGitHubAPIResponses(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			fixture := t.TempDir()
			curlLog := filepath.Join(fixture, "curl.log")
			attemptFile := filepath.Join(fixture, "attempt")
			fakeCurl := filepath.Join(fixture, "curl")
			writeExecutable(t, fakeCurl, `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$CURL_TEST_LOG"
attempt=0
if [ -f "$CURL_TEST_ATTEMPT" ]; then
  attempt="$(cat "$CURL_TEST_ATTEMPT")"
fi
attempt=$((attempt + 1))
printf '%s\n' "$attempt" >"$CURL_TEST_ATTEMPT"
if [ "$attempt" -eq 1 ]; then
  printf '{"truncated":'
else
  printf '[{"tag_name":"v1.2.3"}]\n'
fi
`)
			wrapper := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"curl",
			)
			output, err := runCommand(t, wrapper, []string{
				"-fsSL",
				"https://api.github.com/repos/actions/runner/releases",
			},
				"RUNNER_TEMPLATE_CURL_BIN="+fakeCurl,
				"RUNNER_TEMPLATE_GITHUB_API_RETRY_DELAY=0",
				"CURL_TEST_LOG="+curlLog,
				"CURL_TEST_ATTEMPT="+attemptFile,
			)
			if err != nil {
				t.Fatalf("curl wrapper failed: %v\n%s", err, output)
			}
			outputLines := strings.Split(strings.TrimSpace(string(output)), "\n")
			if got := outputLines[len(outputLines)-1]; got != `[{"tag_name":"v1.2.3"}]` {
				t.Fatalf("curl wrapper output = %q", strings.TrimSpace(string(output)))
			}
			if !strings.Contains(string(output), "GitHub API returned an incomplete response") {
				t.Fatalf("curl wrapper did not report the invalid response retry: %q", output)
			}
			logBytes, err := os.ReadFile(curlLog)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(strings.Split(strings.TrimSpace(string(logBytes)), "\n")); got != 2 {
				t.Fatalf("curl invocation count = %d, want 2\n%s", got, logBytes)
			}
		})
	}
}

func TestUbuntu2204TemplateWgetBoundsStalledTransfers(t *testing.T) {
	root := repositoryRoot(t)
	fixture := t.TempDir()
	wgetLog := filepath.Join(fixture, "wget.log")
	fakeWget := filepath.Join(fixture, "wget")
	writeExecutable(t, fakeWget, `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >"$WGET_TEST_LOG"
`)
	wrapper := filepath.Join(
		root,
		"templates",
		"github-runner-ubuntu-22.04",
		"scripts",
		"wget",
	)
	output, err := runCommand(t, wrapper, []string{
		"-O",
		"/tmp/output",
		"https://raw.githubusercontent.com/example/file",
	},
		"RUNNER_TEMPLATE_WGET_BIN="+fakeWget,
		"WGET_TEST_LOG="+wgetLog,
	)
	if err != nil {
		t.Fatalf("wget wrapper failed: %v\n%s", err, output)
	}
	logBytes, err := os.ReadFile(wgetLog)
	if err != nil {
		t.Fatal(err)
	}
	invocation := string(logBytes)
	for _, option := range []string{
		"--timeout=15",
		"--read-timeout=60",
		"--tries=6",
		"--waitretry=2",
		"--retry-connrefused",
	} {
		if !strings.Contains(invocation, option) {
			t.Fatalf("wget wrapper missing resilient transport option %q: %s", option, invocation)
		}
	}
}

func TestUbuntu2204TemplateUsesCurlRetriesForAptFastRawFiles(t *testing.T) {
	root := repositoryRoot(t)
	fixture := t.TempDir()
	curlLog := filepath.Join(fixture, "curl.log")
	fakeCurl := filepath.Join(fixture, "curl")
	writeExecutable(t, fakeCurl, `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >"$CURL_TEST_LOG"
`)
	wrapper := filepath.Join(
		root,
		"templates",
		"github-runner-ubuntu-22.04",
		"scripts",
		"wget",
	)
	output, err := runCommand(t, wrapper, []string{
		"https://raw.githubusercontent.com/ilikenwf/apt-fast/master/apt-fast",
		"-O",
		"/usr/local/bin/apt-fast",
	},
		"RUNNER_TEMPLATE_CURL_BIN="+fakeCurl,
		"CURL_TEST_LOG="+curlLog,
	)
	if err != nil {
		t.Fatalf("apt-fast curl fallback failed: %v\n%s", err, output)
	}
	logBytes, err := os.ReadFile(curlLog)
	if err != nil {
		t.Fatal(err)
	}
	invocation := string(logBytes)
	for _, option := range []string{
		"--http1.1",
		"--retry 8",
		"--retry-all-errors",
		"--output /usr/local/bin/apt-fast",
		"https://raw.githubusercontent.com/ilikenwf/apt-fast/master/apt-fast",
	} {
		if !strings.Contains(invocation, option) {
			t.Fatalf("apt-fast curl fallback missing %q: %s", option, invocation)
		}
	}
}

func TestRunnerTemplateDockerfilesUseQshellCompatibleRunInstructions(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			dockerfilePath := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"Dockerfile",
			)
			dockerfileBytes, err := os.ReadFile(dockerfilePath)
			if err != nil {
				t.Fatal(err)
			}
			dockerfile := string(dockerfileBytes)
			for _, unsupported := range []string{
				"RUN --mount=",
				"/run/secrets/github_token",
			} {
				if strings.Contains(dockerfile, unsupported) {
					t.Fatalf(
						"qshell v2.19.10 passes RUN arguments to the remote shell and cannot execute BuildKit-only %q",
						unsupported,
					)
				}
			}
			if !strings.Contains(dockerfile, "RUN TEMPLATE_FLAVOR=") {
				t.Fatal("template setup must remain a plain qshell-compatible RUN instruction")
			}
		})
	}
}

func TestVersionedTemplateBuildStagesPinnedUpstreamTests(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			scriptPath := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"setup-template.sh",
			)
			scriptBytes, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			script := string(scriptBytes)
			stage := `cp -a "$HELPER_SCRIPTS/../tests" /imagegeneration/tests`
			helperStage := `cp -a "$HELPER_SCRIPTS" /imagegeneration/helpers`
			validationStart := `for installer in \`
			cleanup := `find /imagegeneration -mindepth 1 -delete`
			stageIndex := strings.Index(script, stage)
			helperStageIndex := strings.Index(script, helperStage)
			validationIndex := strings.LastIndex(script, validationStart)
			cleanupIndex := strings.Index(script, cleanup)
			if stageIndex < 0 || helperStageIndex < 0 || validationIndex < 0 || cleanupIndex < 0 {
				t.Fatalf(
					"setup must stage and clean pinned upstream Pester tests and helpers: tests=%d helpers=%d validation=%d cleanup=%d",
					stageIndex,
					helperStageIndex,
					validationIndex,
					cleanupIndex,
				)
			}
			if stageIndex > validationIndex || helperStageIndex > validationIndex || cleanupIndex < validationIndex {
				t.Fatalf(
					"upstream tests and helpers must exist for installer validation and be removed afterward: tests=%d helpers=%d validation=%d cleanup=%d",
					stageIndex,
					helperStageIndex,
					validationIndex,
					cleanupIndex,
				)
			}
		})
	}
}

func TestDiskBoundedTemplatesSkipOnlyCMakeDependentNinjaAssertions(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			scriptPath := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"setup-template.sh",
			)
			scriptBytes, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			script := string(scriptBytes)
			for _, assertion := range []string{
				`It "Make a simple ninja project" -Skip {`,
				`It "build.ninja file should exist" -Skip {`,
				`It "Ninja" {`,
			} {
				if !strings.Contains(script, assertion) {
					t.Fatalf("setup must pin the disk-bounded Ninja test adjustment %q", assertion)
				}
			}
			if strings.Index(script, `It "Ninja" {`) > strings.Index(script, `install-ninja.sh`) {
				t.Fatal("pinned Ninja test adjustment must be staged before the installer runs")
			}
		})
	}
}

func TestPublicTemplatesUsePinnedMicrosoftAzCopyWithoutActionPrewarm(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			directory := filepath.Join(root, "templates", "github-runner-"+image)
			dockerfileBytes, err := os.ReadFile(filepath.Join(directory, "Dockerfile"))
			if err != nil {
				t.Fatal(err)
			}
			dockerfile := string(dockerfileBytes)
			for _, pinned := range []string{
				"ARG AZCOPY_VERSION=10.32.6",
				"ARG AZCOPY_DEB_SHA256=1a5078a8260ba7524a4400c602519d36905e592cd87a1db91a30af0da528fc86",
			} {
				if !strings.Contains(dockerfile, pinned) {
					t.Fatalf("Dockerfile must pin the Microsoft AzCopy package with %q", pinned)
				}
			}

			scriptBytes, err := os.ReadFile(filepath.Join(directory, "scripts", "setup-template.sh"))
			if err != nil {
				t.Fatal(err)
			}
			script := string(scriptBytes)
			for _, required := range []string{
				"install_azcopy_from_microsoft_package",
				"https://packages.microsoft.com/ubuntu/24.04/prod/pool/main/a/azcopy/",
				"/etc/apt/preferences.d/qiniu-azcopy",
				"Pin: version ${AZCOPY_VERSION}",
				"Pin-Priority: 1001",
				`test "$(azcopy --version)" = "azcopy version $AZCOPY_VERSION"`,
			} {
				if !strings.Contains(script, required) {
					t.Fatalf("setup must use the pinned official AzCopy package with %q", required)
				}
			}
			if strings.Contains(script, "install-actions-cache.sh") {
				t.Fatal("public templates must not prewarm the nonessential GitHub action archive cache")
			}
			if image != "ubuntu-slim" {
				invokeIndex := strings.LastIndex(script, `invoke-tests.sh" Tools azcopy`)
				pesterIndex := strings.LastIndex(script, "install_pester_for_upstream_tests")
				if invokeIndex < 0 {
					t.Fatal("versioned templates must retain the pinned upstream AzCopy test")
				}
				if pesterIndex < 0 || invokeIndex < pesterIndex {
					t.Fatalf("AzCopy Pester must run after pinned Pester is installed: pester=%d invoke=%d", pesterIndex, invokeIndex)
				}
			}
		})
	}
}

func TestPublicTemplatesInstallPinnedAzureDevOpsExtensionAfterAzureCLI(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			directory := filepath.Join(root, "templates", "github-runner-"+image)
			dockerfileBytes, err := os.ReadFile(filepath.Join(directory, "Dockerfile"))
			if err != nil {
				t.Fatal(err)
			}
			dockerfile := string(dockerfileBytes)
			for _, pinned := range []string{
				"ARG AZURE_DEVOPS_EXTENSION_VERSION=1.0.6",
				"ARG AZURE_DEVOPS_EXTENSION_SHA256=fa779e1fd6e6e1b726c3656b6a1968537c208041c6af54d2a7476772d896b34b",
			} {
				if !strings.Contains(dockerfile, pinned) {
					t.Fatalf("Dockerfile must pin the Azure DevOps extension with %q", pinned)
				}
			}

			scriptBytes, err := os.ReadFile(filepath.Join(directory, "scripts", "setup-template.sh"))
			if err != nil {
				t.Fatal(err)
			}
			script := string(scriptBytes)
			for _, required := range []string{
				"install_azure_devops_extension",
				"https://azcliprod.blob.core.windows.net/cli-extensions/azure_devops-",
				"export AZURE_EXTENSION_DIR=/opt/az/azcliextensions",
				`set_etc_environment_variable "AZURE_EXTENSION_DIR" "$AZURE_EXTENSION_DIR"`,
				`test "$(az extension show --name azure-devops --query version -o tsv)" = "$AZURE_DEVOPS_EXTENSION_VERSION"`,
			} {
				if !strings.Contains(script, required) {
					t.Fatalf("setup must install the pinned Azure DevOps extension with %q", required)
				}
			}
			if strings.Contains(script, "install-azure-devops-cli.sh") {
				t.Fatal("setup must not use the upstream Python-client Azure DevOps extension download")
			}
			azureCLIIndex := strings.LastIndex(script, "install-azure-cli.sh")
			extensionIndex := strings.LastIndex(script, "install_azure_devops_extension")
			if azureCLIIndex < 0 || extensionIndex < azureCLIIndex {
				t.Fatalf("Azure DevOps extension must install after Azure CLI: cli=%d extension=%d", azureCLIIndex, extensionIndex)
			}
			if image != "ubuntu-slim" && !strings.Contains(script, `invoke-tests.sh" CLI.Tools "Azure DevOps CLI"`) {
				t.Fatal("versioned templates must retain the pinned upstream Azure DevOps CLI test")
			}
		})
	}
}

func TestUbuntu2604TemplateInstallsICUBeforePowerShell(t *testing.T) {
	root := repositoryRoot(t)
	scriptPath := filepath.Join(
		root,
		"templates",
		"github-runner-ubuntu-26.04",
		"scripts",
		"setup-template.sh",
	)
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	icuIndex := strings.Index(script, "libicu78")
	powershellIndex := strings.LastIndex(script, "install_pinned_powershell_package")
	if icuIndex < 0 || powershellIndex < 0 {
		t.Fatalf(
			"Ubuntu 26.04 setup must install ICU before PowerShell: icu=%d powershell=%d",
			icuIndex,
			powershellIndex,
		)
	}
	if icuIndex > powershellIndex {
		t.Fatalf(
			"Ubuntu 26.04 ICU runtime must exist before PowerShell starts: icu=%d powershell=%d",
			icuIndex,
			powershellIndex,
		)
	}
}

func TestUbuntu2604TemplatePinsMicrosoftPowerShellPackage(t *testing.T) {
	root := repositoryRoot(t)
	dockerfileBytes, err := os.ReadFile(filepath.Join(
		root,
		"templates",
		"github-runner-ubuntu-26.04",
		"Dockerfile",
	))
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(dockerfileBytes)
	for _, expected := range []string{
		"ARG POWERSHELL_VERSION=7.6.4",
		"ARG POWERSHELL_DEB_SHA256=e5688e0569568d48051c49d3e93504cde47af709cdaaabd9a8892bc676b3bdf3",
	} {
		if !strings.Contains(dockerfile, expected) {
			t.Fatalf("Ubuntu 26.04 Dockerfile missing pinned PowerShell input %q", expected)
		}
	}

	scriptBytes, err := os.ReadFile(filepath.Join(
		root,
		"templates",
		"github-runner-ubuntu-26.04",
		"scripts",
		"setup-template.sh",
	))
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	for _, expected := range []string{
		`: "${POWERSHELL_VERSION:?POWERSHELL_VERSION is required}"`,
		`: "${POWERSHELL_DEB_SHA256:?POWERSHELL_DEB_SHA256 is required}"`,
		`"https://packages.microsoft.com/ubuntu/24.04/prod/pool/main/p/powershell/powershell_${POWERSHELL_VERSION}-1.deb_amd64.deb"`,
		`download_checked`,
		`install_pinned_powershell_package`,
		`test "$(pwsh --version)" = "PowerShell $POWERSHELL_VERSION"`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("Ubuntu 26.04 setup missing pinned PowerShell behavior %q", expected)
		}
	}
	if strings.Contains(script, `bash "$upstream_build/install-powershell.sh"`) {
		t.Fatal("Ubuntu 26.04 must not query the GitHub Releases API through the generic upstream installer")
	}
	if strings.Contains(script, "github.com/PowerShell/PowerShell/releases") {
		t.Fatal("Ubuntu 26.04 must use Microsoft's package pool instead of a GitHub release asset")
	}
}

func TestVersionedTemplatesInstallNetcatProviderBeforeAptCommon(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{"ubuntu-24.04", "ubuntu-26.04"} {
		t.Run(image, func(t *testing.T) {
			scriptPath := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"setup-template.sh",
			)
			scriptBytes, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			script := string(scriptBytes)
			netcatIndex := strings.Index(script, "netcat-openbsd")
			aptCommonIndex := strings.Index(script, "install-apt-common.sh")
			if netcatIndex < 0 || aptCommonIndex < 0 {
				t.Fatalf(
					"setup must install a concrete netcat provider before apt-common: netcat=%d apt-common=%d",
					netcatIndex,
					aptCommonIndex,
				)
			}
			if netcatIndex > aptCommonIndex {
				t.Fatalf(
					"netcat provider must exist before apt-common validation: netcat=%d apt-common=%d",
					netcatIndex,
					aptCommonIndex,
				)
			}
		})
	}
}

func TestCompatibilityGeneratorVerifiesConcreteNetcatProvider(t *testing.T) {
	root := repositoryRoot(t)
	const verification = `dpkg-query -W -f='${Status}' 'netcat-openbsd' | grep -qx 'install ok installed' && command -v netcat >/dev/null`

	generatorBytes, err := os.ReadFile(filepath.Join(root, "scripts", "generate-runner-image-compatibility.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generatorBytes), verification) {
		t.Fatal("compatibility generator must verify the concrete netcat provider and command")
	}

	manifestBytes, err := os.ReadFile(filepath.Join(root, "templates", "runner-images-compatibility.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Images map[string]struct {
			Entries []struct {
				UpstreamName string `json:"upstream_name"`
				Verification string `json:"verification"`
			} `json:"entries"`
		} `json:"images"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, image := range []string{"ubuntu-24.04", "ubuntu-26.04"} {
		found := false
		for _, entry := range manifest.Images[image].Entries {
			if entry.UpstreamName != "netcat" {
				continue
			}
			found = true
			if entry.Verification != verification {
				t.Fatalf("%s netcat verification = %q", image, entry.Verification)
			}
		}
		if !found {
			t.Fatalf("%s has no netcat compatibility entry", image)
		}
	}
}

func TestVersionedTemplateBuildDefersOnlyPodmanNetworkingToRuntimeConformance(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			scriptPath := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"setup-template.sh",
			)
			scriptBytes, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			script := string(scriptBytes)
			for _, required := range []string{
				`podman_networking_test=/imagegeneration/tests/Tools.Tests.ps1`,
				`grep -Fxc '    It "podman networking" -TestCases "podman CNI plugins" {' "$podman_networking_test"`,
				`sed -i 's/    It "podman networking" -TestCases "podman CNI plugins" {/    It "podman networking" -Skip -TestCases "podman CNI plugins" {/' "$podman_networking_test"`,
				`grep -Fxc '    It "podman networking" -Skip -TestCases "podman CNI plugins" {' "$podman_networking_test"`,
				`grep -Fxc '    $testCases = @("podman", "buildah", "skopeo") | ForEach-Object { @{ContainerCommand = $_} }' "$podman_networking_test"`,
				`grep -Fxc '    It "<ContainerCommand>" -TestCases $testCases {' "$podman_networking_test"`,
				`grep -Fxc '        "$ContainerCommand -v" | Should -ReturnZeroExitCode' "$podman_networking_test"`,
			} {
				if strings.Count(script, required) != 1 {
					t.Fatalf("setup must contain exactly one %q guard/patch, got %d", required, strings.Count(script, required))
				}
			}
			if strings.Contains(script, `/tmp/runner-images/images/ubuntu/scripts/tests/Tools.Tests.ps1`) {
				t.Fatal("setup must not modify the pinned upstream source tree")
			}
		})
	}
}

func TestCompatibilityGeneratorRequiresPodmanNetworkLifecycle(t *testing.T) {
	root := repositoryRoot(t)
	const verification = `command -v podman >/dev/null || exit 1; podman_network="qiniu-conformance-$$-${RANDOM}"; cleanup_podman_network() { podman network rm "$podman_network" >/dev/null 2>&1 || true; }; trap cleanup_podman_network EXIT; podman network create -d bridge "$podman_network" >/dev/null || exit 1; podman network ls --format "{{.Name}}" | grep -Fx "$podman_network" >/dev/null || exit 1; podman network rm "$podman_network" >/dev/null || exit 1; trap - EXIT`

	generatorBytes, err := os.ReadFile(filepath.Join(root, "scripts", "generate-runner-image-compatibility.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generatorBytes), verification) {
		t.Fatal("compatibility generator must emit the Podman network lifecycle assertion")
	}

	manifestBytes, err := os.ReadFile(filepath.Join(root, "templates", "runner-images-compatibility.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Images map[string]struct {
			Entries []struct {
				UpstreamName string `json:"upstream_name"`
				Status       string `json:"status"`
				Verification string `json:"verification"`
			} `json:"entries"`
		} `json:"images"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, image := range []string{"ubuntu-22.04", "ubuntu-24.04", "ubuntu-26.04"} {
		found := false
		for _, entry := range manifest.Images[image].Entries {
			if entry.UpstreamName != "Podman" {
				continue
			}
			found = true
			if entry.Status != "provided" || entry.Verification != verification {
				t.Fatalf("%s Podman assertion = status %q, verification %q", image, entry.Status, entry.Verification)
			}
		}
		if !found {
			t.Fatalf("%s has no Podman compatibility entry", image)
		}
	}

	conformanceBytes, err := os.ReadFile(filepath.Join(root, "scripts", "run-runner-image-conformance.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(conformanceBytes), `docker run --rm --privileged "$target"`) {
		t.Fatal("Docker conformance must execute the Podman lifecycle assertion with runtime namespace privileges")
	}
}

func TestCompatibilityManifestDocumentsSandboxDiskBoundary(t *testing.T) {
	root := repositoryRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "templates", "runner-images-compatibility.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Images map[string]struct {
			Entries []struct {
				UpstreamName string `json:"upstream_name"`
				Status       string `json:"status"`
				Reason       string `json:"reason"`
			} `json:"entries"`
		} `json:"images"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, image := range []string{"ubuntu-22.04", "ubuntu-24.04", "ubuntu-26.04"} {
		entries := map[string]struct {
			status string
			reason string
		}{}
		for _, entry := range manifest.Images[image].Entries {
			entries[entry.UpstreamName] = struct {
				status string
				reason string
			}{entry.Status, entry.Reason}
		}
		for _, provided := range []string{"PowerShell", "Pester", "apache2", "Buildah", "Podman", "Skopeo", "Ninja"} {
			entry, ok := entries[provided]
			if !ok || entry.status != "provided" {
				t.Fatalf("%s must provide %s, got %#v", image, provided, entry)
			}
		}
		dotnet, ok := entries[".NET Core SDK"]
		if !ok || dotnet.status != "excluded" || !strings.Contains(dotnet.reason, "22,222 MiB") {
			t.Fatalf("%s must explain the disk-bounded .NET exclusion, got %#v", image, dotnet)
		}
	}
}

func TestVersionedTemplateBuildMakesSystemctlShimVisibleToSudoAndCleansIt(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			scriptPath := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"setup-template.sh",
			)
			scriptBytes, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			script := string(scriptBytes)
			expose := `ln -s /tmp/qiniu-runner-build-tools/systemctl /usr/local/bin/systemctl`
			apache := `install-apache.sh`
			cleanup := `rm -f /usr/local/bin/systemctl`
			exposeIndex := strings.Index(script, expose)
			apacheIndex := strings.Index(script, apache)
			cleanupIndex := strings.Index(script, cleanup)
			if exposeIndex < 0 || apacheIndex < 0 || cleanupIndex < 0 {
				t.Fatalf(
					"setup must expose and clean the build-only systemctl shim: expose=%d apache=%d cleanup=%d",
					exposeIndex,
					apacheIndex,
					cleanupIndex,
				)
			}
			if exposeIndex > apacheIndex || cleanupIndex < apacheIndex {
				t.Fatalf(
					"sudo-visible systemctl shim must exist during upstream validation and be removed afterward: expose=%d apache=%d cleanup=%d",
					exposeIndex,
					apacheIndex,
					cleanupIndex,
				)
			}
			if strings.Contains(script, `rm -f /usr/bin/systemctl`) ||
				strings.Contains(script, `/usr/bin/systemctl /usr/local/bin/systemctl`) {
				t.Fatal("setup must preserve the distribution-owned /usr/bin/systemctl")
			}
		})
	}
}

func TestVersionedTemplateSystemctlShimClosesDescriptorsBoundsAndVerifiesServices(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			scriptPath := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"setup-template.sh",
			)
			scriptBytes, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			script := string(scriptBytes)
			requiredFragments := []string{
				`action=${1:-}`,
				`unit=${unit%.service}`,
				`run_isolated() {`,
				`subprocess.run(sys.argv[1:], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, close_fds=True, timeout=30)`,
				`case "$unit:$action" in`,
				`apache2:is-active)`,
				`test -s /run/apache2/apache2.pid && kill -0 "$(cat /run/apache2/apache2.pid)" 2>/dev/null`,
				`nginx:start)`,
				`run_isolated /usr/sbin/nginx`,
				`nginx:stop)`,
				`run_isolated /usr/sbin/nginx -s quit`,
				`nginx:is-active)`,
				`test -s /run/nginx.pid && kill -0 "$(cat /run/nginx.pid)" 2>/dev/null`,
				`if [ -x "/etc/init.d/$unit" ]; then`,
				`service_status() {`,
				`run_isolated /usr/sbin/service "$unit" status`,
				`run_isolated /usr/sbin/service "$unit" "$action"`,
				`service_result=$?`,
				`if service_status; then`,
				`[ "$action" = stop ] && exit "$service_result"`,
				`[ "$action" = stop ] && exit 0`,
			}
			if image == "ubuntu-slim" {
				requiredFragments = append(requiredFragments,
					`apache2:start|apache2:stop|apache2:restart)`,
					`run_isolated /usr/sbin/apachectl "$action"`,
				)
			} else {
				requiredFragments = append(requiredFragments,
					`run_detached_until_tcp_state() {`,
					`subprocess.Popen(`,
					`start_new_session=True`,
					`controller_pid_file = "/tmp/qiniu-runner-build-tools/apache2-controller.pid"`,
					`controller_file.write(str(process.pid))`,
					`socket.create_connection((host, port), timeout=0.2)`,
					`start_apache() {`,
					`run_detached_until_tcp_state active 127.0.0.1 80 /usr/sbin/apachectl -DFOREGROUND`,
					`stop_apache() {`,
					`os.killpg(controller_pid, signal.SIGTERM)`,
					`apache2:start)`,
					`apache2:stop)`,
					`apache2:restart)`,
				)
			}
			for _, required := range requiredFragments {
				if !strings.Contains(script, required) {
					t.Fatalf("systemctl shim must isolate, bound, and verify available SysV services; missing %q", required)
				}
			}
		})
	}
}

func TestVersionedTemplateBuildStopsValidatedServicesBetweenInstallers(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			scriptPath := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"setup-template.sh",
			)
			scriptBytes, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			script := string(scriptBytes)
			installerRun := `run_upstream_installer "$upstream_build/$installer"`
			serviceCleanup := `install-apache.sh) stop_validated_service apache2 ;;`
			installerRunIndex := strings.LastIndex(script, installerRun)
			if installerRunIndex < 0 {
				t.Fatal("versioned build must run the pinned installer loop")
			}
			installerLoop := script[installerRunIndex:]
			serviceCleanupIndex := strings.Index(installerLoop, serviceCleanup)
			loopEndIndex := strings.Index(installerLoop, "\n  done")
			if serviceCleanupIndex < 0 || loopEndIndex < 0 {
				t.Fatalf(
					"versioned build must stop validated services between installers: installer=%d cleanup=%d loop_end=%d",
					installerRunIndex,
					serviceCleanupIndex,
					loopEndIndex,
				)
			}
			if serviceCleanupIndex > loopEndIndex {
				t.Fatalf(
					"service cleanup must run inside the installer loop: cleanup=%d loop_end=%d",
					serviceCleanupIndex,
					loopEndIndex,
				)
			}
			for _, required := range []string{
				`stop_validated_service() {`,
				`if [ "$unit" = apache2 ]; then`,
				`apache2ctl stop`,
				`if ! ss -ltn 'sport = :80' | grep -q LISTEN; then`,
				`echo "validated service kept port 80 busy after cleanup: apache2" >&2`,
				`systemctl stop "$unit" || true`,
				`if systemctl is-active --quiet "$unit"; then`,
				`return 1`,
			} {
				if !strings.Contains(script, required) {
					t.Fatalf("service cleanup must verify the service stopped; missing %q", required)
				}
			}
		})
	}
}

func TestVersionedTemplateBuildReloadsPersistedEnvironmentBeforePipxPackages(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			scriptPath := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"setup-template.sh",
			)
			scriptBytes, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			script := string(scriptBytes)
			configureEnvironment := `bash "$upstream_build/configure-environment.sh"`
			reloadEnvironment := `. "$HELPER_SCRIPTS/etc-environment.sh"
  reload_etc_environment`
			installPipx := `bash "$upstream_build/install-pipx-packages.sh"`
			configureIndex := strings.Index(script, configureEnvironment)
			reloadIndex := strings.Index(script, reloadEnvironment)
			pipxIndex := strings.Index(script, installPipx)
			if configureIndex < 0 || reloadIndex < 0 || pipxIndex < 0 {
				t.Fatalf(
					"versioned build must reload persisted environment before pipx packages: configure=%d reload=%d pipx=%d",
					configureIndex,
					reloadIndex,
					pipxIndex,
				)
			}
			if configureIndex > reloadIndex || reloadIndex > pipxIndex {
				t.Fatalf(
					"persisted environment reload must follow environment configuration and precede pipx packages: configure=%d reload=%d pipx=%d",
					configureIndex,
					reloadIndex,
					pipxIndex,
				)
			}
		})
	}
}

func TestVersionedTemplateBuildUsesDiskBoundedToolset(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			scriptPath := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"setup-template.sh",
			)
			scriptBytes, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			script := string(scriptBytes)
			start := strings.Index(script, "else\n  . /etc/os-release")
			if start < 0 {
				t.Fatal("cannot find versioned installer branch")
			}
			end := strings.Index(script[start:], "\nfi\n\ninstall_docker_for_sandbox")
			if end < 0 {
				t.Fatalf("cannot isolate versioned installer branch: start=%d end=%d", start, end)
			}
			versioned := script[start : start+end]
			for _, required := range []string{
				"install-apt-common.sh",
				"install_azcopy_from_microsoft_package",
				"install-azure-cli.sh",
				"install-apache.sh",
				"install-aws-tools.sh",
				"install-container-tools.sh",
				"install-git.sh",
				"install-github-cli.sh",
				"install-google-cloud-cli.sh",
				"install-nodejs.sh",
				"install-python.sh",
				"install-yq.sh",
				"install-ninja.sh",
				"install_pester_for_upstream_tests",
			} {
				if !strings.Contains(versioned, required) {
					t.Fatalf("disk-bounded versioned toolset must retain %q", required)
				}
			}
			for _, excluded := range []string{
				"install-actions-cache.sh",
				"install-azcopy.sh",
				"install-dotnetcore-sdk.sh",
				"install-android-sdk.sh",
				"install-codeql-bundle.sh",
				"install-homebrew.sh",
				"Install-PowerShellModules.ps1",
				"Install-PowerShellAzModules.ps1",
				"Install-Toolset.ps1",
				"Configure-Toolset.ps1",
			} {
				if strings.Contains(versioned, excluded) {
					t.Fatalf("disk-bounded versioned toolset must not install %q", excluded)
				}
			}
		})
	}
}

func TestRunnerTemplateRetriesGoogleCloudInstaller(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			scriptPath := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"setup-template.sh",
			)
			scriptBytes, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			script := string(scriptBytes)
			for _, required := range []string{
				"run_upstream_installer() {",
				`install-google-cloud-cli.sh) max_attempts=3 ;;`,
				`run_upstream_installer "$upstream_build/$installer"`,
			} {
				if !strings.Contains(script, required) {
					t.Fatalf("template must retry the Google Cloud installer as a bounded unit; missing %q", required)
				}
			}
			if strings.Contains(script, `bash "$upstream_build/$installer"`) {
				t.Fatal("installer loop bypasses bounded upstream installer wrapper")
			}
		})
	}
}

func TestRunnerTemplateBuildUsesBoundedHTTPSAptSources(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			scriptPath := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"setup-template.sh",
			)
			scriptBytes, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			script := string(scriptBytes)
			bootstrapInstall := strings.Index(
				script,
				"\napt-get install -y --no-install-recommends ca-certificates\n",
			)
			firstReliableIndex := strings.Index(script, "\nconfigure_reliable_apt_sources\n")
			if bootstrapInstall < 0 || firstReliableIndex < 0 {
				t.Fatalf(
					"template must configure reliable apt sources after CA bootstrap: install=%d reliable=%d",
					bootstrapInstall,
					firstReliableIndex,
				)
			}
			if firstReliableIndex < bootstrapInstall {
				t.Fatalf(
					"HTTPS apt source hardening must follow CA bootstrap: install=%d reliable=%d",
					bootstrapInstall,
					firstReliableIndex,
				)
			}
			if strings.Count(script, "configure_reliable_apt_sources") < 3 {
				t.Fatal("apt source hardening must run initially and after upstream source configuration")
			}
			for _, required := range []string{
				`configure_reliable_apt_sources() {`,
				`/etc/apt/sources.list`,
				`/etc/apt/sources.list.d/ubuntu.sources`,
				`http://archive.ubuntu.com/ubuntu`,
				`https://archive.ubuntu.com/ubuntu`,
				`http://security.ubuntu.com/ubuntu`,
				`mirror+file:/etc/apt/apt-mirrors.txt`,
				`https://mirrors.tuna.tsinghua.edu.cn/ubuntu/`,
				`https://mirrors.edge.kernel.org/ubuntu/`,
				`https://archive.ubuntu.com/ubuntu/`,
				`Acquire::Retries "5";`,
				`Acquire::http::Timeout "30";`,
				`Acquire::https::Timeout "30";`,
			} {
				if !strings.Contains(script, required) {
					t.Fatalf("apt source hardening missing %q", required)
				}
			}
		})
	}
}

func TestRunnerTemplateBuildBootstrapsTLSCertificatesBeforeStrictHTTPS(t *testing.T) {
	root := repositoryRoot(t)
	for _, image := range []string{
		"ubuntu-slim",
		"ubuntu-22.04",
		"ubuntu-24.04",
		"ubuntu-26.04",
	} {
		t.Run(image, func(t *testing.T) {
			scriptPath := filepath.Join(
				root,
				"templates",
				"github-runner-"+image,
				"scripts",
				"setup-template.sh",
			)
			scriptBytes, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			script := string(scriptBytes)
			bootstrap := strings.Index(
				script,
				"apt-get update\napt-get install -y --no-install-recommends ca-certificates",
			)
			strictSetup := strings.Index(
				script,
				"apt-get install -y --no-install-recommends ca-certificates\nconfigure_reliable_apt_sources\napt-get update",
			)
			if bootstrap < 0 || strictSetup < 0 {
				t.Fatalf(
					"template must bootstrap CA certificates before enabling HTTPS mirrors: bootstrap=%d strict=%d",
					bootstrap,
					strictSetup,
				)
			}
			if strings.Contains(script, "Acquire::https::Verify-Peer=false") {
				t.Fatal("template must never disable apt HTTPS peer verification")
			}
		})
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
	processes = append(processes, qnsandbox.ProcessInfo{PID: 30, Tag: &runnerTag})
	if _, ok := recoveredRunnerPID(processes, 0); ok {
		t.Fatal("expected ambiguous tagged runner processes to be rejected")
	}
	if pid, ok := recoveredRunnerPID(processes, 20); !ok || pid != 20 {
		t.Fatalf("expected persisted PID to disambiguate tagged processes, got pid=%d ok=%t", pid, ok)
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
