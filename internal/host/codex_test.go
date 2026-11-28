package host

import (
	"reflect"
	"testing"
)

func TestCodexProducesNoArtifacts(t *testing.T) {
	strategy := codex{}

	artifacts, err := strategy.Artifacts(Input{Snapshot: fixtureSnapshot(t), Bundles: []string{"dev"}})
	if err != nil {
		t.Fatalf("Artifacts() error = %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("Artifacts() = %v, want no files: Codex runs roles inline", artifacts)
	}

	caps := strategy.Capabilities()
	if caps.Subagents {
		t.Error("Subagents = true, want false")
	}
	if caps.AgentsDir != "" || caps.CommandsDir != "" {
		t.Errorf("adapter directories = (%q, %q), want none", caps.AgentsDir, caps.CommandsDir)
	}
	if got := strategy.AllowedRoutes(); !reflect.DeepEqual(got, []string{"AGENTS.md"}) {
		t.Errorf("AllowedRoutes() = %v, want [AGENTS.md]", got)
	}
	if got := strategy.ConfigFiles(); len(got) != 0 {
		t.Errorf("ConfigFiles() = %v, want none", got)
	}
	if got := strategy.LegacyDirs(); len(got) != 0 {
		t.Errorf("LegacyDirs() = %v, want none", got)
	}
}
