// Package fsutil handles project paths and file writes. Every project path is
// relative, uses forward slashes and must stay under the project root; a path
// that escapes it is an error, never a file outside the project.
package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ValidateRelative rejects empty, absolute, unclean or escaping paths. A
// project path is always a clean relative slash path.
func ValidateRelative(rel string) error {
	switch {
	case rel == "":
		return errors.New("empty path")
	case strings.ContainsRune(rel, '\\'):
		return fmt.Errorf("path %q contains a backslash", rel)
	case strings.ContainsRune(rel, '\x00'):
		return fmt.Errorf("path %q contains a NUL byte", rel)
	case path.IsAbs(rel):
		return fmt.Errorf("path %q is absolute", rel)
	case hasDriveLetter(rel):
		return fmt.Errorf("path %q starts with a drive letter", rel)
	}

	clean := path.Clean(rel)
	if clean != rel {
		return fmt.Errorf("path %q is not clean", rel)
	}
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("path %q escapes the project", rel)
	}
	return nil
}

func hasDriveLetter(rel string) bool {
	return len(rel) >= 2 && rel[1] == ':'
}

// SafeJoin resolves a validated project path under root.
func SafeJoin(root, rel string) (string, error) {
	if root == "" {
		return "", errors.New("empty project root")
	}
	if err := ValidateRelative(rel); err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(rel)), nil
}

// RootReader reads project files under Root. It satisfies the read contract
// used by the classifier: a missing file surfaces as fs.ErrNotExist and an
// escaping path is a policy error.
type RootReader struct {
	Root string
}

// Read reads a project-relative file.
func (r RootReader) Read(rel string) ([]byte, error) {
	absolute, err := SafeJoin(r.Root, rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(absolute)
}

// WriteProjectFile writes a project-relative file atomically.
func WriteProjectFile(root, rel string, data []byte) error {
	absolute, err := SafeJoin(root, rel)
	if err != nil {
		return err
	}
	return WriteFile(absolute, data)
}
