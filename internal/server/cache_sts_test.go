package server

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type cacheSTSRecordingClient struct {
	request *http.Request
}

func (c *cacheSTSRecordingClient) Do(request *http.Request) (*http.Response, error) {
	c.request = request
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(`{"data":{"credential":{"access_key":"STS-test","secret_key":"secret","session_token":"token","expiration":"2030-01-01T00:00:00Z"}}}`)),
	}, nil
}

func TestCacheSTSDurationSecondsAddsHeadroom(t *testing.T) {
	if got, want := cacheSTSDurationSeconds(time.Hour), 3900; got != want {
		t.Fatalf("cacheSTSDurationSeconds(1h) = %d, want %d", got, want)
	}
	if got, want := cacheSTSDurationSeconds(30*time.Minute), 2100; got != want {
		t.Fatalf("cacheSTSDurationSeconds(30m) = %d, want %d", got, want)
	}
}

func TestGenerateCacheSTSUsesRequestedDuration(t *testing.T) {
	client := new(cacheSTSRecordingClient)
	_, err := generateCacheSTSWithClient(t.Context(), cacheS3Config{
		Bucket:          "cache-bucket",
		AccessKeyID:     "access-key",
		SecretAccessKey: "secret-key",
	}, "gh-actions-cache/example/repo", "https://sts.example.test", 3900, client)
	if err != nil {
		t.Fatal(err)
	}
	if client.request == nil {
		t.Fatal("STS request was not captured")
	}
	body, err := io.ReadAll(client.request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"duration_seconds":3900`) {
		t.Fatalf("STS request does not contain requested duration: %s", body)
	}
}

func TestValidateCachePrefixRejectsInvalidComponents(t *testing.T) {
	for _, prefix := range []string{"", "cache/../repo", "cache//repo", "cache/./repo"} {
		t.Run(prefix, func(t *testing.T) {
			if err := validateCachePrefix(prefix); err == nil {
				t.Fatalf("validateCachePrefix(%q) should reject invalid prefix", prefix)
			}
		})
	}
}

func TestGenerateCacheSTSRejectsResourceWildcards(t *testing.T) {
	for _, prefix := range []string{"gh-actions/*/repo", "gh-actions/?/repo"} {
		t.Run(prefix, func(t *testing.T) {
			_, err := generateCacheSTSWithClient(t.Context(), cacheS3Config{Bucket: "cache-bucket"}, prefix, "", 3900, &cacheSTSRecordingClient{})
			if err == nil || !strings.Contains(err.Error(), "resource wildcard") {
				t.Fatalf("expected resource wildcard error, got %v", err)
			}
		})
	}
}
