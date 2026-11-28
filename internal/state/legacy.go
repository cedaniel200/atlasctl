package state

import (
	"encoding/json"
	"fmt"
	"strings"
)

// legacyState is the state shape written by the 0.5 harness. Load maps it to
// the v2 shape but keeps SchemaVersion at 1 so commands still require an
// explicit migration before applying content.
type legacyState struct {
	BootstrapVersion int               `json:"bootstrap_version"`
	TemplateVersion  string            `json:"template_version"`
	Status           string            `json:"status"`
	Host             string            `json:"host"`
	HostSource       string            `json:"host_source"`
	Language         legacyLanguage    `json:"language"`
	Architecture     Architecture      `json:"architecture"`
	Profile          string            `json:"profile"`
	Stacks           json.RawMessage   `json:"stacks"`
	Packs            []string          `json:"packs"`
	OptionalAgents   []string          `json:"optional_agents"`
	Preferences      map[string]string `json:"preferences"`
	Docs             legacyDocs        `json:"docs"`
}

type legacyLanguage struct {
	User  string `json:"user"`
	Agent string `json:"agent"`
}

type legacyDocs struct {
	Root string `json:"root"`
}

func (l legacyState) toState() (*State, error) {
	stacks, err := legacyStacks(l.Stacks)
	if err != nil {
		return nil, err
	}
	cli := l.Language.User
	if cli == "" {
		cli = "en"
	}
	agent := l.Language.Agent
	if agent == "" {
		agent = "en"
	}
	complete := strings.EqualFold(strings.TrimSpace(l.Status), "configured")
	return &State{
		SchemaVersion: 1,
		EngineVersion: l.TemplateVersion,
		Bundles:       []string{"dev"},
		Setup:         Setup{Complete: complete, Pending: []string{}},
		Host: Host{
			ID:       l.Host,
			Source:   legacyHostSource(l.HostSource),
			Evidence: []string{},
		},
		Language:       Language{CLI: cli, User: l.Language.User, Agent: agent},
		Layout:         Layout{Docs: l.Docs.Root},
		MaxParallel:    MaxParallelAuto,
		Content:        Content{BundleVersion: l.TemplateVersion},
		Profile:        l.Profile,
		Stacks:         stacks,
		Packs:          l.Packs,
		OptionalAgents: l.OptionalAgents,
		Architecture:   l.Architecture,
		Preferences:    l.Preferences,
	}, nil
}

func legacyHostSource(source string) string {
	switch source {
	case "files":
		return "evidence"
	case "user":
		return "question"
	default:
		return source
	}
}

func legacyStacks(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []string{}, nil
	}
	var objects []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &objects); err == nil {
		stacks := make([]string, 0, len(objects))
		for _, stack := range objects {
			if stack.ID != "" {
				stacks = append(stacks, stack.ID)
			}
		}
		return stacks, nil
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err == nil {
		return names, nil
	}
	return nil, fmt.Errorf("state: unsupported stacks format")
}

// legacyManifest is the manifest shape written by the 0.5 harness. LoadManifest
// maps ownership to the v2 layer and keeps SchemaVersion at 1.
type legacyManifest struct {
	TemplateVersion string                         `json:"template_version"`
	GeneratedAt     json.RawMessage                `json:"generated_at"`
	Files           map[string]legacyManifestEntry `json:"files"`
}

type legacyManifestEntry struct {
	Origin    string `json:"origin"`
	SHA256    string `json:"sha256"`
	Ownership string `json:"ownership"`
}

func (m legacyManifest) toManifest() (*Manifest, error) {
	files := make(map[string]ManifestFile, len(m.Files))
	for path, entry := range m.Files {
		layer, err := legacyLayer(entry.Ownership)
		if err != nil {
			return nil, fmt.Errorf("manifest: %s: %w", path, err)
		}
		files[path] = ManifestFile{
			Bundle: "dev",
			Origin: entry.Origin,
			Layer:  layer,
			SHA256: entry.SHA256,
		}
	}
	manifest := &Manifest{
		SchemaVersion: 1,
		EngineVersion: m.TemplateVersion,
		Bundles:       []string{"dev"},
		Files:         files,
	}
	if len(m.GeneratedAt) > 0 && string(m.GeneratedAt) != "null" {
		if err := json.Unmarshal(m.GeneratedAt, &manifest.GeneratedAt); err != nil {
			return nil, fmt.Errorf("manifest: generated_at: %w", err)
		}
	}
	return manifest, nil
}

func legacyLayer(ownership string) (Layer, error) {
	switch ownership {
	case "managed":
		return LayerBase, nil
	case "owned":
		return LayerOwned, nil
	case "generated":
		return LayerGenerated, nil
	default:
		return "", fmt.Errorf("unknown ownership %q", ownership)
	}
}
