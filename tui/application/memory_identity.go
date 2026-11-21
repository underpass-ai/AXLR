package application

// MemoryIdentity is what this launch reads from the memory engine for the
// session's recall note: where it reads and whose memory it adds. KMP 1.0
// keeps no agents or contexts, so the console registers nothing.
type MemoryIdentity struct {
	// Recall wakes memory for the recall note; nil appends none.
	Recall RecallPort
	// UserAbout is the person's about, user:<login name>, woken with the
	// project's (decision F6); "" when the login name is unknown.
	UserAbout string
}
