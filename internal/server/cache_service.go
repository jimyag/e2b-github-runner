package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/qiniu/ci-runner/internal/state"
)

// cacheS3 holds the S3-compatible client plus the raw long-term credentials
// used to mint short-lived STS tokens for sandbox runners.
type cacheS3 struct {
	client         *s3.Client
	bucket, prefix string
	region         string
	endpoint       string
	accessKeyID    string
	secretKey      string
}

type cacheS3Config struct {
	Region          string
	Bucket          string
	Prefix          string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
}

func newCacheS3(config cacheS3Config) (*cacheS3, error) {
	config.Region = strings.TrimSpace(config.Region)
	config.Bucket = strings.TrimSpace(config.Bucket)
	if config.Region == "" || config.Bucket == "" || strings.TrimSpace(config.AccessKeyID) == "" || strings.TrimSpace(config.SecretAccessKey) == "" {
		return nil, errors.New("cache S3 region, bucket, access key, and secret key are required")
	}
	awsCfg := aws.Config{Region: config.Region, Credentials: credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.SecretAccessKey, "")}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = strings.TrimSpace(config.Endpoint) != ""
		if config.Endpoint != "" {
			o.BaseEndpoint = aws.String(strings.TrimRight(strings.TrimSpace(config.Endpoint), "/"))
		}
	})
	prefix := strings.Trim(strings.TrimSpace(config.Prefix), "/")
	if prefix == "" {
		prefix = "gh-actions-cache"
	}
	return &cacheS3{client: client, bucket: config.Bucket, prefix: prefix, region: config.Region, endpoint: config.Endpoint, accessKeyID: config.AccessKeyID, secretKey: config.SecretAccessKey}, nil
}

// cacheStorageForInstallation resolves the cache S3 configuration for a
// GitHub installation ID. Installation-scoped configuration wins; a personal
// installation without its own configuration falls back to the account scope.
func (s *Server) cacheStorageForInstallation(installationID int64) (*cacheS3, error) {
	if installationID <= 0 {
		return nil, fmt.Errorf("invalid cache installation id")
	}
	installationScope := accountPreferenceScope{Type: state.AccountScopeTypeGitHubInstall, ID: installationID}
	storage, err := s.cacheStorageForScope(installationScope)
	if err == nil {
		return storage, nil
	}
	if !errors.Is(err, state.ErrCacheServiceNotConfigured) {
		return nil, err
	}
	accountID, ok, err := s.store.AccountScopeForPersonalGitHubInstallation(installationID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, state.ErrCacheServiceNotConfigured
	}
	return s.cacheStorageForScope(accountPreferenceScope{Type: state.AccountScopeTypeAccount, ID: accountID})
}

