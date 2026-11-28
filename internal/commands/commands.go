// Package commands wires create, adopt, configure and apply: it resolves the
// specification engine and layout, selects bundles, renders host artifacts,
// plans, applies, manages the hook configuration and records the state.
package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cedaniel200/atlasctl/internal/apply"
	"github.com/cedaniel200/atlasctl/internal/bundle"
	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/fsutil"
	"github.com/cedaniel200/atlasctl/internal/hooks"
	"github.com/cedaniel200/atlasctl/internal/host"
	"github.com/cedaniel200/atlasctl/internal/i18n"
	"github.com/cedaniel200/atlasctl/internal/plan"
	"github.com/cedaniel200/atlasctl/internal/setup"
	"github.com/cedaniel200/atlasctl/internal/spec"
	"github.com/cedaniel200/atlasctl/internal/state"
)

// EngineVersion is the version recorded by this build. Distribution replaces
// it at release time.
const EngineVersion = "0.6.0"

// ErrNotConfigured reports a command that needs an adopted project.
var ErrNotConfigured = errors.New("commands: project is not configured; run create or adopt first")

// contentRoutes are the project routes the engine may materialize outside
// .atlas/ and the host routes: the shared contract and the canonical skills.
var contentRoutes = []string{"AGENTS.md", ".agents/skills"}

// Request is the input of create, adopt or configure.
type Request struct {
	Project     string
	Answers     setup.Answers
	Snapshot    string
	Interactive bool
	Prompter    setup.Prompter
	Getenv      func(string) string
	Catalog     i18n.Catalog
	CLILanguage string
	LangSet     bool
}

// ApplyRequest is the input of an application over an adopted project.
type ApplyRequest struct {
	Project  string
	Snapshot string
	Bundles  []string
	Preset   string
	Host     string
	Prune    bool
	DryRun   bool
}

// Result is the outcome of the command.
type Result struct {
	State          state.State
	Pending        []string
	Complete       bool
	AlreadyAdopted bool
	DryRun         bool
	Report         *apply.Report
	Hooks          hooks.Report
}

// Create resolves the initial configuration and materializes the minimum.
func Create(request Request) (Result, error) {
	return run(request)
}

// Adopt adds the harness to an existing project without moving its files. An
// already adopted project is reported, not failed: the human can use configure
// or apply.
func Adopt(request Request) (Result, error) {
	existing, err := loadState(filepath.Join(request.Project, ".atlas", "state.json"))
	if err != nil {
		return Result{}, err
	}
	if existing != nil {
		return Result{
			State:          *existing,
			Pending:        existing.Setup.Pending,
			Complete:       existing.Setup.Complete,
			AlreadyAdopted: true,
		}, nil
	}
	return run(request)
}

// Configure updates an adopted project without rewriting owned or override
// content: it reuses the same pipeline, whose plan is idempotent.
func Configure(request Request) (Result, error) {
	existing, err := loadState(filepath.Join(request.Project, ".atlas", "state.json"))
	if err != nil {
		return Result{}, err
	}
	if existing == nil {
		return Result{}, ErrNotConfigured
	}
	return run(request)
}

// Apply updates an adopted project with the existing state and manifest. It
// never asks setup questions; flags only override bundles, host or pruning.
func Apply(request ApplyRequest) (Result, error) {
	if request.Project == "" {
		return Result{}, errors.New("commands: project root is required")
	}
	existing, manifest, err := loadExisting(request.Project)
	if err != nil {
		return Result{}, err
	}
	if existing == nil {
		return Result{}, ErrNotConfigured
	}

	st := *existing
	var strategy host.Strategy
	if st.Host.ID != "" {
		strategy, err = host.Lookup(st.Host.ID)
		if err != nil {
			return Result{}, err
		}
	}
	if request.Host != "" {
		strategy, err = host.Lookup(request.Host)
		if err != nil {
			return Result{}, err
		}
		if request.Host != st.Host.ID {
			st.Host = state.Host{ID: strategy.ID(), Source: "flag", Evidence: []string{}}
		}
	}

	bundles := request.Bundles
	if len(bundles) == 0 {
		bundles = st.Bundles
	}
	if len(bundles) == 0 && request.Preset == "" {
		bundles = []string{setup.DefaultBundle}
	}
	return deploy(request.Project, request.Snapshot, existing, manifest, st, strategy, bundles, request.Preset, request.Prune, request.DryRun)
}

func run(request Request) (Result, error) {
	if request.Project == "" {
		return Result{}, errors.New("commands: project root is required")
	}
	existing, manifest, err := loadExisting(request.Project)
	if err != nil {
		return Result{}, err
	}

	resolved, err := setup.Resolve(setup.Request{
		Project:     request.Project,
		Existing:    existing,
		Answers:     request.Answers,
		Interactive: request.Interactive,
		Prompter:    request.Prompter,
		Catalog:     request.Catalog,
		Getenv:      request.Getenv,
		CLILanguage: request.CLILanguage,
		LangSet:     request.LangSet,
	})
	if err != nil {
		return Result{}, err
	}
	return deploy(request.Project, request.Snapshot, existing, manifest, resolved.State, resolved.Strategy, resolved.Bundles, resolved.Preset, false, false)
}

