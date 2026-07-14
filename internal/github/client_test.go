package github

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVerifyWebhookSignature(t *testing.T) {
	body := []byte(`{"ok":true}`)
	secret := "secret"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !VerifyWebhookSignature(secret, body, sig) {
		t.Fatal("expected valid signature")
	}
	if VerifyWebhookSignature(secret, body, "sha256=deadbeef") {
		t.Fatal("expected invalid signature")
	}
	if VerifyWebhookSignature(secret, body, "") {
		t.Fatal("expected missing signature to fail")
	}
}

func TestLabelsUnmarshalAndMatch(t *testing.T) {
	var event WorkflowJobEvent
	if err := json.Unmarshal([]byte(`{"workflow_job":{"id":1,"labels":[{"name":"self-hosted"},{"name":"e2b"}]}}`), &event); err != nil {
		t.Fatal(err)
	}
	if !LabelsMatch(event.WorkflowJob.Labels, []string{"self-hosted", "e2b"}) {
		t.Fatalf("expected labels to match: %#v", event.WorkflowJob.Labels)
	}
	if !LabelsMatch([]string{"e2b"}, []string{"self-hosted", "e2b", "las-sandbox"}) {
		t.Fatal("expected runner labels to satisfy a subset of requested labels")
	}
	if LabelsMatch(event.WorkflowJob.Labels, []string{"self-hosted", "linux"}) {
		t.Fatalf("expected labels not to match")
	}
}

