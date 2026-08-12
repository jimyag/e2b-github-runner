package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qiniu/ci-runner/internal/config"
)

func TestNewCacheS3UsesDefaultPrefix(t *testing.T) {
	storage, err := newCacheS3(cacheS3Config{
		Region:          "us-north-1",
		Bucket:          "cache-bucket",
		Endpoint:        "https://s3.example.test",
		AccessKeyID:     "access-key",
		SecretAccessKey: "secret-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if storage.prefix != "gh-actions-cache" {
		t.Fatalf("prefix = %q, want default gh-actions-cache", storage.prefix)
	}
}

func TestHandleSandboxRegionsOmitsS3Mappings(t *testing.T) {
	srv := &Server{cfg: config.Config{SandboxRegions: []config.SandboxRegionConfig{{
		ID:            "us-south-1",
		Label:         "United States · Dallas 1",
		SandboxAPIURL: "https://sandbox.example.test",
		S3Region:      "us-north-1",
		S3Endpoint:    "https://internal-s3.example.test",
	}}}}
	recorder := httptest.NewRecorder()
	srv.handleSandboxRegions(recorder, httptest.NewRequest(http.MethodGet, "/sandbox/regions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var entries []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0]["id"] != "us-south-1" || entries[0]["api_url"] != "https://sandbox.example.test" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
	if _, ok := entries[0]["s3_region"]; ok {
		t.Fatalf("public catalog leaked s3_region: %#v", entries[0])
	}
	if _, ok := entries[0]["s3_endpoint"]; ok {
		t.Fatalf("public catalog leaked s3_endpoint: %#v", entries[0])
	}
}
