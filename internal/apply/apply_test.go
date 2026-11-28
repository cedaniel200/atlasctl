package apply

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/fsutil"
	"github.com/cedaniel200/atlasctl/internal/plan"
	"github.com/cedaniel200/atlasctl/internal/state"
)

var fixedTime = time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)

func snapshotWith(t *testing.T, files map[string]string) *content.Snapshot {
	t.Helper()
	fsys := fstest.MapFS{
		"dev/bundle.json": &fstest.MapFile{
			Data: []byte(`{"id":"dev","version":"0.6.0","requires":[]}`),
		},
	}
	for path, data := range files {
		fsys["dev/"+path] = &fstest.MapFile{Data: []byte(data)}
	}
	snapshot, err := content.Load(fsys)
	if err != nil {
		t.Fatalf("content.Load() error = %v", err)
	}
	return snapshot
}

func tracked(layer state.Layer, data string) state.ManifestFile {
	return state.ManifestFile{
		Bundle: "dev",
		Origin: "core",
		Layer:  layer,
		SHA256: content.Checksum([]byte(data)),
	}
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for path, data := range files {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", path, err)
		}
		if err := os.WriteFile(absolute, []byte(data), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
}

func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		tree[filepath.ToSlash(relative)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir() error = %v", err)
	}
	return tree
}

func assertNoNewOrTemporaryFiles(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".new") || strings.Contains(name, ".tmp-") {
			t.Errorf("unexpected artifact: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir() error = %v", err)
	}
}

func assertPaths(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("paths[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func buildPlan(t *testing.T, root string, snapshot *content.Snapshot, manifest *state.Manifest, generated []plan.Generated) *plan.Plan {
	t.Helper()
	return buildPlanWith(t, root, snapshot, manifest, generated, plan.Options{})
}

func buildPlanWith(t *testing.T, root string, snapshot *content.Snapshot, manifest *state.Manifest, generated []plan.Generated, options plan.Options) *plan.Plan {
	t.Helper()
	planned, err := plan.Build(plan.Request{
		Snapshot:  snapshot,
		Manifest:  manifest,
		Reader:    fsutil.RootReader{Root: root},
		Generated: generated,
		Options:   options,
	})
	if err != nil {
		t.Fatalf("plan.Build() error = %v", err)
	}
	return planned
}

func TestApplyWritesCreatedAndUpdatedFilesAndManifest(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"AGENTS.md": "viejo\n"})

	snapshot := snapshotWith(t, map[string]string{
		"AGENTS.md":     "nuevo\n",
		"docs/guide.md": "guia\n",
	})
	manifest := &state.Manifest{
		SchemaVersion: state.SchemaVersion,
		Files:         map[string]state.ManifestFile{"AGENTS.md": tracked(state.LayerBase, "viejo\n")},
	}
	planned := buildPlan(t, root, snapshot, manifest, nil)

	report, err := Apply(Request{
		Root:          root,
		Plan:          planned,
		Allowed:       []string{"AGENTS.md", "docs"},
		EngineVersion: "0.6.0",
		GeneratedAt:   fixedTime,
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	assertPaths(t, report.Updated, "AGENTS.md", "docs/guide.md")
	assertPaths(t, report.Conserved)
	assertPaths(t, report.Divergent)
	assertPaths(t, report.Pruned)

	tree := readTree(t, root)
	if tree["AGENTS.md"] != "nuevo\n" {
		t.Errorf("AGENTS.md = %q, want the updated content", tree["AGENTS.md"])
	}
	if tree["docs/guide.md"] != "guia\n" {
		t.Errorf("docs/guide.md = %q, want the created content", tree["docs/guide.md"])
	}

	written, err := state.LoadManifest(filepath.Join(root, ".atlas", "manifest.json"))
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if written.EngineVersion != "0.6.0" {
		t.Errorf("engine version = %q, want 0.6.0", written.EngineVersion)
	}
	if !written.GeneratedAt.Equal(fixedTime) {
		t.Errorf("generated_at = %v, want %v", written.GeneratedAt, fixedTime)
	}
	if got, want := written.Files["AGENTS.md"].SHA256, content.Checksum([]byte("nuevo\n")); got != want {
		t.Errorf("AGENTS.md sha256 = %q, want %q", got, want)
	}
	if got, want := written.Files["docs/guide.md"].SHA256, content.Checksum([]byte("guia\n")); got != want {
		t.Errorf("docs/guide.md sha256 = %q, want %q", got, want)
	}
	assertNoNewOrTemporaryFiles(t, root)
}

func TestApplyIsIdempotent(t *testing.T) {
	root := t.TempDir()
	snapshot := snapshotWith(t, map[string]string{"AGENTS.md": "nuevo\n"})

	firstPlan := buildPlan(t, root, snapshot, &state.Manifest{SchemaVersion: state.SchemaVersion}, nil)
	firstReport, err := Apply(Request{
		Root: root, Plan: firstPlan, Allowed: []string{"AGENTS.md"},
		EngineVersion: "0.6.0", GeneratedAt: fixedTime,
	})
	if err != nil {
		t.Fatalf("first Apply() error = %v", err)
	}
	assertPaths(t, firstReport.Updated, "AGENTS.md")

	firstTree := readTree(t, root)
	written, err := state.LoadManifest(filepath.Join(root, ".atlas", "manifest.json"))
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	secondPlan := buildPlan(t, root, snapshot, written, nil)
	secondReport, err := Apply(Request{
		Root: root, Plan: secondPlan, Allowed: []string{"AGENTS.md"},
		EngineVersion: "0.6.0", GeneratedAt: fixedTime,
	})
	if err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}

	assertPaths(t, secondReport.Updated)
	assertPaths(t, secondReport.Pruned)
	assertPaths(t, secondReport.Divergent)
	assertPaths(t, secondReport.Conserved, "AGENTS.md")

	if secondTree := readTree(t, root); !reflect.DeepEqual(firstTree, secondTree) {
		t.Errorf("second apply changed the tree:\n first = %v\nsecond = %v", firstTree, secondTree)
	}
	assertNoNewOrTemporaryFiles(t, root)
}

