package bundle

import (
	"encoding/json"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cedaniel200/atlasctl/internal/classify"
	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/plan"
	"github.com/cedaniel200/atlasctl/internal/state"
)

func fixture(t *testing.T, requires map[string][]string) *content.Snapshot {
	t.Helper()
	fsys := fstest.MapFS{}
	for id, dependencies := range requires {
		if dependencies == nil {
			dependencies = []string{}
		}
		descriptor, err := json.Marshal(map[string]any{
			"id":       id,
			"version":  "0.6.0",
			"requires": dependencies,
			"roles":    "roles",
			"commands": "commands",
		})
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		fsys[id+"/bundle.json"] = &fstest.MapFile{Data: descriptor}
		fsys[id+"/"+id+".md"] = &fstest.MapFile{Data: []byte(id + "\n")}
		fsys[id+"/roles/"+id+".md"] = &fstest.MapFile{Data: []byte("# " + id + "\n")}
		fsys[id+"/commands/"+id+".md"] = &fstest.MapFile{Data: []byte("command " + id + "\n")}
	}
	snapshot, err := content.Load(fsys)
	if err != nil {
		t.Fatalf("content.Load() error = %v", err)
	}
	return snapshot
}

func snapshotIDs(snapshot *content.Snapshot) []string {
	ids := make([]string, 0, len(snapshot.Bundles()))
	for _, bundle := range snapshot.Bundles() {
		ids = append(ids, bundle.ID)
	}
	return ids
}

func TestSelectResolvesRequires(t *testing.T) {
	snapshot := fixture(t, map[string][]string{
		"core": {},
		"dev":  {"core"},
		"qa":   {"core"},
	})

	selection, err := Select(snapshot, Request{Bundles: []string{"dev"}})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if want := []string{"core", "dev"}; !reflect.DeepEqual(selection.IDs, want) {
		t.Errorf("IDs = %v, want %v", selection.IDs, want)
	}
	if want := []string{"core", "dev"}; !reflect.DeepEqual(snapshotIDs(selection.Snapshot), want) {
		t.Errorf("snapshot bundles = %v, want %v", snapshotIDs(selection.Snapshot), want)
	}
	if _, ok := selection.Snapshot.Read("qa", "qa.md"); ok {
		t.Error("an inactive bundle is present in the selection")
	}
	if _, ok := selection.Snapshot.Read("dev", "dev.md"); !ok {
		t.Error("the selected bundle is missing from the selection")
	}
	if _, ok := selection.Snapshot.Read("dev", "roles/dev.md"); !ok {
		t.Error("source material was dropped from the selection")
	}
	if _, ok := selection.Snapshot.Read("dev", "commands/dev.md"); !ok {
		t.Error("source commands were dropped from the selection")
	}
}

func TestSelectPresetFull(t *testing.T) {
	snapshot := fixture(t, map[string][]string{
		"core": {},
		"dev":  {"core"},
		"qa":   {"core"},
	})

	selection, err := Select(snapshot, Request{Preset: "full"})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if want := []string{"core", "dev", "qa"}; !reflect.DeepEqual(selection.IDs, want) {
		t.Errorf("IDs = %v, want %v", selection.IDs, want)
	}

	combined, err := Select(snapshot, Request{Bundles: []string{"dev", "dev"}, Preset: "full"})
	if err != nil {
		t.Fatalf("Select() with both flags error = %v", err)
	}
	if !reflect.DeepEqual(combined.IDs, selection.IDs) {
		t.Errorf("combined IDs = %v, want %v", combined.IDs, selection.IDs)
	}
}

func TestSelectMissingBundles(t *testing.T) {
	t.Run("preset bundle", func(t *testing.T) {
		snapshot := fixture(t, map[string][]string{"dev": {"core"}, "core": {}})
		_, err := Select(snapshot, Request{Preset: "full"})
		assertMissing(t, err, []string{"qa"})
	})

	t.Run("dependency", func(t *testing.T) {
		snapshot := fixture(t, map[string][]string{"dev": {"core"}, "qa": {"core"}})
		_, err := Select(snapshot, Request{Preset: "full"})
		assertMissing(t, err, []string{"core"})
	})

	t.Run("explicit bundle", func(t *testing.T) {
		snapshot := fixture(t, map[string][]string{"dev": {}})
		_, err := Select(snapshot, Request{Bundles: []string{"qa"}})
		assertMissing(t, err, []string{"qa"})
	})

	t.Run("several missing", func(t *testing.T) {
		snapshot := fixture(t, map[string][]string{"dev": {}})
		_, err := Select(snapshot, Request{Bundles: []string{"qa", "core"}})
		assertMissing(t, err, []string{"core", "qa"})
	})
}

