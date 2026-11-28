// Package apply executes a plan: it validates every route before touching the
// disk, writes each file atomically and records the resulting manifest.
package apply

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cedaniel200/atlasctl/internal/fsutil"
	"github.com/cedaniel200/atlasctl/internal/plan"
	"github.com/cedaniel200/atlasctl/internal/state"
)

const manifestPath = ".atlas/manifest.json"

// Report groups the outcome of an application by category. A divergent file is
// also listed in Updated when it was regenerated or in Conserved when it was
// left alone.
type Report struct {
	Updated   []string
	Conserved []string
	Divergent []string
	Pruned    []string
}

// Request is the input of an application. Allowed lists the project routes the
// host declares; .atlas/ is always allowed. RemoveDirs lists legacy host
// directories that must disappear, such as the singular OpenCode agents
// directory.
type Request struct {
	Root          string
	Plan          *plan.Plan
	Allowed       []string
	RemoveDirs    []string
	EngineVersion string
	GeneratedAt   time.Time
	// SkipManifestWrite asks the application not to rewrite an already
	// consistent manifest. It keeps repeated applications byte-stable.
	SkipManifestWrite bool
}

// Apply validates the whole plan, then writes, deletes and records the
// manifest.
func Apply(request Request) (*Report, error) {
	if request.Plan == nil {
		return nil, errors.New("apply: nil plan")
	}
	if request.Plan.Manifest == nil {
		return nil, errors.New("apply: plan has no manifest")
	}
	if request.EngineVersion == "" {
		return nil, errors.New("apply: engine version is required")
	}
	if err := Validate(request); err != nil {
		return nil, err
	}

	report, err := execute(request)
	if err != nil {
		return nil, err
	}
	if !request.SkipManifestWrite {
		if err := saveManifest(request); err != nil {
			return nil, err
		}
	}
	return report, nil
}

// Validate checks the conflicts and every route the plan would touch without
// writing anything. A dry run uses it so its verdict matches a real
// application.
func Validate(request Request) error {
	if request.Plan == nil {
		return errors.New("apply: nil plan")
	}
	if err := checkConflicts(request.Plan); err != nil {
		return err
	}
	return preflight(request)
}

func execute(request Request) (*Report, error) {
	report := &Report{}
	for _, action := range request.Plan.Actions {
		switch action.Kind {
		case plan.Create, plan.Update:
			if action.ModeOnly && action.Mode != 0 {
				if err := fsutil.ChmodProjectFile(request.Root, action.Path, action.Mode); err != nil {
					return nil, fmt.Errorf("apply: %w", err)
				}
				report.Updated = append(report.Updated, action.Path)
				break
			}
			mode := action.Mode
			if mode == 0 {
				mode = 0o644
			}
			if err := fsutil.WriteProjectFileMode(request.Root, action.Path, action.Data, mode); err != nil {
				return nil, fmt.Errorf("apply: %w", err)
			}
			report.Updated = append(report.Updated, action.Path)
		case plan.Delete:
			if err := fsutil.RemoveProjectFile(request.Root, action.Path); err != nil {
				return nil, fmt.Errorf("apply: %w", err)
			}
			report.Pruned = append(report.Pruned, action.Path)
		case plan.Keep:
			report.Conserved = append(report.Conserved, action.Path)
		default:
			return nil, fmt.Errorf("apply: unknown action %q for %q", action.Kind, action.Path)
		}
		if action.Divergent {
			report.Divergent = append(report.Divergent, action.Path)
		}
	}

	for _, directory := range request.RemoveDirs {
		removed, err := fsutil.RemoveProjectTree(request.Root, directory)
		if err != nil {
			return nil, fmt.Errorf("apply: %w", err)
		}
		if removed {
			report.Pruned = append(report.Pruned, directory)
		}
	}

	sort.Strings(report.Updated)
	sort.Strings(report.Conserved)
	sort.Strings(report.Divergent)
	sort.Strings(report.Pruned)
	return report, nil
}

// preflight resolves and validates every route before the first write or
// delete, so an unauthorized or symlinked path rejects the whole application
// without partial effects.
func preflight(request Request) error {
	allowed, err := normalizeRoutes(request.Allowed)
	if err != nil {
		return err
	}
	if err := fsutil.CheckNoSymlinks(request.Root, manifestPath); err != nil {
		return fmt.Errorf("apply: %w", err)
	}

	for _, action := range request.Plan.Actions {
		if action.Kind == plan.Keep {
			continue
		}
		if _, err := fsutil.SafeJoin(request.Root, action.Path); err != nil {
			return fmt.Errorf("apply: %w", err)
		}
		if !isAuthorized(action.Path, allowed) {
			return fmt.Errorf("apply: path %q is outside .atlas/ and the routes declared by the host", action.Path)
		}
		if err := fsutil.CheckNoSymlinks(request.Root, action.Path); err != nil {
			return fmt.Errorf("apply: %w", err)
		}
	}
	for _, directory := range request.RemoveDirs {
		if _, err := fsutil.SafeJoin(request.Root, directory); err != nil {
			return fmt.Errorf("apply: %w", err)
		}
		if !isAuthorized(directory, allowed) {
			return fmt.Errorf("apply: path %q is outside .atlas/ and the routes declared by the host", directory)
		}
		if err := fsutil.CheckNoSymlinks(request.Root, directory); err != nil {
			return fmt.Errorf("apply: %w", err)
		}
	}
	return nil
}

func normalizeRoutes(routes []string) ([]string, error) {
	normalized := make([]string, 0, len(routes))
	for _, route := range routes {
		cleaned := strings.TrimSuffix(route, "/")
		if err := fsutil.ValidateRelative(cleaned); err != nil {
			return nil, fmt.Errorf("apply: invalid allowed route %q: %w", route, err)
		}
		normalized = append(normalized, cleaned)
	}
	return normalized, nil
}

func isAuthorized(filePath string, routes []string) bool {
	if filePath == ".atlas" || strings.HasPrefix(filePath, ".atlas/") {
		return true
	}
	for _, route := range routes {
		if filePath == route || strings.HasPrefix(filePath, route+"/") {
			return true
		}
	}
	return false
}

func checkConflicts(planned *plan.Plan) error {
	if len(planned.Conflicts) == 0 {
		return nil
	}
	conflict := planned.Conflicts[0]
	return fmt.Errorf("apply: conflicting routes for %q claimed by %s", conflict.Path, strings.Join(conflict.Bundles, ", "))
}

func saveManifest(request Request) error {
	manifest := *request.Plan.Manifest
	manifest.EngineVersion = request.EngineVersion
	manifest.GeneratedAt = request.GeneratedAt
	if manifest.GeneratedAt.IsZero() {
		manifest.GeneratedAt = time.Now().UTC()
	}

	if err := fsutil.CheckNoSymlinks(request.Root, manifestPath); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	absolute, err := fsutil.SafeJoin(request.Root, manifestPath)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if err := state.SaveManifest(absolute, &manifest); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}