func TestCreateRegistrationToken(t *testing.T) {
	var requests int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/repos/o/r/actions/runners/registration-token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"token":"runner-token","expires_at":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `"}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, ts.Client())
	token, err := client.CreateRegistrationToken(t.Context(), "o/r", "")
	if err != nil {
		t.Fatal(err)
	}
	if token.Token != "runner-token" {
		t.Fatalf("unexpected token: %q", token.Token)
	}
	token, err = client.CreateRegistrationToken(t.Context(), "o/r", "")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("expected cached token to avoid second request, got %d requests", requests)
	}
}

func TestCreateRegistrationTokenFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, ts.Client())
	if _, err := client.CreateRegistrationToken(t.Context(), "o/r", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateRegistrationTokenRequiresRepository(t *testing.T) {
	client := NewClient("https://api.github.com", nil)
	if _, err := client.CreateRegistrationToken(t.Context(), "", ""); err == nil {
		t.Fatal("expected missing repository error")
	}
}

func TestCreateRegistrationTokenUsesRequestRepository(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/other/repo/actions/runners/registration-token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"token":"runner-token","expires_at":"2026-05-18T10:00:00Z"}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, ts.Client())
	token, err := client.CreateRegistrationToken(t.Context(), "other/repo", "")
	if err != nil {
		t.Fatal(err)
	}
	if token.Token != "runner-token" {
		t.Fatalf("unexpected token: %q", token.Token)
	}
	if got, err := client.RunnerURL("other/repo", ""); err != nil || got != "https://github.com/other/repo" {
		t.Fatalf("unexpected runner url %q err=%v", got, err)
	}
}

func TestNewAppClientUsesConfiguredBaseURLForInstallationTransport(t *testing.T) {
	privateKey := testPrivateKeyFile(t)
	client, err := NewAppClient("https://github.example/api/v3", AppAuth{
		AppID:          123,
		InstallationID: 456,
		PrivateKeyFile: privateKey,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if client.appAuth == nil {
		t.Fatal("expected app authenticator")
	}
	if client.appAuth.baseURL != "https://github.example/api/v3" {
		t.Fatalf("unexpected app auth base URL: %s", client.appAuth.baseURL)
	}
	if client.appAuth.staticInstallationID != 456 {
		t.Fatalf("unexpected static installation id: %d", client.appAuth.staticInstallationID)
	}
}

func TestAppClientResolvesDynamicRepositoryInstallation(t *testing.T) {
	privateKey := testPrivateKeyFile(t)
	var installationLookups int
	var tokenRequests int
	var registrationAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/installation":
			installationLookups++
			if r.Header.Get("Authorization") == "" {
				t.Fatal("expected app JWT authorization for installation lookup")
			}
			w.Write([]byte(`{"id":456}`))
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/456/access_tokens":
			tokenRequests++
			if r.Header.Get("Authorization") == "" {
				t.Fatal("expected app JWT authorization for installation token")
			}
			w.Write([]byte(`{"token":"installation-token","expires_at":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/actions/runners/registration-token":
			registrationAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"token":"runner-token","expires_at":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `"}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ts.Close()

	client, err := NewAppClient(ts.URL, AppAuth{
		AppID:          123,
		PrivateKeyFile: privateKey,
	}, ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	token, err := client.CreateRegistrationToken(t.Context(), "o/r", "")
	if err != nil {
		t.Fatal(err)
	}
	if token.Token != "runner-token" {
		t.Fatalf("unexpected token: %q", token.Token)
	}
	if registrationAuth != "token installation-token" {
		t.Fatalf("unexpected installation auth header: %q", registrationAuth)
	}
	if installationLookups != 1 || tokenRequests != 1 {
		t.Fatalf("expected cached dynamic installation, lookups=%d tokenRequests=%d", installationLookups, tokenRequests)
	}
}

func TestAppClientUsesRepositoryInstallationForOrgRunnerTarget(t *testing.T) {
	privateKey := testPrivateKeyFile(t)
	var registrationAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/installation":
			w.Write([]byte(`{"id":456}`))
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/456/access_tokens":
			w.Write([]byte(`{"token":"installation-token","expires_at":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/orgs/o/actions/runners/registration-token":
			registrationAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"token":"runner-token","expires_at":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `"}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ts.Close()

	client, err := NewAppClient(ts.URL, AppAuth{
		AppID:          123,
		PrivateKeyFile: privateKey,
	}, ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	token, err := client.CreateRegistrationToken(t.Context(), "o/r", "default")
	if err != nil {
		t.Fatal(err)
	}
	if token.Token != "runner-token" || registrationAuth != "token installation-token" {
		t.Fatalf("unexpected token=%q auth=%q", token.Token, registrationAuth)
	}
	if got, err := client.RunnerURL("o/r", "default"); err != nil || got != "https://github.com/o" {
		t.Fatalf("unexpected runner url %q err=%v", got, err)
	}
}

func TestCreateOrgRegistrationToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orgs/my-org/actions/runners/registration-token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"token":"runner-token","expires_at":"2026-05-18T10:00:00Z"}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, ts.Client())
	token, err := client.CreateRegistrationToken(t.Context(), "my-org/repo", "default")
	if err != nil {
		t.Fatal(err)
	}
	if token.Token != "runner-token" {
		t.Fatalf("unexpected token: %q", token.Token)
	}
	if got, err := client.RunnerURL("my-org/repo", "default"); err != nil || got != "https://github.com/my-org" {
		t.Fatalf("unexpected runner url: %s", got)
	}
}

func TestTokenAndBasicAuthClientsSetAuthorizationHeader(t *testing.T) {
	var gotTokenAuth string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTokenAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"token":"runner-token","expires_at":"2026-05-18T10:00:00Z"}`))
	}))
	defer tokenServer.Close()

	tokenClient := NewTokenClient(tokenServer.URL, "ghp_test", tokenServer.Client())
	if _, err := tokenClient.CreateRegistrationToken(t.Context(), "o/r", ""); err != nil {
		t.Fatal(err)
	}
	if gotTokenAuth != "Bearer ghp_test" {
		t.Fatalf("unexpected token auth header: %q", gotTokenAuth)
	}

	var gotBasicAuth string
	basicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBasicAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"token":"runner-token","expires_at":"2026-05-18T10:00:00Z"}`))
	}))
	defer basicServer.Close()

	basicClient := NewBasicAuthClient(basicServer.URL, "octo", "secret", basicServer.Client())
	if _, err := basicClient.CreateRegistrationToken(t.Context(), "o/r", ""); err != nil {
		t.Fatal(err)
	}
	wantBasic := "Basic " + base64.StdEncoding.EncodeToString([]byte("octo:secret"))
	if gotBasicAuth != wantBasic {
		t.Fatalf("unexpected basic auth header: %q", gotBasicAuth)
	}
}

func TestListAndRemoveRunnerByName(t *testing.T) {
	var deleted bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/actions/runners":
			w.Write([]byte(`{"runners":[{"id":42,"name":"e2b-1001","status":"offline","busy":false}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/repos/o/r/actions/runners/42":
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, ts.Client())
	removed, err := client.RemoveRunnerByName(t.Context(), "o/r", "", "e2b-1001")
	if err != nil {
		t.Fatal(err)
	}
	if !removed || !deleted {
		t.Fatalf("expected runner to be removed, removed=%v deleted=%v", removed, deleted)
	}
}

func TestListWorkflowRunJobs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/actions/runs/123/jobs" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("per_page") != "100" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jobs":[{"id":1001,"name":"test","status":"queued","labels":["self-hosted","e2b"]}]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, ts.Client())
	jobs, err := client.ListWorkflowRunJobs(t.Context(), "o/r", 123)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != 1001 || jobs[0].Labels[1] != "e2b" {
		t.Fatalf("unexpected jobs: %#v", jobs)
	}
}

