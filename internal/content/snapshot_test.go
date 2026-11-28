package content

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedSnapshotExposesDevBundle(t *testing.T) {
	snapshot, err := Embedded()
	if err != nil {
		t.Fatalf("Embedded() error = %v", err)
	}

	bundles := snapshot.Bundles()
	if len(bundles) != 1 {
		t.Fatalf("got %d bundles, want 1", len(bundles))
	}
	dev := bundles[0]
	if dev.ID != "dev" {
		t.Errorf("bundle ID = %q, want %q", dev.ID, "dev")
	}
	if dev.Version != "0.6.0" {
		t.Errorf("bundle version = %q, want %q", dev.Version, "0.6.0")
	}
	if len(dev.Requires) != 0 {
		t.Errorf("bundle requires = %v, want empty", dev.Requires)
	}
}

func TestEmbeddedSnapshotReadsFileWithPathAndHash(t *testing.T) {
	snapshot, err := Embedded()
	if err != nil {
		t.Fatalf("Embedded() error = %v", err)
	}

	file, ok := snapshot.Read("dev", "README.md")
	if !ok {
		t.Fatal("dev/README.md not found in embedded snapshot")
	}
	if got, want := file.Ref(), "dev/README.md"; got != want {
		t.Errorf("file ref = %q, want %q", got, want)
	}
	if len(file.Data) == 0 {
		t.Error("file data is empty")
	}
	if got, want := file.SHA256, Checksum(file.Data); got != want {
		t.Errorf("file sha256 = %q, want %q", got, want)
	}
	if len(file.SHA256) != 64 || strings.Trim(file.SHA256, "0123456789abcdef") != "" {
		t.Errorf("file sha256 = %q, want 64 lowercase hex characters", file.SHA256)
	}
}

func TestChecksumIgnoresLineEndings(t *testing.T) {
	lf := []byte("primera linea\nsegunda linea\n")
	crlf := []byte("primera linea\r\nsegunda linea\r\n")

	if got, want := Checksum(crlf), Checksum(lf); got != want {
		t.Errorf("Checksum(CRLF) = %q, want %q (LF)", got, want)
	}
	if Checksum(lf) == Checksum([]byte("otro contenido\n")) {
		t.Error("different content produced the same checksum")
	}
}

func TestLoadIndexesFilesSortedByReference(t *testing.T) {
	snapshot, err := Load(fstest.MapFS{
		"dev/bundle.json": {Data: []byte(`{"id":"dev","version":"0.6.0","requires":[]}`)},
		"dev/b.md":        {Data: []byte("b\n")},
		"dev/a.md":        {Data: []byte("a\r\n")},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	files := snapshot.Files()
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	if got, want := files[0].Ref(), "dev/a.md"; got != want {
		t.Errorf("first file = %q, want %q", got, want)
	}
	if got, want := files[1].Ref(), "dev/b.md"; got != want {
		t.Errorf("second file = %q, want %q", got, want)
	}
}

func TestLoadRejectsBundleWithoutDescriptor(t *testing.T) {
	_, err := Load(fstest.MapFS{
		"dev/README.md": {Data: []byte("sin descriptor\n")},
	})
	if err == nil {
		t.Fatal("Load() error = nil, want an error for a bundle without bundle.json")
	}
	if !strings.Contains(err.Error(), "bundle.json") {
		t.Errorf("Load() error = %q, want it to mention bundle.json", err)
	}
}

func TestLoadRejectsMismatchedBundleID(t *testing.T) {
	_, err := Load(fstest.MapFS{
		"dev/bundle.json": {Data: []byte(`{"id":"core","version":"0.6.0","requires":[]}`)},
	})
	if err == nil {
		t.Fatal("Load() error = nil, want an error for a mismatched bundle id")
	}
	if !strings.Contains(err.Error(), "core") || !strings.Contains(err.Error(), "dev") {
		t.Errorf("Load() error = %q, want it to name both the declared and directory ids", err)
	}
}

func TestLoadReadsBundleSourceDirectories(t *testing.T) {
	snapshot, err := Load(fstest.MapFS{
		"dev/bundle.json":      {Data: []byte(`{"id":"dev","version":"0.6.0","requires":[],"roles":"roles","commands":"commands"}`)},
		"dev/roles/analyst.md": {Data: []byte("# analyst\n")},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	bundle := snapshot.Bundles()[0]
	if bundle.Roles != "roles" {
		t.Errorf("roles = %q, want %q", bundle.Roles, "roles")
	}
	if bundle.Commands != "commands" {
		t.Errorf("commands = %q, want %q", bundle.Commands, "commands")
	}
}

func TestSnapshotFilterKeepsOnlySelectedBundles(t *testing.T) {
	snapshot, err := Load(fstest.MapFS{
		"dev/bundle.json": {Data: []byte(`{"id":"dev","version":"0.6.0","requires":[]}`)},
		"dev/README.md":   {Data: []byte("dev\n")},
		"qa/bundle.json":  {Data: []byte(`{"id":"qa","version":"0.6.0","requires":[]}`)},
		"qa/README.md":    {Data: []byte("qa\n")},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	filtered, err := snapshot.Filter([]string{"qa"})
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}
	if bundles := filtered.Bundles(); len(bundles) != 1 || bundles[0].ID != "qa" {
		t.Errorf("Bundles() = %v, want only qa", bundles)
	}
	files := filtered.Files()
	if len(files) != 1 || files[0].Ref() != "qa/README.md" {
		t.Errorf("Files() = %v, want only qa/README.md", files)
	}
	if _, ok := filtered.Read("dev", "README.md"); ok {
		t.Error("Filter() kept a file from an inactive bundle")
	}
	if data, ok := filtered.Read("qa", "README.md"); !ok || string(data.Data) != "qa\n" {
		t.Errorf("Read(qa) = (%+v, %t), want the qa file", data, ok)
	}
	if bundles := snapshot.Bundles(); len(bundles) != 2 {
		t.Errorf("Filter() changed the original snapshot: %v", bundles)
	}

	if _, err := snapshot.Filter([]string{"missing"}); err == nil {
		t.Fatal("Filter() error = nil, want an unknown bundle error")
	}
}
