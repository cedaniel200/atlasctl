package host

import (
	"fmt"

	"github.com/cedaniel200/atlasctl/internal/plan"
)

const cursorID = "cursor"

// Cursor reads AGENTS.md natively and uses its own subagent and command
// directories. It never receives .cursor/rules/: the contract has a single
// source (ADR-0004). The project MCP file is the documented .cursor/mcp.json,
// although the host remains unverified in this environment.
var cursorCapabilities = Capabilities{
	Subagents:   true,
	AgentsDir:   ".cursor/agents",
	CommandsDir: ".cursor/commands",
	StartupFile: "AGENTS.md",
	MCPFile:     ".cursor/mcp.json",
}

type cursor struct{}

func (cursor) ID() string { return cursorID }

func (cursor) Capabilities() Capabilities { return cursorCapabilities }

func (cursor) Artifacts(input Input) ([]plan.Generated, error) {
	agents, err := renderAgents(input, cursorID, cursorCapabilities, cursorFrontmatter)
	if err != nil {
		return nil, err
	}
	commands, err := renderCommands(input, cursorID, cursorCapabilities)
	if err != nil {
		return nil, err
	}
	return append(agents, commands...), nil
}

func (cursor) AllowedRoutes() []string { return allowedRoutes(cursorCapabilities, nil) }

func (cursor) ConfigFiles() []string { return configFiles(cursorCapabilities) }

func (cursor) LegacyDirs() []string { return nil }

// cursorFrontmatter is the frontmatter Cursor reads for a subagent: the host
// inherits its model, so there is no per-role model.
func cursorFrontmatter(name, description string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %s\nmodel: inherit\n---\n\n", name, description)
}
