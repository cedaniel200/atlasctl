package host

import "testing"

func TestClaudeArtifacts(t *testing.T) {
	strategy := claude{}
	artifacts, err := strategy.Artifacts(Input{Snapshot: fixtureSnapshot(t), Bundles: []string{"dev"}})
	if err != nil {
		t.Fatalf("Artifacts() error = %v", err)
	}
	if len(artifacts) != 5 {
		t.Fatalf("got %d artifacts, want 5", len(artifacts))
	}

	byPath := artifactsByPath(t, artifacts)
	want := map[string]string{
		".claude/agents/analyst.md":      "---\nname: analyst\ndescription: Guide analysis.\n---\n\n" + analystRole,
		".claude/agents/engineer.md":     "---\nname: engineer\ndescription: Build the change.\n---\n\n" + engineerRole,
		".claude/agents/reviewer.md":     "---\nname: reviewer\ndescription: Rol reviewer del arnés\n---\n\n" + reviewerRole,
		".claude/commands/opsx-apply.md": applyCommand,
		"CLAUDE.md":                      "@AGENTS.md\n",
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
		if artifact.Origin != "host:claude" {
			t.Errorf("%s origin = %q, want host:claude", path, artifact.Origin)
		}
		if artifact.Bundle != "dev" {
			t.Errorf("%s bundle = %q, want dev", path, artifact.Bundle)
		}
	}

	if got := strategy.LegacyDirs(); len(got) != 0 {
		t.Errorf("LegacyDirs() = %v, want none", got)
	}
}
