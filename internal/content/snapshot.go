// Package content exposes the content snapshot embedded in the binary: one
// directory per bundle, a bundle.json descriptor and the files the engine can
// materialize in a project.
package content

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
)

const descriptorName = "bundle.json"

//go:embed bundles
var embedded embed.FS

var (
	embeddedOnce   sync.Once
	embeddedResult *Snapshot
	embeddedErr    error
)

// Bundle describes a content bundle and the bundles it depends on. Roles and
// Commands are bundle-relative directories with source material: host adapters
// are generated from them, so they are never published as project files.
type Bundle struct {
	ID       string   `json:"id"`
	Version  string   `json:"version"`
	Requires []string `json:"requires"`
	Roles    string   `json:"roles,omitempty"`
	Commands string   `json:"commands,omitempty"`
}

// File is a file published by a bundle. Path is relative to the bundle root and
// always uses forward slashes.
type File struct {
	Bundle string
	Path   string
	Data   []byte
	SHA256 string
}

// Ref returns the snapshot-wide reference of the file, "<bundle>/<path>".
func (f File) Ref() string {
	return f.Bundle + "/" + f.Path
}

// Snapshot is an immutable, indexed view of the embedded or on-disk bundles.
type Snapshot struct {
	bundles []Bundle
	files   []File
	byRef   map[string]File
}

// Embedded returns the snapshot compiled into the binary. The result is shared
// and must be treated as read-only.
func Embedded() (*Snapshot, error) {
	embeddedOnce.Do(func() {
		root, err := fs.Sub(embedded, "bundles")
		if err != nil {
			embeddedErr = fmt.Errorf("open embedded snapshot: %w", err)
			return
		}
		embeddedResult, embeddedErr = Load(root)
	})
	return embeddedResult, embeddedErr
}

// Load indexes every bundle under the root of fsys.
func Load(fsys fs.FS) (*Snapshot, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read snapshot root: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("snapshot has no bundles")
	}

	snapshot := &Snapshot{byRef: make(map[string]File)}
	for _, entry := range entries {
		if !entry.IsDir() {
			return nil, fmt.Errorf("unexpected file %q at snapshot root", entry.Name())
		}
		bundle, files, err := loadBundle(fsys, entry.Name())
		if err != nil {
			return nil, err
		}
		snapshot.bundles = append(snapshot.bundles, bundle)
		for _, file := range files {
			snapshot.files = append(snapshot.files, file)
			snapshot.byRef[file.Ref()] = file
		}
	}

	sort.Slice(snapshot.bundles, func(i, j int) bool {
		return snapshot.bundles[i].ID < snapshot.bundles[j].ID
	})
	sort.Slice(snapshot.files, func(i, j int) bool {
		return snapshot.files[i].Ref() < snapshot.files[j].Ref()
	})
	return snapshot, nil
}

func loadBundle(fsys fs.FS, dir string) (Bundle, []File, error) {
	raw, err := fs.ReadFile(fsys, path.Join(dir, descriptorName))
	if err != nil {
		return Bundle{}, nil, fmt.Errorf("bundle %q: read %s: %w", dir, descriptorName, err)
	}

	var bundle Bundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return Bundle{}, nil, fmt.Errorf("bundle %q: decode %s: %w", dir, descriptorName, err)
	}
	if bundle.ID == "" {
		return Bundle{}, nil, fmt.Errorf("bundle %q: %s declares no id", dir, descriptorName)
	}
	if bundle.ID != dir {
		return Bundle{}, nil, fmt.Errorf("bundle %q declares id %q", dir, bundle.ID)
	}
	if bundle.Version == "" {
		return Bundle{}, nil, fmt.Errorf("bundle %q: %s declares no version", dir, descriptorName)
	}
	if bundle.Requires == nil {
		bundle.Requires = []string{}
	}

	var files []File
	err = fs.WalkDir(fsys, dir, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, dir+"/")
		if rel == descriptorName {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		files = append(files, File{
			Bundle: bundle.ID,
			Path:   rel,
			Data:   data,
			SHA256: Checksum(data),
		})
		return nil
	})
	if err != nil {
		return Bundle{}, nil, fmt.Errorf("bundle %q: %w", dir, err)
	}
	return bundle, files, nil
}

// Bundles returns the bundles sorted by id.
func (s *Snapshot) Bundles() []Bundle {
	return append([]Bundle(nil), s.bundles...)
}

// Files returns every content file sorted by reference.
func (s *Snapshot) Files() []File {
	return append([]File(nil), s.files...)
}

// Read returns the file published at path by bundle.
func (s *Snapshot) Read(bundle, filePath string) (File, bool) {
	file, ok := s.byRef[bundle+"/"+filePath]
	return file, ok
}

// Checksum returns the lowercase hexadecimal SHA-256 of data with line endings
// normalized to LF, as the manifest records the content the engine expects
// regardless of the working tree's line endings.
func Checksum(data []byte) string {
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	sum := sha256.Sum256(normalized)
	return hex.EncodeToString(sum[:])
}
