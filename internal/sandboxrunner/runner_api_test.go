package sandboxrunner

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestService creates an E2BService wired to the given httptest.Server.
func newTestService(t *testing.T, ts *httptest.Server) *E2BService {
	t.Helper()
	svc, err := NewE2BService("test-api-key", ts.URL, ts.Client())
	if err != nil {
		t.Fatalf("NewE2BService: %v", err)
	}
	return svc
}

// templateJSON returns a minimal TemplateWithBuilds JSON payload with the given build status entries.
// Pass no statuses to return an empty builds list.
func templateJSON(id string, statuses ...string) string {
	builds := make([]string, 0, len(statuses))
	for i, s := range statuses {
		builds = append(builds, `{"buildID":"00000000-0000-0000-0000-`+
			strings.Repeat("0", 11)+string(rune('0'+i))+`","cpuCount":2,"memoryMB":1024,`+
			`"createdAt":"2024-01-01T00:00:00Z","updatedAt":"2024-01-01T00:00:00Z",`+
			`"status":"`+s+`"}`)
	}
	body := `{"templateID":"` + id + `","aliases":[],"public":false,"spawnCount":0,` +
		`"createdAt":"2024-01-01T00:00:00Z","updatedAt":"2024-01-01T00:00:00Z",` +
		`"lastSpawnedAt":null,"builds":[` + strings.Join(builds, ",") + `]}`
	return body
}

// serveTemplate returns an http.Handler that serves the given status code and body for all requests.
func serveTemplate(code int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	})
}

func TestValidateTemplate_EmptyID(t *testing.T) {
	// No HTTP call is made; the function short-circuits before calling the API.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected HTTP call for empty template ID")
	}))
	defer ts.Close()

	svc := newTestService(t, ts)
	err := svc.ValidateTemplate(context.Background(), "")
	if !errors.Is(err, ErrTemplateRequired) {
		t.Fatalf("expected ErrTemplateRequired, got %v", err)
	}
}

func TestValidateTemplate_WhitespaceOnlyID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected HTTP call for whitespace template ID")
	}))
	defer ts.Close()

	svc := newTestService(t, ts)
	err := svc.ValidateTemplate(context.Background(), "   ")
	if !errors.Is(err, ErrTemplateRequired) {
		t.Fatalf("expected ErrTemplateRequired, got %v", err)
	}
}

func TestValidateTemplate_NotFound(t *testing.T) {
	ts := httptest.NewServer(serveTemplate(http.StatusNotFound, `{"code":"not_found","message":"template not found"}`))
	defer ts.Close()

	svc := newTestService(t, ts)
	err := svc.ValidateTemplate(context.Background(), "missing-tpl")
	if !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("expected ErrTemplateNotFound, got %v", err)
	}
}

func TestValidateTemplate_NoBuilds(t *testing.T) {
	// Template exists but has no builds — considered valid.
	ts := httptest.NewServer(serveTemplate(http.StatusOK, templateJSON("tpl-no-builds")))
	defer ts.Close()

	svc := newTestService(t, ts)
	err := svc.ValidateTemplate(context.Background(), "tpl-no-builds")
	if err != nil {
		t.Fatalf("expected nil error for template with no builds, got %v", err)
	}
}

func TestValidateTemplate_ReadyBuild(t *testing.T) {
	ts := httptest.NewServer(serveTemplate(http.StatusOK, templateJSON("tpl-ready", "ready")))
	defer ts.Close()

	svc := newTestService(t, ts)
	err := svc.ValidateTemplate(context.Background(), "tpl-ready")
	if err != nil {
		t.Fatalf("expected nil error for template with ready build, got %v", err)
	}
}

func TestValidateTemplate_UploadedBuild(t *testing.T) {
	ts := httptest.NewServer(serveTemplate(http.StatusOK, templateJSON("tpl-uploaded", "uploaded")))
	defer ts.Close()

	svc := newTestService(t, ts)
	err := svc.ValidateTemplate(context.Background(), "tpl-uploaded")
	if err != nil {
		t.Fatalf("expected nil error for template with uploaded build, got %v", err)
	}
}

func TestValidateTemplate_AllBuildsNotReady(t *testing.T) {
	// All builds are in "building" or "error" state — none usable.
	ts := httptest.NewServer(serveTemplate(http.StatusOK, templateJSON("tpl-building", "building", "error")))
	defer ts.Close()

	svc := newTestService(t, ts)
	err := svc.ValidateTemplate(context.Background(), "tpl-building")
	if !errors.Is(err, ErrTemplateNotReady) {
		t.Fatalf("expected ErrTemplateNotReady, got %v", err)
	}
}

func TestValidateTemplate_MixedBuilds_OneReady(t *testing.T) {
	// Mix of non-ready and ready builds — should pass because at least one is usable.
	ts := httptest.NewServer(serveTemplate(http.StatusOK, templateJSON("tpl-mixed", "building", "ready")))
	defer ts.Close()

	svc := newTestService(t, ts)
	err := svc.ValidateTemplate(context.Background(), "tpl-mixed")
	if err != nil {
		t.Fatalf("expected nil error when at least one build is ready, got %v", err)
	}
}

func TestValidateTemplate_ServerError(t *testing.T) {
	// 500 response — should propagate as a non-sentinel API error.
	ts := httptest.NewServer(serveTemplate(http.StatusInternalServerError, `{"code":"internal","message":"server error"}`))
	defer ts.Close()

	svc := newTestService(t, ts)
	err := svc.ValidateTemplate(context.Background(), "tpl-error")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if errors.Is(err, ErrTemplateNotFound) || errors.Is(err, ErrTemplateNotReady) || errors.Is(err, ErrTemplateRequired) {
		t.Fatalf("expected generic API error, got sentinel: %v", err)
	}
}

func TestNewE2BService_ValidConfig(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer ts.Close()

	svc, err := NewE2BService("my-api-key", ts.URL, ts.Client())
	if err != nil {
		t.Fatalf("NewE2BService: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestNewE2BService_EmptyAPIKey(t *testing.T) {
	// Empty API key is allowed — the SDK doesn't validate it at construction time.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer ts.Close()

	svc, err := NewE2BService("", ts.URL, ts.Client())
	if err != nil {
		t.Fatalf("NewE2BService with empty key: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestE2BServiceListDefaultTemplates(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/default-templates" {
			t.Fatalf("request path = %q, want %q", r.URL.Path, "/default-templates")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"templateID":"tpl-public-runner","names":["github-runner-ubuntu-24-04"],"buildStatus":"ready","cpuCount":2,"memoryMB":4096,"diskSizeMB":20480,"public":true,"spawnCount":3,"updatedAt":"2024-01-01T00:00:00Z"}]`))
	}))
	defer ts.Close()

	svc := newTestService(t, ts)
	items, err := svc.ListDefaultTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListDefaultTemplates: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one template, got %d", len(items))
	}
	got := items[0]
	if got.TemplateID != "tpl-public-runner" {
		t.Errorf("TemplateID = %q, want %q", got.TemplateID, "tpl-public-runner")
	}
	if len(got.Names) != 1 || got.Names[0] != "github-runner-ubuntu-24-04" {
		t.Errorf("Names = %#v, want [github-runner-ubuntu-24-04]", got.Names)
	}
	if got.BuildStatus != "ready" {
		t.Errorf("BuildStatus = %q, want %q", got.BuildStatus, "ready")
	}
	if !got.Public {
		t.Error("Public = false, want true")
	}
}
