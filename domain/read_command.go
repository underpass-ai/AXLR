package domain

type ReadCommand struct {
	Path   RelativePath
	Offset ByteOffset
	Limit  ByteLimit
}