func assertMissing(t *testing.T, err error, want []string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Select() error = nil, want missing %v", want)
	}
	var missing *MissingError
	if !errors.As(err, &missing) {
		t.Fatalf("Select() error = %v, want *MissingError", err)
	}
	if !reflect.DeepEqual(missing.Bundles, want) {
		t.Errorf("missing = %v, want %v", missing.Bundles, want)
	}
	for _, id := range want {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("error = %q, want it to name %q", err, id)
		}
	}
}

func TestSelectRejectsEmptySelectionAndUnknownPreset(t *testing.T) {
	snapshot := fixture(t, map[string][]string{"dev": {}})

	if _, err := Select(snapshot, Request{}); err == nil {
		t.Error("Select() error = nil, want an empty selection error")
	}
	_, err := Select(snapshot, Request{Preset: "everything"})
	if err == nil {
		t.Fatal("Select() error = nil, want an unknown preset error")
	}
	if !strings.Contains(err.Error(), "everything") || !strings.Contains(err.Error(), "full") {
		t.Errorf("error = %q, want the unknown preset and the supported list", err)
	}
	if _, err := Select(nil, Request{Bundles: []string{"dev"}}); err == nil {
		t.Error("Select(nil) error = nil, want a nil snapshot error")
	}
}

func TestPresetsAreACopy(t *testing.T) {
	presets := Presets()
	if want := []string{"dev", "qa"}; !reflect.DeepEqual(presets["full"], want) {
		t.Errorf("full = %v, want %v", presets["full"], want)
	}
	presets["full"][0] = "changed"
	if got := Presets()["full"][0]; got != "dev" {
		t.Errorf("Presets() exposed its slice: %q", got)
	}
}

func TestSelectLeavesRouteConflictsToThePlan(t *testing.T) {
	snapshot, err := content.Load(fstest.MapFS{
		"dev/bundle.json": &fstest.MapFile{Data: []byte(`{"id":"dev","version":"0.6.0","requires":[]}`)},
		"dev/AGENTS.md":   &fstest.MapFile{Data: []byte("dev\n")},
		"qa/bundle.json":  &fstest.MapFile{Data: []byte(`{"id":"qa","version":"0.6.0","requires":[]}`)},
		"qa/AGENTS.md":    &fstest.MapFile{Data: []byte("qa\n")},
	})
	if err != nil {
		t.Fatalf("content.Load() error = %v", err)
	}

	both, err := Select(snapshot, Request{Bundles: []string{"dev", "qa"}})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	planned := buildPlan(t, both.Snapshot)
	if len(planned.Conflicts) != 1 {
		t.Fatalf("conflicts = %v, want exactly one", planned.Conflicts)
	}
	conflict := planned.Conflicts[0]
	if conflict.Path != "AGENTS.md" {
		t.Errorf("conflict path = %q, want AGENTS.md", conflict.Path)
	}
	if want := []string{"dev", "qa"}; !reflect.DeepEqual(conflict.Bundles, want) {
		t.Errorf("conflict bundles = %v, want %v", conflict.Bundles, want)
	}

	onlyDev, err := Select(snapshot, Request{Bundles: []string{"dev"}})
	if err != nil {
		t.Fatalf("Select(dev) error = %v", err)
	}
	plannedDev := buildPlan(t, onlyDev.Snapshot)
	if len(plannedDev.Conflicts) != 0 {
		t.Errorf("conflicts = %v, want none with a single active bundle", plannedDev.Conflicts)
	}
}

func buildPlan(t *testing.T, snapshot *content.Snapshot) *plan.Plan {
	t.Helper()
	planned, err := plan.Build(plan.Request{
		Snapshot: snapshot,
		Manifest: &state.Manifest{SchemaVersion: state.SchemaVersion},
		Reader:   classify.ReaderFunc(func(string) ([]byte, error) { return nil, fs.ErrNotExist }),
	})
	if err != nil {
		t.Fatalf("plan.Build() error = %v", err)
	}
	return planned
}