func deploy(project, snapshotPath string, existing *state.State, manifest *state.Manifest, st state.State, strategy host.Strategy, bundles []string, preset string, prune, dryRun bool) (Result, error) {
	if _, err := spec.Resolve(st.SpecEngine, project); err != nil {
		return Result{}, err
	}
	layout, err := spec.ResolveLayout(st.Layout)
	if err != nil {
		return Result{}, err
	}
	st.Layout = layoutState(layout)

	snapshot, err := bundle.Load(snapshotPath)
	if err != nil {
		return Result{}, err
	}
	selection, err := bundle.Select(snapshot, bundle.Request{Bundles: bundles, Preset: preset})
	if err != nil {
		return Result{}, err
	}

	var artifacts []plan.Generated
	if strategy != nil {
		artifacts, err = strategy.Artifacts(host.Input{Snapshot: selection.Snapshot, Bundles: selection.IDs})
		if err != nil {
			return Result{}, err
		}
	}
	artifacts = append(artifacts, plan.Generated{
		Path:   hooks.DispatcherPath,
		Bundle: selection.IDs[0],
		Origin: "core",
		Data:   hooks.Script(st.SpecEngine.Root),
		Mode:   0o755,
	})

	var previous host.Strategy
	if existing != nil && existing.Host.ID != "" {
		previous, err = host.Lookup(existing.Host.ID)
		if err != nil {
			return Result{}, err
		}
	}
	transition := host.Transition(previous, strategy)
	allowed := append([]string(nil), contentRoutes...)
	allowed = append(allowed, transition.Allowed...)

	if manifest == nil {
		manifest = &state.Manifest{SchemaVersion: state.SchemaVersion}
	}
	planned, err := plan.Build(plan.Request{
		Snapshot:  selection.Snapshot,
		Manifest:  manifest,
		Reader:    fsutil.RootReader{Root: project},
		Generated: artifacts,
		Options: plan.Options{
			Prune:          prune,
			SpecEngineRoot: st.SpecEngine.Root,
			HostConfig:     transition.ConfigFiles,
		},
	})
	if err != nil {
		return Result{}, err
	}
	if dryRun {
		if err := apply.Validate(apply.Request{
			Root:       project,
			Plan:       planned,
			Allowed:    allowed,
			RemoveDirs: transition.LegacyDirs,
		}); err != nil {
			return Result{}, err
		}
		return Result{
			State:    st,
			Pending:  st.Setup.Pending,
			Complete: st.Setup.Complete,
			DryRun:   true,
			Report:   preview(planned),
		}, nil
	}

	report, err := apply.Apply(apply.Request{
		Root:          project,
		Plan:          planned,
		Allowed:       allowed,
		RemoveDirs:    transition.LegacyDirs,
		EngineVersion: EngineVersion,
		GeneratedAt:   time.Now().UTC(),
	})
	if err != nil {
		return Result{}, err
	}

	hookReport, err := hooks.Install(project)
	if err != nil {
		return Result{}, err
	}

	st.SchemaVersion = state.SchemaVersion
	st.EngineVersion = EngineVersion
	st.Bundles = selection.IDs
	st.Content.BundleVersion = primaryVersion(selection.Snapshot)
	st.Hooks.Installed = hookReport.Active
	if st.MaxParallel == "" {
		st.MaxParallel = state.MaxParallelAuto
	}
	if st.Architecture.Stance == "" {
		st.Architecture.Stance = "custom"
	}

	if err := fsutil.CheckNoSymlinks(project, ".atlas/state.json"); err != nil {
		return Result{}, err
	}
	statePath := filepath.Join(project, ".atlas", "state.json")
	if err := state.Save(statePath, &st); err != nil {
		return Result{}, err
	}

	return Result{
		State:    st,
		Pending:  st.Setup.Pending,
		Complete: st.Setup.Complete,
		Report:   report,
		Hooks:    hookReport,
	}, nil
}

func preview(planned *plan.Plan) *apply.Report {
	report := &apply.Report{}
	for _, action := range planned.Actions {
		switch action.Kind {
		case plan.Create, plan.Update:
			report.Updated = append(report.Updated, action.Path)
		case plan.Keep:
			report.Conserved = append(report.Conserved, action.Path)
		case plan.Delete:
			report.Pruned = append(report.Pruned, action.Path)
		}
		if action.Divergent {
			report.Divergent = append(report.Divergent, action.Path)
		}
	}
	return report
}

func loadExisting(project string) (*state.State, *state.Manifest, error) {
	statePath := filepath.Join(project, ".atlas", "state.json")
	existing, err := loadState(statePath)
	if err != nil {
		return nil, nil, err
	}

	manifestPath := filepath.Join(project, ".atlas", "manifest.json")
	manifest, err := loadManifest(manifestPath)
	if err != nil {
		return nil, nil, err
	}
	return existing, manifest, nil
}

func loadState(path string) (*state.State, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	loaded, err := state.Load(path)
	if err != nil {
		return nil, err
	}
	if loaded.SchemaVersion != state.SchemaVersion {
		return nil, fmt.Errorf("commands: state schema %d requires atlasctl migrate", loaded.SchemaVersion)
	}
	return loaded, nil
}

func loadManifest(path string) (*state.Manifest, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	loaded, err := state.LoadManifest(path)
	if err != nil {
		return nil, err
	}
	if loaded.SchemaVersion != state.SchemaVersion {
		return nil, fmt.Errorf("commands: manifest schema %d requires atlasctl migrate", loaded.SchemaVersion)
	}
	return loaded, nil
}

func primaryVersion(snapshot *content.Snapshot) string {
	bundles := snapshot.Bundles()
	if len(bundles) == 0 {
		return ""
	}
	return bundles[0].Version
}

func layoutState(layout spec.Layout) state.Layout {
	result := state.Layout{Docs: layout.Docs}
	if layout.DecisionsDeclared {
		value := layout.Decisions
		result.Decisions = &value
	}
	return result
}
