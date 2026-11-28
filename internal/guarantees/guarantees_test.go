package guarantees

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/classify"
	"github.com/cedaniel200/atlasctl/internal/commands"
	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/i18n"
	"github.com/cedaniel200/atlasctl/internal/plan"
	"github.com/cedaniel200/atlasctl/internal/setup"
	"github.com/cedaniel200/atlasctl/internal/state"
)

// TestBundleContractEmbedded checks the snapshot compiled into the binary.
func TestBundleContractEmbedded(t *testing.T) {
	snapshot, err := content.Embedded()
	if err != nil {
		t.Fatalf("bundle contract broken: the embedded snapshot cannot be loaded: %v", err)
	}
	assertBundleContract(t, snapshot)
}

// TestBundleContractDiskSnapshot checks the same contract for a snapshot loaded
// from disk, as used with --snapshot.
func TestBundleContractDiskSnapshot(t *testing.T) {
	root := t.TempDir()
	writeGuaranteeSnapshot(t, root)
	snapshot, err := content.Load(os.DirFS(root))
	if err != nil {
		t.Fatalf("bundle contract broken: the disk snapshot cannot be loaded: %v", err)
	}
	assertBundleContract(t, snapshot)
}

func assertBundleContract(t *testing.T, snapshot *content.Snapshot) {
	t.Helper()
	bundles := snapshot.Bundles()
	if len(bundles) == 0 {
		t.Fatal("bundle contract broken: the snapshot declares no bundles")
	}
	known := make(map[string]bool, len(bundles))
	for _, bundle := range bundles {
		if known[bundle.ID] {
			t.Errorf("bundle contract broken: bundle id %q is declared twice", bundle.ID)
		}
		known[bundle.ID] = true
	}
	for _, bundle := range bundles {
		if bundle.ID == "" || bundle.Version == "" {
			t.Errorf("bundle contract broken: bundle %q declares no id or version", bundle.ID)
		}
		if bundle.Requires == nil {
			t.Errorf("bundle contract broken: bundle %q declares no requires list", bundle.ID)
		}
		for _, required := range bundle.Requires {
			if !known[required] {
				t.Errorf("bundle contract broken: bundle %q requires unknown bundle %q", bundle.ID, required)
			}
		}
		for label, directory := range map[string]string{"roles": bundle.Roles, "commands": bundle.Commands} {
			directory = strings.TrimSuffix(strings.TrimSpace(directory), "/")
			if directory == "" {
				continue
			}
			found := false
			for _, file := range snapshot.Files() {
				if file.Bundle == bundle.ID && strings.HasPrefix(file.Path, directory+"/") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("bundle contract broken: bundle %q declares %s directory %q with no files", bundle.ID, label, directory)
			}
		}
	}

	planned, err := plan.Build(plan.Request{
		Snapshot: snapshot,
		Manifest: &state.Manifest{SchemaVersion: state.SchemaVersion},
		Reader: classify.ReaderFunc(func(string) ([]byte, error) {
			return nil, fs.ErrNotExist
		}),
	})
	if err != nil {
		t.Fatalf("bundle contract broken: the snapshot cannot be planned: %v", err)
	}

	sourceOnly := sourceOnlyFiles(snapshot)
	for _, file := range snapshot.Files() {
		if file.Bundle == "" || file.Path == "" || file.SHA256 == "" {
			t.Errorf("bundle contract broken: snapshot file %q declares no bundle, path or checksum", file.Ref())
		}
		if sourceOnly[file.Ref()] {
			if _, published := planned.Manifest.Files[file.Path]; published {
				t.Errorf("bundle contract broken: source-only %q is published as a project file", file.Ref())
			}
			continue
		}
		entry, ok := planned.Manifest.Files[file.Path]
		if !ok {
			t.Errorf("bundle contract broken: %q declares no bundle, layer or origin in the planned manifest", file.Ref())
			continue
		}
		if entry.Bundle != file.Bundle {
			t.Errorf("bundle contract broken: %q declares bundle %q in the plan, want %q", file.Ref(), entry.Bundle, file.Bundle)
		}
		if entry.Layer != state.LayerBase {
			t.Errorf("bundle contract broken: %q declares layer %q, want base", file.Ref(), entry.Layer)
		}
		if entry.Origin == "" {
			t.Errorf("bundle contract broken: %q declares no origin", file.Ref())
		}
		if entry.SHA256 != file.SHA256 {
			t.Errorf("bundle contract broken: %q declares a different checksum", file.Ref())
		}
	}
	for path, entry := range planned.Manifest.Files {
		if path == "bundle.json" {
			t.Errorf("bundle contract broken: bundle.json is published as a project file")
		}
		if entry.Bundle == "" || entry.Layer == "" || entry.Origin == "" {
			t.Errorf("bundle contract broken: planned entry %q has bundle=%q layer=%q origin=%q", path, entry.Bundle, entry.Layer, entry.Origin)
		}
	}
}

