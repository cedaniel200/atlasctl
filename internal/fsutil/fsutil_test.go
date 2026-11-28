package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRelativeAcceptsProjectPaths(t *testing.T) {
	paths := []string{
		"AGENTS.md",
		".atlas/state.json",
		"docs/project-context.md",
		".agents/skills/reviewer/SKILL.md",
	}
	for _, path := range paths {
		if err := ValidateRelative(path); err != nil {
			t.Errorf("ValidateRelative(%q) error = %v", path, err)
		}
	}
}

func TestValidateRelativeRejectsPathsThatEscapeTheProject(t *testing.T) {
	paths := []string{
		"",
		".",
		"..",
		"../outside.md",
		"a/../b.md",
		"a//b.md",
		"./a.md",
		"a/",
		"/etc/passwd",
		`a\b.md`,
		"C:/windows",
		"a\x00b.md",
	}
	for _, path := range paths {
		if err := ValidateRelative(path); err == nil {
			t.Errorf("ValidateRelative(%q) error = nil, want a rejection", path)
		}
	}
}

func TestSafeJoinKeepsPathsUnderTheRoot(t *testing.T) {
	root := t.TempDir()

	got, err := SafeJoin(root, "docs/guide.md")
	if err != nil {
		t.Fatalf("SafeJoin() error = %v", err)
	}
	if want := filepath.Join(root, "docs", "guide.md"); got != want {
		t.Errorf("SafeJoin() = %q, want %q", got, want)
	}

	if _, err := SafeJoin(root, "../outside.md"); err == nil {
		t.Error("SafeJoin() accepted a path outside the root")
	}
}

func TestRootReaderReadsFilesAndReportsAbsence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "guide.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("contenido\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	reader := RootReader{Root: root}
	data, err := reader.Read("docs/guide.md")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if string(data) != "contenido\n" {
		t.Errorf("Read() = %q, want the file content", data)
	}

	if _, err := reader.Read("docs/missing.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read(missing) error = %v, want fs.ErrNotExist", err)
	}
	if _, err := reader.Read("../outside.md"); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read(escape) error = %v, want a path policy error", err)
	}
}

func TestWriteProjectFileWritesAtomicallyAndCleansTemporaries(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, ".guide.md.tmp-stale")
	if err := os.WriteFile(stale, []byte("resto"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := WriteProjectFile(root, "guide.md", []byte("primera\n")); err != nil {
		t.Fatalf("WriteProjectFile() error = %v", err)
	}
	if err := WriteProjectFile(root, "guide.md", []byte("segunda\n")); err != nil {
		t.Fatalf("WriteProjectFile() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "guide.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "segunda\n" {
		t.Errorf("content = %q, want the last write", data)
	}
	info, err := os.Stat(filepath.Join(root, "guide.md"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o644 {
		t.Errorf("mode = %o, want 0644", mode)
	}
	assertNoTemporaries(t, root)
}

func TestWriteProjectFileLeavesNoTemporaryWhenRenameFails(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "target", "child"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := WriteProjectFile(root, "target", []byte("x")); err == nil {
		t.Fatal("WriteProjectFile() error = nil, want a failure when the target cannot be replaced")
	}
	assertNoTemporaries(t, root)
}

func TestWriteProjectFileRejectsPathsOutsideTheRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside.md")

	if err := WriteProjectFile(root, "../outside.md", []byte("x")); err == nil {
		t.Fatal("WriteProjectFile() accepted a path outside the root")
	}
	if _, err := os.Stat(outside); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(outside) error = %v, want the file to be absent", err)
	}
}

func assertNoTemporaries(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Errorf("temporary file left behind: %s", entry.Name())
		}
	}
}
