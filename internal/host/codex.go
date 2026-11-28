package host

import "github.com/cedaniel200/atlasctl/internal/plan"

const codexID = "codex"

// Codex has no subagents and no project-level adapters: the roles are executed
// inline from AGENTS.md. Its configuration lives outside the project, so it
// declares no MCP file. The host remains unverified in this environment.
var codexCapabilities = Capabilities{
	Subagents:   false,
	StartupFile: "AGENTS.md",
}

type codex struct{}

func (codex) ID() string { return codexID }

func (codex) Capabilities() Capabilities { return codexCapabilities }

// Artifacts returns no files on purpose: Codex reads the contract directly and
// has no role or command adapters to generate.
func (codex) Artifacts(Input) ([]plan.Generated, error) { return nil, nil }

func (codex) AllowedRoutes() []string { return allowedRoutes(codexCapabilities, nil) }

func (codex) ConfigFiles() []string { return configFiles(codexCapabilities) }

func (codex) LegacyDirs() []string { return nil }
