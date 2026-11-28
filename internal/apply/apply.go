// Package apply executes a plan: it validates every route before touching the
// disk, writes each file atomically and records the resulting manifest.
package apply

import (
	"errors"
	"fmt"
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
// host declares; .atlas/ is always allowed.
type Request struct {
	Root          string
	Plan          *plan.Plan
	Allowed       []string
	EngineVersion string
	GeneratedAt   time.Time
}

type step struct {
	absolute string
	action   plan.Action
}

// Apply validates the whole plan, then writes the files and the manifest.
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
	if err := checkConflicts(request.Plan); err != nil {
		return nil, err
	}

	steps, err := preflight(request)
	if err != nil {
		return nil, err
	}

	report := &Report{}
	for _, step := range steps {
		switch step.action.Kind {
		case plan.Create, plan.Update:
			if err := fsutil.WriteFile(step.absolute, step.action.Data); err != nil {
				return nil, fmt.Errorf("apply: %w", err)
			}
			report.Updated = append(report.Updated, step.action.Path)
		case plan.Keep:
			report.Conserved = append(report.Conserved, step.action.Path)
		default:
			return nil, fmt.Errorf("apply: unknown action %q for %q", step.action.Kind, step.action.Path)
		}
		if step.action.Divergent {
			report.Divergent = append(report.Divergent, step.action.Path)
		}
	}

	if err := saveManifest(request); err != nil {
		return nil, err
	}
	return report, nil
}

// preflight resolves and validates every route before the first write, so an
// unauthorized path rejects the whole application without partial effects.
func preflight(request Request) ([]step, error) {
	allowed, err := normalizeRoutes(request.Allowed)
	if err != nil {
		return nil, err
	}

	steps := make([]step, 0, len(request.Plan.Actions))
	for _, action := range request.Plan.Actions {
		if action.Kind == plan.Keep {
			steps = append(steps, step{action: action})
			continue
		}
		absolute, err := fsutil.SafeJoin(request.Root, action.Path)
		if err != nil {
			return nil, fmt.Errorf("apply: %w", err)
		}
		if !isAuthorized(action.Path, allowed) {
			return nil, fmt.Errorf("apply: path %q is outside .atlas/ and the routes declared by the host", action.Path)
		}
		steps = append(steps, step{absolute: absolute, action: action})
	}
	return steps, nil
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

	absolute, err := fsutil.SafeJoin(request.Root, manifestPath)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if err := state.SaveManifest(absolute, &manifest); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}
