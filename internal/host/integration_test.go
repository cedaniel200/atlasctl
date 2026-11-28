package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cedaniel200/atlasctl/internal/apply"
	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/fsutil"
	"github.com/cedaniel200/atlasctl/internal/plan"
	"github.com/cedaniel200/atlasctl/internal/state"
)

var integrationTime = time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)

func writeProjectFile(t *testing.T, root, rel, data string) {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(absolute, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func applyStrategy(t *testing.T, root string, strategy Strategy, snapshot *content.Snapshot) *apply.Report {
	t.Helper()
	artifacts, err := strategy.Artifacts(Input{Snapshot: snapshot, Bundles: []string{"dev"}})
	if err != nil {
		t.Fatalf("Artifacts() error = %v", err)
	}
	planned, err := plan.Build(plan.Request{
		Snapshot:  snapshot,
		Manifest:  &state.Manifest{SchemaVersion: state.SchemaVersion},
		Reader:    fsutil.RootReader{Root: root},
		Generated: artifacts,
	})
	if err != nil {
		t.Fatalf("plan.Build() error = %v", err)
	}
	report, err := apply.Apply(apply.Request{
		Root:          root,
		Plan:          planned,
		Allowed:       strategy.AllowedRoutes(),
		RemoveDirs:    strategy.LegacyDirs(),
		EngineVersion: "0.6.0",
		GeneratedAt:   integrationTime,
	})
	if err != nil {
		t.Fatalf("apply.Apply() error = %v", err)
	}
	return report
}

func TestOpenCodeApplyReplacesLegacyAgentDirectory(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, ".opencode/agent/reviewer.md", "legacy\n")

	report := applyStrategy(t, root, opencode{}, fixtureSnapshot(t))

	if !contains(report.Pruned, ".opencode/agent") {
		t.Errorf("Pruned = %v, want .opencode/agent", report.Pruned)
	}
	if _, err := os.Stat(filepath.Join(root, ".opencode", "agent")); !os.IsNotExist(err) {
		t.Errorf("legacy .opencode/agent still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".opencode", "agents", "analyst.md")); err != nil {
		t.Errorf("new agent adapter was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".opencode", "commands", "opsx-apply.md")); err != nil {
		t.Errorf("command adapter was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "roles")); !os.IsNotExist(err) {
		t.Errorf("bundle roles were published at the project root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "commands")); !os.IsNotExist(err) {
		t.Errorf("bundle commands were published at the project root: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".opencode", "agents", "analyst.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "mode: all") || !strings.Contains(string(data), "# analyst") {
		t.Errorf("analyst adapter = %q, want the OpenCode frontmatter and body", data)
	}

	manifest, err := state.LoadManifest(filepath.Join(root, ".atlas", "manifest.json"))
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	entry := manifest.Files[".opencode/agents/analyst.md"]
	if entry.Origin != "host:opencode" || entry.Bundle != "dev" || entry.Layer != state.LayerGenerated {
		t.Errorf("analyst manifest entry = %+v, want a generated dev adapter", entry)
	}
}

func TestClaudeApplyWritesStartupFile(t *testing.T) {
	root := t.TempDir()
	report := applyStrategy(t, root, claude{}, fixtureSnapshot(t))

	if !contains(report.Updated, "CLAUDE.md") {
		t.Errorf("Updated = %v, want CLAUDE.md", report.Updated)
	}
	data, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if string(data) != "@AGENTS.md\n" {
		t.Errorf("CLAUDE.md = %q, want the AGENTS.md import", data)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "agents", "analyst.md")); err != nil {
		t.Errorf("Claude agent adapter was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "commands", "opsx-apply.md")); err != nil {
		t.Errorf("Claude command adapter was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".mcp.json")); !os.IsNotExist(err) {
		t.Errorf("the MCP config was created by the engine: %v", err)
	}

	manifest, err := state.LoadManifest(filepath.Join(root, ".atlas", "manifest.json"))
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	entry := manifest.Files["CLAUDE.md"]
	if entry.Origin != "host:claude" || entry.Bundle != "dev" || entry.Layer != state.LayerGenerated {
		t.Errorf("CLAUDE.md manifest entry = %+v, want a generated dev adapter", entry)
	}
}
