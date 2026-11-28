// Package hooks manages the project hook configuration: it owns the guards
// directory and enables core.hooksPath. The dispatcher itself is generated
// content published by the plan; adopter guards live in guards.d/ and are never
// written by the engine.
package hooks

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cedaniel200/atlasctl/internal/fsutil"
)

const (
	// HarnessPath is the value the engine sets in core.hooksPath.
	HarnessPath = ".atlas/hooks"
	// DispatcherPath is the generated dispatcher.
	DispatcherPath = ".atlas/hooks/pre-commit"
	// GuardsDir is the owned directory where adopter guards live.
	GuardsDir = ".atlas/hooks/guards.d"
)

// PathState says how core.hooksPath relates to the harness.
type PathState string

const (
	// PathUnset means core.hooksPath is not configured.
	PathUnset PathState = "unset"
	// PathHarness means core.hooksPath points at the harness.
	PathHarness PathState = "harness"
	// PathForeign means another tool owns core.hooksPath; the engine does not
	// touch it and offers chaining instead.
	PathForeign PathState = "foreign"
)

// Report is the current hook installation state. It is what doctor needs to
// explain whether hooks are active and why not.
type Report struct {
	Git        bool
	Repository bool
	HooksPath  string
	PathState  PathState
	Dispatcher bool
	GuardsDir  bool
	Guards     []string
	Active     bool
}

// Install creates the owned guards directory and enables core.hooksPath when
// it is unset. It never touches a foreign hooks path and does nothing outside a
// Git worktree root. The dispatcher itself belongs to the plan and is written
// by the applier.
func Install(project string) (Report, error) {
	report, err := Status(project)
	if err != nil {
		return report, err
	}
	if !report.Git || !report.Repository {
		return report, nil
	}

	for _, rel := range []string{HarnessPath, GuardsDir} {
		if err := fsutil.CheckNoSymlinks(project, rel); err != nil {
			return report, fmt.Errorf("hooks: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(project, filepath.FromSlash(GuardsDir)), 0o755); err != nil {
		return report, fmt.Errorf("hooks: create %s: %w", GuardsDir, err)
	}
	if report.PathState == PathUnset {
		if err := setLocalHooksPath(project, HarnessPath); err != nil {
			return report, err
		}
	}
	return Status(project)
}

// Status reports the current hook configuration without changing anything.
func Status(project string) (Report, error) {
	report := Report{PathState: PathUnset}
	report.Git = gitAvailable()
	if report.Git {
		report.Repository = isRepositoryRoot(project)
	}
	if !report.Repository {
		return report, nil
	}

	hooksPath, found, err := configuredHooksPath(project)
	if err != nil {
		return report, err
	}
	if found {
		report.HooksPath = hooksPath
	}
	report.PathState = hooksPathState(project, hooksPath, found)

	report.Dispatcher, err = executableFile(filepath.Join(project, filepath.FromSlash(DispatcherPath)))
	if err != nil {
		return report, err
	}
	report.Guards, report.GuardsDir, err = listGuards(project)
	if err != nil {
		return report, err
	}
	report.Active = report.Dispatcher && report.PathState == PathHarness
	return report, nil
}

func hooksPathState(project, hooksPath string, found bool) PathState {
	if !found || strings.TrimSpace(hooksPath) == "" {
		return PathUnset
	}
	if isHarnessPath(project, hooksPath) {
		return PathHarness
	}
	return PathForeign
}

func isHarnessPath(project, configured string) bool {
	cleaned := filepath.Clean(strings.TrimSpace(configured))
	if cleaned == HarnessPath {
		return true
	}
	if !filepath.IsAbs(cleaned) {
		return false
	}
	expected, err := filepath.Abs(filepath.Join(project, filepath.FromSlash(HarnessPath)))
	if err != nil {
		return false
	}
	return filepath.Clean(cleaned) == filepath.Clean(expected)
}

func executableFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0, nil
}

func listGuards(project string) ([]string, bool, error) {
	directory := filepath.Join(project, filepath.FromSlash(GuardsDir))
	info, err := os.Stat(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.IsDir() {
		return nil, false, nil
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, false, err
	}
	guards := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, false, err
		}
		if info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			guards = append(guards, entry.Name())
		}
	}
	return guards, true, nil
}
