package host

import (
	"strings"
	"testing"
)

func TestCursorArtifacts(t *testing.T) {
	strategy := cursor{}
	artifacts, err := strategy.Artifacts(Input{Snapshot: fixtureSnapshot(t), Bundles: []string{"dev"}})
	if err != nil {
		t.Fatalf("Artifacts() error = %v", err)
	}
	if len(artifacts) != 4 {
		t.Fatalf("got %d artifacts, want 4", len(artifacts))
	}

	byPath := artifactsByPath(t, artifacts)
	want := map[string]string{
		".cursor/agents/analyst.md":      "---\nname: analyst\ndescription: Guide analysis.\nmodel: inherit\n---\n\n" + analystRole,
		".cursor/agents/engineer.md":     "---\nname: engineer\ndescription: Build the change.\nmodel: inherit\n---\n\n" + engineerRole,
		".cursor/agents/reviewer.md":     "---\nname: reviewer\ndescription: Rol reviewer del arnés\nmodel: inherit\n---\n\n" + reviewerRole,
		".cursor/commands/opsx-apply.md": applyCommand,
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
		if artifact.Origin != "host:cursor" {
			t.Errorf("%s origin = %q, want host:cursor", path, artifact.Origin)
		}
		if artifact.Bundle != "dev" {
			t.Errorf("%s bundle = %q, want dev", path, artifact.Bundle)
		}
	}

	for path := range byPath {
		if strings.HasPrefix(path, ".cursor/rules/") {
			t.Errorf("%s: Cursor must not receive native rules", path)
		}
	}
	for _, route := range strategy.AllowedRoutes() {
		if strings.HasPrefix(route, ".cursor/rules") {
			t.Errorf("AllowedRoutes() contains %q; Cursor must not use rules", route)
		}
	}
	if got := strategy.LegacyDirs(); len(got) != 0 {
		t.Errorf("LegacyDirs() = %v, want none", got)
	}
}
