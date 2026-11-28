package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeFileAtomic writes data to a temporary file in the target directory,
// syncs it, and renames it over the target. A reader therefore sees either the
// previous complete file or the new complete file, and an interrupted write
// leaves no partial target behind.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}

	base := filepath.Base(path)
	removeStaleTemporaries(dir, base)

	tmp, err := os.CreateTemp(dir, "."+base+".tmp-")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	name := tmp.Name()
	discard := func() {
		_ = tmp.Close()
		_ = os.Remove(name)
	}

	if _, err := tmp.Write(data); err != nil {
		discard()
		return fmt.Errorf("write temporary file for %s: %w", path, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		discard()
		return fmt.Errorf("set permissions on temporary file for %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		discard()
		return fmt.Errorf("sync temporary file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("close temporary file for %s: %w", path, err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	syncDirectory(dir)
	return nil
}

// removeStaleTemporaries deletes temporary files left by a previous write of
// the same target that was interrupted before renaming.
func removeStaleTemporaries(dir, base string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	prefix := "." + base + ".tmp-"
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
}

// syncDirectory makes the rename durable where the platform supports syncing a
// directory; the failure is not actionable for the caller.
func syncDirectory(dir string) {
	handle, err := os.Open(dir)
	if err != nil {
		return
	}
	defer handle.Close()
	_ = handle.Sync()
}
