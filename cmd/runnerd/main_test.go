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
