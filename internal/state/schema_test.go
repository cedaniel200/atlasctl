package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsFutureStateSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"schema_version": 3}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("Load() error = %v, want ErrUnsupportedSchema", err)
	}
	assertCompatibleBinaryMessage(t, err)
}

func TestLoadManifestRejectsFutureSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"schema_version": 3}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadManifest(path)
	if !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("LoadManifest() error = %v, want ErrUnsupportedSchema", err)
	}
	assertCompatibleBinaryMessage(t, err)
}

func TestSaveRejectsFutureStateSchemaWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".atlas", "state.json")
	value := sampleState(t)
	value.SchemaVersion = SchemaVersion + 1

	err := Save(path, value)
	if !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("Save() error = %v, want ErrUnsupportedSchema", err)
	}
	assertCompatibleBinaryMessage(t, err)
	assertNotWritten(t, path)
}

func TestSaveManifestRejectsFutureSchemaWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".atlas", "manifest.json")
	value := sampleManifest(t)
	value.SchemaVersion = SchemaVersion + 1

	err := SaveManifest(path, value)
	if !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("SaveManifest() error = %v, want ErrUnsupportedSchema", err)
	}
	assertCompatibleBinaryMessage(t, err)
	assertNotWritten(t, path)
}

func TestSaveRejectsOtherSchemasWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".atlas", "state.json")
	value := sampleState(t)
	value.SchemaVersion = 1

	err := Save(path, value)
	if err == nil {
		t.Fatal("Save() error = nil, want a refusal for a non-current schema")
	}
	if errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("Save() error = %v, want a distinct error for an outdated schema", err)
	}
	assertNotWritten(t, path)
}

func assertCompatibleBinaryMessage(t *testing.T, err error) {
	t.Helper()
	message := err.Error()
	if !strings.Contains(message, "compatible atlasctl binary") {
		t.Errorf("error = %q, want it to ask for a compatible atlasctl binary", message)
	}
	if !strings.Contains(message, "3") || !strings.Contains(message, "2") {
		t.Errorf("error = %q, want it to name the found and supported versions", message)
	}
}

func assertNotWritten(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat(%s) error = %v, want the file to be absent", path, err)
	}
}
