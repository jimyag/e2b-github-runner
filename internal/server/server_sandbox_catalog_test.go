package server

import (
	"testing"

	"github.com/qiniu/ci-runner/internal/sandboxrunner"
)

func TestFilterCatalogSandboxesDoesNotReuseInput(t *testing.T) {
	items := []sandboxrunner.CatalogSandbox{
		{SandboxID: "sandbox-1", TemplateID: "template-1"},
		{SandboxID: "sandbox-2", TemplateID: "template-2"},
	}

	filtered := filterCatalogSandboxes(items, "template-2")

	if len(filtered) != 1 || filtered[0].SandboxID != "sandbox-2" {
		t.Fatalf("unexpected filtered sandboxes: %#v", filtered)
	}
	if items[0].SandboxID != "sandbox-1" {
		t.Fatalf("filter mutated input slice: %#v", items)
	}
}
