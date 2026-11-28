package classify

import (
	"testing"

	"github.com/cedaniel200/atlasctl/internal/state"
)

func claim(path string, layer state.Layer) Claim {
	return Claim{
		Path: path,
		ManifestFile: state.ManifestFile{
			Bundle: "dev",
			Origin: "core",
			Layer:  layer,
		},
	}
}

func TestResolvePrefersOwnedOverOverridesOverBase(t *testing.T) {
	orders := [][]Claim{
		{
			claim("AGENTS.md", state.LayerBase),
			claim("AGENTS.md", state.LayerOverrides),
			claim("AGENTS.md", state.LayerOwned),
		},
		{
			claim("AGENTS.md", state.LayerOwned),
			claim("AGENTS.md", state.LayerBase),
			claim("AGENTS.md", state.LayerOverrides),
		},
		{
			claim("AGENTS.md", state.LayerOverrides),
			claim("AGENTS.md", state.LayerOwned),
			claim("AGENTS.md", state.LayerBase),
		},
	}

	for index, claims := range orders {
		got, ok := Resolve("AGENTS.md", claims)
		if !ok {
			t.Fatalf("order %d: Resolve() found no claim", index)
		}
		if got.Layer != state.LayerOwned {
			t.Errorf("order %d: layer = %q, want %q", index, got.Layer, state.LayerOwned)
		}
	}
}

func TestResolveOverridesWinsOverBase(t *testing.T) {
	base := Claim{
		Path: "AGENTS.md",
		ManifestFile: state.ManifestFile{
			Bundle: "dev",
			Origin: "core",
			Layer:  state.LayerBase,
			SHA256: "base",
		},
	}
	override := Claim{
		Path: "AGENTS.md",
		ManifestFile: state.ManifestFile{
			Bundle: "dev",
			Origin: "adopter",
			Layer:  state.LayerOverrides,
			SHA256: "override",
		},
	}

	for _, claims := range [][]Claim{{base, override}, {override, base}} {
		got, ok := Resolve("AGENTS.md", claims)
		if !ok {
			t.Fatal("Resolve() found no claim")
		}
		if got.Layer != state.LayerOverrides {
			t.Errorf("layer = %q, want %q", got.Layer, state.LayerOverrides)
		}
		if got.SHA256 != "override" {
			t.Errorf("sha256 = %q, want the override claim", got.SHA256)
		}
	}
}

func TestResolveIgnoresGeneratedClaims(t *testing.T) {
	if _, ok := Resolve("AGENTS.md", []Claim{claim("AGENTS.md", state.LayerGenerated)}); ok {
		t.Error("Resolve() accepted a generated-only path; generated content is regenerated, not resolved")
	}

	got, ok := Resolve("AGENTS.md", []Claim{
		claim("AGENTS.md", state.LayerGenerated),
		claim("AGENTS.md", state.LayerBase),
	})
	if !ok {
		t.Fatal("Resolve() found no claim for the base alongside the generated one")
	}
	if got.Layer != state.LayerBase {
		t.Errorf("layer = %q, want %q", got.Layer, state.LayerBase)
	}
}

func TestResolveIgnoresOtherPathsAndUnknownLayers(t *testing.T) {
	if _, ok := Resolve("AGENTS.md", []Claim{claim("README.md", state.LayerBase)}); ok {
		t.Error("Resolve() accepted a claim from another path")
	}
	if _, ok := Resolve("AGENTS.md", []Claim{claim("AGENTS.md", state.Layer("mystery"))}); ok {
		t.Error("Resolve() accepted a claim from an unknown layer")
	}
}
