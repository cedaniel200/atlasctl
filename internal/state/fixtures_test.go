package state

import (
	"strings"
	"testing"
	"time"
)

func sampleManifest(t *testing.T) *Manifest {
	t.Helper()
	return &Manifest{
		SchemaVersion: SchemaVersion,
		EngineVersion: "0.6.0",
		GeneratedAt:   time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC),
		Bundles:       []string{"dev"},
		Files: map[string]ManifestFile{
			"AGENTS.md": {
				Bundle: "dev",
				Origin: "core",
				Layer:  LayerBase,
				SHA256: strings.Repeat("a", 64),
			},
			".opencode/agents/analyst.md": {
				Bundle: "dev",
				Origin: "host:opencode",
				Layer:  LayerGenerated,
				SHA256: strings.Repeat("b", 64),
			},
		},
	}
}
