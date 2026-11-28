package host

import (
	"fmt"

	"github.com/cedaniel200/atlasctl/internal/plan"
)

const claudeID = "claude"

var claudeCapabilities = Capabilities{
	Subagents:   true,
	AgentsDir:   ".claude/agents",
	CommandsDir: ".claude/commands",
	StartupFile: "CLAUDE.md",
	MCPFile:     ".mcp.json",
}

type claude struct{}

func (claude) ID() string { return claudeID }

func (claude) Capabilities() Capabilities { return claudeCapabilities }

func (claude) Artifacts(input Input) ([]plan.Generated, error) {
	agents, err := renderAgents(input, claudeID, claudeCapabilities, claudeFrontmatter)
	if err != nil {
		return nil, err
	}
	commands, err := renderCommands(input, claudeID, claudeCapabilities)
	if err != nil {
		return nil, err
	}
	startup := plan.Generated{
		Path:   claudeCapabilities.StartupFile,
		Bundle: primaryBundle(input.Bundles),
		Origin: "host:" + claudeID,
		Data:   []byte("@AGENTS.md\n"),
	}
	return append(append(agents, commands...), startup), nil
}

func (claude) AllowedRoutes() []string { return allowedRoutes(claudeCapabilities, nil) }

func (claude) ConfigFiles() []string { return configFiles(claudeCapabilities) }

func (claude) LegacyDirs() []string { return nil }

// claudeFrontmatter is the frontmatter Claude Code reads for a subagent.
func claudeFrontmatter(name, description string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n", name, description)
}
