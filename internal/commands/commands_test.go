package commands

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/i18n"
	"github.com/cedaniel200/atlasctl/internal/setup"
	"github.com/cedaniel200/atlasctl/internal/state"
)

func emptyEnv(string) string { return "" }

func readProjectFile(t *testing.T, project, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(project, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", rel, err)
	}
	return string(data)
}

func readState(t *testing.T, project string) *state.State {
	t.Helper()
	loaded, err := state.Load(filepath.Join(project, ".atlas", "state.json"))
	if err != nil {
		t.Fatalf("state.Load() error = %v", err)
	}
	return loaded
}

func embeddedAgents(t *testing.T) string {
	t.Helper()
	snapshot, err := content.Embedded()
	if err != nil {
		t.Fatalf("content.Embedded() error = %v", err)
	}
	file, ok := snapshot.Read("dev", "AGENTS.md")
	if !ok {
		t.Fatal("dev/AGENTS.md is missing from the embedded snapshot")
	}
	return string(file.Data)
}

func request(project string, answers setup.Answers) Request {
	return Request{
		Project:     project,
		Answers:     answers,
		Getenv:      emptyEnv,
		Catalog:     i18n.New(i18n.English),
		CLILanguage: "en",
	}
}

func TestCreateAppliesMinimumAndLeavesPending(t *testing.T) {
	project := t.TempDir()
	result, err := Create(request(project, setup.Answers{}))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	wantPending := []string{setup.PendingHost, setup.PendingLanguage, setup.PendingDocs, setup.PendingDecisions, setup.PendingBundles}
	if !reflect.DeepEqual(result.Pending, wantPending) {
		t.Errorf("pending = %v, want %v", result.Pending, wantPending)
	}
	if result.Complete {
		t.Error("Complete = true, want false")
	}
	if got := readProjectFile(t, project, "AGENTS.md"); got != embeddedAgents(t) {
		t.Error("AGENTS.md does not contain the embedded content")
	}

	written := readState(t, project)
	if written.Setup.Complete || !reflect.DeepEqual(written.Setup.Pending, wantPending) {
		t.Errorf("state setup = %+v, want pending %v", written.Setup, wantPending)
	}
	if written.Host.ID != "" {
		t.Errorf("host = %q, want empty", written.Host.ID)
	}
	if want := []string{"dev"}; !reflect.DeepEqual(written.Bundles, want) {
		t.Errorf("bundles = %v, want %v", written.Bundles, want)
	}
	if written.SpecEngine.ID != "none" {
		t.Errorf("spec engine = %q, want none", written.SpecEngine.ID)
	}
	if written.Language.CLI != "en" || written.Language.User != "en" {
		t.Errorf("language = %+v, want en", written.Language)
	}
	if written.Layout.Docs != "" || written.Layout.Decisions != nil {
		t.Errorf("layout = %+v, want absent routes", written.Layout)
	}
	if written.Hooks.Installed {
		t.Error("hooks installed without a Git repository")
	}
	if written.SchemaVersion != state.SchemaVersion || written.EngineVersion != EngineVersion {
		t.Errorf("state versions = %d/%q", written.SchemaVersion, written.EngineVersion)
	}
}

func TestCreateWithAllFlags(t *testing.T) {
	project := t.TempDir()
	req := request(project, setup.Answers{
		Host:        "opencode",
		Docs:        "docs",
		NoDecisions: true,
		Bundles:     []string{"dev"},
	})
	req.LangSet = true
	req.CLILanguage = "es"

	result, err := Create(req)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !result.Complete || len(result.Pending) != 0 {
		t.Errorf("result = %+v, want complete", result)
	}

	written := readState(t, project)
	if written.Host.ID != "opencode" || written.Host.Source != "flag" {
		t.Errorf("host = %+v, want opencode by flag", written.Host)
	}
	if written.Language.CLI != "es" || written.Language.User != "es" || written.Language.Agent != "en" {
		t.Errorf("language = %+v, want es/es/en", written.Language)
	}
	if written.Layout.Docs != "docs" || written.Layout.Decisions != nil {
		t.Errorf("layout = %+v, want docs and no decisions", written.Layout)
	}
	if want := []string{"dev"}; !reflect.DeepEqual(written.Bundles, want) {
		t.Errorf("bundles = %v, want %v", written.Bundles, want)
	}
	if written.Content.BundleVersion != "0.6.0" {
		t.Errorf("content version = %q, want 0.6.0", written.Content.BundleVersion)
	}
	if !written.Setup.Complete || len(written.Setup.Pending) != 0 {
		t.Errorf("setup = %+v, want complete", written.Setup)
	}
	if got := readProjectFile(t, project, "AGENTS.md"); got != embeddedAgents(t) {
		t.Error("AGENTS.md does not contain the embedded content")
	}
}

