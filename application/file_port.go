package application

import "github.com/underpass-ai/AXLR/domain"

type FilePort interface {
	Read(domain.ReadCommand) (domain.ReadResult, error)
	Load(domain.RelativePath, int) (domain.FileSnapshot, error)
	Create(domain.RelativePath, []byte) error
	Replace(domain.RelativePath, []byte, domain.FilePermissions) error
}
