package host

import (
	"fmt"
	"strings"
)

// registry is the only list of supported hosts. Adding a host is adding its
// strategy here.
var registry = []Strategy{opencode{}, claude{}, cursor{}}

// All returns the registered strategies in declaration order.
func All() []Strategy {
	return append([]Strategy(nil), registry...)
}

// IDs returns the supported host identifiers in declaration order.
func IDs() []string {
	ids := make([]string, 0, len(registry))
	for _, strategy := range registry {
		ids = append(ids, strategy.ID())
	}
	return ids
}

// Lookup resolves a host identifier.
func Lookup(id string) (Strategy, error) {
	for _, strategy := range registry {
		if strategy.ID() == id {
			return strategy, nil
		}
	}
	return nil, fmt.Errorf("host: unknown host %q; supported hosts: %s", id, strings.Join(IDs(), ", "))
}
