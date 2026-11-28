package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/state"
)

func fakeOpenSpec(t *testing.T, exitCode int, stderr string) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "openspec.log")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$PWD\" >> \"$OPENSPEC_LOG\"\n" +
		"for argument in \"$@\"; do printf '%s\\n' \"$argument\" >> \"$OPENSPEC_LOG\"; done\n"
	if stderr != "" {
		script += fmt.Sprintf("printf '%%s\\n' %q >&2\n", stderr)
	}
	script += fmt.Sprintf("exit %d\n", exitCode)
	if err := os.WriteFile(filepath.Join(bin, "openspec"), []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(fake openspec) error = %v", err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("OPENSPEC_LOG", log)
	return log
}

func readInvocation(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("ReadFile(log) error = %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func clearLog(t *testing.T, log string) {
	t.Helper()
	if err := os.WriteFile(log, nil, 0o644); err != nil {
		t.Fatalf("reset log error = %v", err)
	}
}

func absolutePath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("Abs(%s) error = %v", path, err)
	}
	return resolved
}

func TestOpenSpecDelegatesOperations(t *testing.T) {
	project := t.TempDir()
	log := fakeOpenSpec(t, 0, "")
	adapter, err := Resolve(state.SpecEngine{ID: "openspec", Root: "openspec"}, project)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	cwd := absolutePath(t, project)

	if err := adapter.Validate("demo"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	want := []string{cwd, "validate", "--no-interactive", "--type", "change", "demo"}
	if got := readInvocation(t, log); !reflect.DeepEqual(got, want) {
		t.Errorf("validate invocation = %v, want %v", got, want)
	}

	clearLog(t, log)
	if err := adapter.Archive("demo"); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	want = []string{cwd, "archive", "--yes", "demo"}
	if got := readInvocation(t, log); !reflect.DeepEqual(got, want) {
		t.Errorf("archive invocation = %v, want %v", got, want)
	}

	clearLog(t, log)
	if err := adapter.Sync("demo"); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	want = []string{cwd, "sync", "demo"}
	if got := readInvocation(t, log); !reflect.DeepEqual(got, want) {
		t.Errorf("sync invocation = %v, want %v", got, want)
	}
}

func TestOpenSpecRunsFromTheDeclaredRootParent(t *testing.T) {
	project := t.TempDir()
	parent := filepath.Join(project, "documentation")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	log := fakeOpenSpec(t, 0, "")
	adapter, err := Resolve(state.SpecEngine{ID: "openspec", Root: "documentation/openspec"}, project)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if err := adapter.Validate("demo"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	got := readInvocation(t, log)
	if want := absolutePath(t, parent); got[0] != want {
		t.Errorf("cwd = %q, want %q", got[0], want)
	}
}

func TestOpenSpecMissingCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	project := t.TempDir()
	adapter, err := Resolve(state.SpecEngine{ID: "openspec", Root: "openspec"}, project)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	operations := []struct {
		name string
		run  func() error
	}{
		{"validate", func() error { return adapter.Validate("demo") }},
		{"archive", func() error { return adapter.Archive("demo") }},
		{"sync", func() error { return adapter.Sync("demo") }},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			err := operation.run()
			if err == nil {
				t.Fatal("operation error = nil, want a missing CLI error")
			}
			if !strings.Contains(err.Error(), "openspec") || !strings.Contains(err.Error(), "install") {
				t.Errorf("error = %q, want an actionable installation message", err)
			}
		})
	}
}

func TestOpenSpecCLIErrorIsSurfaced(t *testing.T) {
	project := t.TempDir()
	fakeOpenSpec(t, 1, "boom")
	adapter, err := Resolve(state.SpecEngine{ID: "openspec", Root: "openspec"}, project)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	err = adapter.Validate("demo")
	if err == nil {
		t.Fatal("Validate() error = nil, want the CLI error")
	}
	if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "validate") {
		t.Errorf("error = %q, want the operation and the CLI output", err)
	}
}

func TestOpenSpecRejectsInvalidChangeNames(t *testing.T) {
	project := t.TempDir()
	log := fakeOpenSpec(t, 0, "")
	adapter, err := Resolve(state.SpecEngine{ID: "openspec", Root: "openspec"}, project)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	for _, change := range []string{"", "-flag"} {
		if err := adapter.Validate(change); err == nil {
			t.Errorf("Validate(%q) error = nil, want an invalid change error", change)
		}
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Errorf("the CLI was invoked for an invalid change: %v", err)
	}
}
