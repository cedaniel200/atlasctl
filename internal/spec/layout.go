// Package spec resolves the specification engine and the project layout
// declared in the state. It never assumes a documentation or decisions
// directory: a route that is not declared stays absent.
package spec

import (
	"fmt"
	"strings"

	"github.com/cedaniel200/atlasctl/internal/fsutil"
	"github.com/cedaniel200/atlasctl/internal/state"
)

// Layout is the declared documentation and decisions layout. A false Declared
// flag means the state did not declare the route; the engine must not assume
// or create it.
type Layout struct {
	Docs              string
	Decisions         string
	DocsDeclared      bool
	DecisionsDeclared bool
}

// ResolveLayout normalizes and validates the layout declared in the state.
// Trailing slashes are accepted because the state stores directories that way.
func ResolveLayout(declared state.Layout) (Layout, error) {
	docs, docsDeclared, err := declaredPath("docs", declared.Docs)
	if err != nil {
		return Layout{}, err
	}

	decisions := ""
	decisionsDeclared := false
	if declared.Decisions != nil {
		decisions, decisionsDeclared, err = declaredPath("decisions", *declared.Decisions)
		if err != nil {
			return Layout{}, err
		}
	}

	return Layout{
		Docs:              docs,
		Decisions:         decisions,
		DocsDeclared:      docsDeclared,
		DecisionsDeclared: decisionsDeclared,
	}, nil
}

func declaredPath(name, value string) (string, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false, nil
	}
	cleaned := strings.TrimSuffix(value, "/")
	if err := fsutil.ValidateRelative(cleaned); err != nil {
		return "", false, fmt.Errorf("spec: invalid %s path %q: %w", name, value, err)
	}
	return cleaned, true, nil
}
