package spec

import (
	"os"
	"path/filepath"

	"github.com/cedaniel200/atlasctl/internal/state"
)

const (
	openSpecRoot = "openspec"
	noneID       = "none"
)

// Detect returns the engine implied by the project's conventional directories:
// an openspec/ root means the openspec adapter, anything else means none.
// Detection only chooses the initial value; the state remains authoritative.
func Detect(project string) state.SpecEngine {
	info, err := os.Stat(filepath.Join(project, openSpecRoot))
	if err == nil && info.IsDir() {
		return state.SpecEngine{ID: openSpecID, Root: openSpecRoot}
	}
	return state.SpecEngine{ID: noneID}
}
