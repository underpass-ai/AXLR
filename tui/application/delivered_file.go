package application

// DeliveredFile is what DeliverablesPort read of a deliverable's bytes:
// their size and hex sha256.
type DeliveredFile struct {
	Size   int64
	SHA256 string
}
