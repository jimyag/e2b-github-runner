//go:build !development

package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestFallbackContentTypeForWebAssets(t *testing.T) {
	tests := map[string]string{
		".html": "text/html; charset=utf-8",
		".js":   "text/javascript; charset=utf-8",
		".mjs":  "text/javascript; charset=utf-8",
		".css":  "text/css; charset=utf-8",
		".svg":  "image/svg+xml",
		".png":  "image/png",
		".json": "application/json",
	}
	for ext, want := range tests {
		if got := fallbackContentType(ext); got != want {
			t.Fatalf("fallbackContentType(%q) = %q, want %q", ext, got, want)
		}
	}
	if got := fallbackContentType(".txt"); got != "" {
		t.Fatalf("fallbackContentType for unknown extension = %q, want empty", got)
	}
}

func TestServeUIAssetUsesRouteSpecificCachePolicy(t *testing.T) {
	hashedAsset := firstEmbeddedUIAssetWithSuffix(t, ".js")
	tests := []struct {
		name         string
		requestPath  string
		assetName    string
		cacheControl string
	}{
		{
			name:         "html shell",
			requestPath:  "/",
			assetName:    "/index.html",
			cacheControl: "no-store",
		},
		{
			name:         "spa fallback",
			requestPath:  "/github/pulls/qiniu/ci-runner/39/jobs",
			assetName:    "/github/pulls/qiniu/ci-runner/39/jobs",
			cacheControl: "no-store",
		},
		{
			name:         "content hashed asset",
			requestPath:  hashedAsset,
			assetName:    hashedAsset,
			cacheControl: "public, max-age=31536000, immutable",
		},
		{
			name:         "unversioned static asset",
			requestPath:  "/favicon.svg",
			assetName:    "/favicon.svg",
			cacheControl: "public, max-age=3600",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.requestPath, nil)
			rec := httptest.NewRecorder()

			if served := (&Server{}).serveUIAsset(rec, req, tt.assetName); !served {
				t.Fatalf("serveUIAsset(%q) = false, want true", tt.assetName)
			}
			if got := rec.Header().Get("Cache-Control"); got != tt.cacheControl {
				t.Fatalf("Cache-Control = %q, want %q", got, tt.cacheControl)
			}
		})
	}
}

func TestServeUIAssetCompressesLargeTextAssetsWhenGzipIsAccepted(t *testing.T) {
	assetName := firstEmbeddedUIAssetWithSuffix(t, ".js")
	want, err := uiAssets.ReadFile("ui" + assetName)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, assetName, nil)
	req.Header.Set("Accept-Encoding", "br, gzip")
	rec := httptest.NewRecorder()

	if served := (&Server{}).serveUIAsset(rec, req, assetName); !served {
		t.Fatalf("serveUIAsset(%q) = false, want true", assetName)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := rec.Header().Values("Vary"); !containsHeaderToken(got, "Accept-Encoding") {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}
	reader, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("decompressed response does not match embedded asset")
	}
}

func TestServeUIAssetDoesNotCompressWhenGzipIsRejected(t *testing.T) {
	assetName := firstEmbeddedUIAssetWithSuffix(t, ".js")
	want, err := uiAssets.ReadFile("ui" + assetName)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, assetName, nil)
	req.Header.Set("Accept-Encoding", "br, gzip;q=0")
	rec := httptest.NewRecorder()

	if served := (&Server{}).serveUIAsset(rec, req, assetName); !served {
		t.Fatalf("serveUIAsset(%q) = false, want true", assetName)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if got := rec.Header().Values("Vary"); !containsHeaderToken(got, "Accept-Encoding") {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), want) {
		t.Fatal("identity response does not match embedded asset")
	}
}

func TestGzipUIAssetReusesOneResultAcrossConcurrentRequests(t *testing.T) {
	data := bytes.Repeat([]byte("immutable-ui-asset"), 1024)
	const requestCount = 16
	start := make(chan struct{})
	results := make(chan []byte, requestCount)
	var wg sync.WaitGroup
	for range requestCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- gzipUIAsset("/assets/cache-test.js", data)
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var first []byte
	for result := range results {
		if len(result) == 0 {
			t.Fatal("gzipUIAsset returned an empty result")
		}
		if first == nil {
			first = result
			continue
		}
		if &result[0] != &first[0] {
			t.Fatal("gzipUIAsset did not reuse the cached compressed bytes")
		}
	}
}

func firstEmbeddedUIAssetWithSuffix(t *testing.T, suffix string) string {
	t.Helper()
	entries, err := uiAssets.ReadDir("ui/assets")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			return "/assets/" + entry.Name()
		}
	}
	t.Fatalf("no embedded UI asset with suffix %q", suffix)
	return ""
}

func containsHeaderToken(values []string, token string) bool {
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}