func sourceOnlyFiles(snapshot *content.Snapshot) map[string]bool {
	result := map[string]bool{}
	for _, bundle := range snapshot.Bundles() {
		for _, directory := range []string{bundle.Roles, bundle.Commands} {
			directory = strings.TrimSuffix(strings.TrimSpace(directory), "/")
			if directory == "" {
				continue
			}
			prefix := directory + "/"
			for _, file := range snapshot.Files() {
				if file.Bundle == bundle.ID && strings.HasPrefix(file.Path, prefix) {
					result[file.Ref()] = true
				}
			}
		}
	}
	return result
}

func TestBundleContractRejectsMalformedDescriptor(t *testing.T) {
	cases := map[string]string{
		"missing id":      `{"version":"0.6.0"}`,
		"missing version": `{"id":"dev"}`,
		"id mismatch":     `{"id":"other","version":"0.6.0"}`,
		"invalid json":    `{`,
	}
	for name, descriptor := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "dev/bundle.json", descriptor)
			if _, err := content.Load(os.DirFS(root)); err == nil {
				t.Fatalf("bundle contract broken: %s was accepted", name)
			}
		})
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	snapshot := t.TempDir()
	writeGuaranteeSnapshot(t, snapshot)
	project := t.TempDir()
	createRequest := commands.Request{
		Project:     project,
		Answers:     setup.Answers{Host: "opencode", Docs: "docs", NoDecisions: true, Bundles: []string{"dev"}},
		Snapshot:    snapshot,
		Getenv:      func(string) string { return "" },
		Catalog:     i18n.New(i18n.English),
		CLILanguage: "en",
		LangSet:     true,
	}
	if _, err := commands.Create(createRequest); err != nil {
		t.Fatalf("idempotence setup failed: Create() error = %v", err)
	}
	writeFile(t, snapshot, "dev/AGENTS.md", "# Contract v2\n")
	if _, err := commands.Apply(commands.ApplyRequest{Project: project, Snapshot: snapshot}); err != nil {
		t.Fatalf("idempotence setup failed: first Apply() error = %v", err)
	}
	before := snapshotTree(t, project)
	if _, err := commands.Apply(commands.ApplyRequest{Project: project, Snapshot: snapshot}); err != nil {
		t.Fatalf("idempotence broken: second Apply() error = %v", err)
	}
	after := snapshotTree(t, project)

	for path, want := range before {
		got, ok := after[path]
		if !ok {
			t.Fatalf("idempotence broken: %q disappeared on the second apply", path)
		}
		if !reflect.DeepEqual(got, want) {
			if path == ".atlas/manifest.json" {
				t.Fatalf("idempotence broken: the manifest changed on the second apply")
			}
			t.Fatalf("idempotence broken: %q changed on the second apply", path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Fatalf("idempotence broken: %q appeared on the second apply", path)
		}
	}
}

