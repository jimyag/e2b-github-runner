package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/qiniu/go-sdk/v7/auth"
)

const cacheSTSHeadroom = 5 * time.Minute

// CacheSTSCredentials represents temporary S3 credentials generated for a sandbox.
type CacheSTSCredentials struct {
	AccessKeyID     string    `json:"access_key_id"`
	SecretAccessKey string    `json:"secret_access_key"`
	SessionToken    string    `json:"session_token"`
	Expiration      time.Time `json:"expiration"`
}

// qiniuSTSRequest mirrors the Qiniu IAM STS request body. The Policy field is
// a JSON-encoded string (double encoding).
type qiniuSTSRequest struct {
	Name            string `json:"name"`
	DurationSeconds int    `json:"duration_seconds"`
	Policy          string `json:"policy"`
}

type qiniuSTSResponse struct {
	Code    int           `json:"code,omitempty"`
	Error   string        `json:"error,omitempty"`
	Message string        `json:"message,omitempty"`
	Data    *qiniuSTSData `json:"data"`
}

type qiniuSTSData struct {
	Credential qiniuSTSCredential `json:"credential"`
}

type qiniuSTSCredential struct {
	AccessKey    string `json:"access_key"`
	SecretKey    string `json:"secret_key"`
	SessionToken string `json:"session_token"`
	Expiration   string `json:"expiration"`
}

type cacheSTSCredentialConfig struct {
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
}

// cacheSTSClient performs the Qiniu-signed HTTP call so the endpoint can be
// swapped in tests.
type cacheSTSClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func scopeForBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	branch = strings.TrimPrefix(branch, "refs/heads/")
	if branch == "" {
		branch = "default"
	}
	h := sha256.Sum256([]byte(branch))
	return fmt.Sprintf("scopes/branch-%x", h[:16])
}

func scopeForPR(prNumber int64) string {
	if prNumber <= 0 {
		return "scopes/pr-unknown"
	}
	return fmt.Sprintf("scopes/pr-%d", prNumber)
}

// generateCacheSTS generates a temporary S3-compatible credential via the
// Qiniu IAM federation token API. The session policy uses resource-level
// scope isolation: bucket-level actions (list) are scoped to the bucket,
// object read actions (stat/get) cover all specified readScopes, and object
// write actions (upload/delete/etc.) are strictly restricted to ownScope.
// If ownScope is empty, write permissions are omitted (read-only mode).
func generateCacheSTS(ctx context.Context, config cacheSTSCredentialConfig, cachePrefix, ownScope string, readScopes []string, endpoint string, durationSeconds int, client cacheSTSClient) (CacheSTSCredentials, error) {
	if client == nil {
		return CacheSTSCredentials{}, fmt.Errorf("cache STS HTTP client is required")
	}
	return generateCacheSTSWithClient(ctx, config, cachePrefix, ownScope, readScopes, endpoint, durationSeconds, client)
}