func (s *Server) cacheStorageForScope(scope accountPreferenceScope) (*cacheS3, error) {
	preference, err := s.store.GetAccountPreference(scope.Type, scope.ID, accountPreferenceNamespaceCache, accountPreferenceKeyCacheS3)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return nil, state.ErrCacheServiceNotConfigured
		}
		return nil, err
	}
	var value accountCacheServicePreferenceValue
	if err := json.Unmarshal([]byte(preference.ValueJSON), &value); err != nil {
		return nil, err
	}
	// Region and endpoint are operator-owned mappings. Re-resolve them at use
	// time so old preferences cannot keep using a stale or user-supplied
	// endpoint after the Sandbox region catalog changes.
	_, sandboxSnapshot, sandboxErr := s.sandboxServiceForScopeWithDefault(scope)
	if sandboxErr != nil {
		if errors.Is(sandboxErr, errSandboxServiceNotConfigured) {
			return nil, state.ErrCacheServiceNotConfigured
		}
		return nil, sandboxErr
	}
	mappedRegion, mappedEndpoint, mapped := s.cacheS3SettingsForSandboxAPIURL(sandboxSnapshot.APIURL)
	if !mapped || mappedRegion == "" || mappedEndpoint == "" {
		return nil, state.ErrCacheServiceNotConfigured
	}
	value.Region = mappedRegion
	value.Endpoint = mappedEndpoint
	accessKey, err := s.store.GetAccountSecret(scope.Type, scope.ID, state.AccountSecretTypeCacheAccessKeyID)
	if err != nil {
		return nil, state.ErrCacheServiceNotConfigured
	}
	secretKey, err := s.store.GetAccountSecret(scope.Type, scope.ID, state.AccountSecretTypeCacheSecretAccessKey)
	if err != nil {
		return nil, state.ErrCacheServiceNotConfigured
	}
	accessKeyValue, err := decryptSecret(accessKey.EncryptedValue, s.cfg.AuthEncryptionKey.Value())
	if err != nil {
		return nil, err
	}
	secretKeyValue, err := decryptSecret(secretKey.EncryptedValue, s.cfg.AuthEncryptionKey.Value())
	if err != nil {
		return nil, err
	}
	return newCacheS3(cacheS3Config{
		Region: value.Region, Bucket: value.Bucket, Prefix: value.Prefix, Endpoint: value.Endpoint,
		AccessKeyID: accessKeyValue, SecretAccessKey: secretKeyValue,
	})
}

func (s *Server) cacheS3SettingsForSandboxAPIURL(apiURL string) (string, string, bool) {
	normalized := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	for _, region := range s.cfg.SandboxRegions {
		if !strings.EqualFold(strings.TrimRight(strings.TrimSpace(region.SandboxAPIURL), "/"), normalized) {
			continue
		}
		endpoint := strings.TrimSpace(region.S3Endpoint)
		if endpoint != "" && !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			endpoint = "https://" + endpoint
		}
		return strings.TrimSpace(region.S3Region), endpoint, true
	}
	return "", "", false
}

func validateCacheS3(ctx context.Context, storage *cacheS3) error {
	out, err := storage.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(storage.bucket)})
	if err != nil {
		return fmt.Errorf("validate cache S3 bucket %q: %w", storage.bucket, err)
	}
	if out.BucketRegion == nil || strings.TrimSpace(*out.BucketRegion) == "" {
		return fmt.Errorf("cache S3 bucket %q did not report its region", storage.bucket)
	}
	if strings.TrimSpace(*out.BucketRegion) != storage.region {
		return fmt.Errorf("cache S3 bucket %q is in region %q, want %q", storage.bucket, *out.BucketRegion, storage.region)
	}
	return nil
}

// handleSandboxRegions returns the sandbox region catalog configured in
// runnerd.yaml. This is a public endpoint — no auth required — so the frontend
// can display region labels, API URLs, and the associated S3 cache
// region/endpoint mapping.
func (s *Server) handleSandboxRegions(w http.ResponseWriter, r *http.Request) {
	type regionEntry struct {
		ID            string `json:"id"`
		Label         string `json:"label"`
		SandboxAPIURL string `json:"api_url"`
		S3Region      string `json:"s3_region,omitempty"`
		S3Endpoint    string `json:"s3_endpoint,omitempty"`
	}
	entries := make([]regionEntry, 0, len(s.cfg.SandboxRegions))
	for _, region := range s.cfg.SandboxRegions {
		id := strings.TrimSpace(region.ID)
		if id == "" {
			continue
		}
		endpoint := strings.TrimSpace(region.S3Endpoint)
		if endpoint != "" && !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			endpoint = "https://" + endpoint
		}
		entries = append(entries, regionEntry{
			ID:            id,
			Label:         strings.TrimSpace(region.Label),
			SandboxAPIURL: strings.TrimSpace(region.SandboxAPIURL),
			S3Region:      strings.TrimSpace(region.S3Region),
			S3Endpoint:    endpoint,
		})
	}
	writeJSON(w, http.StatusOK, entries)
}
