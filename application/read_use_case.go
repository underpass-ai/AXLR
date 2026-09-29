package application

import "github.com/underpass-ai/AXLR/domain"

type ReadUseCase struct{ Files FilePort }

func (u ReadUseCase) Execute(c domain.ReadCommand) (domain.ReadResult, error) { return u.Files.Read(c) }
