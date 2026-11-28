package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveCleansStaleTemporaries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	orphan := filepath.Join(dir, ".state.json.tmp-stale")
	if err := os.WriteFile(orphan, []byte("resto de una escritura interrumpida"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := Save(path, sampleState(t)); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	assertNoTemporaries(t, dir, "state.json")
}

func TestSaveLeavesNoTemporaryWhenRenameFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	if err := os.MkdirAll(filepath.Join(path, "child"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := Save(path, sampleState(t)); err == nil {
		t.Fatal("Save() error = nil, want a failure when the target cannot be replaced")
	}
	assertNoTemporaries(t, dir, "state.json")

	if err := os.RemoveAll(path); err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}
	if err := Save(path, sampleState(t)); err != nil {
		t.Fatalf("Save() after failure error = %v, want the interrupted write to resume", err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("Load() after resume error = %v", err)
	}
	assertNoTemporaries(t, dir, "state.json")
}

func TestSaveManifestLeavesNoTemporary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	if err := SaveManifest(path, sampleManifest(t)); err != nil {
		t.Fatalf("SaveManifest() error = %v", err)
	}
	if _, err := LoadManifest(path); err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	assertNoTemporaries(t, dir, "manifest.json")
}

func assertNoTemporaries(t *testing.T, dir, base string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	prefix := "." + base + ".tmp-"
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			t.Errorf("temporary file left behind: %s", entry.Name())
		}
	}
}
