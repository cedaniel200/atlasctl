package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectProfileProposals(t *testing.T) {
	cases := []struct {
		name     string
		files    []string
		id       string
		evidence []string
		language string
		runtime  string
		manager  string
	}{
		{
			name: "node", files: []string{"package.json"}, id: "node-scripts",
			evidence: []string{"package.json"}, language: "JavaScript/TypeScript",
			runtime: "TBD", manager: "TBD",
		},
		{
			name: "go", files: []string{"go.mod"}, id: "go",
			evidence: []string{"go.mod"}, language: "Go",
			runtime: "TBD", manager: "Go modules",
		},
		{
			name: "python both markers", files: []string{"pyproject.toml", "requirements.txt"}, id: "python",
			evidence: []string{"pyproject.toml", "requirements.txt"}, language: "Python",
			runtime: "TBD", manager: "TBD",
		},
		{
			name: "rust", files: []string{"Cargo.toml"}, id: "rust",
			evidence: []string{"Cargo.toml"}, language: "Rust",
			runtime: "N/A", manager: "Cargo",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			project := t.TempDir()
			for _, file := range testCase.files {
				writeProfileFile(t, project, file)
			}
			proposal, ok := DetectProfile(project)
			if !ok {
				t.Fatalf("DetectProfile() found no profile for %v", testCase.files)
			}
			if proposal.ID != testCase.id {
				t.Errorf("id = %q, want %q", proposal.ID, testCase.id)
			}
			if strings.Join(proposal.Evidence, ",") != strings.Join(testCase.evidence, ",") {
				t.Errorf("evidence = %v, want %v", proposal.Evidence, testCase.evidence)
			}
			if proposal.Language != testCase.language || proposal.Runtime != testCase.runtime || proposal.PackageManager != testCase.manager {
				t.Errorf("derived = %q/%q/%q, want %q/%q/%q",
					proposal.Language, proposal.Runtime, proposal.PackageManager,
					testCase.language, testCase.runtime, testCase.manager)
			}
			if proposal.Source != "TBD" || proposal.Tests != "TBD" {
				t.Errorf("structure = %q/%q, want TBD", proposal.Source, proposal.Tests)
			}
			if !strings.Contains(proposal.Description, testCase.evidence[0]) {
				t.Errorf("description = %q, want the evidence", proposal.Description)
			}
		})
	}
}

func TestDetectProfilePriorityAndNoMarker(t *testing.T) {
	project := t.TempDir()
	if proposal, ok := DetectProfile(project); ok {
		t.Fatalf("DetectProfile() = %+v, want none", proposal)
	}
	writeProfileFile(t, project, "go.mod")
	writeProfileFile(t, project, "package.json")
	proposal, ok := DetectProfile(project)
	if !ok || proposal.ID != "node-scripts" {
		t.Errorf("proposal = %+v, want node-scripts priority", proposal)
	}
}

func writeProfileFile(t *testing.T, project, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(project, name), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
}
