package axlrplugin

import (
	"embed"
	"errors"
	"io/fs"
	"strings"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/tui/application"
)

const builtinSessionSkill = "axlr-session"
const builtinSessionRoot = "builtin/axlr/skills/" + builtinSessionSkill + "/"

// Built-in guidance is available without a copied package or a live MCP engine.
// The ceremony definitions the console drives are not a skill: they live in
// tui/adapters/ceremonyhost/definitions and are published through /mcp → P.
//
//go:embed builtin/axlr/skills/axlr-session
var builtinResources embed.FS

func builtinSkillIndex() (string, error) {
	data, err := builtinResources.ReadFile(builtinSessionRoot + "SKILL.md")
	if err != nil {
		return "", err
	}
	return "\n- axlr:" + builtinSessionSkill + ": " + skillDescription(data), nil
}

func readBuiltinSkill(plugin, skill, resource string) ([]byte, error) {
	root := ""
	if plugin == "axlr" && skill == builtinSessionSkill {
		root = builtinSessionRoot
	}
	if root == "" || !fs.ValidPath(resource) ||
		(resource != "SKILL.md" && resource != "agents/openai.yaml" && !strings.HasPrefix(resource, "references/")) {
		return nil, errors.New("built-in skill resource is unavailable")
	}
	return builtinResources.ReadFile(root + resource)
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
