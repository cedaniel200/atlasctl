package spec

import (
	"errors"
	"fmt"

	"github.com/cedaniel200/atlasctl/internal/state"
)

// Adapter resolves the specification engine of a project. Root is the declared
// project-relative directory (empty for none); the operations delegate to the
// engine and never touch the network.
type Adapter interface {
	ID() string
	Root() string
	Validate(change string) error
	Archive(change string) error
	Sync(change string) error
}

// ErrNotApplicable reports that the project has no specification engine for
// this operation. Callers must treat it as information, not as a failure.
var ErrNotApplicable = errors.New("spec: operation not applicable")

// Resolve returns the adapter declared by the state. The command layer calls it
// immediately after loading the state, before any write: an unknown adapter is
// rejected here and nothing is created.
func Resolve(engine state.SpecEngine, project string) (Adapter, error) {
	switch engine.ID {
	case "openspec":
		return resolveOpenSpec(engine, project)
	case "none":
		return noneAdapter{}, nil
	default:
		return nil, fmt.Errorf(
			"spec: unknown spec engine %q; supported: openspec, none", engine.ID,
		)
	}
}
