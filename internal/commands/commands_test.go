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
