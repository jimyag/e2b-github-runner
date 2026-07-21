package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qiniu/ci-runner/internal/config"
	"github.com/qiniu/ci-runner/internal/state"
	"gopkg.in/yaml.v3"
)

func TestWriteObfuscatedConfigValueReadsSecretFromStdin(t *testing.T) {
	const plaintext = "secret-from-stdin"
	var output bytes.Buffer
	if err := writeObfuscatedConfigValue(strings.NewReader(plaintext+"\n"), &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), plaintext) {
		t.Fatalf("output exposed plaintext: %q", output.String())
	}
	var decoded struct {
		Value config.Secret `yaml:"value"`
	}
	if err := yaml.Unmarshal([]byte("value: "+strings.TrimSpace(output.String())+"\n"), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Value.Value() != plaintext {
		t.Fatalf("decoded value = %q", decoded.Value.Value())
	}
}

func TestWriteObfuscatedConfigValueTrimsTrailingLineEndings(t *testing.T) {
	const plaintext = "secret-from-stdin"
	var output bytes.Buffer
	if err := writeObfuscatedConfigValue(strings.NewReader(plaintext+"\r\n\r\n"), &output); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Value config.Secret `yaml:"value"`
	}
	if err := yaml.Unmarshal([]byte("value: "+strings.TrimSpace(output.String())+"\n"), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Value.Value() != plaintext {
		t.Fatalf("decoded value = %q", decoded.Value.Value())
	}
}

func TestRunObfuscateConfigValueWritesErrorsToStderr(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if runObfuscateConfigValue(strings.NewReader(""), &stdout, &stderr) {
		t.Fatal("expected obfuscation to fail")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "secret input is empty") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestBootstrapAdminAccount(t *testing.T) {
	store := state.New(t.TempDir())
	if err := bootstrapAdminAccount(store, "github:12345"); err != nil {
		t.Fatal(err)
	}
	account, _, err := store.GetAccountByOAuthIdentity("github", "12345")
	if err != nil {
		t.Fatal(err)
	}
	if account.Role != "admin" {
		t.Fatalf("expected admin role, got %#v", account)
	}
}

func TestBootstrapAdminAccountDefaultsToGitHub(t *testing.T) {
	store := state.New(t.TempDir())
	if err := bootstrapAdminAccount(store, "12345"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.GetAccountByOAuthIdentity("github", "12345"); err != nil {
		t.Fatal(err)
	}
}
