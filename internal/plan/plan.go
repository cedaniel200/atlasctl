// Package plan computes what an application would create, update or keep
// before anything is written. It is pure: it receives the snapshot, the
// manifest and a read-only reader, and returns actions plus the manifest a
// successful application would leave behind.
package plan

import (
	"errors"
	"fmt"
	"sort"

	"github.com/cedaniel200/atlasctl/internal/classify"
	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/state"
)

// Kind is the action the application will take for a path.
type Kind string

const (
	// Create writes a file that does not exist on disk.
	Create Kind = "create"
	// Update writes over a file the engine governs.
	Update Kind = "update"
	// Keep leaves the file untouched.
	Keep Kind = "keep"
)

// Action is one planned file operation.
type Action struct {
	Path  string
	Kind  Kind
	Layer state.Layer
	// Data is the content to write for Create and Update; nil otherwise.
	Data []byte
	// Divergent reports local content that differs from what the engine
	// expects. A divergent base is kept; a divergent generated file is
	// regenerated and the lost edit is reported.
	Divergent bool
}

// Conflict is a path claimed by more than one source. A plan with conflicts is
// never applied.
type Conflict struct {
	Path    string
	Bundles []string
}

// Generated is a file a host derives from the state, such as a role adapter.
// Hosts provide these; the planner treats them as generated content.
type Generated struct {
	Path   string
	Bundle string
	Origin string
	Data   []byte
}

// Request is the input of a plan.
type Request struct {
	Snapshot  *content.Snapshot
	Manifest  *state.Manifest
	Reader    classify.Reader
	Generated []Generated
}

// Plan is the result: the actions to execute, the conflicts that block the
// application and the manifest the project should have afterwards.
type Plan struct {
	Actions   []Action
	Conflicts []Conflict
	Manifest  *state.Manifest
}

type desiredFile struct {
	layer  state.Layer
	bundle string
	origin string
	sha    string
	data   []byte
}

// Build calculates the plan without touching the filesystem.
func Build(request Request) (*Plan, error) {
	if request.Snapshot == nil {
		return nil, errors.New("plan: nil snapshot")
	}
	if request.Reader == nil {
		return nil, errors.New("plan: nil reader")
	}

	desired, conflicts := desiredFiles(request.Snapshot, request.Generated)
	manifestFiles := map[string]state.ManifestFile{}
	if request.Manifest != nil {
		manifestFiles = request.Manifest.Files
	}

	conflicted := make(map[string]bool, len(conflicts))
	for _, conflict := range conflicts {
		conflicted[conflict.Path] = true
	}

	paths := unionPaths(desired, manifestFiles)
	probes, keepOnly := probeManifest(paths, desired, manifestFiles)
	report, err := classify.Classify(probes, request.Reader)
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}

	planned := &Plan{
		Conflicts: conflicts,
		Manifest: &state.Manifest{
			SchemaVersion: state.SchemaVersion,
			Bundles:       bundleIDs(request.Snapshot, request.Generated),
			Files:         make(map[string]state.ManifestFile, len(paths)),
		},
	}
	for _, filePath := range paths {
		entry, tracked := manifestFiles[filePath]
		if conflicted[filePath] {
			if tracked {
				planned.Manifest.Files[filePath] = entry
			}
			continue
		}
		if keepOnly[filePath] {
			planned.Actions = append(planned.Actions, Action{Path: filePath, Kind: Keep, Layer: entry.Layer})
			if tracked {
				planned.Manifest.Files[filePath] = entry
			}
			continue
		}

		probe, ok := report.File(filePath)
		if !ok {
			return nil, fmt.Errorf("plan: no classification for %q", filePath)
		}
		file, wanted := desired[filePath]
		action, newEntry, include := decide(filePath, file, wanted, entry, tracked, probe)
		if action.Path != "" {
			planned.Actions = append(planned.Actions, action)
		}
		if include {
			planned.Manifest.Files[filePath] = newEntry
		}
	}
	return planned, nil
}

// probeManifest selects the files whose disk content matters and the paths that
// are kept without reading them. Overrides and owned content is never compared
// against the disk: an override's hash is the base it was written on, not its
// own content.
func probeManifest(paths []string, desired map[string]desiredFile, manifest map[string]state.ManifestFile) (*state.Manifest, map[string]bool) {
	probes := &state.Manifest{
		SchemaVersion: state.SchemaVersion,
		Files:         make(map[string]state.ManifestFile, len(paths)),
	}
	keepOnly := map[string]bool{}

	for _, filePath := range paths {
		file, wanted := desired[filePath]
		entry, tracked := manifest[filePath]

		claims := make([]classify.Claim, 0, 2)
		if wanted {
			claims = append(claims, classify.Claim{Path: filePath, ManifestFile: state.ManifestFile{
				Bundle: file.bundle, Origin: file.origin, Layer: file.layer, SHA256: file.sha,
			}})
		}
		if tracked {
			claims = append(claims, classify.Claim{Path: filePath, ManifestFile: entry})
		}
		if governing, ok := classify.Resolve(filePath, claims); ok {
			if governing.Layer == state.LayerOverrides || governing.Layer == state.LayerOwned {
				keepOnly[filePath] = true
				continue
			}
		}

		switch {
		case tracked:
			probes.Files[filePath] = entry
		case wanted:
			probes.Files[filePath] = state.ManifestFile{
				Bundle: file.bundle, Origin: file.origin, Layer: file.layer, SHA256: file.sha,
			}
		}
	}
	return probes, keepOnly
}

