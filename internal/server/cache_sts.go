package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/qiniu/go-sdk/v7/auth"
)

// defaultCacheSTSAPI is the fallback Qiniu IAM federation token endpoint.
const (
	defaultCacheSTSAPI = "https://sts-ov.qiniuapi.com"
	cacheSTSHeadroom   = 5 * time.Minute
)

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

// cacheSTSClient performs the Qiniu-signed HTTP call so the endpoint can be
// swapped in tests.
type cacheSTSClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// generateCacheSTS generates a temporary S3-compatible credential via the
// Qiniu IAM federation token API. The session policy uses resource-level
// prefix isolation: bucket-level actions (list) are scoped to the bucket,
// while object-level actions (upload/get/delete) are restricted to
// qrn:kodo:::bucket/<bucket>/<cachePrefix>/*. Resource wildcards in the
// configured prefix are rejected so they cannot broaden this pattern. The
// credential duration is supplied by the caller as the sandbox lifecycle plus
// post-job headroom.
func generateCacheSTS(ctx context.Context, config cacheS3Config, cachePrefix, endpoint string, durationSeconds int) (CacheSTSCredentials, error) {
	return generateCacheSTSWithClient(ctx, config, cachePrefix, endpoint, durationSeconds, &http.Client{Timeout: 30 * time.Second})
}

func generateCacheSTSWithClient(ctx context.Context, config cacheS3Config, cachePrefix, endpoint string, durationSeconds int, client cacheSTSClient) (CacheSTSCredentials, error) {
	bucket := strings.TrimSpace(config.Bucket)
	if bucket == "" {
		return CacheSTSCredentials{}, fmt.Errorf("cache S3 bucket is required to generate STS")
	}
	cachePrefix = strings.TrimPrefix(strings.TrimSpace(cachePrefix), "/")
	cachePrefix = strings.TrimSuffix(cachePrefix, "/")
	if err := validateCachePrefix(cachePrefix); err != nil {
		return CacheSTSCredentials{}, err
	}

	// The GetFederationToken endpoint accepts only Qiniu-native kodo actions
	// with qrn resources. Condition keys (kodo:prefix / s3:prefix) are not
	// supported, but resource-level prefix isolation works: object-level
	// actions restricted to qrn:kodo:::bucket/<bucket>/<prefix>/* are enforced
	// by the storage backend (verified 403 on writes outside the prefix).
	bucketResource := fmt.Sprintf("qrn:kodo:::bucket/%s", bucket)
	prefixObjectResource := fmt.Sprintf("qrn:kodo:::bucket/%s/%s/*", bucket, cachePrefix)

	// Bucket-level actions need bucket-level resource; list is needed by
	// runs-on/cache to discover existing cache entries.
	bucketActions := []string{"kodo/list", "kodo/listMultipartUploads"}
	// Object-level actions restricted to the per-repository prefix.
	objectActions := []string{
		"kodo/upload",
		"kodo/mkfile",
		"kodo/listParts",
		"kodo/abortMultipartUpload",
		"kodo/stat",
		"kodo/get",
		"kodo/delete",
	}

	policy := map[string]any{
		"statement": []map[string]any{
			{
				"effect":   "Allow",
				"action":   bucketActions,
				"resource": []string{bucketResource},
			},
			{
				"effect":   "Allow",
				"action":   objectActions,
				"resource": []string{prefixObjectResource},
			},
		},
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

	slog.Info("cache STS request", "bucket", bucket, "prefix", cachePrefix, "policy", string(policyJSON))

	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		endpoint = defaultCacheSTSAPI
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
		expiration = time.Now().UTC().Add(time.Hour)
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
