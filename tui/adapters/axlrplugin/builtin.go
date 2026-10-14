package axlrplugin

import (
	"errors"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/tui/application"
)

// The console ships no built-in skill: the axlr-session skill, which asked
// the model to settle the session's context, read KMP's guide and compare
// abouts, was retired on 13 October 2026 (decision D8 of the lifecycle
// integration design) when the console took over the session's memory
// protocol; its title duty is one sentence of the system prompt and its
// inter-about reference is in docs/runbooks/kmp.md. The ceremony
// definitions the console drives are not a skill either: they live in
// tui/adapters/ceremonyhost/definitions and are published through /mcp → P.
// The axlr and made namespaces stay reserved, so a copied package cannot
// pose as the console's own guidance.

func builtinSkillIndex() (string, error) { return "", nil }

func readBuiltinSkill(string, string, string) ([]byte, error) {
	return nil, errors.New("built-in skill resource is unavailable")
}

func skillPage(plugin, skill, resource string, data []byte, offset, limit int) (application.SkillPage, error) {
	var page application.SkillPage
	if len(data) > maxSkillBytes || !utf8.Valid(data) {
		return page, errors.New("installed skill must be UTF-8 text of at most 1 MiB")
	}
	if offset > len(data) || offset < len(data) && !utf8.RuneStart(data[offset]) {
		return page, errors.New("offset_bytes must start at a UTF-8 character boundary")
	}
	end := min(offset+limit, len(data))
	for end > offset && end < len(data) && !utf8.RuneStart(data[end]) {
		end--
	}
	if end == offset && offset < len(data) {
		return page, errors.New("limit_bytes is too small for the next UTF-8 character")
	}
	return application.SkillPage{Plugin: plugin, Skill: skill, Path: resource, OffsetBytes: offset, NextOffsetBytes: end, TotalBytes: len(data), HasMore: end < len(data), Text: string(data[offset:end])}, nil
}
