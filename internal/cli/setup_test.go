package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/state"
)

func runProject(t *testing.T, project string, args ...string) (string, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(args, Env{
		Getenv:  func(string) string { return "" },
		Stdout:  &out,
		Stderr:  &errOut,
		Project: project,
	})
	return out.String(), errOut.String(), code
}

func runProjectInteractive(t *testing.T, project, input string, args ...string) (string, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(args, Env{
		Getenv:      func(string) string { return "" },
		Stdout:      &out,
		Stderr:      &errOut,
		Stdin:       strings.NewReader(input),
		Interactive: true,
		Project:     project,
	})
	return out.String(), errOut.String(), code
}

func cliState(t *testing.T, project string) *state.State {
	t.Helper()
	loaded, err := state.Load(filepath.Join(project, ".atlas", "state.json"))
	if err != nil {
		t.Fatalf("state.Load() error = %v", err)
	}
	return loaded
}

func TestCreateWithAllFlagsViaCLI(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project,
		"--lang", "es", "create",
		"--host", "opencode", "--docs", "docs", "--no-decisions", "--bundle", "dev",
	)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}

	written := cliState(t, project)
	if !written.Setup.Complete || len(written.Setup.Pending) != 0 {
		t.Errorf("setup = %+v, want complete", written.Setup)
	}
	if written.Host.ID != "opencode" || written.Host.Source != "flag" {
		t.Errorf("host = %+v, want opencode by flag", written.Host)
	}
	if written.Language.CLI != "es" || written.Language.User != "es" {
		t.Errorf("language = %+v, want es", written.Language)
	}
	if written.Layout.Docs != "docs" || written.Layout.Decisions != nil {
		t.Errorf("layout = %+v, want docs and no decisions", written.Layout)
	}
	if _, err := os.Stat(filepath.Join(project, "AGENTS.md")); err != nil {
		t.Errorf("AGENTS.md was not written: %v", err)
	}
}

func TestCreateLeavesPendingAndSucceedsViaCLI(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project, "create", "--host", "opencode")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}

	written := cliState(t, project)
	if written.Setup.Complete {
		t.Error("setup complete, want pending")
	}
	want := []string{"language", "docs", "decisions", "bundles"}
	if got := written.Setup.Pending; len(got) != len(want) {
		t.Errorf("pending = %v, want %v", got, want)
	}
	if written.Host.ID != "opencode" {
		t.Errorf("host = %q, want the answered host", written.Host.ID)
	}
}

func TestCreateUnknownFlagIsUsageViaCLI(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project, "create", "--nope")
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if _, err := os.Stat(filepath.Join(project, ".atlas")); !os.IsNotExist(err) {
		t.Errorf(".atlas was created on a usage error: %v", err)
	}
}

func TestCreateConflictingFlagsIsUsageViaCLI(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project, "create", "--docs", "docs", "--no-docs")
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if _, err := os.Stat(filepath.Join(project, ".atlas")); !os.IsNotExist(err) {
		t.Errorf(".atlas was created on a usage error: %v", err)
	}
}

func TestCreateInteractiveViaCLI(t *testing.T) {
	project := t.TempDir()
	input := "opencode\nes\ndocs\n\ndev\n"
	_, _, code := runProjectInteractive(t, project, input, "create")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}

	written := cliState(t, project)
	if !written.Setup.Complete {
		t.Errorf("setup = %+v, want complete", written.Setup)
	}
	if written.Host.ID != "opencode" || written.Host.Source != "question" {
		t.Errorf("host = %+v, want opencode from the interview", written.Host)
	}
	if written.Language.User != "es" {
		t.Errorf("language = %+v, want the interview answer", written.Language)
	}
	if written.Layout.Docs != "docs" || written.Layout.Decisions != nil {
		t.Errorf("layout = %+v, want docs and no decisions", written.Layout)
	}
}

func TestCreateInvalidLayoutViaCLI(t *testing.T) {
	project := t.TempDir()
	_, stderr, code := runProject(t, project,
		"create", "--host", "opencode", "--docs", "../docs", "--no-decisions", "--bundle", "dev",
	)
	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Error("stderr is empty, want the failure message")
	}
	if _, err := os.Stat(filepath.Join(project, ".atlas")); !os.IsNotExist(err) {
		t.Errorf(".atlas was created on an invalid layout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md was created on an invalid layout: %v", err)
	}
}

func writeCLIFile(t *testing.T, project, rel, data string) {
	t.Helper()
	path := filepath.Join(project, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", rel, err)
	}
}

func TestAdoptWithDocsViaCLI(t *testing.T) {
	project := t.TempDir()
	writeCLIFile(t, project, "docs/guide.md", "# Guide\n")
	writeCLIFile(t, project, "README.md", "# Project\n")

	_, _, code := runProject(t, project,
		"--lang", "es", "adopt",
		"--host", "opencode", "--no-decisions", "--bundle", "dev",
	)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}

	written := cliState(t, project)
	if written.Layout.Docs != "docs" {
		t.Errorf("docs = %q, want the existing route", written.Layout.Docs)
	}
	if data, err := os.ReadFile(filepath.Join(project, "docs", "guide.md")); err != nil || string(data) != "# Guide\n" {
		t.Errorf("docs/guide.md = (%q, %v), want it untouched", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(project, "README.md")); err != nil || string(data) != "# Project\n" {
		t.Errorf("README.md = (%q, %v), want it untouched", data, err)
	}
}

func TestAdoptAlreadyAdoptedViaCLI(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project, "--lang", "es", "create", "--host", "opencode")
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}
	statePath := filepath.Join(project, ".atlas", "state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(state) error = %v", err)
	}

	_, _, code = runProject(t, project, "--lang", "es", "adopt")
	if code != ExitOK {
		t.Errorf("adopt exit code = %d, want %d", code, ExitOK)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(state) error = %v", err)
	}
	if string(before) != string(after) {
		t.Error("adopt changed the state of an already adopted project")
	}
}

func TestConfigureViaCLI(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project,
		"--lang", "es", "create",
		"--host", "opencode", "--docs", "docs", "--no-decisions", "--bundle", "dev",
	)
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}
	agents, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}

	_, _, code = runProject(t, project, "--lang", "en", "configure", "--docs", "documentation")
	if code != ExitOK {
		t.Fatalf("configure exit code = %d, want %d", code, ExitOK)
	}

	written := cliState(t, project)
	if written.Language.User != "en" || written.Layout.Docs != "documentation" {
		t.Errorf("state = %+v, want en and documentation", written)
	}
	if written.Host.ID != "opencode" {
		t.Errorf("host = %q, want the preserved host", written.Host.ID)
	}
	after, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	if string(agents) != string(after) {
		t.Error("configure rewrote AGENTS.md")
	}
}

func TestConfigureWithoutStateViaCLI(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project, "configure", "--host", "opencode")
	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if _, err := os.Stat(filepath.Join(project, ".atlas")); !os.IsNotExist(err) {
		t.Errorf(".atlas was created: %v", err)
	}
}
