package host

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/plan"
)

const (
	analystRole  = "# analyst\n\n> Guide analysis.\n\nBody.\n"
	engineerRole = "# engineer\n\n> Build the change.\n\nBody.\n"
	reviewerRole = "# reviewer\n\nNo description line.\n"
	applyCommand = "---\ndescription: Apply\n---\n\nApply body.\n"
)

func snapshotFrom(t *testing.T, fsys fstest.MapFS) *content.Snapshot {
	t.Helper()
	snapshot, err := content.Load(fsys)
	if err != nil {
		t.Fatalf("content.Load() error = %v", err)
	}
	return snapshot
}

func fixtureSnapshot(t *testing.T) *content.Snapshot {
	t.Helper()
	return snapshotFrom(t, fstest.MapFS{
		"dev/bundle.json": {
			Data: []byte(`{"id":"dev","version":"0.6.0","requires":[],"roles":"roles","commands":"commands"}`),
		},
		"dev/AGENTS.md":               {Data: []byte("contrato\n")},
		"dev/roles/analyst.md":        {Data: []byte(analystRole)},
		"dev/roles/engineer.md":       {Data: []byte(engineerRole)},
		"dev/roles/reviewer.md":       {Data: []byte(reviewerRole)},
		"dev/roles/README.md":         {Data: []byte("not a role\n")},
		"dev/roles/nested/ignored.md": {Data: []byte("nested\n")},
		"dev/commands/opsx-apply.md":  {Data: []byte(applyCommand)},
		"dev/commands/README.md":      {Data: []byte("not a command\n")},
	})
}

func twoBundleSnapshot(t *testing.T) *content.Snapshot {
	t.Helper()
	return snapshotFrom(t, fstest.MapFS{
		"dev/bundle.json": {
			Data: []byte(`{"id":"dev","version":"0.6.0","requires":[],"roles":"roles","commands":"commands"}`),
		},
		"dev/roles/analyst.md": {Data: []byte(analystRole)},
		"qa/bundle.json": {
			Data: []byte(`{"id":"qa","version":"0.6.0","requires":["core"],"roles":"roles","commands":"commands"}`),
		},
		"qa/roles/qa-analyst.md": {Data: []byte("# qa analyst\n\n> Quality role.\n\n")},
		"qa/commands/qa-run.md":  {Data: []byte("qa command\n")},
	})
}

func artifactsByPath(t *testing.T, artifacts []plan.Generated) map[string]plan.Generated {
	t.Helper()
	byPath := make(map[string]plan.Generated, len(artifacts))
	for _, artifact := range artifacts {
		if _, exists := byPath[artifact.Path]; exists {
			t.Fatalf("duplicate artifact path %q", artifact.Path)
		}
		byPath[artifact.Path] = artifact
	}
	return byPath
}

func TestRegistry(t *testing.T) {
	wantIDs := []string{"opencode", "claude", "cursor"}
	if got := IDs(); !reflect.DeepEqual(got, wantIDs) {
		t.Errorf("IDs() = %v, want %v", got, wantIDs)
	}

	seen := map[string]bool{}
	for _, strategy := range All() {
		if strategy == nil {
			t.Fatal("All() returned a nil strategy")
		}
		if seen[strategy.ID()] {
			t.Errorf("duplicate host id %q", strategy.ID())
		}
		seen[strategy.ID()] = true

		found, err := Lookup(strategy.ID())
		if err != nil {
			t.Errorf("Lookup(%q) error = %v", strategy.ID(), err)
			continue
		}
		if found.ID() != strategy.ID() {
			t.Errorf("Lookup(%q).ID() = %q", strategy.ID(), found.ID())
		}
	}

	all := All()
	all[0] = nil
	if All()[0] == nil {
		t.Error("All() exposed the registry slice")
	}

	if _, err := Lookup("unknown"); err == nil {
		t.Error("Lookup(unknown) error = nil, want an unsupported host error")
	} else if !strings.Contains(err.Error(), "unknown") || !strings.Contains(err.Error(), "opencode") {
		t.Errorf("Lookup error = %q, want the requested id and the supported hosts", err)
	}
}

