package domain

type ReadResult struct {
	Content       string
	StartOffset   ByteOffset
	ReturnedBytes int
	NextOffset    ByteOffset
	Truncated     bool
	Digest        Digest
}
