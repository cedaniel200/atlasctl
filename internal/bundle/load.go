package bundle

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/fsutil"
)

// Load returns the embedded snapshot when dir is empty, or the snapshot at dir.
// Both sources get the same descriptor validation and path safety; a disk
// snapshot rejects symbolic links anywhere in its tree.
func Load(dir string) (*content.Snapshot, error) {
	if dir == "" {
		snapshot, err := content.Embedded()
		if err != nil {
			return nil, fmt.Errorf("bundle: embedded snapshot: %w", err)
		}
		if err := validateSources(snapshot); err != nil {
			return nil, err
		}
		return snapshot, nil
	}

	info, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("bundle: snapshot %q: %w", dir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("bundle: snapshot %q is a symbolic link", dir)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("bundle: snapshot %q is not a directory", dir)
	}
	if err := rejectSymlinks(dir); err != nil {
		return nil, err
	}

	snapshot, err := content.Load(os.DirFS(dir))
	if err != nil {
		return nil, fmt.Errorf("bundle: load snapshot %q: %w", dir, err)
	}
	if err := validateSources(snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func rejectSymlinks(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			relative = path
		}
		return fmt.Errorf("bundle: snapshot contains a symbolic link at %q", filepath.ToSlash(relative))
	})
}

func validateSources(snapshot *content.Snapshot) error {
	for _, entry := range snapshot.Bundles() {
		for _, directory := range []string{entry.Roles, entry.Commands} {
			if directory == "" {
				continue
			}
			cleaned := strings.TrimSuffix(strings.TrimSpace(directory), "/")
			if err := fsutil.ValidateRelative(cleaned); err != nil {
				return fmt.Errorf("bundle: bundle %q declares an invalid source directory %q: %w", entry.ID, directory, err)
			}
		}
	}
	return nil
}
