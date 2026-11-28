package commands

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/hooks"
	"github.com/cedaniel200/atlasctl/internal/state"
)

func confirmMigration(MigratePlan) (bool, error) { return true, nil }

func TestMigrateWorkshopV1(t *testing.T) {
	project := t.TempDir()
	stateBytes := copyFixture(t, project, "state.json")
	manifestBytes := copyFixture(t, project, "manifest.json")

	result, err := Migrate(MigrateRequest{Project: project, Confirm: confirmMigration})
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if result.Already || result.BackupDir == "" {
		t.Fatalf("result = %+v, want a completed migration with backup", result)
	}
	assertBackup(t, result.BackupDir, stateBytes, manifestBytes)

	written := readState(t, project)
	if written.SchemaVersion != state.SchemaVersion || written.EngineVersion != EngineVersion {
		t.Errorf("versions = %d/%q, want %d/%q", written.SchemaVersion, written.EngineVersion, state.SchemaVersion, EngineVersion)
	}
	if written.Host.ID != "opencode" || written.Language.User != "es" || written.Language.CLI != "es" {
		t.Errorf("host/language = %+v/%+v", written.Host, written.Language)
	}
	if written.Layout.Docs != "docs" || written.Layout.Decisions != nil {
		t.Errorf("layout = %+v, want docs and no decisions", written.Layout)
	}
	if len(written.Bundles) != 1 || written.Bundles[0] != "dev" {
		t.Errorf("bundles = %v, want dev", written.Bundles)
	}
	if written.Content.BundleVersion != "0.4.0" {
		t.Errorf("content bundle version = %q, want the v1 version", written.Content.BundleVersion)
	}
	if !written.Setup.Complete || len(written.Setup.Pending) != 0 {
		t.Errorf("setup = %+v, want complete", written.Setup)
	}
	if len(written.Stacks) != 1 || written.Stacks[0] != "node-scripts" {
		t.Errorf("stacks = %v, want the stack ids", written.Stacks)
	}

	manifest, err := state.LoadManifest(filepath.Join(project, ".atlas", "manifest.json"))
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if manifest.SchemaVersion != state.SchemaVersion || manifest.EngineVersion != EngineVersion {
		t.Errorf("manifest versions = %d/%q", manifest.SchemaVersion, manifest.EngineVersion)
	}
	if len(manifest.Bundles) != 1 || manifest.Bundles[0] != "dev" {
		t.Errorf("manifest bundles = %v, want dev", manifest.Bundles)
	}
	if _, ok := manifest.Files[hooks.DispatcherPath]; !ok {
		t.Errorf("manifest has no dispatcher entry")
	}
	if entry := manifest.Files[".atlas/scripts/actualizar-arnes.mjs"]; entry.Layer != state.LayerBase {
		t.Errorf("script layer = %q, want base until the scripts piece", entry.Layer)
	}
	if entry := manifest.Files[".agents/skills/README.md"]; entry.Layer != state.LayerBase || entry.Bundle != "dev" {
		t.Errorf("managed skill = %+v, want base in dev", entry)
	}
	if entry := manifest.Files["AGENTS.md"]; entry.Layer != state.LayerOwned || entry.Origin != "core" {
		t.Errorf("AGENTS.md = %+v, want owned with the preserved origin", entry)
	}
	if entry := manifest.Files[".opencode/agent/analyst.md"]; entry.Layer != state.LayerGenerated || entry.Origin != "host:opencode" {
		t.Errorf("host adapter = %+v, want generated with the preserved origin", entry)
	}

	beforeState := readProjectFile(t, project, ".atlas/state.json")
	beforeManifest := readProjectFile(t, project, ".atlas/manifest.json")
	again, err := Migrate(MigrateRequest{Project: project, Confirm: confirmMigration})
	if err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
	if !again.Already {
		t.Errorf("second result = %+v, want Already", again)
	}
	if got := readProjectFile(t, project, ".atlas/state.json"); got != beforeState {
		t.Error("second migration changed the state")
	}
	if got := readProjectFile(t, project, ".atlas/manifest.json"); got != beforeManifest {
		t.Error("second migration changed the manifest")
	}
	backups, err := filepath.Glob(filepath.Join(project, ".atlas", "backups", "*"))
	if err != nil {
		t.Fatalf("Glob(backups) error = %v", err)
	}
	if len(backups) != 1 {
		t.Errorf("backups = %v, want exactly one", backups)
	}
}

func TestMigrateProducesGreenCheck(t *testing.T) {
	project := t.TempDir()
	writeV1State(t, project, "configured", "docs")
	writeV1Manifest(t, project, "0.4.0", map[string]v1File{
		"AGENTS.md": {Origin: "core", SHA256: content.Checksum([]byte("base\n")), Ownership: "managed"},
	})
	writeProjectFile(t, project, "AGENTS.md", "base\n")

	if _, err := Migrate(MigrateRequest{Project: project, Confirm: confirmMigration}); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	report, err := Check(CheckRequest{Project: project})
	if err != nil {
		t.Fatalf("Check() after migration error = %v", err)
	}
	if report.Recommendations != 0 {
		t.Errorf("check recommendations = %d, want a green check: %+v", report.Recommendations, report)
	}
}

