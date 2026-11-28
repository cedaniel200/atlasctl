package state

import "time"

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

// LoadManifest reads and decodes a manifest file.
func LoadManifest(path string) (*Manifest, error) {
	var value Manifest
	if err := loadJSON(path, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

// SaveManifest encodes and writes a manifest file.
func SaveManifest(path string, value *Manifest) error {
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
