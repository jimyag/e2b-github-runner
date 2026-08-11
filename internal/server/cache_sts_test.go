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
	ownScope := scopeForBranch("main")
	_, err := generateCacheSTSWithClient(t.Context(), cacheS3Config{
		Bucket:          "cache-bucket",
		AccessKeyID:     "access-key",
		SecretAccessKey: "secret-key",
	}, "gh-actions-cache/example/repo", ownScope, []string{ownScope}, "https://sts.example.test", 3900, client)
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
			_, err := generateCacheSTSWithClient(t.Context(), cacheS3Config{Bucket: "cache-bucket"}, prefix, "scopes/branch-main", []string{"scopes/branch-main"}, "", 3900, &cacheSTSRecordingClient{})
			if err == nil || !strings.Contains(err.Error(), "resource wildcard") {
				t.Fatalf("expected resource wildcard error, got %v", err)
			}
		})
	}
}

func TestGenerateCacheSTSScopedBranch(t *testing.T) {
	client := new(cacheSTSRecordingClient)
	ownScope := scopeForBranch("feature-a")
	defaultScope := scopeForBranch("main")

	_, err := generateCacheSTSWithClient(t.Context(), cacheS3Config{
		Bucket:          "my-bucket",
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
	}, "gh-cache/org/repo", ownScope, []string{ownScope, defaultScope}, "https://sts.example.test", 1800, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(client.request.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, ownScope) {
		t.Fatalf("expected policy to contain ownScope %s, got: %s", ownScope, bodyStr)
	}
	if !strings.Contains(bodyStr, defaultScope) {
		t.Fatalf("expected policy to contain defaultScope %s, got: %s", defaultScope, bodyStr)
	}
	if !strings.Contains(bodyStr, "kodo/upload") {
		t.Fatalf("expected branch policy to allow write actions, got: %s", bodyStr)
	}
}

func TestGenerateCacheSTSReadOnlyForkPR(t *testing.T) {
	client := new(cacheSTSRecordingClient)
	defaultScope := scopeForBranch("main")

	// ownScope is empty for Fork PR
	_, err := generateCacheSTSWithClient(t.Context(), cacheS3Config{
		Bucket:          "my-bucket",
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
	}, "gh-cache/org/repo", "", []string{defaultScope}, "https://sts.example.test", 1800, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(client.request.Body)
	bodyStr := string(body)
	if strings.Contains(bodyStr, "kodo/upload") || strings.Contains(bodyStr, "kodo/delete") {
		t.Fatalf("fork PR policy must NOT contain write actions, got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "kodo/get") {
		t.Fatalf("fork PR policy should allow read action kodo/get, got: %s", bodyStr)
	}
}

func TestGenerateCacheSTSRequiresReadScope(t *testing.T) {
	_, err := generateCacheSTSWithClient(t.Context(), cacheS3Config{Bucket: "my-bucket"}, "gh-cache/org/repo", "", nil, "https://sts.example.test", 1800, &cacheSTSRecordingClient{})
	if err == nil || !strings.Contains(err.Error(), "read scope") {
		t.Fatalf("expected missing read scope error, got %v", err)
	}
}

func TestIsForkPullRequestPayload(t *testing.T) {
	forkPayload := `{
		"workflow_run": {
			"head_repository": { "full_name": "fork-owner/repo" }
		},
		"repository": { "full_name": "upstream-owner/repo" }
	}`
	if !isForkPullRequestPayload(forkPayload) {
		t.Fatalf("expected forkPayload to be recognized as fork PR")
	}

	internalPayload := `{
		"workflow_run": {
			"head_repository": { "full_name": "upstream-owner/repo" }
		},
		"repository": { "full_name": "upstream-owner/repo" }
	}`
	if isForkPullRequestPayload(internalPayload) {
		t.Fatalf("expected internalPayload to NOT be recognized as fork PR")
	}
}