func TestMigrateStopsOnIncompleteManifest(t *testing.T) {
	project := t.TempDir()
	writeV1State(t, project, "configured", "docs")
	writeV1Manifest(t, project, "0.4.0", map[string]v1File{
		"AGENTS.md": {Origin: "core", Ownership: "managed"},
	})
	stateBefore := readProjectFile(t, project, ".atlas/state.json")
	manifestBefore := readProjectFile(t, project, ".atlas/manifest.json")

	_, err := Migrate(MigrateRequest{Project: project, Confirm: confirmMigration})
	if err == nil {
		t.Fatal("Migrate() error = nil, want an incomplete manifest error")
	}
	if !strings.Contains(err.Error(), "reinstall") {
		t.Errorf("error = %q, want reinstall guidance", err)
	}
	assertNoMigrationWrites(t, project, stateBefore, manifestBefore)
}

func TestMigrateStopsOnContradictorySchemas(t *testing.T) {
	project := t.TempDir()
	writeV1Manifest(t, project, "0.4.0", map[string]v1File{
		"AGENTS.md": {Origin: "core", SHA256: content.Checksum([]byte("base\n")), Ownership: "owned"},
	})
	value := &state.State{SchemaVersion: state.SchemaVersion, EngineVersion: EngineVersion}
	if err := state.Save(filepath.Join(project, ".atlas", "state.json"), value); err != nil {
		t.Fatalf("state.Save() error = %v", err)
	}
	stateBefore := readProjectFile(t, project, ".atlas/state.json")
	manifestBefore := readProjectFile(t, project, ".atlas/manifest.json")

	_, err := Migrate(MigrateRequest{Project: project, Confirm: confirmMigration})
	if err == nil {
		t.Fatal("Migrate() error = nil, want a contradictory schema error")
	}
	assertNoMigrationWrites(t, project, stateBefore, manifestBefore)
}

func TestMigrateCancelledWritesNothing(t *testing.T) {
	project := t.TempDir()
	writeV1State(t, project, "configured", "docs")
	writeV1Manifest(t, project, "0.4.0", map[string]v1File{
		"AGENTS.md": {Origin: "core", SHA256: content.Checksum([]byte("base\n")), Ownership: "owned"},
	})
	writeProjectFile(t, project, "AGENTS.md", "base\n")
	stateBefore := readProjectFile(t, project, ".atlas/state.json")
	manifestBefore := readProjectFile(t, project, ".atlas/manifest.json")

	_, err := Migrate(MigrateRequest{
		Project: project,
		Confirm: func(MigratePlan) (bool, error) { return false, nil },
	})
	if !errors.Is(err, ErrMigrationCancelled) {
		t.Fatalf("Migrate() error = %v, want ErrMigrationCancelled", err)
	}
	assertNoMigrationWrites(t, project, stateBefore, manifestBefore)
}

type v1File struct {
	Origin    string
	SHA256    string
	Ownership string
}

func copyFixture(t *testing.T, project, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "workshop-v1", name))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", name, err)
	}
	writeProjectFile(t, project, ".atlas/"+name, string(data))
	return data
}

func writeV1State(t *testing.T, project, status, docs string) {
	t.Helper()
	value := map[string]any{
		"bootstrap_version": 1,
		"template_version":  "0.4.0",
		"status":            status,
		"host":              "opencode",
		"host_source":       "env",
		"language":          map[string]string{"user": "es", "agent": "en"},
		"architecture":      map[string]string{"stance": "custom", "notes": ""},
		"profile":           "node-scripts",
		"stacks":            []map[string]string{{"id": "node-scripts"}},
		"packs":             []string{},
		"optional_agents":   []string{},
		"preferences":       map[string]string{},
		"docs":              map[string]any{"root": docs},
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(state) error = %v", err)
	}
	writeProjectFile(t, project, ".atlas/state.json", string(data)+"\n")
}

func writeV1Manifest(t *testing.T, project, templateVersion string, files map[string]v1File) {
	t.Helper()
	raw := make(map[string]any, len(files))
	for path, file := range files {
		raw[path] = map[string]string{
			"origin": file.Origin, "sha256": file.SHA256, "ownership": file.Ownership,
		}
	}
	value := map[string]any{
		"template_version": templateVersion,
		"generated_at":     "2026-11-22T01:45:29Z",
		"files":            raw,
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(manifest) error = %v", err)
	}
	writeProjectFile(t, project, ".atlas/manifest.json", string(data)+"\n")
}

func assertBackup(t *testing.T, directory string, stateBytes, manifestBytes []byte) {
	t.Helper()
	for name, want := range map[string][]byte{"state.json": stateBytes, "manifest.json": manifestBytes} {
		got, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Errorf("ReadFile(%s) error = %v", name, err)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("backup %s differs from the original", name)
		}
	}
}

func assertNoMigrationWrites(t *testing.T, project, stateBefore, manifestBefore string) {
	t.Helper()
	if got := readProjectFile(t, project, ".atlas/state.json"); got != stateBefore {
		t.Error("state was written")
	}
	if got := readProjectFile(t, project, ".atlas/manifest.json"); got != manifestBefore {
		t.Error("manifest was written")
	}
	if _, err := os.Stat(filepath.Join(project, ".atlas", "backups")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("backups were created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, filepath.FromSlash(hooks.DispatcherPath))); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dispatcher was written: %v", err)
	}
}
