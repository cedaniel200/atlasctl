// Package state reads and writes the engine's project state and manifest.
package state

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cedaniel200/atlasctl/internal/fsutil"
)

const (
	// SchemaVersion is the schema version this build reads and writes.
	SchemaVersion = 2

	// MaxParallelAuto lets the engine derive the parallelism from the machine.
	MaxParallelAuto = "auto"
)

// State is the project state stored in .atlas/state.json.
type State struct {
	SchemaVersion  int               `json:"schema_version"`
	EngineVersion  string            `json:"engine_version"`
	Bundles        []string          `json:"bundles"`
	Setup          Setup             `json:"setup"`
	Host           Host              `json:"host"`
	SpecEngine     SpecEngine        `json:"spec_engine"`
	Language       Language          `json:"language"`
	Layout         Layout            `json:"layout"`
	Hooks          Hooks             `json:"hooks"`
	MaxParallel    string            `json:"max_parallel"`
	Content        Content           `json:"content"`
	Profile        string            `json:"profile"`
	Stacks         []string          `json:"stacks"`
	Packs          []string          `json:"packs"`
	OptionalAgents []string          `json:"optional_agents"`
	Architecture   Architecture      `json:"architecture"`
	Preferences    map[string]string `json:"preferences"`
}

// Setup records whether the initial configuration is complete and which
// questions are still pending. A project with pending items is operational;
// check reports them without failing.
type Setup struct {
	Complete bool     `json:"complete"`
	Pending  []string `json:"pending"`
}

// Host identifies the active host, how it was resolved and the evidence used.
// Source is one of env, evidence, flag or question.
type Host struct {
	ID       string   `json:"id"`
	Source   string   `json:"source"`
	Evidence []string `json:"evidence"`
}

// SpecEngine declares which specification engine the project uses and where it
// lives, so the engine never hardcodes a directory.
type SpecEngine struct {
	ID   string `json:"id"`
	Root string `json:"root"`
}

// Language separates the language of the binary from the language of the
// content it applies.
type Language struct {
	CLI   string `json:"cli"`
	User  string `json:"user"`
	Agent string `json:"agent"`
}

// Layout declares where the project keeps its documentation and decisions. A
// nil Decisions means the project has no decisions directory.
type Layout struct {
	Docs      string  `json:"docs"`
	Decisions *string `json:"decisions"`
}

// Hooks records whether the Git hooks are installed.
type Hooks struct {
	Installed bool `json:"installed"`
}

// Content records the version of the content the project was built from.
type Content struct {
	BundleVersion string `json:"bundle_version"`
}

// Architecture records the project's stance and any notes the human provided.
type Architecture struct {
	Stance string `json:"stance"`
	Notes  string `json:"notes"`
}

// Load reads and decodes a state file. A v1 state is mapped to the v2 shape
// but keeps SchemaVersion at 1, so callers still require an explicit migration.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var probe struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := checkReadableSchema(probe.SchemaVersion); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if probe.SchemaVersion >= SchemaVersion {
		var value State
		if err := json.Unmarshal(data, &value); err != nil {
			return nil, fmt.Errorf("decode %s: %w", path, err)
		}
		return &value, nil
	}

	var legacy legacyState
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("decode %s as schema v1: %w", path, err)
	}
	value, err := legacy.toState()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return value, nil
}

// Save encodes and writes a state file.
func Save(path string, value *State) error {
	if err := requireWritableSchema(value.SchemaVersion); err != nil {
		return err
	}
	return saveJSON(path, value.normalized())
}

func (s *State) normalized() State {
	out := *s
	out.Bundles = nonNil(out.Bundles)
	out.Setup.Pending = nonNil(out.Setup.Pending)
	out.Host.Evidence = nonNil(out.Host.Evidence)
	out.Stacks = nonNil(out.Stacks)
	out.Packs = nonNil(out.Packs)
	out.OptionalAgents = nonNil(out.OptionalAgents)
	if out.Preferences == nil {
		out.Preferences = map[string]string{}
	}
	return out
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func saveJSON(path string, value any) error {
	data, err := encode(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := fsutil.WriteFile(path, data); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func encode(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
