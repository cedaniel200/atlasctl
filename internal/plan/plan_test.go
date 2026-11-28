package plan

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/cedaniel200/atlasctl/internal/classify"
	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/state"
)

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

func manifestWith(files map[string]state.ManifestFile) *state.Manifest {
	return &state.Manifest{SchemaVersion: state.SchemaVersion, Files: files}
}

func tracked(layer state.Layer, data string) state.ManifestFile {
	return state.ManifestFile{
		Bundle: "dev",
		Origin: "core",
		Layer:  layer,
		SHA256: content.Checksum([]byte(data)),
	}
}

func diskWith(files map[string]string) classify.ReaderFunc {
	return func(path string) ([]byte, error) {
		data, ok := files[path]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return []byte(data), nil
	}
}

func forbiddenReads(t *testing.T) classify.ReaderFunc {
	return func(path string) ([]byte, error) {
		t.Errorf("unexpected read of %q", path)
		return nil, fs.ErrNotExist
	}
}

func buildPlan(t *testing.T, request Request) *Plan {
	t.Helper()
	planned, err := Build(request)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return planned
}

func actionAt(t *testing.T, planned *Plan, path string) Action {
	t.Helper()
	for _, action := range planned.Actions {
		if action.Path == path {
			return action
		}
	}
	t.Fatalf("no action for %q in %v", path, planned.Actions)
	return Action{}
}

func hasAction(planned *Plan, path string) bool {
	for _, action := range planned.Actions {
		if action.Path == path {
			return true
		}
	}
	return false
}

func TestBuildCreatesMissingBaseFile(t *testing.T) {
	planned := buildPlan(t, Request{
		Snapshot: snapshotWith(t, map[string]string{"docs/new.md": "nuevo\n"}),
		Manifest: manifestWith(nil),
		Reader:   diskWith(nil),
	})

	action := actionAt(t, planned, "docs/new.md")
	if action.Kind != Create {
		t.Errorf("kind = %q, want %q", action.Kind, Create)
	}
	if action.Layer != state.LayerBase {
		t.Errorf("layer = %q, want %q", action.Layer, state.LayerBase)
	}
	if string(action.Data) != "nuevo\n" {
		t.Errorf("data = %q, want the snapshot content", action.Data)
	}

	entry, ok := planned.Manifest.Files["docs/new.md"]
	if !ok {
		t.Fatal("new file is absent from the planned manifest")
	}
	if entry.SHA256 != content.Checksum([]byte("nuevo\n")) {
		t.Errorf("sha256 = %q, want the snapshot hash", entry.SHA256)
	}
	if entry.Origin != "core" || entry.Bundle != "dev" {
		t.Errorf("entry = %+v, want bundle dev and origin core", entry)
	}
}

func TestBuildUpdatesIntactBaseWhenTheSnapshotChanges(t *testing.T) {
	planned := buildPlan(t, Request{
		Snapshot: snapshotWith(t, map[string]string{"AGENTS.md": "nuevo\n"}),
		Manifest: manifestWith(map[string]state.ManifestFile{
			"AGENTS.md": tracked(state.LayerBase, "viejo\n"),
		}),
		Reader: diskWith(map[string]string{"AGENTS.md": "viejo\n"}),
	})

	action := actionAt(t, planned, "AGENTS.md")
	if action.Kind != Update {
		t.Errorf("kind = %q, want %q", action.Kind, Update)
	}
	if action.Divergent {
		t.Error("an intact base update is not a divergence")
	}
	if string(action.Data) != "nuevo\n" {
		t.Errorf("data = %q, want the new snapshot content", action.Data)
	}
	if got, want := planned.Manifest.Files["AGENTS.md"].SHA256, content.Checksum([]byte("nuevo\n")); got != want {
		t.Errorf("manifest sha256 = %q, want %q", got, want)
	}
}

func TestBuildKeepsModifiedBaseAndFlagsDivergence(t *testing.T) {
	planned := buildPlan(t, Request{
		Snapshot: snapshotWith(t, map[string]string{"AGENTS.md": "nuevo\n"}),
		Manifest: manifestWith(map[string]state.ManifestFile{
			"AGENTS.md": tracked(state.LayerBase, "viejo\n"),
		}),
		Reader: diskWith(map[string]string{"AGENTS.md": "propio\n"}),
	})

	action := actionAt(t, planned, "AGENTS.md")
	if action.Kind != Keep {
		t.Errorf("kind = %q, want %q", action.Kind, Keep)
	}
	if !action.Divergent {
		t.Error("a modified base file must be flagged as divergent")
	}
	if len(action.Data) != 0 {
		t.Errorf("data = %q, want no write for a modified base", action.Data)
	}
	if got, want := planned.Manifest.Files["AGENTS.md"].SHA256, content.Checksum([]byte("nuevo\n")); got != want {
		t.Errorf("manifest sha256 = %q, want the new expectation %q", got, want)
	}
}

func TestBuildKeepsUntrackedDivergentBase(t *testing.T) {
	planned := buildPlan(t, Request{
		Snapshot: snapshotWith(t, map[string]string{"AGENTS.md": "base\n"}),
		Manifest: manifestWith(nil),
		Reader:   diskWith(map[string]string{"AGENTS.md": "mio\n"}),
	})

	action := actionAt(t, planned, "AGENTS.md")
	if action.Kind != Keep || !action.Divergent {
		t.Errorf("action = %+v, want a divergent keep", action)
	}
	if _, ok := planned.Manifest.Files["AGENTS.md"]; !ok {
		t.Error("the divergent file was not recorded in the manifest")
	}
}