func TestCapabilitiesAndRoutes(t *testing.T) {
	cases := []struct {
		name     string
		strategy Strategy
		caps     Capabilities
		routes   []string
		configs  []string
		legacy   []string
	}{
		{
			name:     "opencode",
			strategy: opencode{},
			caps: Capabilities{
				Subagents: true, AgentsDir: ".opencode/agents", CommandsDir: ".opencode/commands",
				StartupFile: "AGENTS.md", MCPFile: "opencode.json",
			},
			routes:  []string{".opencode/agent", ".opencode/agents", ".opencode/commands", "AGENTS.md", "opencode.json"},
			configs: []string{"opencode.json"},
			legacy:  []string{".opencode/agent"},
		},
		{
			name:     "claude",
			strategy: claude{},
			caps: Capabilities{
				Subagents: true, AgentsDir: ".claude/agents", CommandsDir: ".claude/commands",
				StartupFile: "CLAUDE.md", MCPFile: ".mcp.json",
			},
			routes:  []string{".claude/agents", ".claude/commands", ".mcp.json", "AGENTS.md", "CLAUDE.md"},
			configs: []string{".mcp.json"},
			legacy:  nil,
		},
		{
			name:     "cursor",
			strategy: cursor{},
			caps: Capabilities{
				Subagents: true, AgentsDir: ".cursor/agents", CommandsDir: ".cursor/commands",
				StartupFile: "AGENTS.md", MCPFile: ".cursor/mcp.json",
			},
			routes:  []string{".cursor/agents", ".cursor/commands", ".cursor/mcp.json", "AGENTS.md"},
			configs: []string{".cursor/mcp.json"},
			legacy:  nil,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.strategy.Capabilities(); got != testCase.caps {
				t.Errorf("Capabilities() = %+v, want %+v", got, testCase.caps)
			}
			if got := testCase.strategy.AllowedRoutes(); !reflect.DeepEqual(got, testCase.routes) {
				t.Errorf("AllowedRoutes() = %v, want %v", got, testCase.routes)
			}
			if got := testCase.strategy.ConfigFiles(); !reflect.DeepEqual(got, testCase.configs) {
				t.Errorf("ConfigFiles() = %v, want %v", got, testCase.configs)
			}
			if got := testCase.strategy.LegacyDirs(); !reflect.DeepEqual(got, testCase.legacy) {
				t.Errorf("LegacyDirs() = %v, want %v", got, testCase.legacy)
			}

			routes := testCase.strategy.AllowedRoutes()
			for _, config := range testCase.strategy.ConfigFiles() {
				if !contains(routes, config) {
					t.Errorf("config %q is not an allowed route", config)
				}
			}
			for _, legacy := range testCase.strategy.LegacyDirs() {
				if !contains(routes, legacy) {
					t.Errorf("legacy route %q is not allowed", legacy)
				}
			}
		})
	}
}

func TestConfigFilesAreOnlyConfiguration(t *testing.T) {
	for _, strategy := range All() {
		caps := strategy.Capabilities()
		for _, config := range strategy.ConfigFiles() {
			if config == caps.AgentsDir || strings.HasPrefix(config, caps.AgentsDir+"/") {
				t.Errorf("%s: config file %q is inside the agents directory", strategy.ID(), config)
			}
			if config == caps.CommandsDir || strings.HasPrefix(config, caps.CommandsDir+"/") {
				t.Errorf("%s: config file %q is inside the commands directory", strategy.ID(), config)
			}
		}
	}
}

func TestArtifactsRequireActiveBundle(t *testing.T) {
	fixture := fixtureSnapshot(t)
	cases := []struct {
		name  string
		input Input
	}{
		{"nil snapshot", Input{Bundles: []string{"dev"}}},
		{"no bundles", Input{Snapshot: fixture}},
		{"empty bundle id", Input{Snapshot: fixture, Bundles: []string{""}}},
		{"missing bundle", Input{Snapshot: fixture, Bundles: []string{"missing"}}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := (opencode{}).Artifacts(testCase.input); err == nil {
				t.Fatal("Artifacts() error = nil, want an input error")
			}
		})
	}
}

func TestArtifactsValidateSourceDirectories(t *testing.T) {
	snapshot := snapshotFrom(t, fstest.MapFS{
		"dev/bundle.json": {
			Data: []byte(`{"id":"dev","version":"0.6.0","requires":[],"roles":"../roles","commands":"commands"}`),
		},
		"dev/roles/analyst.md": {Data: []byte(analystRole)},
	})

	_, err := (opencode{}).Artifacts(Input{Snapshot: snapshot, Bundles: []string{"dev"}})
	if err == nil {
		t.Fatal("Artifacts() error = nil, want an invalid source directory error")
	}
	if !strings.Contains(err.Error(), "invalid source directory") {
		t.Errorf("error = %q, want it to name the invalid source directory", err)
	}
}

func TestArtifactsRespectActiveBundles(t *testing.T) {
	snapshot := twoBundleSnapshot(t)

	devArtifacts, err := (opencode{}).Artifacts(Input{Snapshot: snapshot, Bundles: []string{"dev"}})
	if err != nil {
		t.Fatalf("Artifacts(dev) error = %v", err)
	}
	dev := artifactsByPath(t, devArtifacts)
	if _, ok := dev[".opencode/agents/analyst.md"]; !ok {
		t.Error("dev role is missing from the dev artifacts")
	}
	if _, ok := dev[".opencode/agents/qa-analyst.md"]; ok {
		t.Error("inactive qa role was rendered")
	}

	qaArtifacts, err := (opencode{}).Artifacts(Input{Snapshot: snapshot, Bundles: []string{"qa"}})
	if err != nil {
		t.Fatalf("Artifacts(qa) error = %v", err)
	}
	qa := artifactsByPath(t, qaArtifacts)
	if _, ok := qa[".opencode/agents/qa-analyst.md"]; !ok {
		t.Error("qa role is missing from the qa artifacts")
	}
	if _, ok := qa[".opencode/agents/analyst.md"]; ok {
		t.Error("inactive dev role was rendered")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
