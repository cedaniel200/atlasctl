package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func featureRepo(t *testing.T) string {
	t.Helper()
	root := initRepo(t, "main", true)
	installHooks(t, root, Options{SpecEngineRoot: "openspec"})
	runGit(t, root, "checkout", "--quiet", "-b", "feature/test")
	return root
}

func TestDispatcherAbortsOnFirstFailingGuard(t *testing.T) {
	root := featureRepo(t)
	marker := filepath.Join(t.TempDir(), "marker")
	t.Setenv("MARKER", marker)

	writeGuard(t, root, "10-fail.sh", "#!/bin/sh\nprintf '10\\n' >> \"$MARKER\"\nexit 1\n", true)
	writeGuard(t, root, "20-never.sh", "#!/bin/sh\nprintf '20\\n' >> \"$MARKER\"\nexit 0\n", true)
	stageFile(t, root, "src/app.js", "export const ok = true;\n")

	output, code := attemptCommit(t, root)
	if code != 1 {
		t.Errorf("exit code = %d, want 1: %s", code, output)
	}
	if !strings.Contains(output, "10-fail.sh") || !strings.Contains(output, "failed") {
		t.Errorf("output = %q, want the failing guard name", output)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("ReadFile(marker) error = %v", err)
	}
	if string(data) != "10\n" {
		t.Errorf("marker = %q, want only the first guard", data)
	}
}

func TestDispatcherRunsAllGuardsInOrder(t *testing.T) {
	root := featureRepo(t)
	marker := filepath.Join(t.TempDir(), "marker")
	t.Setenv("MARKER", marker)

	writeGuard(t, root, "10-a.sh", "#!/bin/sh\nprintf '10\\n' >> \"$MARKER\"\nexit 0\n", true)
	writeGuard(t, root, "20-b.sh", "#!/bin/sh\nprintf '20\\n' >> \"$MARKER\"\nexit 0\n", true)
	stageFile(t, root, "src/app.js", "export const ok = true;\n")

	output, code := attemptCommit(t, root)
	if code != 0 {
		t.Errorf("exit code = %d, want 0: %s", code, output)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("ReadFile(marker) error = %v", err)
	}
	if string(data) != "10\n20\n" {
		t.Errorf("marker = %q, want both guards in order", data)
	}
}

func TestNewGuardRunsWithoutEditingTheDispatcher(t *testing.T) {
	root := featureRepo(t)
	marker := filepath.Join(t.TempDir(), "marker")
	t.Setenv("MARKER", marker)

	path := filepath.Join(root, filepath.FromSlash(DispatcherPath))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	writeGuard(t, root, "30-new.sh", "#!/bin/sh\nprintf '30\\n' >> \"$MARKER\"\nexit 0\n", true)
	stageFile(t, root, "src/app.js", "export const ok = true;\n")

	output, code := attemptCommit(t, root)
	if code != 0 {
		t.Errorf("exit code = %d, want 0: %s", code, output)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "30\n" {
		t.Errorf("marker = (%q, %v), want the new guard to run", data, err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(before) != string(after) {
		t.Error("the dispatcher changed when a guard was added")
	}
}

func TestNonExecutableGuardIsSkipped(t *testing.T) {
	root := featureRepo(t)
	marker := filepath.Join(t.TempDir(), "marker")
	t.Setenv("MARKER", marker)

	writeGuard(t, root, "10-skip.sh", "#!/bin/sh\nprintf '10\\n' >> \"$MARKER\"\nexit 1\n", false)
	writeGuard(t, root, "20-ok.sh", "#!/bin/sh\nprintf '20\\n' >> \"$MARKER\"\nexit 0\n", true)
	stageFile(t, root, "src/app.js", "export const ok = true;\n")

	output, code := attemptCommit(t, root)
	if code != 0 {
		t.Errorf("exit code = %d, want 0: %s", code, output)
	}
	if !strings.Contains(output, "skipping non-executable guard '10-skip.sh'") {
		t.Errorf("output = %q, want a non-executable guard warning", output)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "20\n" {
		t.Errorf("marker = (%q, %v), want only the executable guard", data, err)
	}
}
