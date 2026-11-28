package host

import (
	"reflect"
	"testing"
)

func TestTransitionUnionsBothHosts(t *testing.T) {
	union := Transition(opencode{}, claude{})

	wantAllowed := []string{
		".claude/agents", ".claude/commands", ".mcp.json",
		".opencode/agent", ".opencode/agents", ".opencode/commands",
		"AGENTS.md", "CLAUDE.md", "opencode.json",
	}
	if !reflect.DeepEqual(union.Allowed, wantAllowed) {
		t.Errorf("Allowed = %v, want %v", union.Allowed, wantAllowed)
	}
	if want := []string{".mcp.json", "opencode.json"}; !reflect.DeepEqual(union.ConfigFiles, want) {
		t.Errorf("ConfigFiles = %v, want %v", union.ConfigFiles, want)
	}
	if want := []string{".opencode/agent"}; !reflect.DeepEqual(union.LegacyDirs, want) {
		t.Errorf("LegacyDirs = %v, want %v", union.LegacyDirs, want)
	}
}

func TestTransitionAllowsFirstAdoption(t *testing.T) {
	union := Transition(nil, codex{})
	if want := []string{"AGENTS.md"}; !reflect.DeepEqual(union.Allowed, want) {
		t.Errorf("Allowed = %v, want %v", union.Allowed, want)
	}
	if len(union.ConfigFiles) != 0 || len(union.LegacyDirs) != 0 {
		t.Errorf("union = %+v, want no configuration or legacy directories", union)
	}
}

func TestTransitionWithoutNextHostKeepsThePreviousRoutes(t *testing.T) {
	union := Transition(opencode{}, nil)
	wantAllowed := opencode{}.AllowedRoutes()
	if !reflect.DeepEqual(union.Allowed, wantAllowed) {
		t.Errorf("Allowed = %v, want %v", union.Allowed, wantAllowed)
	}
	if want := []string{"opencode.json"}; !reflect.DeepEqual(union.ConfigFiles, want) {
		t.Errorf("ConfigFiles = %v, want %v", union.ConfigFiles, want)
	}
	if want := []string{".opencode/agent"}; !reflect.DeepEqual(union.LegacyDirs, want) {
		t.Errorf("LegacyDirs = %v, want %v", union.LegacyDirs, want)
	}
}
