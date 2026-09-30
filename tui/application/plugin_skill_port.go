package application

import "context"

// SkillPage is a bounded page of a text resource in an installed AXLR plugin.
type SkillPage struct {
	Plugin          string `json:"plugin"`
	Skill           string `json:"skill"`
	Path            string `json:"path"`
	OffsetBytes     int    `json:"offset_bytes"`
	NextOffsetBytes int    `json:"next_offset_bytes"`
	TotalBytes      int    `json:"total_bytes"`
	HasMore         bool   `json:"has_more"`
	Text            string `json:"text"`
}

type PluginSkillPort interface {
	ReadSkill(context.Context, string, string, string, int, int) (SkillPage, error)
}
