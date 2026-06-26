package main

import (
	"testing"

	"github.com/qiniu/ci-runner/internal/state"
)

func TestBootstrapAdminUser(t *testing.T) {
	store := state.New(t.TempDir())
	if err := bootstrapAdminUser(store, "github:octocat"); err != nil {
		t.Fatal(err)
	}
	user, err := store.GetUserByOAuthIdentity("github", "octocat")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != "admin" {
		t.Fatalf("expected admin role, got %#v", user)
	}
}

func TestBootstrapAdminUserDefaultsToGitHub(t *testing.T) {
	store := state.New(t.TempDir())
	if err := bootstrapAdminUser(store, "octocat"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetUserByOAuthIdentity("github", "octocat"); err != nil {
		t.Fatal(err)
	}
}
