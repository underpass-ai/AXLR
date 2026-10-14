package application

// HostIdentity is how this console names itself to MADE.
type HostIdentity struct {
	// Owner is the lease owner of every claim; MADE accepts a status report
	// only from the claim's owner, so it is the host agent id as well
	// (seen on 13 October 2026: "agent status reporter is not the current
	// claim owner" for any other host agent id).
	Owner string
	// AgentID is the install id and Incarnation the console run's token;
	// together they are the incarnation MADE records.
	AgentID, Incarnation string
	Hostname             string
}

// incarnation is the host incarnation as reported: install and run.
func (h HostIdentity) incarnation() string {
	switch {
	case h.AgentID == "" && h.Incarnation == "":
		return "unknown"
	case h.AgentID == "":
		return h.Incarnation
	case h.Incarnation == "":
		return h.AgentID
	}
	return h.AgentID + ":" + h.Incarnation
}
