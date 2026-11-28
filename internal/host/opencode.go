package host

import (
	"fmt"

	"github.com/cedaniel200/atlasctl/internal/plan"
)

const (
	openCodeID           = "opencode"
	openCodeLegacyAgents = ".opencode/agent"
)

var openCodeCapabilities = Capabilities{
	Subagents:   true,
	AgentsDir:   ".opencode/agents",
	CommandsDir: ".opencode/commands",
	StartupFile: "AGENTS.md",
	MCPFile:     "opencode.json",
}

type opencode struct{}

func (opencode) ID() string { return openCodeID }

func (opencode) Capabilities() Capabilities { return openCodeCapabilities }

func (opencode) Artifacts(input Input) ([]plan.Generated, error) {
	agents, err := renderAgents(input, openCodeID, openCodeCapabilities, openCodeFrontmatter)
	if err != nil {
		return nil, err
	}
	commands, err := renderCommands(input, openCodeID, openCodeCapabilities)
	if err != nil {
		return nil, err
	}
	return append(agents, commands...), nil
}

func (opencode) AllowedRoutes() []string {
	return allowedRoutes(openCodeCapabilities, []string{openCodeLegacyAgents})
}

func (opencode) ConfigFiles() []string { return configFiles(openCodeCapabilities) }

func (opencode) LegacyDirs() []string { return []string{openCodeLegacyAgents} }

// openCodeFrontmatter is the frontmatter OpenCode 2 reads for a role adapter.
func openCodeFrontmatter(_ string, description string) string {
	return fmt.Sprintf("---\ndescription: %s\nmode: all\n---\n\n", description)
}
