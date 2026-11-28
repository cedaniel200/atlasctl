// Package bundle selects the active content bundles, resolves their
// dependencies and loads snapshots from the binary or from a directory.
package bundle

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cedaniel200/atlasctl/internal/content"
)

// presets are the named bundle lists the engine supports.
var presets = map[string][]string{
	"full": {"dev", "qa"},
}

// Presets returns the supported presets and their bundles. The result is a
// copy so callers cannot change the selection policy.
func Presets() map[string][]string {
	result := make(map[string][]string, len(presets))
	for name, ids := range presets {
		result[name] = append([]string(nil), ids...)
	}
	return result
}

// MissingError lists the bundles that a selection needs but the snapshot does
// not contain. The command layer can render the list through its catalog.
type MissingError struct {
	Bundles []string
}

func (e *MissingError) Error() string {
	return "bundle: missing bundles: " + strings.Join(e.Bundles, ", ")
}

// Request selects bundles explicitly, through a preset or both.
type Request struct {
	Bundles []string
	Preset  string
}

// Selection is the active bundle set and a snapshot filtered to those bundles.
type Selection struct {
	IDs      []string
	Snapshot *content.Snapshot
}

// Select resolves the requested bundles and all their transitive dependencies.
// A missing bundle stops the selection and is listed, never ignored. The
// returned snapshot contains only the active bundles; route conflicts are left
// to the plan, which is the only place that sees both bundle content and
// generated artifacts.
func Select(snapshot *content.Snapshot, request Request) (Selection, error) {
	if snapshot == nil {
		return Selection{}, errors.New("bundle: nil snapshot")
	}

	requested := append([]string(nil), request.Bundles...)
	if request.Preset != "" {
		preset, ok := presets[request.Preset]
		if !ok {
			return Selection{}, fmt.Errorf(
				"bundle: unknown preset %q; supported presets: %s",
				request.Preset, strings.Join(presetNames(), ", "),
			)
		}
		requested = append(requested, preset...)
	}
	requested = uniqueSorted(requested)
	if len(requested) == 0 {
		return Selection{}, errors.New("bundle: at least one bundle is required")
	}

	selected, missing := resolve(snapshot, requested)
	if len(missing) > 0 {
		return Selection{}, &MissingError{Bundles: missing}
	}
	filtered, err := snapshot.Filter(selected)
	if err != nil {
		return Selection{}, fmt.Errorf("bundle: %w", err)
	}
	return Selection{IDs: selected, Snapshot: filtered}, nil
}

func resolve(snapshot *content.Snapshot, requested []string) (selected, missing []string) {
	available := make(map[string]content.Bundle, len(snapshot.Bundles()))
	for _, entry := range snapshot.Bundles() {
		available[entry.ID] = entry
	}

	seen := map[string]bool{}
	absent := map[string]bool{}
	pending := append([]string(nil), requested...)
	for len(pending) > 0 {
		id := pending[0]
		pending = pending[1:]
		if seen[id] {
			continue
		}
		entry, ok := available[id]
		if !ok {
			absent[id] = true
			continue
		}
		seen[id] = true
		pending = append(pending, entry.Requires...)
	}
	return sortedKeys(seen), sortedKeys(absent)
}

func presetNames() []string {
	names := make([]string, 0, len(presets))
	for name := range presets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
