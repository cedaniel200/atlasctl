package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMapsV1StateWithoutClaimingV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	data := `{
	  "bootstrap_version": 1,
	  "template_version": "0.4.0",
	  "status": "configured",
	  "host": "opencode",
	  "host_source": "files",
	  "language": {"user": "es", "agent": "en"},
	  "architecture": {"stance": "custom", "notes": "meta"},
	  "profile": "node-scripts",
	  "stacks": [{"id": "node-scripts"}, {"id": "docs"}],
	  "packs": ["core"],
	  "optional_agents": ["mentor"],
	  "preferences": {"openspec_commands": "ask"},
	  "docs": {"root": "documentation"}
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.SchemaVersion != 1 {
		t.Errorf("schema = %d, want the legacy marker 1", loaded.SchemaVersion)
	}
	if loaded.Host.ID != "opencode" || loaded.Host.Source != "evidence" {
		t.Errorf("host = %+v, want opencode with evidence source", loaded.Host)
	}
	if loaded.Language.User != "es" || loaded.Language.CLI != "es" || loaded.Language.Agent != "en" {
		t.Errorf("language = %+v, want es/es/en", loaded.Language)
	}
	if loaded.Layout.Docs != "documentation" || loaded.Layout.Decisions != nil {
		t.Errorf("layout = %+v, want docs and no decisions", loaded.Layout)
	}
	if len(loaded.Stacks) != 2 || loaded.Stacks[0] != "node-scripts" || loaded.Stacks[1] != "docs" {
		t.Errorf("stacks = %v, want the ids", loaded.Stacks)
	}
	if loaded.Content.BundleVersion != "0.4.0" {
		t.Errorf("content bundle version = %q, want the v1 template version", loaded.Content.BundleVersion)
	}
	if !loaded.Setup.Complete || loaded.Profile != "node-scripts" {
		t.Errorf("setup/profile = %+v/%q", loaded.Setup, loaded.Profile)
	}
}

func TestLoadManifestMapsV1Ownership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	data := `{
	  "template_version": "0.4.0",
	  "generated_at": "2026-11-22T01:45:29Z",
	  "files": {
	    "AGENTS.md": {"origin": "core", "sha256": "aa", "ownership": "managed"},
	    "docs/project-context.md": {"origin": "core", "sha256": "bb", "ownership": "owned"},
	    ".opencode/agents/analyst.md": {"origin": "host:opencode", "sha256": "cc", "ownership": "generated"}
	  }
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	loaded, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if loaded.SchemaVersion != 1 || loaded.EngineVersion != "0.4.0" {
		t.Errorf("manifest versions = %d/%q, want legacy 1/0.4.0", loaded.SchemaVersion, loaded.EngineVersion)
	}
	if len(loaded.Bundles) != 1 || loaded.Bundles[0] != "dev" {
		t.Errorf("bundles = %v, want dev", loaded.Bundles)
	}
	want := map[string]Layer{
		"AGENTS.md":                   LayerBase,
		"docs/project-context.md":     LayerOwned,
		".opencode/agents/analyst.md": LayerGenerated,
	}
	for path, layer := range want {
		entry, ok := loaded.Files[path]
		if !ok {
			t.Errorf("missing entry %q", path)
			continue
		}
		if entry.Layer != layer || entry.Bundle != "dev" {
			t.Errorf("%s = %+v, want layer %s in dev", path, entry, layer)
		}
	}
	if loaded.Files["AGENTS.md"].Origin != "core" || loaded.Files["AGENTS.md"].SHA256 != "aa" {
		t.Errorf("AGENTS.md = %+v, want the preserved origin and hash", loaded.Files["AGENTS.md"])
	}
}

func TestLoadManifestRejectsUnknownOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	data := `{"files": {"AGENTS.md": {"origin": "core", "sha256": "aa", "ownership": "custom"}}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadManifest(path)
	if err == nil || !strings.Contains(err.Error(), "unknown ownership") {
		t.Errorf("LoadManifest() error = %v, want unknown ownership", err)
	}
}

func TestLoadV1WithStacksAsStrings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	data := `{"host": "opencode", "status": "configured", "stacks": ["node-scripts"]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded.Stacks) != 1 || loaded.Stacks[0] != "node-scripts" {
		t.Errorf("stacks = %v, want the string form", loaded.Stacks)
	}
}

func TestLoadV1RejectsInvalidStacks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	data := `{"host": "opencode", "stacks": {"bad": true}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported stacks") {
		t.Errorf("Load() error = %v, want an unsupported stacks error", err)
	}
}
