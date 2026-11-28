package hooks

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func isolateGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func initRepo(t *testing.T, branch string, withCommit bool) string {
	t.Helper()
	isolateGit(t)
	root := t.TempDir()
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	runGit(t, root, "config", "user.name", "Hooks Test")
	runGit(t, root, "config", "user.email", "hooks@example.test")
	if withCommit {
		writeProjectFile(t, root, "README.md", "# repo\n")
		runGit(t, root, "add", "README.md")
		runGit(t, root, "commit", "--quiet", "--no-verify", "-m", "chore: base")
	}
	return root
}

func writeProjectFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", rel, err)
	}
}

func stageFile(t *testing.T, root, rel, content string) {
	t.Helper()
	writeProjectFile(t, root, rel, content)
	runGit(t, root, "add", rel)
}

func writeGuard(t *testing.T, root, name, body string, executable bool) {
	t.Helper()
	writeProjectFile(t, root, GuardsDir+"/"+name, body)
	mode := os.FileMode(0o644)
	if executable {
		mode = 0o755
	}
	if err := os.Chmod(filepath.Join(root, filepath.FromSlash(GuardsDir), name), mode); err != nil {
		t.Fatalf("Chmod(%s) error = %v", name, err)
	}
}

func installHooks(t *testing.T, root string, options Options) Report {
	t.Helper()
	report, err := Install(root, options)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	return report
}

func attemptCommit(t *testing.T, root string) (string, int) {
	t.Helper()
	cmd := exec.Command("git", "-C", root, "commit", "-m", "test: commit")
	output, err := cmd.CombinedOutput()
	if err == nil {
		return string(output), 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(output), exitErr.ExitCode()
	}
	t.Fatalf("git commit error = %v: %s", err, output)
	return "", -1
}

func TestInstallSetsHarnessPathAndWritesDispatcher(t *testing.T) {
	root := initRepo(t, "main", true)

	report := installHooks(t, root, Options{SpecEngineRoot: "openspec"})
	if !report.Git || !report.Repository {
		t.Fatalf("report = %+v, want a git repository", report)
	}
	if report.PathState != PathHarness {
		t.Errorf("path state = %q, want %q", report.PathState, PathHarness)
	}
	if report.HooksPath != HarnessPath {
		t.Errorf("hooks path = %q, want %q", report.HooksPath, HarnessPath)
	}
	if !report.Dispatcher || !report.GuardsDir || !report.Active {
		t.Errorf("report = %+v, want an active installation", report)
	}

	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(DispatcherPath)))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), "{{") {
		t.Error("the installed dispatcher kept a template placeholder")
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(DispatcherPath)))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o755 {
		t.Errorf("dispatcher mode = %o, want 0755", mode)
	}
	if got := runGit(t, root, "config", "--get", "core.hooksPath"); got != HarnessPath {
		t.Errorf("core.hooksPath = %q, want %q", got, HarnessPath)
	}

	status, err := Status(root)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !reflect.DeepEqual(status, report) {
		t.Errorf("Status() = %+v, want %+v", status, report)
	}
}

func TestInstallKeepsForeignHooksPath(t *testing.T) {
	root := initRepo(t, "main", true)
	runGit(t, root, "config", "--local", "core.hooksPath", "other-hooks")

	report := installHooks(t, root, Options{SpecEngineRoot: "openspec"})
	if report.PathState != PathForeign {
		t.Errorf("path state = %q, want %q", report.PathState, PathForeign)
	}
	if report.HooksPath != "other-hooks" {
		t.Errorf("hooks path = %q, want other-hooks", report.HooksPath)
	}
	if report.Active {
		t.Error("Active = true, want false for a foreign hooks path")
	}
	if !report.Dispatcher {
		t.Error("Dispatcher = false, want the dispatcher written for chaining")
	}
	if got := runGit(t, root, "config", "--get", "core.hooksPath"); got != "other-hooks" {
		t.Errorf("core.hooksPath = %q, want it untouched", got)
	}

	status, err := Status(root)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.PathState != PathForeign || status.HooksPath != "other-hooks" {
		t.Errorf("Status() = %+v, want the foreign path", status)
	}
}

func TestInstallWithoutRepositorySkipsHooks(t *testing.T) {
	isolateGit(t)
	root := t.TempDir()

	report := installHooks(t, root, Options{SpecEngineRoot: "openspec"})
	if report.Repository {
		t.Error("Repository = true, want false")
	}
	if report.Dispatcher || report.GuardsDir || report.Active {
		t.Errorf("report = %+v, want no installed hooks", report)
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf(".atlas was created without a repository: %v", err)
	}

	status, err := Status(root)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Repository || status.Active {
		t.Errorf("Status() = %+v, want no active hooks", status)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	root := initRepo(t, "main", true)
	installHooks(t, root, Options{SpecEngineRoot: "openspec"})

	path := filepath.Join(root, filepath.FromSlash(DispatcherPath))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	report := installHooks(t, root, Options{SpecEngineRoot: "openspec"})
	if report.PathState != PathHarness || !report.Active {
		t.Errorf("report = %+v, want the harness path active", report)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(before) != string(after) {
		t.Error("a second installation changed the dispatcher")
	}
}

func TestInstallRejectsUnsafeSpecEngineRoot(t *testing.T) {
	root := initRepo(t, "main", true)

	if _, err := Install(root, Options{SpecEngineRoot: "../outside"}); err == nil {
		t.Fatal("Install() error = nil, want an invalid spec engine root error")
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf(".atlas was created before validating the spec engine root: %v", err)
	}
}

func TestStatusListsExecutableGuards(t *testing.T) {
	root := initRepo(t, "main", true)
	installHooks(t, root, Options{SpecEngineRoot: "openspec"})
	writeGuard(t, root, "20-b", "#!/bin/sh\nexit 0\n", true)
	writeGuard(t, root, "10-a", "#!/bin/sh\nexit 0\n", true)
	writeGuard(t, root, "30-c", "#!/bin/sh\nexit 0\n", false)

	status, err := Status(root)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.GuardsDir {
		t.Error("GuardsDir = false, want true")
	}
	if want := []string{"10-a", "20-b"}; !reflect.DeepEqual(status.Guards, want) {
		t.Errorf("Guards = %v, want %v", status.Guards, want)
	}
}

func TestStatusBeforeInstall(t *testing.T) {
	root := initRepo(t, "main", true)
	status, err := Status(root)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Git || !status.Repository {
		t.Errorf("status = %+v, want a git repository", status)
	}
	if status.PathState != PathUnset || status.HooksPath != "" {
		t.Errorf("status = %+v, want an unset hooks path", status)
	}
	if status.Dispatcher || status.GuardsDir || status.Active {
		t.Errorf("status = %+v, want no installed hooks", status)
	}
}
