package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/setup"
)

func TestWriteProfileCreatesYAMLAndUpdatesState(t *testing.T) {
	project := t.TempDir()
	if _, err := Create(request(project, setup.Answers{Host: "opencode"})); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	writeProjectFile(t, project, "go.mod", "module example\n")

	detection := DetectProfile(project)
	if !detection.Found {
		t.Fatal("DetectProfile() found no profile")
	}
	if detection.Path != ".atlas/profiles/go.yaml" {
		t.Errorf("path = %q, want the profile data path", detection.Path)
	}
	result, err := WriteProfile(ProfileWriteRequest{Project: project, Proposal: detection.Proposal})
	if err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}
	if result.Path != ".atlas/profiles/go.yaml" || result.Profile != "go" {
		t.Errorf("result = %+v", result)
	}
	data := readProjectFile(t, project, ".atlas/profiles/go.yaml")
	for _, want := range []string{
		"name: go", "description:", "stack:", "structure:", "commands:",
		`install: "TBD"`, `build: "TBD"`, `test: "TBD"`,
	} {
		if !strings.Contains(data, want) {
			t.Errorf("profile YAML = %q, want %q", data, want)
		}
	}
	if written := readState(t, project); written.Profile != "go" {
		t.Errorf("state.Profile = %q, want go", written.Profile)
	}
}

func TestWriteProfileRefusesExistingWithoutForce(t *testing.T) {
	project := t.TempDir()
	if _, err := Create(request(project, setup.Answers{Host: "opencode"})); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	writeProjectFile(t, project, "go.mod", "module example\n")
	detection := DetectProfile(project)
	if _, err := WriteProfile(ProfileWriteRequest{Project: project, Proposal: detection.Proposal}); err != nil {
		t.Fatalf("first WriteProfile() error = %v", err)
	}
	writeProjectFile(t, project, ".atlas/profiles/go.yaml", "custom\n")
	before := readProjectFile(t, project, ".atlas/profiles/go.yaml")

	_, err := WriteProfile(ProfileWriteRequest{Project: project, Proposal: detection.Proposal})
	if !errors.Is(err, ErrProfileExists) {
		t.Fatalf("second WriteProfile() error = %v, want ErrProfileExists", err)
	}
	if got := readProjectFile(t, project, ".atlas/profiles/go.yaml"); got != before {
		t.Error("existing profile was overwritten without force")
	}
}

func TestWriteProfileForceOverwrites(t *testing.T) {
	project := t.TempDir()
	if _, err := Create(request(project, setup.Answers{Host: "opencode"})); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	writeProjectFile(t, project, "go.mod", "module example\n")
	detection := DetectProfile(project)
	writeProjectFile(t, project, ".atlas/profiles/go.yaml", "custom\n")

	if _, err := WriteProfile(ProfileWriteRequest{Project: project, Proposal: detection.Proposal, Force: true}); err != nil {
		t.Fatalf("WriteProfile(force) error = %v", err)
	}
	data := readProjectFile(t, project, ".atlas/profiles/go.yaml")
	if !strings.Contains(data, "name: go") || strings.Contains(data, "custom") {
		t.Errorf("profile was not regenerated: %q", data)
	}
}

func TestWriteProfileRequiresState(t *testing.T) {
	project := t.TempDir()
	writeProjectFile(t, project, "go.mod", "module example\n")
	detection := DetectProfile(project)
	if _, err := WriteProfile(ProfileWriteRequest{Project: project, Proposal: detection.Proposal}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("WriteProfile() error = %v, want ErrNotConfigured", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".atlas", "profiles", "go.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("profile was written without state: %v", err)
	}
}

func TestProfilePathRejectsTraversal(t *testing.T) {
	for _, id := range []string{"", "..", "../go", "a/b", `a\b`} {
		if _, err := ProfilePath(id); err == nil {
			t.Errorf("ProfilePath(%q) error = nil, want rejection", id)
		}
	}
}
