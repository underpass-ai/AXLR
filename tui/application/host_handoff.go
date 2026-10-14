package application

// HostHandoff is the declaration that a claim's incarnation is gone: the
// claim's fence and owner, the incarnation that held it, and the evidence
// (the earlier run, the host and the time of its last report).
type HostHandoff struct {
	Instance, Step, Fence string
	// Incarnation is the one that held the claim; State is lost.
	Incarnation string
	Evidence    string
}