func generateCacheSTSWithClient(ctx context.Context, config cacheSTSCredentialConfig, cachePrefix, ownScope string, readScopes []string, endpoint string, durationSeconds int, client cacheSTSClient) (CacheSTSCredentials, error) {
	bucket := strings.TrimSpace(config.Bucket)
	if bucket == "" {
		return CacheSTSCredentials{}, fmt.Errorf("cache S3 bucket is required to generate STS")
	}
	cachePrefix = strings.TrimPrefix(strings.TrimSpace(cachePrefix), "/")
	cachePrefix = strings.TrimSuffix(cachePrefix, "/")
	if err := validateCachePrefix(cachePrefix); err != nil {
		return CacheSTSCredentials{}, err
	}

	bucketResource := fmt.Sprintf("qrn:kodo:::bucket/%s", bucket)
	bucketActions := []string{"kodo/list", "kodo/listMultipartUploads"}

	statements := []map[string]any{
		{
			"effect":   "Allow",
			"action":   bucketActions,
			"resource": []string{bucketResource},
		},
	}

	readActions := []string{"kodo/stat", "kodo/get"}
	var readResources []string
	seenRead := make(map[string]bool)
	for _, scope := range readScopes {
		scope = strings.Trim(strings.TrimSpace(scope), "/")
		if scope == "" {
			continue
		}
		if err := validateCachePrefix(scope); err != nil {
			return CacheSTSCredentials{}, fmt.Errorf("invalid read scope %q: %w", scope, err)
		}
		res := fmt.Sprintf("qrn:kodo:::bucket/%s/%s/%s/*", bucket, cachePrefix, scope)
		if !seenRead[res] {
			seenRead[res] = true
			readResources = append(readResources, res)
		}
	}
	if len(readResources) == 0 {
		return CacheSTSCredentials{}, fmt.Errorf("at least one cache read scope is required")
	}
	statements = append(statements, map[string]any{
		"effect":   "Allow",
		"action":   readActions,
		"resource": readResources,
	})

	ownScope = strings.Trim(strings.TrimSpace(ownScope), "/")
	if ownScope != "" {
		if err := validateCachePrefix(ownScope); err != nil {
			return CacheSTSCredentials{}, fmt.Errorf("invalid own scope %q: %w", ownScope, err)
		}
		writeActions := []string{
			"kodo/upload",
			"kodo/mkfile",
			"kodo/listParts",
			"kodo/abortMultipartUpload",
			"kodo/delete",
		}
		writeResource := fmt.Sprintf("qrn:kodo:::bucket/%s/%s/%s/*", bucket, cachePrefix, ownScope)
		statements = append(statements, map[string]any{
			"effect":   "Allow",
			"action":   writeActions,
			"resource": []string{writeResource},
		})
	}

	policy := map[string]any{
		"statement": statements,
	}
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return CacheSTSCredentials{}, fmt.Errorf("marshal STS policy: %w", err)
	}
	if durationSeconds <= 0 {
		return CacheSTSCredentials{}, fmt.Errorf("STS duration must be positive")
	}
	body, err := json.Marshal(qiniuSTSRequest{
		Name:            "runnerd-cache",
		DurationSeconds: durationSeconds,
		Policy:          string(policyJSON),
	})
	if err != nil {
		return CacheSTSCredentials{}, fmt.Errorf("marshal STS request: %w", err)
	}

	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return CacheSTSCredentials{}, fmt.Errorf("cache STS endpoint is required")
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	stsURL := endpoint + "/sts/federation/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stsURL, bytes.NewReader(body))
	if err != nil {
		return CacheSTSCredentials{}, fmt.Errorf("build STS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Host = req.URL.Host
	cred := auth.New(strings.TrimSpace(config.AccessKeyID), strings.TrimSpace(config.SecretAccessKey))
	authToken, err := cred.SignRequestV2(req)
	if err != nil {
		return CacheSTSCredentials{}, fmt.Errorf("sign STS request: %w", err)
	}
	req.Header.Set("Authorization", "Qiniu "+authToken)

	resp, err := client.Do(req)
	if err != nil {
		return CacheSTSCredentials{}, fmt.Errorf("call STS API: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return CacheSTSCredentials{}, fmt.Errorf("read STS response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CacheSTSCredentials{}, fmt.Errorf("STS API status %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var stResp qiniuSTSResponse
	if err := json.Unmarshal(data, &stResp); err != nil {
		return CacheSTSCredentials{}, fmt.Errorf("parse STS response: %w", err)
	}
	if stResp.Code != 0 || stResp.Error != "" {
		return CacheSTSCredentials{}, fmt.Errorf("STS API error (code=%d): %s %s", stResp.Code, stResp.Error, stResp.Message)
	}
	if stResp.Data == nil || strings.TrimSpace(stResp.Data.Credential.AccessKey) == "" {
		return CacheSTSCredentials{}, fmt.Errorf("STS returned no credential")
	}
	creds := stResp.Data.Credential
	expiration, err := time.Parse(time.RFC3339, creds.Expiration)
	if err != nil {
		return CacheSTSCredentials{}, fmt.Errorf("parse STS credential expiration %q: %w", creds.Expiration, err)
	}
	return CacheSTSCredentials{
		AccessKeyID:     creds.AccessKey,
		SecretAccessKey: creds.SecretKey,
		SessionToken:    creds.SessionToken,
		Expiration:      expiration,
	}, nil
}

// cacheSTSDurationSeconds covers the complete sandbox lifecycle and adds
// five minutes for runs-on/cache's post-job save step. There is no refresh
// flow from the sandbox yet, so the credential must outlive the timeout.
func cacheSTSDurationSeconds(sandboxTimeout time.Duration) int {
	if sandboxTimeout <= 0 {
		sandboxTimeout = time.Hour
	}
	return int((sandboxTimeout + cacheSTSHeadroom).Seconds())
}

func validateCachePrefix(prefix string) error {
	if prefix == "" {
		return fmt.Errorf("cache prefix is required for STS prefix isolation")
	}
	for _, component := range strings.Split(prefix, "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("cache prefix contains an invalid path component")
		}
		if strings.ContainsAny(component, "*?") {
			return fmt.Errorf("cache prefix contains a resource wildcard")
		}
	}
	return nil
}
