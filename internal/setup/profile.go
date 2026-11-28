package setup

import "strings"

// ProfileProposal is a detected stack profile and the evidence that justifies
// it. Unknown values are explicit TBD or N/A; detection never guesses commands.
type ProfileProposal struct {
	ID             string
	Evidence       []string
	Description    string
	Language       string
	Runtime        string
	PackageManager string
	Source         string
	Tests          string
}

// DetectProfile proposes a profile from the project markers. The order matches
// the setup heuristic: Node, Go, Python and Rust.
func DetectProfile(project string) (ProfileProposal, bool) {
	switch {
	case existsFile(project, "package.json"):
		return proposal("node-scripts", []string{"package.json"},
			"JavaScript/TypeScript", "TBD", "TBD"), true
	case existsFile(project, "go.mod"):
		return proposal("go", []string{"go.mod"},
			"Go", "TBD", "Go modules"), true
	case existsFile(project, "pyproject.toml"), existsFile(project, "requirements.txt"):
		var evidence []string
		if existsFile(project, "pyproject.toml") {
			evidence = append(evidence, "pyproject.toml")
		}
		if existsFile(project, "requirements.txt") {
			evidence = append(evidence, "requirements.txt")
		}
		return proposal("python", evidence, "Python", "TBD", "TBD"), true
	case existsFile(project, "Cargo.toml"):
		return proposal("rust", []string{"Cargo.toml"},
			"Rust", "N/A", "Cargo"), true
	default:
		return ProfileProposal{}, false
	}
}

func proposal(id string, evidence []string, language, runtime, packageManager string) ProfileProposal {
	return ProfileProposal{
		ID:             id,
		Evidence:       evidence,
		Description:    "Profile proposed from " + strings.Join(evidence, ", ") + ". Commands and paths are TBD until validated.",
		Language:       language,
		Runtime:        runtime,
		PackageManager: packageManager,
		Source:         "TBD",
		Tests:          "TBD",
	}
}
