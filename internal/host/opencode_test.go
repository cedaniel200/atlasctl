package host

import (
	"reflect"
	"testing"
)

func TestOpenCodeArtifacts(t *testing.T) {
	strategy := opencode{}
	artifacts, err := strategy.Artifacts(Input{Snapshot: fixtureSnapshot(t), Bundles: []string{"dev"}})
	if err != nil {
		t.Fatalf("Artifacts() error = %v", err)
	}
	if len(artifacts) != 4 {
		t.Fatalf("got %d artifacts, want 4", len(artifacts))
	}

	byPath := artifactsByPath(t, artifacts)
	want := map[string]string{
		".opencode/agents/analyst.md":      "---\ndescription: Guide analysis.\nmode: all\n---\n\n" + analystRole,
		".opencode/agents/engineer.md":     "---\ndescription: Build the change.\nmode: all\n---\n\n" + engineerRole,
		".opencode/agents/reviewer.md":     "---\ndescription: Rol reviewer del arnés\nmode: all\n---\n\n" + reviewerRole,
		".opencode/commands/opsx-apply.md": applyCommand,
	}
	for path, content := range want {
		artifact, ok := byPath[path]
		if !ok {
			t.Errorf("%s was not generated", path)
			continue
		}
		if string(artifact.Data) != content {
			t.Errorf("%s data = %q, want %q", path, artifact.Data, content)
		}
		if artifact.Origin != "host:opencode" {
			t.Errorf("%s origin = %q, want host:opencode", path, artifact.Origin)
		}
		if artifact.Bundle != "dev" {
			t.Errorf("%s bundle = %q, want dev", path, artifact.Bundle)
		}
	}

	if _, ok := byPath[".opencode/agents/README.md"]; ok {
		t.Error("README.md was rendered as a role")
	}
	if _, ok := byPath[".opencode/agents/ignored.md"]; ok {
		t.Error("a nested role was rendered")
	}
	if _, ok := byPath[".opencode/commands/README.md"]; ok {
		t.Error("README.md was rendered as a command")
	}
	if got := strategy.LegacyDirs(); !reflect.DeepEqual(got, []string{".opencode/agent"}) {
		t.Errorf("LegacyDirs() = %v, want [.opencode/agent]", got)
	}
}
