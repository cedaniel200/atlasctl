package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/i18n"
)

func TestApplyViaCLI(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project,
		"--lang", "es", "create",
		"--host", "opencode", "--docs", "docs", "--no-decisions", "--bundle", "dev",
	)
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}

	out, errOut, code := runProject(t, project, "--lang", "es", "apply")
	if code != ExitOK {
		t.Fatalf("apply exit code = %d, want %d: %s", code, ExitOK, errOut)
	}

	catalog := i18n.New(i18n.Spanish)
	for _, marker := range []struct {
		id   string
		args map[string]string
	}{
		{MsgApplyDone, nil},
		{MsgApplyUpdated, map[string]string{"count": "0"}},
		{MsgApplyDivergent, map[string]string{"count": "0"}},
		{MsgApplyPruned, map[string]string{"count": "0"}},
	} {
		if want := catalog.T(marker.id, marker.args); !strings.Contains(out, want) {
			t.Errorf("output = %q, want %q", out, want)
		}
	}
	if !strings.Contains(out, "Conservados: ") {
		t.Errorf("output = %q, want the conserved report", out)
	}
	if written := cliState(t, project); written.Language.User != "es" || written.Layout.Docs != "docs" {
		t.Errorf("state = %+v, want the stored configuration preserved", written)
	}
}

func TestApplyDryRunViaCLI(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project, "create", "--host", "opencode", "--bundle", "dev")
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}
	if err := os.Remove(filepath.Join(project, "AGENTS.md")); err != nil {
		t.Fatalf("Remove(AGENTS.md) error = %v", err)
	}

	out, errOut, code := runProject(t, project, "--lang", "es", "apply", "--dry-run")
	if code != ExitOK {
		t.Fatalf("apply exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	if _, err := os.Stat(filepath.Join(project, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("dry run wrote AGENTS.md: %v", err)
	}
	catalog := i18n.New(i18n.Spanish)
	if want := catalog.T(MsgApplyDryRun, nil); !strings.Contains(out, want) {
		t.Errorf("output = %q, want %q", out, want)
	}
	if want := catalog.T(MsgApplyUpdated, map[string]string{"count": "1"}); !strings.Contains(out, want) {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestApplyChangesHostViaCLI(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project, "create", "--host", "opencode")
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}

	_, errOut, code := runProject(t, project, "apply", "--host", "claude")
	if code != ExitOK {
		t.Fatalf("apply exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	if written := cliState(t, project); written.Host.ID != "claude" {
		t.Errorf("host = %q, want claude", written.Host.ID)
	}
	if _, err := os.Stat(filepath.Join(project, "CLAUDE.md")); err != nil {
		t.Errorf("CLAUDE.md was not written: %v", err)
	}
}

func TestApplyWithoutStateViaCLI(t *testing.T) {
	project := t.TempDir()
	_, errOut, code := runProject(t, project, "--lang", "es", "apply")
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if want := i18n.New(i18n.Spanish).T(MsgApplyNotConfigured, nil); !strings.Contains(errOut, want) {
		t.Errorf("stderr = %q, want %q", errOut, want)
	}
	if _, err := os.Stat(filepath.Join(project, ".atlas")); !os.IsNotExist(err) {
		t.Errorf(".atlas was created: %v", err)
	}
}

// TestHookMessagesStayEnglishWithSpanishCLI pins the hook output, the exit code
// and the machine-facing names to English even when the CLI runs in Spanish.
func TestHookMessagesStayEnglishWithSpanishCLI(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}

	project := t.TempDir()
	gitCLI(t, project, "init", "--quiet")
	gitCLI(t, project, "symbolic-ref", "HEAD", "refs/heads/main")
	gitCLI(t, project, "config", "user.name", "CLI Test")
	gitCLI(t, project, "config", "user.email", "cli@example.test")
	gitCLI(t, project, "config", "commit.gpgsign", "false")
	writeCLIFile(t, project, "README.md", "# repo\n")
	gitCLI(t, project, "add", "README.md")
	gitCLI(t, project, "commit", "--quiet", "--no-verify", "-m", "chore: base")

	_, _, code := runProject(t, project, "--lang", "es", "create", "--host", "opencode")
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}

	writeCLIFile(t, project, "README.md", "# changed\n")
	gitCLI(t, project, "add", "README.md")
	output, exitCode := gitCLIFailure(t, project, "commit", "-m", "test: commit")
	if exitCode != 1 {
		t.Errorf("hook exit code = %d, want 1: %s", exitCode, output)
	}
	if !strings.Contains(output, "Commit blocked") {
		t.Errorf("hook output = %q, want the English message", output)
	}
	for _, translated := range []string{"bloqueado", "Confirmación", "rama base"} {
		if strings.Contains(output, translated) {
			t.Errorf("hook output = %q, want it untranslated", output)
		}
	}
}

func gitCLI(t *testing.T, project string, args ...string) string {
	t.Helper()
	output, exitCode := gitCLIFailure(t, project, args...)
	if exitCode != 0 {
		t.Fatalf("git %v exit code = %d: %s", args, exitCode, output)
	}
	return output
}

func gitCLIFailure(t *testing.T, project string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", project}, args...)...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return string(output), 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("git %v error = %v: %s", args, err, output)
	}
	return string(output), exitErr.ExitCode()
}

func TestApplyRepairsHooksWhenTheRepositoryAppears(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}

	project := t.TempDir()
	_, _, code := runProject(t, project, "create", "--host", "opencode")
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}
	if written := cliState(t, project); written.Hooks.Installed {
		t.Fatal("hooks installed without a Git repository")
	}

	gitCLI(t, project, "init", "--quiet")
	gitCLI(t, project, "config", "user.name", "CLI Test")
	gitCLI(t, project, "config", "user.email", "cli@example.test")

	_, errOut, code := runProject(t, project, "apply")
	if code != ExitOK {
		t.Fatalf("apply exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	if written := cliState(t, project); !written.Hooks.Installed {
		t.Error("apply did not enable the hooks in the new repository")
	}
}

func TestApplyErrorTextStaysEnglishWithSpanishCLI(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project, "create", "--host", "opencode")
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}

	missing := filepath.Join(project, "missing-snapshot")
	_, errOut, code := runProject(t, project, "--lang", "es", "apply", "--snapshot", missing)
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut, "bundle: snapshot") {
		t.Errorf("stderr = %q, want the English engine error", errOut)
	}
	if want := i18n.New(i18n.Spanish).T(MsgApplyFailed, map[string]string{"error": "bundle: snapshot"}); !strings.Contains(errOut, want) {
		t.Errorf("stderr = %q, want the localized wrapper with the English error", errOut)
	}
}
