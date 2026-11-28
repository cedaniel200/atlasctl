package setup

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/cedaniel200/atlasctl/internal/state"
)

// Pending identifiers are stable and in English. A project with pending items
// is operational; check reports them without failing.
const (
	PendingHost      = "host"
	PendingLanguage  = "language"
	PendingDocs      = "docs"
	PendingDecisions = "decisions"
	PendingBundles   = "bundles"
)

var pendingOrder = []string{PendingHost, PendingLanguage, PendingDocs, PendingDecisions, PendingBundles}

func pendingSet(existing *state.State) map[string]bool {
	pending := map[string]bool{}
	if existing == nil {
		for _, id := range pendingOrder {
			pending[id] = true
		}
		return pending
	}
	for _, id := range existing.Setup.Pending {
		pending[id] = true
	}
	return pending
}

func sortPending(set map[string]bool) []string {
	ordered := make([]string, 0, len(set))
	for _, id := range pendingOrder {
		if set[id] {
			ordered = append(ordered, id)
		}
	}
	return ordered
}

func sortedStrings(set map[string]bool) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func existingDirs(project string, candidates []string) []string {
	found := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		info, err := os.Stat(filepath.Join(project, filepath.FromSlash(candidate)))
		if err == nil && info.IsDir() {
			found = append(found, candidate)
		}
	}
	return found
}

func existsFile(project, name string) bool {
	info, err := os.Stat(filepath.Join(project, name))
	return err == nil && !info.IsDir()
}

func detectProfile(project string) string {
	switch {
	case existsFile(project, "package.json"):
		return "node-scripts"
	case existsFile(project, "go.mod"):
		return "go"
	case existsFile(project, "pyproject.toml"), existsFile(project, "requirements.txt"):
		return "python"
	case existsFile(project, "Cargo.toml"):
		return "rust"
	default:
		return ""
	}
}
