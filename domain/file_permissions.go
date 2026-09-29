package domain

type FilePermissions uint32

func NewFilePermissions(bits uint32) FilePermissions { return FilePermissions(bits & 0777) }
