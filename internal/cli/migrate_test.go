package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/hooks"
	"github.com/cedaniel200/atlasctl/internal/i18n"
	"github.com/cedaniel200/atlasctl/internal/state"
)

func TestMigrateRequiresConfirmationViaCLI(t *testing.T) {
	project := cliV1Project(t)
	before := readCLIFile(t, project, ".atlas/state.json")

	out, errOut, code := runProject(t, project, "--lang", "en", "migrate")
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	catalog := i18n.New(i18n.English)
	if want := catalog.T(MsgMigrateConfirmationRequired, nil); !strings.Contains(errOut, want) {
		t.Errorf("stderr = %q, want %q", errOut, want)
	}
	if want := catalog.T(MsgMigratePlan, map[string]string{
		"files": "1", "scripts": "0", "remove": "0", "keep": "0",
	}); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if got := readCLIFile(t, project, ".atlas/state.json"); got != before {
		t.Error("state changed without confirmation")
	}
	assertNoCLIBackups(t, project)
}

func TestMigrateYesViaCLI(t *testing.T) {
	project := cliV1Project(t)
	out, errOut, code := runProject(t, project, "migrate", "--yes")
	if code != ExitOK {
		t.Fatalf("migrate exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	catalog := i18n.New(i18n.English)
	if want := catalog.T(MsgMigrateDone, map[string]string{"backup": ""}); strings.Contains(want, "{backup}") {
		t.Fatal("message is missing the backup marker")
	}
	if !strings.Contains(out, "Migration complete.") {
		t.Errorf("stdout = %q, want the completion line", out)
	}
	if written := cliState(t, project); written.SchemaVersion != state.SchemaVersion {
		t.Errorf("schema = %d, want %d", written.SchemaVersion, state.SchemaVersion)
	}
	backups, err := filepath.Glob(filepath.Join(project, ".atlas", "backups", "*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v (%v), want one", backups, err)
	}
	if _, err := os.Stat(filepath.Join(project, filepath.FromSlash(hooks.DispatcherPath))); err != nil {
		t.Errorf("dispatcher was not written: %v", err)
	}

	if _, _, code := runProject(t, project, "check"); code != ExitOK {
		t.Errorf("check exit code = %d, want a green check", code)
	}
}

func TestMigrateAlreadyViaCLI(t *testing.T) {
	project := t.TempDir()
	if _, _, code := runProject(t, project, "create", "--host", "opencode"); code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}

	out, errOut, code := runProject(t, project, "migrate")
	if code != ExitOK {
		t.Fatalf("migrate exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	if want := i18n.New(i18n.English).T(MsgMigrateAlready, nil); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	assertNoCLIBackups(t, project)
}

func TestMigrateInteractiveConfirmationViaCLI(t *testing.T) {
	t.Run("yes", func(t *testing.T) {
		project := cliV1Project(t)
		_, errOut, code := runProjectInteractive(t, project, "y\n", "migrate")
		if code != ExitOK {
			t.Fatalf("migrate exit code = %d, want %d: %s", code, ExitOK, errOut)
		}
		if written := cliState(t, project); written.SchemaVersion != state.SchemaVersion {
			t.Errorf("schema = %d, want %d", written.SchemaVersion, state.SchemaVersion)
		}
	})

	t.Run("no", func(t *testing.T) {
		project := cliV1Project(t)
		before := readCLIFile(t, project, ".atlas/state.json")
		out, errOut, code := runProjectInteractive(t, project, "n\n", "migrate")
		if code != ExitOK {
			t.Fatalf("migrate exit code = %d, want %d: %s", code, ExitOK, errOut)
		}
		if want := i18n.New(i18n.English).T(MsgMigrateCancelled, nil); !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want %q", out, want)
		}
		if got := readCLIFile(t, project, ".atlas/state.json"); got != before {
			t.Error("state changed after cancellation")
		}
		assertNoCLIBackups(t, project)
	})
}

func TestMigrateIncompleteViaCLI(t *testing.T) {
	project := t.TempDir()
	writeCLIV1State(t, project, "configured", "docs")
	writeCLIV1Manifest(t, project, map[string]cliV1File{
		"AGENTS.md": {Origin: "core", Ownership: "managed"},
	})
	before := readCLIFile(t, project, ".atlas/state.json")

	_, errOut, code := runProject(t, project, "migrate", "--yes")
	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut, "reinstall") {
		t.Errorf("stderr = %q, want reinstall guidance", errOut)
	}
	if got := readCLIFile(t, project, ".atlas/state.json"); got != before {
		t.Error("state changed after an incomplete migration")
	}
	assertNoCLIBackups(t, project)
}

type cliV1File struct {
	Origin    string
	SHA256    string
	Ownership string
}

func cliV1Project(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	writeCLIV1State(t, project, "configured", "docs")
	writeCLIV1Manifest(t, project, map[string]cliV1File{
		"AGENTS.md": {Origin: "core", SHA256: content.Checksum([]byte("base\n")), Ownership: "owned"},
	})
	writeCLIFile(t, project, "AGENTS.md", "base\n")
	return project
}

func writeCLIV1State(t *testing.T, project, status, docs string) {
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
	writeCLIFile(t, project, ".atlas/state.json", string(data)+"\n")
}

func writeCLIV1Manifest(t *testing.T, project string, files map[string]cliV1File) {
	t.Helper()
	raw := make(map[string]any, len(files))
	for path, file := range files {
		raw[path] = map[string]string{
			"origin": file.Origin, "sha256": file.SHA256, "ownership": file.Ownership,
		}
	}
	value := map[string]any{
		"template_version": "0.4.0",
		"generated_at":     "2026-11-22T01:45:29Z",
		"files":            raw,
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(manifest) error = %v", err)
	}
	writeCLIFile(t, project, ".atlas/manifest.json", string(data)+"\n")
}

func readCLIFile(t *testing.T, project, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(project, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", rel, err)
	}
	return string(data)
}

func assertNoCLIBackups(t *testing.T, project string) {
	t.Helper()
	backups, err := filepath.Glob(filepath.Join(project, ".atlas", "backups", "*"))
	if err != nil {
		t.Fatalf("Glob(backups) error = %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("backups = %v, want none", backups)
	}
}

func TestMigrateReportsScriptsAndNewFilesViaCLI(t *testing.T) {
	project := t.TempDir()
	writeCLIV1State(t, project, "configured", "docs")
	writeCLIV1Manifest(t, project, map[string]cliV1File{
		"AGENTS.md":               {Origin: "core", SHA256: content.Checksum([]byte("base\n")), Ownership: "managed"},
		"docs/note.md":            {Origin: "core", SHA256: content.Checksum([]byte("base note\n")), Ownership: "managed"},
		".atlas/scripts/tool.mjs": {Origin: "core", SHA256: content.Checksum([]byte("tool\n")), Ownership: "managed"},
	})
	writeCLIFile(t, project, "AGENTS.md", "base\n")
	writeCLIFile(t, project, "docs/note.md", "base note\n")
	writeCLIFile(t, project, ".atlas/scripts/tool.mjs", "tool\n")
	writeCLIFile(t, project, "AGENTS.md.new", "incoming\r\n")
	writeCLIFile(t, project, "docs/note.md.new", "different\n")
	writeCLIFile(t, project, ".atlas/scripts/tool.mjs.new", "tool v2\n")

	snapshot := t.TempDir()
	writeCLIFile(t, snapshot, "dev/bundle.json", `{"id":"dev","version":"0.6.0","requires":[]}`)
	writeCLIFile(t, snapshot, "dev/AGENTS.md", "incoming\n")
	writeCLIFile(t, snapshot, "dev/docs/note.md", "incoming note\n")

	out, errOut, code := runProject(t, project, "migrate", "--yes", "--snapshot", snapshot)
	if code != ExitOK {
		t.Fatalf("migrate exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	catalog := i18n.New(i18n.English)
	for id, args := range map[string]map[string]string{
		MsgMigrateScripts:    {"count": "1"},
		MsgMigrateNewRemoved: {"paths": "AGENTS.md.new"},
		MsgMigrateNewKept:    {"paths": ".atlas/scripts/tool.mjs.new, docs/note.md.new"},
	} {
		if want := catalog.T(id, args); !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want %q", out, want)
		}
	}
	if _, err := os.Stat(filepath.Join(project, "AGENTS.md.new")); !os.IsNotExist(err) {
		t.Errorf("matching .new was not removed: %v", err)
	}
	for _, rel := range []string{"docs/note.md.new", ".atlas/scripts/tool.mjs.new"} {
		if _, err := os.Stat(filepath.Join(project, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s was removed: %v", rel, err)
		}
	}

	manifest, err := state.LoadManifest(filepath.Join(project, ".atlas", "manifest.json"))
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if entry := manifest.Files[".atlas/scripts/tool.mjs"]; entry.Layer != state.LayerOwned {
		t.Errorf("script layer = %q, want owned", entry.Layer)
	}
	if _, _, code := runProject(t, project, "check"); code != ExitOK {
		t.Errorf("check exit code = %d, want a green check", code)
	}
}
