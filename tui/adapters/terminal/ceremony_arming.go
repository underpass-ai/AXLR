package terminal

// ceremonyArm is what /ceremony asked to arm once the definitions are
// listed: a definition, its version ("" for the newest) and a brief.
type ceremonyArm struct {
	name, version, brief string
}