func TestCreateResumesFromPending(t *testing.T) {
	project := t.TempDir()

	first, err := Create(request(project, setup.Answers{Host: "opencode"}))
	if err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	wantPending := []string{setup.PendingLanguage, setup.PendingDocs, setup.PendingDecisions, setup.PendingBundles}
	if !reflect.DeepEqual(first.Pending, wantPending) {
		t.Fatalf("first pending = %v, want %v", first.Pending, wantPending)
	}
	agents := readProjectFile(t, project, "AGENTS.md")

	secondRequest := request(project, setup.Answers{
		Docs:        "docs",
		NoDecisions: true,
		Bundles:     []string{"dev"},
	})
	secondRequest.LangSet = true
	secondRequest.CLILanguage = "es"
	second, err := Create(secondRequest)
	if err != nil {
		t.Fatalf("second Create() error = %v", err)
	}
	if !second.Complete || len(second.Pending) != 0 {
		t.Errorf("second result = %+v, want complete", second)
	}
	if readProjectFile(t, project, "AGENTS.md") != agents {
		t.Error("the resumed create changed AGENTS.md")
	}

	written := readState(t, project)
	if written.Host.ID != "opencode" || written.Language.User != "es" {
		t.Errorf("state = %+v, want the preserved host and the new language", written)
	}
	if written.Layout.Docs != "docs" {
		t.Errorf("docs = %q, want docs", written.Layout.Docs)
	}
	if !written.Setup.Complete {
		t.Error("setup is not complete after resuming")
	}
}

func TestCreateInvalidLayoutWritesNothing(t *testing.T) {
	project := t.TempDir()
	req := request(project, setup.Answers{
		Host:        "opencode",
		Docs:        "../docs",
		NoDecisions: true,
		Bundles:     []string{"dev"},
	})
	req.LangSet = true

	if _, err := Create(req); err == nil {
		t.Fatal("Create() error = nil, want an invalid layout error")
	}
	if _, err := os.Stat(filepath.Join(project, ".atlas")); !os.IsNotExist(err) {
		t.Errorf(".atlas was created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md was created: %v", err)
	}
}

func TestCreateUnknownHostWritesNothing(t *testing.T) {
	project := t.TempDir()
	if _, err := Create(request(project, setup.Answers{Host: "unknown"})); err == nil {
		t.Fatal("Create() error = nil, want an unknown host error")
	}
	if _, err := os.Stat(filepath.Join(project, ".atlas")); !os.IsNotExist(err) {
		t.Errorf(".atlas was created: %v", err)
	}
}

func writeProjectFile(t *testing.T, root, rel, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", rel, err)
	}
}

func TestAdoptRespectsExistingDocs(t *testing.T) {
	project := t.TempDir()
	writeProjectFile(t, project, "docs/guide.md", "# Guide\n")
	writeProjectFile(t, project, "README.md", "# Project\n")
	writeProjectFile(t, project, "src/app.js", "export const ok = true;\n")
	writeProjectFile(t, project, "go.mod", "module example\n")

	req := request(project, setup.Answers{
		Host:        "opencode",
		NoDecisions: true,
		Bundles:     []string{"dev"},
	})
	req.LangSet = true
	req.CLILanguage = "es"
	result, err := Adopt(req)
	if err != nil {
		t.Fatalf("Adopt() error = %v", err)
	}
	if result.AlreadyAdopted || !result.Complete {
		t.Errorf("result = %+v, want a completed adoption", result)
	}

	written := readState(t, project)
	if written.Layout.Docs != "docs" {
		t.Errorf("docs = %q, want the existing route", written.Layout.Docs)
	}
	if written.Profile != "go" {
		t.Errorf("profile = %q, want the detected profile", written.Profile)
	}
	if got := readProjectFile(t, project, "docs/guide.md"); got != "# Guide\n" {
		t.Errorf("docs/guide.md = %q, want it untouched", got)
	}
	if got := readProjectFile(t, project, "README.md"); got != "# Project\n" {
		t.Errorf("README.md = %q, want it untouched", got)
	}
	if got := readProjectFile(t, project, "src/app.js"); got != "export const ok = true;\n" {
		t.Errorf("src/app.js = %q, want it untouched", got)
	}
}

func TestAdoptAlreadyAdopted(t *testing.T) {
	project := t.TempDir()
	if _, err := Create(request(project, setup.Answers{Host: "opencode"})); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	statePath := filepath.Join(project, ".atlas", "state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(state) error = %v", err)
	}

	result, err := Adopt(request(project, setup.Answers{}))
	if err != nil {
		t.Fatalf("Adopt() error = %v", err)
	}
	if !result.AlreadyAdopted {
		t.Error("AlreadyAdopted = false, want true")
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(state) error = %v", err)
	}
	if string(before) != string(after) {
		t.Error("adopt changed the state of an already adopted project")
	}
}

