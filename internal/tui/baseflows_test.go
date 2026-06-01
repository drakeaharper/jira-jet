package tui

import (
	"strings"
	"testing"
)

// expectedBaseFlows is the set of templates that must ship embedded:
// seven foundation flows (base-*, one per -auto flow) plus two composite
// pipelines (pipeline-*).
var expectedBaseFlows = []string{
	"base-address-feedback-auto",
	"base-canvas-parallel-env-auto",
	"base-comments-and-votes-auto",
	"base-qa-auto",
	"base-resolve-change-from-ticket",
	"base-review-auto",
	"base-setup-test-auto",
	"base-start-ticket-auto",
	"pipeline-canvas-review",
	"pipeline-canvas-ticket",
}

// templatePrefixes are the filename prefixes BaseFlows() recognizes.
var templatePrefixes = []string{"base-", "pipeline-"}

func hasTemplatePrefix(name string) bool {
	for _, p := range templatePrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func TestBaseFlowsReturnsAllExpected(t *testing.T) {
	flows := BaseFlows()
	if len(flows) != len(expectedBaseFlows) {
		t.Fatalf("expected %d base flows, got %d", len(expectedBaseFlows), len(flows))
	}

	got := make(map[string]Workflow, len(flows))
	for _, f := range flows {
		got[f.Name] = f
	}
	for _, name := range expectedBaseFlows {
		f, ok := got[name]
		if !ok {
			t.Errorf("missing base flow %q", name)
			continue
		}
		if !hasTemplatePrefix(f.Name) {
			t.Errorf("template %q has no recognized prefix (base-/pipeline-)", f.Name)
		}
		if strings.TrimSpace(f.Content) == "" {
			t.Errorf("base flow %q has empty content", f.Name)
		}
		if f.Path != "(base template)" {
			t.Errorf("base flow %q Path = %q, want %q", f.Name, f.Path, "(base template)")
		}
	}
}

func TestBaseFlowsExcludesReadme(t *testing.T) {
	for _, f := range BaseFlows() {
		if f.Name == "README" || f.Name == "readme" {
			t.Errorf("README must not be exposed as a base flow")
		}
	}
}

// TestBaseFlowsNotOnDisk guards the key invariant: base flows are embedded only,
// so the on-disk discovery used by the Claude-task launcher never lists them.
func TestBaseFlowsNotInDiscover(t *testing.T) {
	disk, err := DiscoverWorkflows()
	if err != nil {
		t.Fatalf("DiscoverWorkflows error: %v", err)
	}
	for _, w := range disk {
		if hasTemplatePrefix(w.Name) {
			t.Errorf("template %q leaked into on-disk DiscoverWorkflows (launcher would show it)", w.Name)
		}
	}
}
