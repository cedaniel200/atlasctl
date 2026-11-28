// Package commands wires create: it resolves the setup, validates
// the specification engine and layout, selects bundles, renders host
// artifacts, plans, applies, installs hooks and records the state.
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

// contentRoutes are the project routes the engine may materialize outside
// .atlas/ and the host routes: the shared contract and the canonical skills.
var contentRoutes = []string{"AGENTS.md", ".agents/skills"}

// Request is the input of create or adopt.
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

// Result is the outcome of the command.
type Result struct {
	State    state.State
	Pending  []string
	Complete bool
	Report   *apply.Report
	Hooks    hooks.Report
}

// Create resolves the initial configuration and materializes the minimum.
func Create(request Request) (Result, error) {
	return run(request)
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
	if _, err := spec.Resolve(resolved.State.SpecEngine, request.Project); err != nil {
		return Result{}, err
	}
	layout, err := spec.ResolveLayout(resolved.State.Layout)
	if err != nil {
		return Result{}, err
	}

	snapshot, err := bundle.Load(request.Snapshot)
	if err != nil {
		return Result{}, err
	}
	selection, err := bundle.Select(snapshot, bundle.Request{Bundles: resolved.Bundles, Preset: resolved.Preset})
	if err != nil {
		return Result{}, err
	}

	var artifacts []plan.Generated
	if resolved.Strategy != nil {
		artifacts, err = resolved.Strategy.Artifacts(host.Input{
			Snapshot: selection.Snapshot,
			Bundles:  selection.IDs,
		})
		if err != nil {
			return Result{}, err
		}
	}

	allowed := append([]string(nil), contentRoutes...)
	var removeDirs, hostConfig []string
	if resolved.Strategy != nil {
		allowed = append(allowed, resolved.Strategy.AllowedRoutes()...)
		removeDirs = resolved.Strategy.LegacyDirs()
		hostConfig = resolved.Strategy.ConfigFiles()
	}

	if manifest == nil {
		manifest = &state.Manifest{SchemaVersion: state.SchemaVersion}
	}
	planned, err := plan.Build(plan.Request{
		Snapshot:  selection.Snapshot,
		Manifest:  manifest,
		Reader:    fsutil.RootReader{Root: request.Project},
		Generated: artifacts,
		Options: plan.Options{
			SpecEngineRoot: resolved.State.SpecEngine.Root,
			HostConfig:     hostConfig,
		},
	})
	if err != nil {
		return Result{}, err
	}

	report, err := apply.Apply(apply.Request{
		Root:          request.Project,
		Plan:          planned,
		Allowed:       allowed,
		RemoveDirs:    removeDirs,
		EngineVersion: EngineVersion,
		GeneratedAt:   time.Now().UTC(),
	})
	if err != nil {
		return Result{}, err
	}

	hookReport, err := hooks.Install(request.Project, hooks.Options{SpecEngineRoot: resolved.State.SpecEngine.Root})
	if err != nil {
		return Result{}, err
	}

	st := resolved.State
	st.SchemaVersion = state.SchemaVersion
	st.EngineVersion = EngineVersion
	st.Bundles = selection.IDs
	st.Content.BundleVersion = primaryVersion(selection.Snapshot)
	st.Layout = layoutState(layout)
	st.Hooks.Installed = hookReport.Active
	if st.MaxParallel == "" {
		st.MaxParallel = state.MaxParallelAuto
	}
	if st.Architecture.Stance == "" {
		st.Architecture.Stance = "custom"
	}
	st.Setup = state.Setup{Complete: resolved.Complete, Pending: resolved.Pending}

	if err := fsutil.CheckNoSymlinks(request.Project, ".atlas/state.json"); err != nil {
		return Result{}, err
	}
	statePath := filepath.Join(request.Project, ".atlas", "state.json")
	if err := state.Save(statePath, &st); err != nil {
		return Result{}, err
	}

	return Result{
		State:    st,
		Pending:  resolved.Pending,
		Complete: resolved.Complete,
		Report:   report,
		Hooks:    hookReport,
	}, nil
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
