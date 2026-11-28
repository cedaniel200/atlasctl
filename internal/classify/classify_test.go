package classify

import (
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/state"
)

func manifestWith(files map[string]state.ManifestFile) *state.Manifest {
	return &state.Manifest{
		SchemaVersion: state.SchemaVersion,
		Files:         files,
	}
}

func baseEntry(hash string) state.ManifestFile {
	return state.ManifestFile{
		Bundle: "dev",
		Origin: "core",
		Layer:  state.LayerBase,
		SHA256: hash,
	}
}

func readerWithFiles(files map[string][]byte) ReaderFunc {
	return func(path string) ([]byte, error) {
		data, ok := files[path]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return data, nil
	}
}

func TestClassifyDoesNotTreatLineEndingChangesAsModifications(t *testing.T) {
	lf := []byte("primera linea\nsegunda linea\n")
	manifest := manifestWith(map[string]state.ManifestFile{
		"AGENTS.md": baseEntry(content.Checksum(lf)),
	})
	reader := readerWithFiles(map[string][]byte{
		"AGENTS.md": []byte("primera linea\r\nsegunda linea\r\n"),
	})

	report, err := Classify(manifest, reader)
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	file, ok := report.File("AGENTS.md")
	if !ok {
		t.Fatal("AGENTS.md not found in the report")
	}
	if file.Status != StatusIntact {
		t.Errorf("status = %q, want %q", file.Status, StatusIntact)
	}
	if file.Divergent() {
		t.Error("a line-ending change must not be reported as divergent")
	}
	if got, want := file.ActualSHA256, content.Checksum(lf); got != want {
		t.Errorf("actual sha256 = %q, want %q", got, want)
	}
}

func TestClassifyReportsModifiedBaseAsDivergent(t *testing.T) {
	expected := []byte("contenido del arnes\n")
	manifest := manifestWith(map[string]state.ManifestFile{
		"AGENTS.md": baseEntry(content.Checksum(expected)),
	})
	reader := readerWithFiles(map[string][]byte{
		"AGENTS.md": []byte("contenido del adoptante\n"),
	})

	report, err := Classify(manifest, reader)
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	file, ok := report.File("AGENTS.md")
	if !ok {
		t.Fatal("AGENTS.md not found in the report")
	}
	if file.Status != StatusModified {
		t.Errorf("status = %q, want %q", file.Status, StatusModified)
	}
	if file.Layer != state.LayerBase {
		t.Errorf("layer = %q, want %q", file.Layer, state.LayerBase)
	}
	if !file.Divergent() {
		t.Error("a modified base file must be reported as divergent")
	}
	divergent := report.Divergent()
	if len(divergent) != 1 || divergent[0].Path != "AGENTS.md" {
		t.Errorf("Divergent() = %v, want the modified AGENTS.md", divergent)
	}
}

func TestClassifyReportsEditedGeneratedFile(t *testing.T) {
	manifest := manifestWith(map[string]state.ManifestFile{
		".atlas/hooks/pre-commit": {
			Bundle: "dev",
			Origin: "core",
			Layer:  state.LayerGenerated,
			SHA256: content.Checksum([]byte("despachador generado\n")),
		},
	})
	reader := readerWithFiles(map[string][]byte{
		".atlas/hooks/pre-commit": []byte("despachador editado a mano\n"),
	})

	report, err := Classify(manifest, reader)
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	file, ok := report.File(".atlas/hooks/pre-commit")
	if !ok {
		t.Fatal("generated file not found in the report")
	}
	if file.Layer != state.LayerGenerated {
		t.Errorf("layer = %q, want %q", file.Layer, state.LayerGenerated)
	}
	if file.Status != StatusModified {
		t.Errorf("status = %q, want %q", file.Status, StatusModified)
	}
	if !file.Divergent() {
		t.Error("an edited generated file must be reported as divergent")
	}
}

func TestClassifyReportsAbsentFile(t *testing.T) {
	manifest := manifestWith(map[string]state.ManifestFile{
		"docs/project-context.md": {
			Bundle: "dev",
			Origin: "core",
			Layer:  state.LayerOwned,
			SHA256: content.Checksum([]byte("inicial\n")),
		},
	})

	report, err := Classify(manifest, readerWithFiles(nil))
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	file, ok := report.File("docs/project-context.md")
	if !ok {
		t.Fatal("missing file not reported")
	}
	if file.Status != StatusAbsent {
		t.Errorf("status = %q, want %q", file.Status, StatusAbsent)
	}
	if file.Divergent() {
		t.Error("an absent file is not a content divergence")
	}
	if file.ActualSHA256 != "" {
		t.Errorf("actual sha256 = %q, want empty", file.ActualSHA256)
	}
}

func TestClassifyReturnsReaderErrors(t *testing.T) {
	manifest := manifestWith(map[string]state.ManifestFile{
		"AGENTS.md": baseEntry(content.Checksum([]byte("x\n"))),
	})
	reader := ReaderFunc(func(string) ([]byte, error) {
		return nil, errors.New("disco no disponible")
	})

	_, err := Classify(manifest, reader)
	if err == nil {
		t.Fatal("Classify() error = nil, want the reader error")
	}
	if !strings.Contains(err.Error(), "AGENTS.md") {
		t.Errorf("error = %q, want it to name the failing path", err)
	}
}

func TestClassifyIsDeterministicRegardlessOfInputOrder(t *testing.T) {
	paths := []string{"zeta.md", "alpha.md", "middle.md"}
	reader := readerWithFiles(map[string][]byte{
		"zeta.md":   []byte("zeta.md\n"),
		"alpha.md":  []byte("alpha.md\n"),
		"middle.md": []byte("middle.md\n"),
	})

	first, err := Classify(manifestInOrder(paths), reader)
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	second, err := Classify(manifestInOrder([]string{"middle.md", "zeta.md", "alpha.md"}), reader)
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	if !reflect.DeepEqual(first.Files(), second.Files()) {
		t.Errorf("reports differ by input order:\n first = %v\nsecond = %v", first.Files(), second.Files())
	}
	files := first.Files()
	for index := 1; index < len(files); index++ {
		if files[index-1].Path >= files[index].Path {
			t.Fatalf("files are not sorted by path: %q before %q", files[index-1].Path, files[index].Path)
		}
	}
}

func manifestInOrder(paths []string) *state.Manifest {
	files := make(map[string]state.ManifestFile, len(paths))
	for _, path := range paths {
		files[path] = baseEntry(content.Checksum([]byte(path + "\n")))
	}
	return manifestWith(files)
}
