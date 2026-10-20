package axlrplugin

import (
	"embed"
	"errors"
	"io/fs"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/tui/application"
)

// The console's own skills ship inside the binary, readable without a
// copied package or a live MCP engine. The axlr-session skill was retired
// on 13 October 2026 (decision D8 of the lifecycle integration design)
// when the console took over the session's memory protocol. On 20 October
// 2026 axlr-publish-tool joined: the person asked the agent to publish a
// forged tool and it did not know the files a registry needs. The ceremony
// definitions the console drives are not a skill: they live in
// tui/adapters/ceremonyhost/definitions and are published through /mcp → P.
// The axlr and made namespaces stay reserved, so a copied package cannot
// pose as the console's own guidance.

const builtinPublishSkill = "axlr-publish-tool"
const builtinPublishRoot = "builtin/axlr/skills/" + builtinPublishSkill + "/"

//go:embed builtin/axlr/skills/axlr-publish-tool
var builtinResources embed.FS

func builtinSkillIndex() (string, error) {
	data, err := builtinResources.ReadFile(builtinPublishRoot + "SKILL.md")
	if err != nil {
		return "", err
	}
	return "\n- axlr: The console's own skills. " + builtinPublishSkill + " (" + skillDescription(string(data)) + ")", nil
}

func readBuiltinSkill(plugin, skill, resource string) ([]byte, error) {
	if plugin != "axlr" || skill != builtinPublishSkill || !fs.ValidPath(resource) || resource != "SKILL.md" {
		return nil, errors.New("built-in skill resource is unavailable")
	}
	return builtinResources.ReadFile(builtinPublishRoot + resource)
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
