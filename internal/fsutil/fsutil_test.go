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

func TestCheckNoSymlinksAcceptsRealAndMissingPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := CheckNoSymlinks(root, "docs/guide.md"); err != nil {
		t.Errorf("CheckNoSymlinks(real path) error = %v", err)
	}
	if err := CheckNoSymlinks(root, "missing/guide.md"); err != nil {
		t.Errorf("CheckNoSymlinks(missing path) error = %v", err)
	}
}

func TestCheckNoSymlinksRejectsSymlinkAtDestination(t *testing.T) {
	root, target := symlinkedFile(t, "AGENTS.md")

	err := CheckNoSymlinks(root, "AGENTS.md")
	if err == nil {
		t.Fatal("CheckNoSymlinks() error = nil, want a rejection")
	}
	if !strings.Contains(err.Error(), "AGENTS.md") || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("error = %q, want the path and the reason", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "fuera\n" {
		t.Errorf("target = (%q, %v), want it untouched", data, err)
	}
}

func TestCheckNoSymlinksRejectsSymlinkInIntermediateDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "file.md"), []byte("fuera\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	mustSymlink(t, outside, filepath.Join(root, "docs"))

	err := CheckNoSymlinks(root, "docs/file.md")
	if err == nil {
		t.Fatal("CheckNoSymlinks() error = nil, want a rejection")
	}
	if !strings.Contains(err.Error(), "docs") || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("error = %q, want the component and the reason", err)
	}
}

func TestWriteProjectFileRejectsSymlinkDestination(t *testing.T) {
	root, target := symlinkedFile(t, "AGENTS.md")

	if err := WriteProjectFile(root, "AGENTS.md", []byte("nuevo\n")); err == nil {
		t.Fatal("WriteProjectFile() error = nil, want a rejection")
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "fuera\n" {
		t.Errorf("target = (%q, %v), want it untouched", data, err)
	}
}

func TestRemoveProjectFileRejectsSymlinkDestination(t *testing.T) {
	root, target := symlinkedFile(t, "AGENTS.md")

	if err := RemoveProjectFile(root, "AGENTS.md"); err == nil {
		t.Fatal("RemoveProjectFile() error = nil, want a rejection")
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("external target was removed: %v", err)
	}
}

func TestRemoveProjectFileRemovesAndIgnoresAbsence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "old.md")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := RemoveProjectFile(root, "old.md"); err != nil {
		t.Fatalf("RemoveProjectFile() error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("file still exists: %v", err)
	}
	if err := RemoveProjectFile(root, "old.md"); err != nil {
		t.Errorf("RemoveProjectFile(absent) error = %v, want a no-op", err)
	}
}

func symlinkedFile(t *testing.T, name string) (root, target string) {
	t.Helper()
	root = t.TempDir()
	outside := t.TempDir()
	target = filepath.Join(outside, "target.md")
	if err := os.WriteFile(target, []byte("fuera\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	mustSymlink(t, target, filepath.Join(root, name))
	return root, target
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
}

func TestRemoveProjectTreeRemovesDirectoryWithoutFollowingSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "legacy", "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "legacy", "a.md"), []byte("a"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "legacy", "sub", "b.md"), []byte("b"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	outside := t.TempDir()
	target := filepath.Join(outside, "target.md")
	if err := os.WriteFile(target, []byte("fuera\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	mustSymlink(t, outside, filepath.Join(root, "legacy", "link"))

	removed, err := RemoveProjectTree(root, "legacy")
	if err != nil {
		t.Fatalf("RemoveProjectTree() error = %v", err)
	}
	if !removed {
		t.Error("RemoveProjectTree() = false, want true")
	}
	if _, err := os.Stat(filepath.Join(root, "legacy")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("legacy directory still exists: %v", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "fuera\n" {
		t.Errorf("external target = (%q, %v), want it untouched", data, err)
	}
}

func TestRemoveProjectTreeReportsAbsence(t *testing.T) {
	removed, err := RemoveProjectTree(t.TempDir(), "missing")
	if err != nil {
		t.Fatalf("RemoveProjectTree() error = %v", err)
	}
	if removed {
		t.Error("RemoveProjectTree() = true for an absent path")
	}
}

func TestRemoveProjectTreeRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "legacy"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "legacy", "file.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	mustSymlink(t, filepath.Join(outside, "legacy"), filepath.Join(root, "legacy"))
	if _, err := RemoveProjectTree(root, "legacy"); err == nil {
		t.Fatal("RemoveProjectTree() accepted a symlink at the destination")
	}

	root = t.TempDir()
	mustSymlink(t, outside, filepath.Join(root, "parent"))
	if _, err := RemoveProjectTree(root, "parent/legacy"); err == nil {
		t.Fatal("RemoveProjectTree() accepted a symlink in an intermediate directory")
	}
	if _, err := os.Stat(filepath.Join(outside, "legacy", "file.md")); err != nil {
		t.Errorf("external content was removed: %v", err)
	}
}

func TestWriteProjectFileModeSetsExecutable(t *testing.T) {
	root := t.TempDir()
	if err := WriteProjectFileMode(root, "hooks/pre-commit", []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteProjectFileMode() error = %v", err)
	}

	info, err := os.Stat(filepath.Join(root, "hooks", "pre-commit"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o755 {
		t.Errorf("mode = %o, want 0755", mode)
	}
	assertNoTemporaries(t, filepath.Join(root, "hooks"))
}