func TestConfigurePreservesOwnedAndOverrides(t *testing.T) {
	project := t.TempDir()
	createRequest := request(project, setup.Answers{
		Host:        "opencode",
		Docs:        "docs",
		NoDecisions: true,
		Bundles:     []string{"dev"},
	})
	createRequest.LangSet = true
	createRequest.CLILanguage = "es"
	if _, err := Create(createRequest); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	manifestPath := filepath.Join(project, ".atlas", "manifest.json")
	manifest, err := state.LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	manifest.Files["AGENTS.md"] = state.ManifestFile{
		Bundle: "dev", Origin: "core", Layer: state.LayerOverrides,
		SHA256: content.Checksum([]byte("base original\n")),
	}
	manifest.Files["docs/project-context.md"] = state.ManifestFile{
		Bundle: "dev", Origin: "core", Layer: state.LayerOwned,
		SHA256: content.Checksum([]byte("propio docs\n")),
	}
	if err := state.SaveManifest(manifestPath, manifest); err != nil {
		t.Fatalf("SaveManifest() error = %v", err)
	}
	writeProjectFile(t, project, "AGENTS.md", "propio\n")
	writeProjectFile(t, project, "docs/project-context.md", "propio docs\n")

	configureRequest := request(project, setup.Answers{
		Docs:    "documentation",
		Bundles: []string{"dev"},
	})
	configureRequest.LangSet = true
	configureRequest.CLILanguage = "en"
	if _, err := Configure(configureRequest); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	written := readState(t, project)
	if written.Language.User != "en" {
		t.Errorf("language = %+v, want en", written.Language)
	}
	if written.Layout.Docs != "documentation" {
		t.Errorf("docs = %q, want documentation", written.Layout.Docs)
	}
	if written.Host.ID != "opencode" {
		t.Errorf("host = %q, want the preserved host", written.Host.ID)
	}
	if got := readProjectFile(t, project, "AGENTS.md"); got != "propio\n" {
		t.Errorf("override = %q, want it untouched", got)
	}
	if got := readProjectFile(t, project, "docs/project-context.md"); got != "propio docs\n" {
		t.Errorf("owned = %q, want it untouched", got)
	}

	writtenManifest, err := state.LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadManifest() after configure error = %v", err)
	}
	if writtenManifest.Files["AGENTS.md"].Layer != state.LayerOverrides {
		t.Errorf("AGENTS.md layer = %q, want overrides", writtenManifest.Files["AGENTS.md"].Layer)
	}
	if writtenManifest.Files["docs/project-context.md"].Layer != state.LayerOwned {
		t.Errorf("project-context layer = %q, want owned", writtenManifest.Files["docs/project-context.md"].Layer)
	}
}

func TestConfigureChangesHost(t *testing.T) {
	snapshot := t.TempDir()
	writeProjectFile(t, snapshot, "dev/bundle.json", `{"id":"dev","version":"0.6.0","requires":[],"roles":"roles","commands":"commands"}`)
	writeProjectFile(t, snapshot, "dev/AGENTS.md", "# Contract\n")
	writeProjectFile(t, snapshot, "dev/roles/analyst.md", "# analyst\n\n> Guide.\n")
	writeProjectFile(t, snapshot, "dev/commands/apply.md", "---\ndescription: Apply\n---\n\nBody.\n")

	project := t.TempDir()
	createRequest := request(project, setup.Answers{Host: "opencode", NoDecisions: true, Bundles: []string{"dev"}})
	createRequest.Snapshot = snapshot
	createRequest.LangSet = true
	if _, err := Create(createRequest); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".opencode", "agents", "analyst.md")); err != nil {
		t.Fatalf("opencode adapter was not written: %v", err)
	}

	configureRequest := request(project, setup.Answers{Host: "claude", NoDecisions: true, Bundles: []string{"dev"}})
	configureRequest.Snapshot = snapshot
	configureRequest.LangSet = true
	if _, err := Configure(configureRequest); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(project, ".opencode", "agents", "analyst.md")); !os.IsNotExist(err) {
		t.Errorf("old opencode adapter still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "agents", "analyst.md")); err != nil {
		t.Errorf("new claude adapter was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "CLAUDE.md")); err != nil {
		t.Errorf("CLAUDE.md was not written: %v", err)
	}
	if written := readState(t, project); written.Host.ID != "claude" {
		t.Errorf("host = %q, want claude", written.Host.ID)
	}
}

func TestConfigureWithoutState(t *testing.T) {
	project := t.TempDir()
	if _, err := Configure(request(project, setup.Answers{Host: "opencode"})); err == nil {
		t.Fatal("Configure() error = nil, want a not-configured error")
	}
	if _, err := os.Stat(filepath.Join(project, ".atlas")); !os.IsNotExist(err) {
		t.Errorf(".atlas was created: %v", err)
	}
}
