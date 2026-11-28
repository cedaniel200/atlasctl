package commands

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cedaniel200/atlasctl/internal/classify"
	"github.com/cedaniel200/atlasctl/internal/fsutil"
	"github.com/cedaniel200/atlasctl/internal/hooks"
	"github.com/cedaniel200/atlasctl/internal/host"
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

// SpecStatus reports the resolved specification adapter. Doctor also fills the
// external CLI fields.
type SpecStatus struct {
	ID            string
	Root          string
	Error         string
	CLIApplicable bool
	CLIPath       string
	CLIFound      bool
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

// DoctorRequest is the input of a diagnostic report.
type DoctorRequest struct {
	Project string
	Getenv  func(string) string
}

// HostEvidence is one host's detection signals and evidence paths.
type HostEvidence struct {
	ID           string
	Verification host.Verification
	Signals      []string
	Paths        []string
}

// DoctorReport is the read-only diagnostic of an adopted project.
type DoctorReport struct {
	State                  *state.State
	Manifest               *state.Manifest
	Host                   state.Host
	Resolution             host.Resolution
	ResolutionVerification host.Verification
	Hosts                  []HostEvidence
	Hooks                  hooks.Report
	Spec                   SpecStatus
	Layout                 spec.Layout
	LayoutError            string
	Setup                  state.Setup
	Profile                string
	NextAction             string
}

// Doctor inspects the host, hooks, specification adapter, layout and versions
// without writing. A declaration that cannot be resolved is reported in the
// report instead of stopping the diagnosis.
func Doctor(request DoctorRequest) (DoctorReport, error) {
	if request.Project == "" {
		return DoctorReport{}, errors.New("commands: project root is required")
	}
	existing, manifest, err := loadExisting(request.Project)
	if err != nil {
		return DoctorReport{}, err
	}
	if existing == nil {
		return DoctorReport{}, ErrNotConfigured
	}

	report := DoctorReport{
		State:    existing,
		Manifest: manifest,
		Host:     existing.Host,
		Setup:    existing.Setup,
		Profile:  existing.Profile,
	}

	resolution, err := host.Resolve(request.Project, "", request.Getenv)
	if err != nil {
		return DoctorReport{}, err
	}
	report.Resolution = resolution
	for _, strategy := range host.All() {
		evidence, _ := strategy.Detect(request.Project, request.Getenv)
		report.Hosts = append(report.Hosts, HostEvidence{
			ID:           strategy.ID(),
			Verification: strategy.Verification(),
			Signals:      evidence.Signals,
			Paths:        evidence.Paths,
		})
		if strategy.ID() == resolution.Host {
			report.ResolutionVerification = strategy.Verification()
		}
	}

	report.Hooks, err = hooks.Status(request.Project)
	if err != nil {
		return DoctorReport{}, err
	}

	adapter, err := spec.Resolve(existing.SpecEngine, request.Project)
	if err != nil {
		report.Spec = SpecStatus{
			ID:    existing.SpecEngine.ID,
			Root:  existing.SpecEngine.Root,
			Error: err.Error(),
		}
	} else {
		report.Spec = SpecStatus{ID: adapter.ID(), Root: adapter.Root()}
		if binary := spec.Binary(adapter); binary != "" {
			report.Spec.CLIApplicable = true
			if path, err := exec.LookPath(binary); err == nil {
				report.Spec.CLIPath = path
				report.Spec.CLIFound = true
			}
		}
	}

	layout, err := spec.ResolveLayout(existing.Layout)
	if err != nil {
		report.LayoutError = err.Error()
	} else {
		report.Layout = layout
	}
	report.NextAction = nextAction(report)
	return report, nil
}

// nextAction recommends the next command without changing anything. A foreign
// hooks path needs a human decision, not an automatic application.
func nextAction(report DoctorReport) string {
	switch {
	case report.Spec.Error != "" || report.LayoutError != "":
		return "review"
	case report.Hooks.Repository && report.Hooks.PathState == hooks.PathForeign:
		return "review"
	case !report.Setup.Complete:
		return "configure"
	case report.Manifest == nil:
		return "apply"
	case report.Hooks.Repository && (!report.Hooks.Dispatcher || !report.Hooks.GuardsDir || report.Hooks.PathState != hooks.PathHarness):
		return "apply"
	default:
		return "none"
	}
}
