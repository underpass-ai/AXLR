package application

// CompareRequest asks for KMP's proposals between the session's about and
// the others named.
type CompareRequest struct {
	About  string
	Others []string
}
