package domain

import axlr "github.com/underpass-ai/AXLR/domain"

type ToolOutcome struct {
	Content   axlr.Text
	IsError   bool
	Uncertain bool
}