func decide(filePath string, file desiredFile, wanted bool, entry state.ManifestFile, tracked bool, probe classify.File) (Action, state.ManifestFile, bool) {
	if !wanted {
		return decideOrphan(filePath, entry, probe)
	}
	switch file.layer {
	case state.LayerBase:
		return decideBase(filePath, file, entry, tracked, probe)
	case state.LayerGenerated:
		return decideGenerated(filePath, file, entry, tracked, probe)
	default:
		return Action{Path: filePath, Kind: Keep, Layer: file.layer}, entry, tracked
	}
}

func decideBase(filePath string, file desiredFile, entry state.ManifestFile, tracked bool, probe classify.File) (Action, state.ManifestFile, bool) {
	action := Action{Path: filePath, Kind: Keep, Layer: state.LayerBase}
	switch {
	case probe.Status == classify.StatusAbsent:
		action.Kind = Create
	case probe.ActualSHA256 == file.sha:
		// The disk already holds the published content.
	case tracked && probe.ActualSHA256 == entry.SHA256:
		action.Kind = Update
	default:
		// The adoptter changed the file: keep it and report the divergence.
		action.Divergent = true
	}
	if action.Kind != Keep {
		action.Data = file.data
	}

	origin := "core"
	if tracked && entry.Layer == state.LayerBase && entry.Origin != "" {
		origin = entry.Origin
	}
	return action, state.ManifestFile{Bundle: file.bundle, Origin: origin, Layer: state.LayerBase, SHA256: file.sha}, true
}

func decideGenerated(filePath string, file desiredFile, entry state.ManifestFile, tracked bool, probe classify.File) (Action, state.ManifestFile, bool) {
	action := Action{Path: filePath, Kind: Keep, Layer: state.LayerGenerated}
	switch {
	case probe.Status == classify.StatusAbsent:
		action.Kind = Create
	case probe.ActualSHA256 == file.sha:
		// Already regenerated, including a resumed application.
	case tracked && probe.ActualSHA256 == entry.SHA256:
		action.Kind = Update
	default:
		// Generated content is engine-owned: regenerate it and report the lost
		// manual edit.
		action.Kind = Update
		action.Divergent = true
	}
	if action.Kind != Keep {
		action.Data = file.data
	}
	return action, state.ManifestFile{Bundle: file.bundle, Origin: file.origin, Layer: state.LayerGenerated, SHA256: file.sha}, true
}

func decideOrphan(filePath string, entry state.ManifestFile, probe classify.File) (Action, state.ManifestFile, bool) {
	if entry.Layer == state.LayerBase {
		if probe.Status == classify.StatusAbsent {
			// Nothing to conserve; the entry leaves the manifest.
			return Action{}, state.ManifestFile{}, false
		}
		if probe.ActualSHA256 != entry.SHA256 {
			return Action{Path: filePath, Kind: Keep, Layer: entry.Layer, Divergent: true}, entry, true
		}
	}
	// Generated, overrides, owned and intact base content is kept. Deleting a
	// file that left the desired set is a separate pruning decision.
	return Action{Path: filePath, Kind: Keep, Layer: entry.Layer}, entry, true
}

func desiredFiles(snapshot *content.Snapshot, generated []Generated) (map[string]desiredFile, []Conflict) {
	desired := map[string]desiredFile{}
	sources := map[string][]string{}

	add := func(file desiredFile, filePath string) {
		if _, exists := desired[filePath]; exists {
			sources[filePath] = append(sources[filePath], file.bundle)
			return
		}
		desired[filePath] = file
		sources[filePath] = []string{file.bundle}
	}
	for _, file := range snapshot.Files() {
		// The 0.6 snapshot carries no per-file origin; published content is
		// recorded as core until the bundle descriptor can declare an origin.
		add(desiredFile{
			layer: state.LayerBase, bundle: file.Bundle, origin: "core", sha: file.SHA256, data: file.Data,
		}, file.Path)
	}
	for _, file := range generated {
		add(desiredFile{
			layer: state.LayerGenerated, bundle: file.Bundle, origin: file.Origin,
			sha: content.Checksum(file.Data), data: file.Data,
		}, file.Path)
	}

	var conflicts []Conflict
	for filePath, bundles := range sources {
		if len(bundles) < 2 {
			continue
		}
		conflicts = append(conflicts, Conflict{Path: filePath, Bundles: uniqueSorted(bundles)})
		delete(desired, filePath)
	}
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].Path < conflicts[j].Path })
	return desired, conflicts
}

func unionPaths(desired map[string]desiredFile, manifest map[string]state.ManifestFile) []string {
	set := make(map[string]struct{}, len(desired)+len(manifest))
	for filePath := range desired {
		set[filePath] = struct{}{}
	}
	for filePath := range manifest {
		set[filePath] = struct{}{}
	}
	paths := make([]string, 0, len(set))
	for filePath := range set {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	return paths
}

func bundleIDs(snapshot *content.Snapshot, generated []Generated) []string {
	set := map[string]struct{}{}
	for _, bundle := range snapshot.Bundles() {
		set[bundle.ID] = struct{}{}
	}
	for _, file := range generated {
		if file.Bundle != "" {
			set[file.Bundle] = struct{}{}
		}
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}
