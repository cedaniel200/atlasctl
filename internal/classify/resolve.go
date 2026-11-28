package classify

import "github.com/cedaniel200/atlasctl/internal/state"

// Claim is one layer's claim over a path. The manifest holds the governing
// claim of each path; a planner can also build claims from the snapshot to
// compare layers before deciding what to write.
type Claim struct {
	Path string
	state.ManifestFile
}

// Resolve returns the claim that governs path. Precedence is owned over
// overrides over base: the adoptter's content wins over the published one, and
// an owned path is never published by the engine. Generated claims are ignored
// because generated content is regenerated from the state, not resolved. The
// boolean is false when no claim applies.
//
// If two claims share the highest layer, the first one in the slice wins;
// inventories are expected to declare at most one claim per layer and path.
func Resolve(path string, claims []Claim) (Claim, bool) {
	var (
		best     Claim
		bestRank int
		found    bool
	)
	for _, candidate := range claims {
		if candidate.Path != path {
			continue
		}
		rank := layerRank(candidate.Layer)
		if rank == 0 {
			continue
		}
		if !found || rank > bestRank {
			best, bestRank, found = candidate, rank, true
		}
	}
	return best, found
}

func layerRank(layer state.Layer) int {
	switch layer {
	case state.LayerOwned:
		return 3
	case state.LayerOverrides:
		return 2
	case state.LayerBase:
		return 1
	default:
		// Generated and unknown layers never govern a path.
		return 0
	}
}
