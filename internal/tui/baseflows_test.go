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

// TestBaseFlowsAreEmbeddedOnly guards the key invariant: templates come from the
// embedded FS, NOT from the on-disk ~/.jet/workflows dir that the Claude-task
// launcher reads via DiscoverWorkflows. Embedded entries carry the sentinel
// Path "(base template)"; on-disk workflows carry a real filesystem path. This
// is hermetic — it does not depend on the contents of the user's home dir.
//
// (A user may legitimately save a workflow named e.g. "pipeline-canvas-review"
// to disk to run it; that on-disk copy is separate from the embedded template
// and is expected to appear in the launcher.)
func TestBaseFlowsAreEmbeddedOnly(t *testing.T) {
	for _, f := range BaseFlows() {
		if f.Path != "(base template)" {
			t.Errorf("template %q has Path %q; embedded templates must not be sourced from disk", f.Name, f.Path)
		}
	}
}
