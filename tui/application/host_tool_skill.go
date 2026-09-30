package application

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"

	root "github.com/underpass-ai/AXLR/domain"
)

var skillSelector = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func (u HostToolUseCase) readSkill(ctx context.Context, arguments root.JSONValue) (any, error) {
	if u.Skills == nil {
		return nil, errors.New("installed plugin skills are unavailable")
	}
	args, err := decodeHostArguments(arguments, "plugin", "skill", "path", "offset_bytes", "limit_bytes")
	if err != nil {
		return nil, err
	}
	var plugin, skill string
	if err := json.Unmarshal(args["plugin"], &plugin); err != nil || !skillSelector.MatchString(plugin) {
		return nil, errors.New("plugin must be an installed plugin name")
	}
	if err := json.Unmarshal(args["skill"], &skill); err != nil || !skillSelector.MatchString(skill) {
		return nil, errors.New("skill must be an installed skill name")
	}
	path := "SKILL.md"
	if raw, ok := args["path"]; ok {
		if err := json.Unmarshal(raw, &path); err != nil || path == "" {
			return nil, errors.New("path must be a relative text file in the installed plugin")
		}
	}
	offset, err := hostInteger(args, "offset_bytes", 0)
	if err != nil || offset < 0 {
		return nil, errors.New("offset_bytes must be a nonnegative integer")
	}
	limit, err := hostInteger(args, "limit_bytes", 4096)
	if err != nil || limit < 1 || limit > 4096 {
		return nil, errors.New("limit_bytes must be between 1 and 4096")
	}
	return u.Skills.ReadSkill(ctx, plugin, skill, path, offset, limit)
}
