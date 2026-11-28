// Package plan computes what an application would create, update, keep or
// delete before anything is written. It is pure: it receives the snapshot, the
// manifest and a read-only reader, and returns actions plus the manifest a
// successful application would leave behind.
package plan

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

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
	// Delete removes a file the engine governs.
	Delete Kind = "delete"
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
	// Podeable reports an intact base file that left the snapshot and would be
	// removed by a pruning application.
	Podeable bool
	// Mode is the permission mode used when writing; zero means 0644.
	Mode fs.FileMode
	// ModeOnly reports an intact file whose permission bits differ from the
	// expected mode: the application changes the mode without rewriting it.
	ModeOnly bool
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
	// Mode is the permission mode used when writing; zero means 0644.
	Mode fs.FileMode
}

// ModeReader reports the permission bits of a project-relative file. A
// nonexistent file must return an error that matches fs.ErrNotExist.
type ModeReader interface {
	Mode(path string) (fs.FileMode, error)
}

// ModeReaderFunc adapts a function to ModeReader.
type ModeReaderFunc func(path string) (fs.FileMode, error)

// Mode implements ModeReader.
func (f ModeReaderFunc) Mode(path string) (fs.FileMode, error) { return f(path) }

// Options tunes the plan. Prune allows deleting intact base files that left the
// snapshot. SpecEngineRoot and HostConfig feed the closed list of routes that
// are never pruned.
type Options struct {
	Prune          bool
	SpecEngineRoot string
	HostConfig     []string
}

// Request is the input of a plan.
type Request struct {
	Snapshot  *content.Snapshot
	Manifest  *state.Manifest
	Reader    classify.Reader
	Modes     ModeReader
	Generated []Generated
	Options   Options
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
	mode   fs.FileMode
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
	exclusions := newExclusions(request.Options)

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
		action, newEntry, include := decide(filePath, file, wanted, entry, tracked, probe, request.Options, exclusions)
		if action.Path != "" {
			if err := correctMode(&action, filePath, file, wanted, request.Modes); err != nil {
				return nil, err
			}
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

// correctMode turns an intact generated file into a mode-only update when its
// permission bits differ from the expected ones. Content is never rewritten
// for a permission change.
func correctMode(action *Action, filePath string, file desiredFile, wanted bool, modes ModeReader) error {
	if !wanted || action.Kind != Keep || file.layer != state.LayerGenerated || file.mode == 0 || modes == nil {
		return nil
	}
	mode, err := modes.Mode(filePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("plan: %w", err)
	}
	if mode.Perm() == file.mode.Perm() {
		return nil
	}
	action.Kind = Update
	action.Data = file.data
	action.Mode = file.mode
	action.ModeOnly = true
	return nil
}

func decide(filePath string, file desiredFile, wanted bool, entry state.ManifestFile, tracked bool, probe classify.File, options Options, exclusions exclusions) (Action, state.ManifestFile, bool) {
	if !wanted {
		return decideOrphan(filePath, entry, probe, options, exclusions)
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
		action.Mode = file.mode
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
		action.Mode = file.mode
	}
	return action, state.ManifestFile{Bundle: file.bundle, Origin: file.origin, Layer: state.LayerGenerated, SHA256: file.sha}, true
}

func decideOrphan(filePath string, entry state.ManifestFile, probe classify.File, options Options, exclusions exclusions) (Action, state.ManifestFile, bool) {
	switch entry.Layer {
	case state.LayerGenerated:
		if exclusions.protects(filePath) {
			return Action{Path: filePath, Kind: Keep, Layer: entry.Layer}, entry, true
		}
		return Action{Path: filePath, Kind: Delete, Layer: entry.Layer}, state.ManifestFile{}, false
	case state.LayerBase:
		switch {
		case probe.Status == classify.StatusAbsent:
			// Nothing to conserve or delete; the entry leaves the manifest.
			return Action{}, state.ManifestFile{}, false
		case probe.ActualSHA256 != entry.SHA256:
			// A modified base is never pruned.
			return Action{Path: filePath, Kind: Keep, Layer: entry.Layer, Divergent: true}, entry, true
		case exclusions.protects(filePath):
			return Action{Path: filePath, Kind: Keep, Layer: entry.Layer}, entry, true
		case options.Prune:
			return Action{Path: filePath, Kind: Delete, Layer: entry.Layer}, state.ManifestFile{}, false
		default:
			return Action{Path: filePath, Kind: Keep, Layer: entry.Layer, Podeable: true}, entry, true
		}
	default:
		// Overrides, owned and unknown layers are never touched.
		return Action{Path: filePath, Kind: Keep, Layer: entry.Layer}, entry, true
	}
}

func desiredFiles(snapshot *content.Snapshot, generated []Generated) (map[string]desiredFile, []Conflict) {
	desired := map[string]desiredFile{}
	sources := map[string][]string{}
	sourceOnly := bundleSourceFiles(snapshot)

	add := func(file desiredFile, filePath string) {
		if _, exists := desired[filePath]; exists {
			sources[filePath] = append(sources[filePath], file.bundle)
			return
		}
		desired[filePath] = file
		sources[filePath] = []string{file.bundle}
	}
	for _, file := range snapshot.Files() {
		if sourceOnly[file.Ref()] {
			continue
		}
		// The 0.6 snapshot carries no per-file origin; published content is
		// recorded as core until the bundle descriptor can declare an origin.
		add(desiredFile{
			layer: state.LayerBase, bundle: file.Bundle, origin: "core", sha: file.SHA256, data: file.Data,
		}, file.Path)
	}
	for _, file := range generated {
		add(desiredFile{
			layer: state.LayerGenerated, bundle: file.Bundle, origin: file.Origin,
			sha: content.Checksum(file.Data), data: file.Data, mode: file.Mode,
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

// exclusions is the closed list of routes that are never pruned: Git metadata,
// the engine's own state and manifest, the specification engine root and the
// host configuration files. It applies to every deletion, including generated
// content.
type exclusions []string

func newExclusions(options Options) exclusions {
	roots := []string{".git", ".atlas/state.json", ".atlas/manifest.json"}
	if options.SpecEngineRoot != "" {
		roots = append(roots, options.SpecEngineRoot)
	}
	roots = append(roots, options.HostConfig...)

	result := make(exclusions, 0, len(roots))
	for _, root := range roots {
		cleaned := path.Clean(strings.TrimSpace(root))
		if cleaned == "" || cleaned == "." {
			continue
		}
		result = append(result, cleaned)
	}
	return result
}

func (e exclusions) protects(filePath string) bool {
	for _, root := range e {
		if filePath == root || strings.HasPrefix(filePath, root+"/") {
			return true
		}
	}
	return false
}

// bundleSourceFiles returns the snapshot references that are source material
// for the host renderers, not files to publish in the project.
func bundleSourceFiles(snapshot *content.Snapshot) map[string]bool {
	sourceOnly := map[string]bool{}
	for _, bundle := range snapshot.Bundles() {
		for _, directory := range []string{bundle.Roles, bundle.Commands} {
			directory = strings.TrimSuffix(strings.TrimSpace(directory), "/")
			if directory == "" {
				continue
			}
			prefix := directory + "/"
			for _, file := range snapshot.Files() {
				if file.Bundle == bundle.ID && strings.HasPrefix(file.Path, prefix) {
					sourceOnly[file.Ref()] = true
				}
			}
		}
	}
	return sourceOnly
}