func TestApplyRejectsUnauthorizedPathBeforeWriting(t *testing.T) {
	root := t.TempDir()
	snapshot := snapshotWith(t, map[string]string{"AGENTS.md": "nuevo\n"})
	planned := buildPlan(t, root, snapshot, &state.Manifest{SchemaVersion: state.SchemaVersion}, nil)

	_, err := Apply(Request{
		Root: root, Plan: planned, EngineVersion: "0.6.0", GeneratedAt: fixedTime,
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want a rejection of a path outside .atlas/ and the host routes")
	}
	if !strings.Contains(err.Error(), "AGENTS.md") {
		t.Errorf("error = %q, want it to name the rejected path", err)
	}

	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md was written despite the rejection: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas", "manifest.json")); !os.IsNotExist(err) {
		t.Errorf("the manifest was written despite the rejection: %v", err)
	}
}

func TestApplyRejectsInvalidAllowedRoutes(t *testing.T) {
	root := t.TempDir()
	snapshot := snapshotWith(t, map[string]string{"AGENTS.md": "nuevo\n"})
	planned := buildPlan(t, root, snapshot, &state.Manifest{SchemaVersion: state.SchemaVersion}, nil)

	_, err := Apply(Request{
		Root: root, Plan: planned, Allowed: []string{"../escape"},
		EngineVersion: "0.6.0", GeneratedAt: fixedTime,
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want a rejection of an escaping allowed route")
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md was written despite the rejection: %v", err)
	}
}

func TestApplyKeepsModifiedBaseAndReportsDivergence(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"AGENTS.md": "propio\n"})

	snapshot := snapshotWith(t, map[string]string{"AGENTS.md": "nuevo\n"})
	manifest := &state.Manifest{
		SchemaVersion: state.SchemaVersion,
		Files:         map[string]state.ManifestFile{"AGENTS.md": tracked(state.LayerBase, "viejo\n")},
	}
	planned := buildPlan(t, root, snapshot, manifest, nil)

	report, err := Apply(Request{
		Root: root, Plan: planned, Allowed: []string{"AGENTS.md"},
		EngineVersion: "0.6.0", GeneratedAt: fixedTime,
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	assertPaths(t, report.Updated)
	assertPaths(t, report.Conserved, "AGENTS.md")
	assertPaths(t, report.Divergent, "AGENTS.md")

	tree := readTree(t, root)
	if tree["AGENTS.md"] != "propio\n" {
		t.Errorf("AGENTS.md = %q, want the adoptter content untouched", tree["AGENTS.md"])
	}
	written, err := state.LoadManifest(filepath.Join(root, ".atlas", "manifest.json"))
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if got, want := written.Files["AGENTS.md"].SHA256, content.Checksum([]byte("nuevo\n")); got != want {
		t.Errorf("manifest sha256 = %q, want the new expectation %q", got, want)
	}
}

func TestApplyRegeneratesEditedGeneratedFileAndReportsLoss(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".opencode/agents/analyst.md": "editado\n"})

	snapshot := snapshotWith(t, nil)
	manifest := &state.Manifest{
		SchemaVersion: state.SchemaVersion,
		Files: map[string]state.ManifestFile{
			".opencode/agents/analyst.md": {
				Bundle: "dev", Origin: "host:test", Layer: state.LayerGenerated,
				SHA256: content.Checksum([]byte("viejo\n")),
			},
		},
	}
	generated := []plan.Generated{{
		Path: ".opencode/agents/analyst.md", Bundle: "dev", Origin: "host:test", Data: []byte("nuevo\n"),
	}}
	planned := buildPlan(t, root, snapshot, manifest, generated)

	report, err := Apply(Request{
		Root: root, Plan: planned, Allowed: []string{".opencode/agents"},
		EngineVersion: "0.6.0", GeneratedAt: fixedTime,
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	assertPaths(t, report.Updated, ".opencode/agents/analyst.md")
	assertPaths(t, report.Divergent, ".opencode/agents/analyst.md")

	tree := readTree(t, root)
	if tree[".opencode/agents/analyst.md"] != "nuevo\n" {
		t.Errorf("generated file = %q, want the regenerated content", tree[".opencode/agents/analyst.md"])
	}
	written, err := state.LoadManifest(filepath.Join(root, ".atlas", "manifest.json"))
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	entry := written.Files[".opencode/agents/analyst.md"]
	if entry.Origin != "host:test" || entry.Layer != state.LayerGenerated {
		t.Errorf("entry = %+v, want a generated host entry", entry)
	}
	assertNoNewOrTemporaryFiles(t, root)
}

func TestApplyRefusesConflictingPlanWithoutWriting(t *testing.T) {
	root := t.TempDir()
	fsys := fstest.MapFS{
		"dev/bundle.json": &fstest.MapFile{Data: []byte(`{"id":"dev","version":"0.6.0","requires":[]}`)},
		"dev/AGENTS.md":   &fstest.MapFile{Data: []byte("dev\n")},
		"dev/README.md":   &fstest.MapFile{Data: []byte("readme\n")},
		"qa/bundle.json":  &fstest.MapFile{Data: []byte(`{"id":"qa","version":"0.6.0","requires":["core"]}`)},
		"qa/AGENTS.md":    &fstest.MapFile{Data: []byte("qa\n")},
	}
	snapshot, err := content.Load(fsys)
	if err != nil {
		t.Fatalf("content.Load() error = %v", err)
	}
	planned := buildPlan(t, root, snapshot, &state.Manifest{SchemaVersion: state.SchemaVersion}, nil)

	_, err = Apply(Request{
		Root: root, Plan: planned, Allowed: []string{"README.md"},
		EngineVersion: "0.6.0", GeneratedAt: fixedTime,
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want a refusal for a conflicting plan")
	}
	if _, err := os.Stat(filepath.Join(root, "README.md")); !os.IsNotExist(err) {
		t.Errorf("README.md was written despite the conflict: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas", "manifest.json")); !os.IsNotExist(err) {
		t.Errorf("the manifest was written despite the conflict: %v", err)
	}
}

func TestApplyPrunesOnlyEligibleFiles(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"gen.md":  "g\n",
		"base.md": "b\n",
		"mod.md":  "propio\n",
		"ov.md":   "override\n",
		"own.md":  "owned\n",
	})
	manifest := &state.Manifest{
		SchemaVersion: state.SchemaVersion,
		Files: map[string]state.ManifestFile{
			"gen.md": {
				Bundle: "dev", Origin: "host:test", Layer: state.LayerGenerated,
				SHA256: content.Checksum([]byte("g\n")),
			},
			"base.md": tracked(state.LayerBase, "b\n"),
			"mod.md":  tracked(state.LayerBase, "viejo\n"),
			"ov.md":   tracked(state.LayerOverrides, "base\n"),
			"own.md":  tracked(state.LayerOwned, "x\n"),
		},
	}
	planned := buildPlanWith(t, root, snapshotWith(t, nil), manifest, nil, plan.Options{Prune: true})

	report, err := Apply(Request{
		Root: root, Plan: planned, Allowed: []string{"base.md", "gen.md"},
		EngineVersion: "0.6.0", GeneratedAt: fixedTime,
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	assertPaths(t, report.Updated)
	assertPaths(t, report.Pruned, "base.md", "gen.md")
	assertPaths(t, report.Conserved, "mod.md", "ov.md", "own.md")
	assertPaths(t, report.Divergent, "mod.md")

	tree := readTree(t, root)
	for _, gone := range []string{"base.md", "gen.md"} {
		if _, ok := tree[gone]; ok {
			t.Errorf("%s was not pruned", gone)
		}
	}
	if tree["mod.md"] != "propio\n" {
		t.Errorf("mod.md = %q, want the adoptter content", tree["mod.md"])
	}
	if tree["ov.md"] != "override\n" || tree["own.md"] != "owned\n" {
		t.Errorf("override or owned content changed: %v", tree)
	}

	written, err := state.LoadManifest(filepath.Join(root, ".atlas", "manifest.json"))
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	for _, gone := range []string{"base.md", "gen.md"} {
		if _, ok := written.Files[gone]; ok {
			t.Errorf("%s is still in the manifest", gone)
		}
	}
	assertNoNewOrTemporaryFiles(t, root)
}

func TestApplyHonorsClosedPruneExclusions(t *testing.T) {
	root := t.TempDir()
	protected := []string{
		"openspec/changes/a.md",
		".opencode/opencode.json",
		".git/hooks/pre-commit",
		".atlas/state.json",
		".atlas/manifest.json",
	}
	files := map[string]string{"docs/old.md": "viejo\n"}
	entries := map[string]state.ManifestFile{"docs/old.md": tracked(state.LayerBase, "viejo\n")}
	for _, path := range protected {
		files[path] = "viejo\n"
		entries[path] = tracked(state.LayerBase, "viejo\n")
	}
	writeFiles(t, root, files)
	manifest := &state.Manifest{SchemaVersion: state.SchemaVersion, Files: entries}
	planned := buildPlanWith(t, root, snapshotWith(t, nil), manifest, nil, plan.Options{
		Prune:          true,
		SpecEngineRoot: "openspec",
		HostConfig:     []string{".opencode/opencode.json"},
	})

	report, err := Apply(Request{
		Root: root, Plan: planned, Allowed: []string{"docs"},
		EngineVersion: "0.6.0", GeneratedAt: fixedTime,
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	assertPaths(t, report.Pruned, "docs/old.md")
	for _, path := range protected {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Errorf("protected path %s did not survive pruning: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "old.md")); !os.IsNotExist(err) {
		t.Errorf("docs/old.md survived pruning: %v", err)
	}
}

func TestApplyRejectsSymlinkAtDestination(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "target.md")
	if err := os.WriteFile(target, []byte("viejo\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	mustSymlink(t, target, filepath.Join(root, "AGENTS.md"))

	snapshot := snapshotWith(t, map[string]string{"AGENTS.md": "nuevo\n"})
	manifest := &state.Manifest{
		SchemaVersion: state.SchemaVersion,
		Files:         map[string]state.ManifestFile{"AGENTS.md": tracked(state.LayerBase, "viejo\n")},
	}
	planned := buildPlan(t, root, snapshot, manifest, nil)

	_, err := Apply(Request{
		Root: root, Plan: planned, Allowed: []string{"AGENTS.md"},
		EngineVersion: "0.6.0", GeneratedAt: fixedTime,
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want a symlink rejection")
	}
	if !strings.Contains(err.Error(), "symbolic link") || !strings.Contains(err.Error(), "AGENTS.md") {
		t.Errorf("error = %q, want the path and the reason", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "viejo\n" {
		t.Errorf("external target = (%q, %v), want it untouched", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas", "manifest.json")); !os.IsNotExist(err) {
		t.Errorf("the manifest was written despite the rejection: %v", err)
	}
}

func TestApplyRejectsSymlinkInIntermediateDirectory(t *testing.T) {
	t.Run("write", func(t *testing.T) {
		outside := t.TempDir()
		if err := os.WriteFile(filepath.Join(outside, "file.md"), []byte("viejo\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		root := t.TempDir()
		mustSymlink(t, outside, filepath.Join(root, "docs"))

		snapshot := snapshotWith(t, map[string]string{"docs/file.md": "nuevo\n"})
		manifest := &state.Manifest{
			SchemaVersion: state.SchemaVersion,
			Files:         map[string]state.ManifestFile{"docs/file.md": tracked(state.LayerBase, "viejo\n")},
		}
		planned := buildPlan(t, root, snapshot, manifest, nil)

		_, err := Apply(Request{
			Root: root, Plan: planned, Allowed: []string{"docs"},
			EngineVersion: "0.6.0", GeneratedAt: fixedTime,
		})
		if err == nil {
			t.Fatal("Apply() error = nil, want a symlink rejection")
		}
		if !strings.Contains(err.Error(), "docs") || !strings.Contains(err.Error(), "symbolic link") {
			t.Errorf("error = %q, want the component and the reason", err)
		}
		if data, err := os.ReadFile(filepath.Join(outside, "file.md")); err != nil || string(data) != "viejo\n" {
			t.Errorf("external file = (%q, %v), want it untouched", data, err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		outside := t.TempDir()
		externalDir := filepath.Join(outside, "managed")
		if err := os.MkdirAll(externalDir, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(externalDir, "file.md"), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		root := t.TempDir()
		mustSymlink(t, externalDir, filepath.Join(root, "docs"))

		manifest := &state.Manifest{
			SchemaVersion: state.SchemaVersion,
			Files: map[string]state.ManifestFile{
				"docs/file.md": {
					Bundle: "dev", Origin: "host:test", Layer: state.LayerGenerated,
					SHA256: content.Checksum([]byte("x\n")),
				},
			},
		}
		planned := buildPlan(t, root, snapshotWith(t, nil), manifest, nil)

		_, err := Apply(Request{
			Root: root, Plan: planned, Allowed: []string{"docs"},
			EngineVersion: "0.6.0", GeneratedAt: fixedTime,
		})
		if err == nil {
			t.Fatal("Apply() error = nil, want a symlink rejection")
		}
		if _, err := os.Stat(filepath.Join(externalDir, "file.md")); err != nil {
			t.Errorf("external file was deleted: %v", err)
		}
	})
}

func TestApplyRejectsSymlinkedManifestDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	mustSymlink(t, outside, filepath.Join(root, ".atlas"))

	planned := buildPlan(t, root, snapshotWith(t, nil), &state.Manifest{SchemaVersion: state.SchemaVersion}, nil)

	_, err := Apply(Request{
		Root: root, Plan: planned, Allowed: nil,
		EngineVersion: "0.6.0", GeneratedAt: fixedTime,
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want a symlink rejection for the manifest directory")
	}
	if _, err := os.Stat(filepath.Join(outside, "manifest.json")); !os.IsNotExist(err) {
		t.Errorf("the manifest was written outside the project: %v", err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
}
