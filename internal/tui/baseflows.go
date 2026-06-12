package tui

import (
	"embed"
	"sort"
	"strings"
)

// baseFlowsFS holds the read-only composite pipeline templates that ship with
// the binary. They are NEVER written to ~/.jet/workflows/, so DiscoverWorkflows
// (and thus the Claude-task launcher) never lists them — they are only offered
// as starting templates when creating a new workflow in the editor.
//
//go:embed baseflows/*.md
var baseFlowsFS embed.FS

// BaseFlows returns the embedded workflow templates, sorted by name: the
// composite pipelines (pipeline-*.md), which orchestrate the dragon-canvas
// `/dragon-canvas:*` autonomous commands directly. README.md and anything else
// is excluded.
func BaseFlows() []Workflow {
	entries, err := baseFlowsFS.ReadDir("baseflows")
	if err != nil {
		return nil
	}

	var flows []Workflow
	for _, e := range entries {
		name := e.Name()
		isTemplate := strings.HasPrefix(name, "pipeline-")
		if e.IsDir() || !strings.HasSuffix(name, ".md") || !isTemplate {
			continue
		}
		content, err := baseFlowsFS.ReadFile("baseflows/" + name)
		if err != nil {
			continue
		}
		flows = append(flows, Workflow{
			Name:    strings.TrimSuffix(name, ".md"),
			Path:    "(base template)",
			Content: string(content),
		})
	}

	sort.Slice(flows, func(i, j int) bool {
		return flows[i].Name < flows[j].Name
	})

	return flows
}
