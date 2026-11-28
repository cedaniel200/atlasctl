package spec

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/state"
)

func TestResolveNoneAdapter(t *testing.T) {
	adapter, err := Resolve(state.SpecEngine{ID: "none"}, "")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if adapter.ID() != "none" {
		t.Errorf("ID() = %q, want none", adapter.ID())
	}
	if adapter.Root() != "" {
		t.Errorf("Root() = %q, want empty", adapter.Root())
	}

	operations := map[string]func(string) error{
		"validate": adapter.Validate,
		"archive":  adapter.Archive,
		"sync":     adapter.Sync,
	}
	for name, operation := range operations {
		if err := operation("demo"); !errors.Is(err, ErrNotApplicable) {
			t.Errorf("%s error = %v, want ErrNotApplicable", name, err)
		}
	}
}

func TestResolveOpenSpecRoot(t *testing.T) {
	adapter, err := Resolve(state.SpecEngine{ID: "openspec", Root: "documentation/openspec/"}, t.TempDir())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if adapter.ID() != "openspec" {
		t.Errorf("ID() = %q, want openspec", adapter.ID())
	}
	if adapter.Root() != "documentation/openspec" {
		t.Errorf("Root() = %q, want documentation/openspec", adapter.Root())
	}
}

func TestResolveOpenSpecRequiresRootAndProject(t *testing.T) {
	project := t.TempDir()
	cases := []struct {
		name   string
		engine state.SpecEngine
		root   string
	}{
		{"empty root", state.SpecEngine{ID: "openspec"}, project},
		{"empty project", state.SpecEngine{ID: "openspec", Root: "openspec"}, ""},
		{"traversal", state.SpecEngine{ID: "openspec", Root: "../openspec"}, project},
		{"absolute", state.SpecEngine{ID: "openspec", Root: "/openspec"}, project},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := Resolve(testCase.engine, testCase.root); err == nil {
				t.Fatal("Resolve() error = nil, want an invalid configuration error")
			}
		})
	}
}

func TestResolveUnknownAdapterDoesNotWrite(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "app.txt"), []byte("sin cambios\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Resolve(state.SpecEngine{ID: "unknown", Root: "spec"}, project)
	if err == nil {
		t.Fatal("Resolve() error = nil, want an unknown adapter error")
	}
	for _, want := range []string{"unknown", "openspec", "none"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
	entries, err := os.ReadDir(project)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "app.txt" {
		t.Errorf("project entries = %v, want only app.txt", entries)
	}
	if _, err := os.Stat(filepath.Join(project, ".atlas")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the unknown adapter created .atlas: %v", err)
	}
}
