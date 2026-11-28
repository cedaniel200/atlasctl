package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/content"
)

func writeBundle(t *testing.T, root, id, requires string) {
	t.Helper()
	descriptor := fmt.Sprintf(
		`{"id":%q,"version":"0.6.0","requires":%s,"roles":"roles","commands":"commands"}`,
		id, requires,
	)
	writeSnapshotFile(t, root, id+"/bundle.json", descriptor)
	writeSnapshotFile(t, root, id+"/"+id+".md", id+"\n")
	writeSnapshotFile(t, root, id+"/roles/"+id+".md", "# "+id+"\n")
	writeSnapshotFile(t, root, id+"/commands/"+id+".md", "command "+id+"\n")
}

func writeSnapshotFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", rel, err)
	}
}

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
}

func TestLoadEmbeddedSnapshot(t *testing.T) {
	snapshot, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := snapshotIDs(snapshot), []string{"dev"}; !reflect.DeepEqual(got, want) {
		t.Errorf("bundles = %v, want %v", got, want)
	}

	selection, err := Select(snapshot, Request{Bundles: []string{"dev"}})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if want := []string{"dev"}; !reflect.DeepEqual(selection.IDs, want) {
		t.Errorf("IDs = %v, want %v", selection.IDs, want)
	}
}

func TestLoadDirectorySnapshot(t *testing.T) {
	root := t.TempDir()
	writeBundle(t, root, "core", `[]`)
	writeBundle(t, root, "dev", `["core"]`)
	writeBundle(t, root, "qa", `["core"]`)

	snapshot, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := snapshotIDs(snapshot), []string{"core", "dev", "qa"}; !reflect.DeepEqual(got, want) {
		t.Errorf("bundles = %v, want %v", got, want)
	}

	selection, err := Select(snapshot, Request{Preset: "full"})
	if err != nil {
		t.Fatalf("Select(full) error = %v", err)
	}
	if got, want := selection.IDs, []string{"core", "dev", "qa"}; !reflect.DeepEqual(got, want) {
		t.Errorf("IDs = %v, want %v", got, want)
	}
	if _, ok := selection.Snapshot.Read("qa", "roles/qa.md"); !ok {
		t.Error("the disk snapshot lost the qa role source")
	}
}

