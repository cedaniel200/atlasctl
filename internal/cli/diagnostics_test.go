package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/hooks"
	"github.com/cedaniel200/atlasctl/internal/i18n"
)

func TestCheckWithoutStateViaCLI(t *testing.T) {
	project := t.TempDir()
	_, errOut, code := runProject(t, project, "--lang", "es", "check")
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if want := i18n.New(i18n.Spanish).T(MsgCheckNotConfigured, nil); !strings.Contains(errOut, want) {
		t.Errorf("stderr = %q, want %q", errOut, want)
	}
}

func TestCheckSucceedsWithoutRecommendations(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project,
		"--lang", "es", "create",
		"--host", "opencode", "--docs", "docs", "--no-decisions", "--bundle", "dev",
	)
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}

	out, errOut, code := runProject(t, project, "--lang", "es", "check")
	if code != ExitOK {
		t.Fatalf("check exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	if want := i18n.New(i18n.Spanish).T(MsgCheckSummaryOK, nil); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestCheckReportsPendingAndDispatcherPermission(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project, "create", "--host", "opencode")
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}
	dispatcher := filepath.Join(project, filepath.FromSlash(hooks.DispatcherPath))
	if err := os.Chmod(dispatcher, 0o644); err != nil {
		t.Fatalf("Chmod(dispatcher) error = %v", err)
	}

	out, errOut, code := runProject(t, project, "--lang", "en", "check")
	if code != ExitFindings {
		t.Fatalf("check exit code = %d, want %d: %s", code, ExitFindings, errOut)
	}
	catalog := i18n.New(i18n.English)
	for id, args := range map[string]map[string]string{
		MsgCheckStatePending:         nil,
		MsgCheckDispatcherPermission: nil,
		MsgCheckSummaryFindings:      {"count": "5"},
	} {
		if want := catalog.T(id, args); !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want %q", out, want)
		}
	}
}

func TestCheckDistinguishesDispatcherContentAndPermission(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project, "create", "--host", "opencode")
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}
	dispatcher := filepath.Join(project, filepath.FromSlash(hooks.DispatcherPath))
	if err := os.Chmod(dispatcher, 0o644); err != nil {
		t.Fatalf("Chmod(dispatcher) error = %v", err)
	}

	out, _, code := runProject(t, project, "--lang", "en", "check")
	if code != ExitFindings {
		t.Fatalf("check exit code = %d, want %d", code, ExitFindings)
	}
	if want := i18n.New(i18n.English).T(MsgCheckDispatcherPermission, nil); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want the permission message %q", out, want)
	}

	if err := os.WriteFile(dispatcher, []byte("#!/bin/sh\necho edited\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(dispatcher) error = %v", err)
	}
	out, _, code = runProject(t, project, "--lang", "en", "check")
	if code != ExitFindings {
		t.Fatalf("check exit code = %d, want %d", code, ExitFindings)
	}
	catalog := i18n.New(i18n.English)
	if want := catalog.T(MsgCheckDispatcherModified, nil); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want the edited-content message %q", out, want)
	}
	if !strings.Contains(out, catalog.T(MsgCheckNo, nil)) && strings.Contains(out, catalog.T(MsgCheckDispatcherPermission, nil)) {
		t.Errorf("stdout = %q, want the permission message replaced", out)
	}
}

func TestCheckUnreadableStateIsError(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".atlas"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.atlas) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, ".atlas", "state.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("WriteFile(state) error = %v", err)
	}

	_, errOut, code := runProject(t, project, "check")
	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if errOut == "" {
		t.Error("stderr is empty, want the inspection error")
	}
}

func TestApplyRestoresDispatcherAndCheckConfirms(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project,
		"--lang", "es", "create",
		"--host", "opencode", "--docs", "docs", "--no-decisions", "--bundle", "dev",
	)
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}
	dispatcher := filepath.Join(project, filepath.FromSlash(hooks.DispatcherPath))
	if err := os.Chmod(dispatcher, 0o644); err != nil {
		t.Fatalf("Chmod(dispatcher) error = %v", err)
	}

	if _, _, code := runProject(t, project, "--lang", "es", "check"); code != ExitFindings {
		t.Fatalf("check exit code = %d, want %d", code, ExitFindings)
	}
	if _, errOut, code := runProject(t, project, "--lang", "es", "apply"); code != ExitOK {
		t.Fatalf("apply exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	info, err := os.Stat(dispatcher)
	if err != nil {
		t.Fatalf("Stat(dispatcher) error = %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("dispatcher mode = %o, want 0755", info.Mode().Perm())
	}
	out, errOut, code := runProject(t, project, "--lang", "es", "check")
	if code != ExitOK {
		t.Fatalf("check exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	if want := i18n.New(i18n.Spanish).T(MsgCheckDispatcherOK, nil); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}
