package config

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var (
	plaintextFlag = flag.String("plaintext", "", "plaintext string to obfuscate")
	encryptedFlag = flag.String("encrypted", "", "RUNNERD_ENC(v1:...) value to decrypt")
)

func TestObfuscateSecretRoundTripThroughYAML(t *testing.T) {
	const plaintext = "github-webhook-secret"
	encoded, err := ObfuscateSecret([]byte(plaintext))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, "RUNNERD_ENC(v1:") || strings.Contains(encoded, plaintext) {
		t.Fatalf("unexpected obfuscated value: %q", encoded)
	}

	var decoded struct {
		Value Secret `yaml:"value"`
	}
	if err := yaml.Unmarshal([]byte("value: "+encoded+"\n"), &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded.Value.Value(); got != plaintext {
		t.Fatalf("decoded secret = %q, want %q", got, plaintext)
	}
}

// TestPrintObfuscatedSecret prints an obfuscated value for manual use.
//
// Usage:
//
//	go test ./internal/config -run '^TestPrintObfuscatedSecret$' -v -args -plaintext='your-secret'
func TestPrintObfuscatedSecret(t *testing.T) {
	if *plaintextFlag == "" {
		t.Skip("pass -args -plaintext=... to print its obfuscated value")
	}

	encoded, err := ObfuscateSecret([]byte(*plaintextFlag))
	if err != nil {
		t.Fatal(err)
	}
	t.Log(encoded)
}

// TestPrintDeobfuscatedSecret prints the plaintext of an obfuscated value.
//
// Usage:
//
//	go test ./internal/config -run '^TestPrintDeobfuscatedSecret$' -v -args -encrypted='RUNNERD_ENC(v1:...)'
func TestPrintDeobfuscatedSecret(t *testing.T) {
	if *encryptedFlag == "" {
		t.Skip("pass -args -encrypted=... to print its plaintext value")
	}
	if !strings.HasPrefix(*encryptedFlag, obfuscatedSecretPrefix) {
		t.Fatal("-encrypted must be a RUNNERD_ENC(v1:...) value")
	}

	plaintext, err := revealObfuscatedSecret(*encryptedFlag)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(plaintext)
}

func TestSecretYAMLAcceptsPlaintextForCompatibility(t *testing.T) {
	var decoded struct {
		Value Secret `yaml:"value"`
	}
	if err := yaml.Unmarshal([]byte("value: existing-plaintext\n"), &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded.Value.Value(); got != "existing-plaintext" {
		t.Fatalf("decoded secret = %q", got)
	}
}

func TestSecretJSONRevealsObfuscatedSecret(t *testing.T) {
	const plaintext = "github-webhook-secret"
	encoded, err := ObfuscateSecret([]byte(plaintext))
	if err != nil {
		t.Fatal(err)
	}

	var decoded struct {
		Value Secret `json:"value"`
	}
	if err := json.Unmarshal([]byte(fmt.Sprintf(`{"value":%q}`, encoded)), &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded.Value.Value(); got != plaintext {
		t.Fatalf("decoded secret = %q, want %q", got, plaintext)
	}
}

func TestSecretYAMLRejectsTamperedObfuscatedValue(t *testing.T) {
	encoded, err := ObfuscateSecret([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	tamperedBytes := []byte(encoded)
	payloadIndex := len(obfuscatedSecretPrefix)
	if tamperedBytes[payloadIndex] == 'A' {
		tamperedBytes[payloadIndex] = 'B'
	} else {
		tamperedBytes[payloadIndex] = 'A'
	}
	tampered := string(tamperedBytes)

	var decoded struct {
		Value Secret `yaml:"value"`
	}
	if err := yaml.Unmarshal([]byte("value: "+tampered+"\n"), &decoded); err == nil {
		t.Fatal("expected tampered obfuscated secret to be rejected")
	}
}

func TestSecretMasksFormattingJSONAndStructuredLogs(t *testing.T) {
	secret := Secret("do-not-log-me")
	format := func(pattern string) string {
		return fmt.Sprintf(pattern, secret)
	}
	for name, got := range map[string]string{
		"string":        fmt.Sprint(secret),
		"value":         format("%v"),
		"quoted":        format("%q"),
		"go-syntax":     format("%#v"),
		"hex":           format("%x"),
		"integer":       format("%d"),
		"character":     format("%c"),
		"alternate-hex": format("%#x"),
		"width":         format("%10s"),
		"precision":     format("%.2s"),
	} {
		if strings.Contains(got, secret.Value()) || got != `******` && got != `"******"` {
			t.Fatalf("%s formatting exposed secret: %q", name, got)
		}
	}

	encoded, err := json.Marshal(struct {
		Token Secret `json:"token"`
	}{Token: secret})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret.Value()) || string(encoded) != `{"token":"******"}` {
		t.Fatalf("JSON exposed secret: %s", encoded)
	}
	yamlEncoded, err := yaml.Marshal(struct {
		Token Secret `yaml:"token"`
	}{Token: secret})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(yamlEncoded), secret.Value()) || string(yamlEncoded) != "token: '******'\n" {
		t.Fatalf("YAML exposed secret: %s", yamlEncoded)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	logger.Info("configured", "token", secret)
	if strings.Contains(logs.String(), secret.Value()) || !strings.Contains(logs.String(), `"token":"******"`) {
		t.Fatalf("structured log exposed secret: %s", logs.String())
	}
}

func TestLoadFileRevealsObfuscatedSecretsAndMasksConfigFormatting(t *testing.T) {
	values := map[string]string{
		"dsn":                 "runner:database-password@tcp(db.example:3306)/runnerd",
		"session_secret":      "session-secret",
		"encryption_key":      "encryption-key",
		"webhook_secret":      "webhook-secret",
		"token":               "github-token",
		"oauth_client_secret": "oauth-client-secret",
	}
	encoded := make(map[string]string, len(values))
	for name, value := range values {
		var err error
		encoded[name], err = ObfuscateSecret([]byte(value))
		if err != nil {
			t.Fatalf("obfuscate %s: %v", name, err)
		}
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "runnerd.yaml")
	contents := fmt.Sprintf(`
database:
  backend: mysql
  dsn: %s
auth:
  session_secret: %s
  encryption_key: %s
github:
  webhook_secret: %s
  token: %s
  oauth:
    client_id: Iv1.test
    client_secret: %s
`, encoded["dsn"], encoded["session_secret"], encoded["encryption_key"], encoded["webhook_secret"], encoded["token"], encoded["oauth_client_secret"])
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{
		"dsn":                 cfg.StateDatabaseDSN.Value(),
		"session_secret":      cfg.AuthSessionSecret.Value(),
		"encryption_key":      cfg.AuthEncryptionKey.Value(),
		"webhook_secret":      cfg.GitHubWebhookSecret.Value(),
		"token":               cfg.GitHubToken.Value(),
		"oauth_client_secret": cfg.GitHubOAuthClientSecret.Value(),
	}
	for name, want := range values {
		if got[name] != want {
			t.Fatalf("%s = %q, want %q", name, got[name], want)
		}
	}
	formatted := fmt.Sprintf("%+v", cfg)
	for name, value := range values {
		if strings.Contains(formatted, value) {
			t.Fatalf("formatted config exposed %s: %s", name, formatted)
		}
	}
}