func TestLoadRejectsSymbolicLinks(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		snapshotDir := t.TempDir()
		writeBundle(t, snapshotDir, "dev", `[]`)
		link := filepath.Join(t.TempDir(), "link")
		symlinkOrSkip(t, snapshotDir, link)

		if _, err := Load(link); err == nil {
			t.Fatal("Load() error = nil, want a symbolic link rejection")
		} else if !strings.Contains(err.Error(), "symbolic link") {
			t.Errorf("error = %q, want a symbolic link rejection", err)
		}
	})

	t.Run("inside a bundle", func(t *testing.T) {
		snapshotDir := t.TempDir()
		writeBundle(t, snapshotDir, "dev", `[]`)
		outside := t.TempDir()
		external := filepath.Join(outside, "external.md")
		if err := os.WriteFile(external, []byte("fuera\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		symlinkOrSkip(t, external, filepath.Join(snapshotDir, "dev", "linked.md"))

		_, err := Load(snapshotDir)
		if err == nil {
			t.Fatal("Load() error = nil, want a symbolic link rejection")
		}
		if !strings.Contains(err.Error(), "symbolic link") || !strings.Contains(err.Error(), "linked.md") {
			t.Errorf("error = %q, want the link and its path", err)
		}
		if data, err := os.ReadFile(external); err != nil || string(data) != "fuera\n" {
			t.Errorf("external file = (%q, %v), want it untouched", data, err)
		}
	})

	t.Run("bundle directory", func(t *testing.T) {
		snapshotDir := t.TempDir()
		outside := t.TempDir()
		writeBundle(t, outside, "dev", `[]`)
		symlinkOrSkip(t, filepath.Join(outside, "dev"), filepath.Join(snapshotDir, "dev"))

		if _, err := Load(snapshotDir); err == nil {
			t.Fatal("Load() error = nil, want a symbolic link rejection")
		}
	})
}

func TestLoadRejectsInvalidSnapshots(t *testing.T) {
	t.Run("not a directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "snapshot")
		writeSnapshotFile(t, filepath.Dir(path), "snapshot", "not a directory\n")
		if _, err := Load(path); err == nil {
			t.Fatal("Load() error = nil, want a non-directory error")
		}
	})

	t.Run("missing descriptor", func(t *testing.T) {
		root := t.TempDir()
		writeSnapshotFile(t, root, "dev/README.md", "# dev\n")
		_, err := Load(root)
		if err == nil || !strings.Contains(err.Error(), "bundle.json") {
			t.Errorf("Load() error = %v, want a missing descriptor error", err)
		}
	})

	t.Run("bad json", func(t *testing.T) {
		root := t.TempDir()
		writeSnapshotFile(t, root, "dev/bundle.json", "{")
		if _, err := Load(root); err == nil {
			t.Fatal("Load() error = nil, want a JSON error")
		}
	})

	t.Run("id mismatch", func(t *testing.T) {
		root := t.TempDir()
		writeSnapshotFile(t, root, "dev/bundle.json", `{"id":"qa","version":"0.6.0","requires":[]}`)
		if _, err := Load(root); err == nil {
			t.Fatal("Load() error = nil, want an id mismatch error")
		}
	})

	t.Run("stray root file", func(t *testing.T) {
		root := t.TempDir()
		writeBundle(t, root, "dev", `[]`)
		writeSnapshotFile(t, root, "README.md", "stray\n")
		if _, err := Load(root); err == nil {
			t.Fatal("Load() error = nil, want a stray root file error")
		}
	})

	t.Run("invalid source directory", func(t *testing.T) {
		root := t.TempDir()
		writeSnapshotFile(t, root, "dev/bundle.json",
			`{"id":"dev","version":"0.6.0","requires":[],"roles":"../roles","commands":"commands"}`)
		_, err := Load(root)
		if err == nil || !strings.Contains(err.Error(), "invalid source directory") {
			t.Errorf("Load() error = %v, want an invalid source directory error", err)
		}
	})
}

func TestLoadInvalidSnapshotDoesNotWrite(t *testing.T) {
	project := t.TempDir()
	writeSnapshotFile(t, project, "app.txt", "sin cambios\n")

	snapshotDir := t.TempDir()
	writeSnapshotFile(t, snapshotDir, "dev/README.md", "# dev\n")

	if _, err := Load(snapshotDir); err == nil {
		t.Fatal("Load() error = nil, want an invalid snapshot error")
	}

	data, err := os.ReadFile(filepath.Join(project, "app.txt"))
	if err != nil || string(data) != "sin cambios\n" {
		t.Errorf("project file = (%q, %v), want it untouched", data, err)
	}
	entries, err := os.ReadDir(project)
	if err != nil {
		t.Fatalf("ReadDir(project) error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "app.txt" {
		t.Errorf("project entries = %v, want only app.txt", entries)
	}
	if _, err := os.Stat(filepath.Join(project, ".atlas")); !os.IsNotExist(err) {
		t.Errorf("the invalid snapshot created .atlas: %v", err)
	}

	snapshotEntries, err := os.ReadDir(snapshotDir)
	if err != nil {
		t.Fatalf("ReadDir(snapshot) error = %v", err)
	}
	if len(snapshotEntries) != 1 || snapshotEntries[0].Name() != "dev" {
		t.Errorf("snapshot entries = %v, want only dev", snapshotEntries)
	}
}

func TestLoadEmptySnapshot(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("Load() error = nil, want a snapshot with no bundles error")
	}
}

func TestLoadedSnapshotIsReadable(t *testing.T) {
	root := t.TempDir()
	writeBundle(t, root, "dev", `[]`)

	snapshot, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	file, ok := snapshot.Read("dev", "dev.md")
	if !ok {
		t.Fatal("dev/dev.md is missing from the loaded snapshot")
	}
	if want := content.Checksum([]byte("dev\n")); file.SHA256 != want {
		t.Errorf("sha256 = %q, want %q", file.SHA256, want)
	}
}
