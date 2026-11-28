// Package classify compares the files recorded in a manifest with the project
// on disk and decides which layer governs a path. It is read-only: the caller
// supplies a Reader and the package never touches the filesystem itself.
package classify

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"

	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/state"
)

// Status classifies a file on disk against the hash the manifest expects.
type Status string

const (
	// StatusIntact means the disk content matches the expected content once
	// line endings are normalized to LF.
	StatusIntact Status = "intact"
	// StatusModified means the disk content differs from the expected content.
	StatusModified Status = "modified"
	// StatusAbsent means the manifest records the file but it is not on disk.
	StatusAbsent Status = "absent"
)

// Reader reads a project-relative file. A nonexistent path must return an
// error that matches fs.ErrNotExist; any other error aborts the classification.
type Reader interface {
	Read(path string) ([]byte, error)
}

// ReaderFunc adapts a function to Reader.
type ReaderFunc func(path string) ([]byte, error)

// Read implements Reader.
func (f ReaderFunc) Read(path string) ([]byte, error) {
	return f(path)
}

// File is the classification of one manifest entry.
type File struct {
	Path string
	state.ManifestFile
	// Status is the comparison between the manifest hash and the disk.
	Status Status
	// ActualSHA256 is the hash of the disk content; empty when the file is
	// absent.
	ActualSHA256 string
}

// Divergent reports whether the file on disk differs from what the manifest
// expects. An absent file is a different category, not a divergence.
func (f File) Divergent() bool {
	return f.Status == StatusModified
}

// Report is the classification of every file in a manifest, sorted by path.
type Report struct {
	files  []File
	byPath map[string]File
}

// Files returns the classified files sorted by path.
func (r *Report) Files() []File {
	return append([]File(nil), r.files...)
}

// File returns the classification of path.
func (r *Report) File(path string) (File, bool) {
	file, ok := r.byPath[path]
	return file, ok
}

// Divergent returns the files whose disk content differs from the manifest,
// sorted by path.
func (r *Report) Divergent() []File {
	var divergent []File
	for _, file := range r.files {
		if file.Divergent() {
			divergent = append(divergent, file)
		}
	}
	return divergent
}

// Classify compares every manifest file with the content read by reader. The
// hash comes from content.Checksum, which normalizes line endings to LF, so a
// CRLF working tree is not a modification.
func Classify(manifest *state.Manifest, reader Reader) (*Report, error) {
	if manifest == nil {
		return nil, errors.New("classify: nil manifest")
	}
	if reader == nil {
		return nil, errors.New("classify: nil reader")
	}

	paths := make([]string, 0, len(manifest.Files))
	for path := range manifest.Files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	report := &Report{
		files:  make([]File, 0, len(paths)),
		byPath: make(map[string]File, len(paths)),
	}
	for _, path := range paths {
		file, err := classifyFile(path, manifest.Files[path], reader)
		if err != nil {
			return nil, err
		}
		report.files = append(report.files, file)
		report.byPath[path] = file
	}
	return report, nil
}

func classifyFile(path string, entry state.ManifestFile, reader Reader) (File, error) {
	data, err := reader.Read(path)
	if errors.Is(err, fs.ErrNotExist) {
		return File{Path: path, ManifestFile: entry, Status: StatusAbsent}, nil
	}
	if err != nil {
		return File{}, fmt.Errorf("classify %s: %w", path, err)
	}

	actual := content.Checksum(data)
	status := StatusModified
	if actual == entry.SHA256 {
		status = StatusIntact
	}
	return File{
		Path:         path,
		ManifestFile: entry,
		Status:       status,
		ActualSHA256: actual,
	}, nil
}
