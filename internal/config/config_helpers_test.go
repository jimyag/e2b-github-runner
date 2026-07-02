package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultStringReturnsFallbackWhenEmpty(t *testing.T) {
	if got := defaultString("", "fallback"); got != "fallback" {
		t.Errorf("defaultString(\"\", \"fallback\") = %q, want %q", got, "fallback")
	}
}

func TestDefaultStringReturnsFallbackWhenWhitespace(t *testing.T) {
	if got := defaultString("   ", "fallback"); got != "fallback" {
		t.Errorf("defaultString(spaces, \"fallback\") = %q, want %q", got, "fallback")
	}
}

func TestDefaultStringReturnsValueWhenNonEmpty(t *testing.T) {
	if got := defaultString("value", "fallback"); got != "value" {
		t.Errorf("defaultString(\"value\", \"fallback\") = %q, want %q", got, "value")
	}
}

func TestDefaultIntReturnsFallbackWhenZero(t *testing.T) {
	if got := defaultInt(0, 10); got != 10 {
		t.Errorf("defaultInt(0, 10) = %d, want 10", got)
	}
}

func TestDefaultIntReturnsFallbackWhenNegative(t *testing.T) {
	if got := defaultInt(-1, 10); got != 10 {
		t.Errorf("defaultInt(-1, 10) = %d, want 10", got)
	}
}

func TestDefaultIntReturnsValueWhenPositive(t *testing.T) {
	if got := defaultInt(5, 10); got != 5 {
		t.Errorf("defaultInt(5, 10) = %d, want 5", got)
	}
}

func TestDurationSecondsReturnsFallbackWhenZero(t *testing.T) {
	if got := durationSeconds(0, 30); got != 30*time.Second {
		t.Errorf("durationSeconds(0, 30) = %v, want 30s", got)
	}
}

func TestDurationSecondsReturnsFallbackWhenNegative(t *testing.T) {
	if got := durationSeconds(-5, 30); got != 30*time.Second {
		t.Errorf("durationSeconds(-5, 30) = %v, want 30s", got)
	}
}

func TestDurationSecondsReturnsValueWhenPositive(t *testing.T) {
	if got := durationSeconds(60, 30); got != 60*time.Second {
		t.Errorf("durationSeconds(60, 30) = %v, want 60s", got)
	}
}

func TestNormalizePatternsDeduplicatesAndFiltersEmpty(t *testing.T) {
	input := []string{"octo/*", "  ", "octo/*", "other/*", ""}
	result := normalizePatterns(input)
	if len(result) != 2 {
		t.Fatalf("normalizePatterns: expected 2 patterns, got %d: %v", len(result), result)
	}
	if result[0] != "octo/*" || result[1] != "other/*" {
		t.Errorf("normalizePatterns: got %v, want [octo/* other/*]", result)
	}
}

func TestNormalizePatternsReturnsNilForNilInput(t *testing.T) {
	if got := normalizePatterns(nil); got != nil {
		t.Errorf("normalizePatterns(nil): got %v, want nil", got)
	}
}

func TestNormalizePatternsPreservesOrder(t *testing.T) {
	input := []string{"z/*", "a/*", "m/*"}
	result := normalizePatterns(input)
	if len(result) != 3 || result[0] != "z/*" || result[1] != "a/*" || result[2] != "m/*" {
		t.Errorf("normalizePatterns: expected order preserved, got %v", result)
	}
}

func TestRepositoryMatchesExactMatch(t *testing.T) {
	if !repositoryMatches("octo/myrepo", "octo/myrepo") {
		t.Error("exact pattern should match identical repository")
	}
}

func TestRepositoryMatchesExactMismatch(t *testing.T) {
	if repositoryMatches("octo/myrepo", "octo/other") {
		t.Error("exact pattern should not match different repository")
	}
}

func TestRepositoryMatchesWildcardPrefix(t *testing.T) {
	if !repositoryMatches("octo/*", "octo/myrepo") {
		t.Error("wildcard pattern should match org/repo")
	}
}

func TestRepositoryMatchesWildcardMismatchOrg(t *testing.T) {
	if repositoryMatches("octo/*", "other/myrepo") {
		t.Error("wildcard pattern should not match different org")
	}
}

func TestRepositoryMatchesEmptyPattern(t *testing.T) {
	if repositoryMatches("", "octo/myrepo") {
		t.Error("empty pattern should not match any repository")
	}
}

func TestRepositoryMatchesEmptyRepository(t *testing.T) {
	if repositoryMatches("octo/myrepo", "") {
		t.Error("any pattern should not match empty repository")
	}
}

func TestRepositoryMatchesTrimsWhitespace(t *testing.T) {
	if !repositoryMatches("  octo/myrepo  ", "octo/myrepo") {
		t.Error("leading/trailing whitespace in pattern should be trimmed")
	}
}

func TestResolveConfigPathReturnsEmptyForEmpty(t *testing.T) {
	if got := resolveConfigPath("/dir", ""); got != "" {
		t.Errorf("resolveConfigPath empty: got %q, want empty", got)
	}
}

func TestResolveConfigPathReturnsAbsoluteUnchanged(t *testing.T) {
	if got := resolveConfigPath("/dir", "/abs/path"); got != "/abs/path" {
		t.Errorf("resolveConfigPath abs: got %q, want /abs/path", got)
	}
}

func TestResolveConfigPathJoinsRelativePath(t *testing.T) {
	want := filepath.Join("/dir", "secrets/app.pem")
	if got := resolveConfigPath("/dir", "secrets/app.pem"); got != want {
		t.Errorf("resolveConfigPath rel: got %q, want %q", got, want)
	}
}

func TestResolveConfigPathTrimsWhitespace(t *testing.T) {
	if got := resolveConfigPath("/dir", "  "); got != "" {
		t.Errorf("resolveConfigPath whitespace: got %q, want empty", got)
	}
}

// ---------- Load ----------

func TestLoadDelegatesToLoadFileWithExplicitPath(t *testing.T) {
	dir := t.TempDir()
	configPath := writeMinimalConfig(t, dir, "explicit.yaml")
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load explicit path: %v", err)
	}
	if cfg.ConfigPath != configPath {
		t.Errorf("Load: ConfigPath = %q, want %q", cfg.ConfigPath, configPath)
	}
}

func TestLoadUsesRunnerdYAMLWhenPathEmpty(t *testing.T) {
	dir := t.TempDir()
	writeMinimalConfig(t, dir, "runnerd.yaml")
	// Change to the temp dir so "runnerd.yaml" resolves
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load empty path: %v", err)
	}
	// Load("") uses "runnerd.yaml" as the path literal
	if cfg.ConfigPath != "runnerd.yaml" {
		t.Errorf("Load empty: ConfigPath = %q, want %q", cfg.ConfigPath, "runnerd.yaml")
	}
}

func TestLoadReturnsErrorForMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/runnerd.yaml")
	if err == nil {
		t.Error("Load missing file: expected error, got nil")
	}
}

// writeMinimalConfig writes a minimal valid runnerd YAML to dir/name and returns the full path.
func writeMinimalConfig(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	content := `
admin:
  token: test-token
auth:
  session_secret: test-session-secret
  encryption_key: test-encryption-key
github:
  webhook_secret: webhook-secret
  token: ghp_test
  oauth:
    client_id: Iv1.test
    client_secret: secret
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