func TestBuildNeverReadsOverridesOrOwned(t *testing.T) {
	planned := buildPlan(t, Request{
		Snapshot: snapshotWith(t, map[string]string{"AGENTS.md": "base nuevo\n"}),
		Manifest: manifestWith(map[string]state.ManifestFile{
			"AGENTS.md":               tracked(state.LayerOverrides, "base viejo\n"),
			"docs/project-context.md": tracked(state.LayerOwned, "propio\n"),
		}),
		Reader: forbiddenReads(t),
	})

	override := actionAt(t, planned, "AGENTS.md")
	if override.Kind != Keep || override.Layer != state.LayerOverrides || override.Divergent {
		t.Errorf("override action = %+v, want an untouched keep", override)
	}
	owned := actionAt(t, planned, "docs/project-context.md")
	if owned.Kind != Keep || owned.Layer != state.LayerOwned {
		t.Errorf("owned action = %+v, want an untouched keep", owned)
	}
	if got, want := planned.Manifest.Files["AGENTS.md"].SHA256, content.Checksum([]byte("base viejo\n")); got != want {
		t.Errorf("override sha256 = %q, want the base hash it was written on %q", got, want)
	}
}

func TestBuildRegeneratesEditedGeneratedFile(t *testing.T) {
	planned := buildPlan(t, Request{
		Snapshot: snapshotWith(t, nil),
		Manifest: manifestWith(map[string]state.ManifestFile{
			".atlas/hooks/pre-commit": tracked(state.LayerGenerated, "viejo\n"),
		}),
		Generated: []Generated{{
			Path: ".atlas/hooks/pre-commit", Bundle: "dev", Origin: "host:test", Data: []byte("nuevo\n"),
		}},
		Reader: diskWith(map[string]string{".atlas/hooks/pre-commit": "editado\n"}),
	})

	action := actionAt(t, planned, ".atlas/hooks/pre-commit")
	if action.Kind != Update {
		t.Errorf("kind = %q, want %q", action.Kind, Update)
	}
	if !action.Divergent {
		t.Error("an edited generated file must be flagged as divergent")
	}
	if string(action.Data) != "nuevo\n" {
		t.Errorf("data = %q, want the regenerated content", action.Data)
	}
}

func TestBuildCreatesGeneratedFile(t *testing.T) {
	planned := buildPlan(t, Request{
		Snapshot: snapshotWith(t, nil),
		Manifest: manifestWith(nil),
		Generated: []Generated{{
			Path: ".opencode/agents/analyst.md", Bundle: "dev", Origin: "host:opencode", Data: []byte("rol\n"),
		}},
		Reader: diskWith(nil),
	})

	action := actionAt(t, planned, ".opencode/agents/analyst.md")
	if action.Kind != Create || action.Layer != state.LayerGenerated {
		t.Errorf("action = %+v, want a generated create", action)
	}
	entry := planned.Manifest.Files[".opencode/agents/analyst.md"]
	if entry.Origin != "host:opencode" || entry.Layer != state.LayerGenerated {
		t.Errorf("entry = %+v, want a generated host entry", entry)
	}
}

func TestBuildKeepsFilesThatLeftTheDesiredSet(t *testing.T) {
	planned := buildPlan(t, Request{
		Snapshot: snapshotWith(t, nil),
		Manifest: manifestWith(map[string]state.ManifestFile{
			"old.md":                  tracked(state.LayerBase, "viejo\n"),
			".opencode/agents/old.md": tracked(state.LayerGenerated, "viejo\n"),
			"docs/project-context.md": tracked(state.LayerOwned, "propio\n"),
		}),
		Reader: diskWith(map[string]string{
			"old.md":                  "viejo\n",
			".opencode/agents/old.md": "viejo\n",
			"docs/project-context.md": "propio\n",
		}),
	})

	for _, path := range []string{"old.md", ".opencode/agents/old.md", "docs/project-context.md"} {
		action := actionAt(t, planned, path)
		if action.Kind != Keep {
			t.Errorf("%s kind = %q, want %q", path, action.Kind, Keep)
		}
		if _, ok := planned.Manifest.Files[path]; !ok {
			t.Errorf("%s left the planned manifest", path)
		}
	}
}

func TestBuildReportsConflictsBetweenBundles(t *testing.T) {
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

	planned := buildPlan(t, Request{
		Snapshot: snapshot,
		Manifest: manifestWith(nil),
		Reader:   diskWith(nil),
	})

	if len(planned.Conflicts) != 1 {
		t.Fatalf("conflicts = %v, want exactly one", planned.Conflicts)
	}
	conflict := planned.Conflicts[0]
	if conflict.Path != "AGENTS.md" {
		t.Errorf("conflict path = %q, want %q", conflict.Path, "AGENTS.md")
	}
	if len(conflict.Bundles) != 2 || conflict.Bundles[0] != "dev" || conflict.Bundles[1] != "qa" {
		t.Errorf("conflict bundles = %v, want [dev qa]", conflict.Bundles)
	}
	if hasAction(planned, "AGENTS.md") {
		t.Error("a conflicted path must not have an action")
	}
	if action := actionAt(t, planned, "README.md"); action.Kind != Create {
		t.Errorf("README.md kind = %q, want %q", action.Kind, Create)
	}
}