func TestListWorkflowRunJobsFollowsPagination(t *testing.T) {
	var requests int
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/repos/o/r/actions/runs/123/jobs" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("per_page") != "100" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "":
			w.Header().Set("Link", `<`+ts.URL+`/repos/o/r/actions/runs/123/jobs?per_page=100&page=2>; rel="next"`)
			w.Write([]byte(`{"jobs":[{"id":1001,"name":"first","status":"queued","labels":["self-hosted"]}]}`))
		case "2":
			w.Write([]byte(`{"jobs":[{"id":1002,"name":"second","status":"queued","labels":["e2b"]}]}`))
		default:
			t.Fatalf("unexpected page: %s", r.URL.Query().Get("page"))
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, ts.Client())
	jobs, err := client.ListWorkflowRunJobs(t.Context(), "o/r", 123)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("expected 2 requests, got %d", requests)
	}
	if len(jobs) != 2 || jobs[0].ID != 1001 || jobs[1].ID != 1002 {
		t.Fatalf("unexpected jobs: %#v", jobs)
	}
}

func TestGetWorkflowJob(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/actions/jobs/1001" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":1001,"name":"test","status":"completed","conclusion":"cancelled","labels":["self-hosted","e2b"]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, ts.Client())
	job, err := client.GetWorkflowJob(t.Context(), "o/r", 1001)
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != 1001 || job.Status != "completed" || job.Conclusion != "cancelled" {
		t.Fatalf("unexpected job: %#v", job)
	}
}

func TestDownloadWorkflowJobLogs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/actions/jobs/1001/logs" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("github log\n"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, ts.Client())
	body, contentType, err := client.DownloadWorkflowJobLogs(t.Context(), "o/r", 1001)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "github log\n" || !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("unexpected logs body=%q contentType=%q", string(body), contentType)
	}
}

func TestDownloadWorkflowJobLogsFollowsRedirectWithoutGitHubAuth(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/actions/jobs/1001/logs":
			if r.Header.Get("Authorization") != "Bearer github-token" {
				t.Fatalf("expected github authorization on api request, got %q", r.Header.Get("Authorization"))
			}
			http.Redirect(w, r, ts.URL+"/download/logs.zip?sig=temporary", http.StatusFound)
		case "/download/logs.zip":
			if r.Header.Get("Authorization") != "" {
				t.Fatalf("expected redirect download without github authorization, got %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/zip")
			w.Write([]byte("zip bytes\n"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	client := NewTokenClient(ts.URL, "github-token", ts.Client())
	body, contentType, err := client.DownloadWorkflowJobLogs(t.Context(), "o/r", 1001)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "zip bytes\n" || !strings.HasPrefix(contentType, "application/zip") {
		t.Fatalf("unexpected logs body=%q contentType=%q", string(body), contentType)
	}
}

func TestListUserInstallationsUsesOAuthToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user/installations" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer user-token" {
			t.Fatalf("expected bearer user token, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"installations":[{"id":987,"account":{"login":"octo-org","name":"Octo Org","avatar_url":"https://avatars.example/o.png"}}]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, ts.Client())
	installations, err := client.ListUserInstallations(t.Context(), "user-token")
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 1 || installations[0].ID != 987 || installations[0].AccountLogin != "octo-org" {
		t.Fatalf("unexpected installations: %#v", installations)
	}
}

func TestDownloadWorkflowJobLogsRedirectUsesConfiguredTransport(t *testing.T) {
	client := NewTokenClient("https://api.github.test", "github-token", &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.String() {
			case "https://api.github.test/repos/o/r/actions/jobs/1001/logs":
				if r.Header.Get("Authorization") != "Bearer github-token" {
					t.Fatalf("expected github authorization on api request, got %q", r.Header.Get("Authorization"))
				}
				return textResponse(http.StatusFound, "", map[string]string{
					"Location": "https://actions-results.test/download/logs.zip?sig=temporary",
				}), nil
			case "https://actions-results.test/download/logs.zip?sig=temporary":
				if r.Header.Get("Authorization") != "" {
					t.Fatalf("expected redirect download without github authorization, got %q", r.Header.Get("Authorization"))
				}
				return textResponse(http.StatusOK, "zip bytes\n", map[string]string{
					"Content-Type": "application/zip",
				}), nil
			default:
				t.Fatalf("unexpected request: %s", r.URL.String())
				return nil, nil
			}
		}),
		Timeout: time.Second,
	})
	body, contentType, err := client.DownloadWorkflowJobLogs(t.Context(), "o/r", 1001)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "zip bytes\n" || !strings.HasPrefix(contentType, "application/zip") {
		t.Fatalf("unexpected logs body=%q contentType=%q", string(body), contentType)
	}
}

func textResponse(status int, body string, headers map[string]string) *http.Response {
	resp := &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	for key, value := range headers {
		resp.Header.Set(key, value)
	}
	return resp
}

func TestReadActionsLogBodyRejectsOversizedLog(t *testing.T) {
	_, err := readActionsLogBody(strings.NewReader("123456"), 5)
	if err == nil {
		t.Fatal("expected oversized log error")
	}
	if !strings.Contains(err.Error(), "exceeds 5 bytes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func testPrivateKeyFile(t *testing.T) string {
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
