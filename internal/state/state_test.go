package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleState(t *testing.T) *State {
	t.Helper()
	decisions := "adr/"
	return &State{
		SchemaVersion: SchemaVersion,
		EngineVersion: "0.6.0",
		Bundles:       []string{"dev"},
		Setup:         Setup{Complete: false, Pending: []string{"docs", "decisions"}},
		Host: Host{
			ID:       "opencode",
			Source:   "env",
			Evidence: []string{"OPENCODE_SESSION_ID"},
		},
		SpecEngine:     SpecEngine{ID: "openspec", Root: "openspec"},
		Language:       Language{CLI: "es", User: "es", Agent: "en"},
		Layout:         Layout{Docs: "documentation/", Decisions: &decisions},
		Hooks:          Hooks{Installed: true},
		MaxParallel:    MaxParallelAuto,
		Content:        Content{BundleVersion: "0.6.0"},
		Profile:        "node-scripts",
		Stacks:         []string{"node-scripts"},
		Packs:          []string{},
		OptionalAgents: []string{},
		Architecture:   Architecture{Stance: "custom", Notes: "meta-workspace"},
		Preferences:    map[string]string{"openspec": "ask"},
	}
}

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".atlas", "state.json")
	original := sampleState(t)

	if err := Save(path, original); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	got, err := json.Marshal(loaded)
	if err != nil {
		t.Fatalf("json.Marshal(loaded) error = %v", err)
	}
	want, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal(original) error = %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("round trip mismatch:\n got = %s\nwant = %s", got, want)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".atlas", "manifest.json")
	original := sampleManifest(t)

	if err := SaveManifest(path, original); err != nil {
		t.Fatalf("SaveManifest() error = %v", err)
	}
	loaded, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	got, err := json.Marshal(loaded)
	if err != nil {
		t.Fatalf("json.Marshal(loaded) error = %v", err)
	}
	want, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal(original) error = %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("round trip mismatch:\n got = %s\nwant = %s", got, want)
	}
}

func TestSaveWritesSchemaV2WithCanonicalEmptyCollections(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".atlas", "state.json")
	if err := Save(path, &State{SchemaVersion: SchemaVersion}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if got := doc["schema_version"]; got != float64(SchemaVersion) {
		t.Errorf("schema_version = %v, want %d", got, SchemaVersion)
	}
	for _, key := range []string{"bundles", "stacks", "packs", "optional_agents"} {
		if _, ok := doc[key].([]any); !ok {
			t.Errorf("%s = %v, want an empty array", key, doc[key])
		}
	}
	if _, ok := doc["preferences"].(map[string]any); !ok {
		t.Errorf("preferences = %v, want an object", doc["preferences"])
	}
	layout, ok := doc["layout"].(map[string]any)
	if !ok {
		t.Fatalf("layout = %v, want an object", doc["layout"])
	}
	if value, present := layout["decisions"]; !present || value != nil {
		t.Errorf("layout.decisions = %v (present %t), want an explicit null", value, present)
	}
	host, ok := doc["host"].(map[string]any)
	if !ok {
		t.Fatalf("host = %v, want an object", doc["host"])
	}
	if _, ok := host["evidence"].([]any); !ok {
		t.Errorf("host.evidence = %v, want an empty array", host["evidence"])
	}
	if strings.Contains(string(raw), `"bundles": null`) {
		t.Error("state contains a null collection")
	}
}
