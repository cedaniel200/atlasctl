package state

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Layer is the ownership layer of a managed file. A file is only ever owned by
// one layer.
type Layer string

const (
	// LayerBase is content published by the engine and replaced while intact.
	LayerBase Layer = "base"
	// LayerOverrides is adopter content that wins over a path the engine also
	// publishes.
	LayerOverrides Layer = "overrides"
	// LayerGenerated is content the engine derives from the state and always
	// regenerates.
	LayerGenerated Layer = "generated"
	// LayerOwned is adopter content on a path the engine never claims.
	LayerOwned Layer = "owned"
)

// Manifest is the file inventory stored in .atlas/manifest.json.
type Manifest struct {
	SchemaVersion int                     `json:"schema_version"`
	EngineVersion string                  `json:"engine_version"`
	GeneratedAt   time.Time               `json:"generated_at"`
	Bundles       []string                `json:"bundles"`
	Files         map[string]ManifestFile `json:"files"`
}

// ManifestFile records the bundle, origin, layer and expected content hash of a
// file written by the engine. SHA256 is the hash of the content with line
// endings normalized to LF.
type ManifestFile struct {
	Bundle string `json:"bundle"`
	Origin string `json:"origin"`
	Layer  Layer  `json:"layer"`
	SHA256 string `json:"sha256"`
}

// LoadManifest reads and decodes a manifest file. A v1 manifest is mapped to
// the v2 shape but keeps SchemaVersion at 1, so callers still require an
// explicit migration.
func LoadManifest(path string) (*Manifest, error) {
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
		var value Manifest
		if err := json.Unmarshal(data, &value); err != nil {
			return nil, fmt.Errorf("decode %s: %w", path, err)
		}
		return &value, nil
	}

	var legacy legacyManifest
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("decode %s as schema v1: %w", path, err)
	}
	value, err := legacy.toManifest()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return value, nil
}

// SaveManifest encodes and writes a manifest file.
func SaveManifest(path string, value *Manifest) error {
	if err := requireWritableSchema(value.SchemaVersion); err != nil {
		return err
	}
	return saveJSON(path, value.normalized())
}

func (m *Manifest) normalized() Manifest {
	out := *m
	out.Bundles = nonNil(out.Bundles)
	if out.Files == nil {
		out.Files = map[string]ManifestFile{}
	}
	return out
}
