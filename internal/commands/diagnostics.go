package commands

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cedaniel200/atlasctl/internal/classify"
	"github.com/cedaniel200/atlasctl/internal/fsutil"
	"github.com/cedaniel200/atlasctl/internal/hooks"
	"github.com/cedaniel200/atlasctl/internal/setup"
	"github.com/cedaniel200/atlasctl/internal/spec"
	"github.com/cedaniel200/atlasctl/internal/state"
)

// SetupProfile is the profile identifier reported by check. It is
// not a setup question: an absent profile is information, never a failure.
const SetupProfile = "profile"

// CheckRequest is the input of a read-only project inspection.
type CheckRequest struct {
	Project string
}

// SetupStatus is one setup identifier, its resolved value and whether it is
// still pending.
type SetupStatus struct {
	ID      string
	Value   string
	Pending bool
}

// FileStatus is a file that needs attention: a base or generated file that is
// absent or changed on disk.
type FileStatus struct {
	Path       string
	Layer      state.Layer
	Status     classify.Status
	Tracked    bool
	Executable bool
}

// SpecStatus reports the resolved specification adapter.
type SpecStatus struct {
	ID   string
	Root string
}

// CheckReport is the read-only inspection of an adopted project.
type CheckReport struct {
	State            *state.State
	Manifest         *state.Manifest
	Setup            []SetupStatus
	Files            []FileStatus
	Dispatcher       *FileStatus
	Hooks            hooks.Report
	HooksRecommended bool
	Spec             SpecStatus
	Layout           spec.Layout
	Recommendations  int
}

// Check inspects an adopted project without writing anything. Pending setup is
// a state, not an error; an unreadable state, manifest or invalid declaration
// is an error.
func Check(request CheckRequest) (CheckReport, error) {
	if request.Project == "" {
		return CheckReport{}, errors.New("commands: project root is required")
	}
	existing, manifest, err := loadExisting(request.Project)
	if err != nil {
		return CheckReport{}, err
	}
	if existing == nil {
		return CheckReport{}, ErrNotConfigured
	}

	report := CheckReport{State: existing, Manifest: manifest}
	report.Setup = setupStatuses(existing)
	for _, item := range report.Setup {
		if item.Pending {
			report.Recommendations++
		}
	}

	report.Hooks, err = hooks.Status(request.Project)
	if err != nil {
		return CheckReport{}, err
	}

	if manifest == nil {
		report.Recommendations++
	} else {
		classified, err := classify.Classify(manifest, fsutil.RootReader{Root: request.Project})
		if err != nil {
			return CheckReport{}, err
		}
		var dispatcherContent *classify.File
		for _, file := range classified.Files() {
			if file.Path == hooks.DispatcherPath {
				copy := file
				dispatcherContent = &copy
				continue
			}
			if file.Layer != state.LayerBase && file.Layer != state.LayerGenerated {
				continue
			}
			if file.Status == classify.StatusIntact {
				continue
			}
			report.Files = append(report.Files, FileStatus{
				Path: file.Path, Layer: file.Layer, Status: file.Status, Tracked: true,
			})
			report.Recommendations++
		}
		report.Dispatcher = dispatcherStatus(request.Project, dispatcherContent)
	}
	if report.Dispatcher == nil {
		report.Dispatcher = dispatcherStatus(request.Project, nil)
	}
	if !report.Dispatcher.Tracked || report.Dispatcher.Status != classify.StatusIntact || !report.Dispatcher.Executable {
		report.Recommendations++
	}

	if report.Hooks.Repository && (report.Hooks.PathState != hooks.PathHarness || !report.Hooks.GuardsDir) {
		report.HooksRecommended = true
		report.Recommendations++
	}

	adapter, err := spec.Resolve(existing.SpecEngine, request.Project)
	if err != nil {
		return CheckReport{}, err
	}
	report.Spec = SpecStatus{ID: adapter.ID(), Root: adapter.Root()}
	layout, err := spec.ResolveLayout(existing.Layout)
	if err != nil {
		return CheckReport{}, err
	}
	report.Layout = layout
	return report, nil
}

func setupStatuses(existing *state.State) []SetupStatus {
	pending := make(map[string]bool, len(existing.Setup.Pending))
	for _, id := range existing.Setup.Pending {
		pending[id] = true
	}
	decisions := ""
	if existing.Layout.Decisions != nil {
		decisions = *existing.Layout.Decisions
	}
	return []SetupStatus{
		{ID: setup.PendingHost, Value: existing.Host.ID, Pending: pending[setup.PendingHost]},
		{ID: setup.PendingLanguage, Value: existing.Language.User, Pending: pending[setup.PendingLanguage]},
		{ID: setup.PendingDocs, Value: existing.Layout.Docs, Pending: pending[setup.PendingDocs]},
		{ID: setup.PendingDecisions, Value: decisions, Pending: pending[setup.PendingDecisions]},
		{ID: setup.PendingBundles, Value: joinValues(existing.Bundles), Pending: pending[setup.PendingBundles]},
		{ID: SetupProfile, Value: existing.Profile},
	}
}

func dispatcherStatus(project string, content *classify.File) *FileStatus {
	status := &FileStatus{Path: hooks.DispatcherPath, Layer: state.LayerGenerated}
	info, err := os.Stat(filepath.Join(project, filepath.FromSlash(hooks.DispatcherPath)))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		status.Status = classify.StatusAbsent
	case err != nil:
		status.Status = classify.StatusModified
	default:
		status.Executable = info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
	}
	if content != nil {
		status.Tracked = true
		status.Status = content.Status
	}
	return status
}

func joinValues(values []string) string {
	result := ""
	for _, value := range values {
		if value == "" {
			continue
		}
		if result != "" {
			result += ", "
		}
		result += value
	}
	return result
}
