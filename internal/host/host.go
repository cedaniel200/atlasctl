// Package host declares the host adapters: their capabilities, the generated
// files each one needs and the project routes the application may touch.
package host

import (
	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/plan"
)

// Capabilities declares what a host supports and where it reads its adapters.
type Capabilities struct {
	Subagents   bool
	AgentsDir   string
	CommandsDir string
	StartupFile string
	MCPFile     string
}

// Input carries the snapshot and the active bundles whose roles and commands
// are rendered.
type Input struct {
	Snapshot *content.Snapshot
	Bundles  []string
}

// Strategy generates the adapters of one host. Artifacts is pure: it reads the
// snapshot and returns the files the application should write. AllowedRoutes
// tells the application which project routes belong to the host, LegacyDirs
// which obsolete host directories must disappear and ConfigFiles which files
// are user configuration and must never be pruned.
//
// Detection lives in the strategy so the registry is the only host table:
// there is no parallel list of signals or evidence paths. Detect is read-only;
// the caller injects the environment lookup so detection is testable.
type Strategy interface {
	ID() string
	Capabilities() Capabilities
	Artifacts(Input) ([]plan.Generated, error)
	AllowedRoutes() []string
	ConfigFiles() []string
	LegacyDirs() []string
	Detect(project string, getenv func(string) string) (Evidence, bool)
	Verification() Verification
}
