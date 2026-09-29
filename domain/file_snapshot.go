package domain

type FileSnapshot struct {
	Content     []byte
	Permissions FilePermissions
}
