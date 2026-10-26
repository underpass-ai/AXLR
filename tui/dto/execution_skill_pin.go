package dto

// ExecutionSkillPin is one catalog skill of an execution
// (docs/runtime-contract.md, "Skills per execution"): what `axlr-tui --exec
// --skills` reads and CreateSession takes, with the catalog optional when
// one catalog's install has the name; GetSession and the receipt show it
// resolved, with the catalog and the installed bundle's digest.
type ExecutionSkillPin struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Catalog      string `json:"catalog,omitempty"`
	BundleSHA256 string `json:"bundle_sha256,omitempty"`
}