type treeEntry struct {
	Directory bool
	Mode      fs.FileMode
	Data      []byte
}

func snapshotTree(t *testing.T, root string) map[string]treeEntry {
	t.Helper()
	tree := map[string]treeEntry{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		item := treeEntry{Directory: entry.IsDir(), Mode: info.Mode().Perm()}
		if !entry.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			item.Data = data
		}
		tree[relative] = item
		return nil
	})
	if err != nil {
		t.Fatalf("could not snapshot the project tree: %v", err)
	}
	return tree
}

func TestNoNetworkDependencies(t *testing.T) {
	// Static guarantee: the binary has no network client because no network
	// package is linked. It does not sandbox sockets at runtime.
	cmd := exec.Command("go", "list", "-deps", "./...")
	cmd.Dir = filepath.Join("..", "..")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zero-network guarantee could not enumerate dependencies: %v\n%s", err, output)
	}
	var found []string
	for _, line := range strings.Split(string(output), "\n") {
		pkg := strings.TrimSpace(line)
		if pkg == "net" || strings.HasPrefix(pkg, "net/") ||
			pkg == "crypto/tls" || strings.HasPrefix(pkg, "golang.org/x/net") {
			found = append(found, pkg)
		}
	}
	if len(found) > 0 {
		sort.Strings(found)
		t.Fatalf("zero-network guarantee broken: network packages are linked: %s", strings.Join(found, ", "))
	}
}

func TestProjectRootFootprint(t *testing.T) {
	snapshot := t.TempDir()
	writeGuaranteeSnapshot(t, snapshot)
	project := t.TempDir()
	createRequest := commands.Request{
		Project:     project,
		Answers:     setup.Answers{Host: "opencode", Docs: "docs", NoDecisions: true, Bundles: []string{"dev"}},
		Snapshot:    snapshot,
		Getenv:      func(string) string { return "" },
		Catalog:     i18n.New(i18n.English),
		CLILanguage: "en",
		LangSet:     true,
	}
	if _, err := commands.Create(createRequest); err != nil {
		t.Fatalf("footprint setup failed: Create() error = %v", err)
	}
	writeFile(t, project, "docs/guide.md", "# Guide\n")
	writeFile(t, project, "openspec/config.yaml", "schema: v1\n")
	if _, err := commands.Apply(commands.ApplyRequest{Project: project, Snapshot: snapshot}); err != nil {
		t.Fatalf("footprint setup failed: Apply() error = %v", err)
	}

	entries, err := os.ReadDir(project)
	if err != nil {
		t.Fatalf("could not read the project root: %v", err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	sort.Strings(got)
	want := []string{".agents", ".atlas", ".opencode", "AGENTS.md", "docs", "openspec"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("minimal footprint broken: root = %v, want %v (unexpected %v, missing %v)",
			got, want, difference(got, want), difference(want, got))
	}
}

func difference(left, right []string) []string {
	set := make(map[string]bool, len(right))
	for _, value := range right {
		set[value] = true
	}
	var result []string
	for _, value := range left {
		if !set[value] {
			result = append(result, value)
		}
	}
	return result
}

func writeGuaranteeSnapshot(t *testing.T, root string) {
	t.Helper()
	writeFile(t, root, "dev/bundle.json", `{"id":"dev","version":"0.6.0","requires":[],"roles":"roles","commands":"commands"}`)
	writeFile(t, root, "dev/AGENTS.md", "# Contract\n")
	writeFile(t, root, "dev/.agents/skills/demo/SKILL.md", "---\nname: demo\n---\n\n# Demo\n")
	writeFile(t, root, "dev/roles/analyst.md", "# analyst\n\n> Guide.\n")
	writeFile(t, root, "dev/commands/apply.md", "---\ndescription: Apply\n---\n\nBody.\n")
}

func writeFile(t *testing.T, root, relative, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", relative, err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", relative, err)
	}
}
