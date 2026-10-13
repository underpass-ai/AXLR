package application

// MemoryIdentity is this launch's identity in the memory engine: the
// store it opened, the install's registration key and where registrations
// are kept (decision D3 of the lifecycle integration design).
type MemoryIdentity struct {
	// Store is the resolved store path, the key of the agent registry.
	Store string
	// RegistrationKey is axlr:<install id>; the same key names the same
	// agent in a store.
	RegistrationKey string
	Agents          MemoryAgentsPort
	Guide           MemoryIdentityPort
	// Recall wakes memory for the recall note; nil appends none.
	Recall RecallPort
	// UserAbout is the person's about, user:<login name>, woken with the
	// project's (decision F6); "" when the login name is unknown.
	UserAbout string
}

func (i *MemoryIdentity) usable() bool {
	return i != nil && i.Store != "" && i.RegistrationKey != "" && i.Agents != nil && i.Guide != nil
}
