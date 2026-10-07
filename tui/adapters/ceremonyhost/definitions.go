package ceremonyhost

import (
	"embed"
	"fmt"
)

//go:embed definitions/*.yaml
var definitionFiles embed.FS

// Definition is one console-driven ceremony definition AXLR ships and pins.
type Definition struct {
	Name, Version string
	// Digest is MADE's semantic digest of the published definition. A YAML
	// change must update it, or the pin test fails.
	Digest string
}

var definitions = []Definition{
	{Name: "axlr_debug", Version: "2.0", Digest: "345f57fdab6165ad65e3847ee8336206161c433eac053dd1afdb17bc496c14c3"},
	{Name: "axlr_delivery", Version: "2.0", Digest: "257e2dd1bc461f5d799ce6c40e888f882b39f833510c98a3d03ccaaeafee88a2"},
	{Name: "axlr_incident", Version: "1.0", Digest: "f81e9aa53f7d77c72086ac3baec1a4037df1d0fa3d24e62270e5b62fabe6f6df"},
	{Name: "axlr_repair", Version: "1.0", Digest: "62264d45ed6cf59b18b1c9e4a6cd01285cf8e52b7b894f251cee8edc2e91161e"},
	{Name: "axlr_plan", Version: "1.0", Digest: "379619a6f6b49c799f64b43e23c2686507d30f1b1b27e368d14f512d52ef2ad4"},
}

// Definitions lists the pinned definitions.
func Definitions() []Definition { return append([]Definition(nil), definitions...) }

// YAML is the exact definition text published by the /mcp → P action.
func (d Definition) YAML() (string, error) {
	data, err := definitionFiles.ReadFile(fmt.Sprintf("definitions/%s-%s.yaml", d.Name, d.Version))
	return string(data), err
}

func pinned(name, version string) (Definition, bool) {
	for _, d := range definitions {
		if d.Name == name && d.Version == version {
			return d, true
		}
	}
	return Definition{}, false
}
