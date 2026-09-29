package terminal

import "github.com/underpass-ai/AXLR/tui/domain"

type operationComplete struct {
	Session domain.Session
	Err     error
}
