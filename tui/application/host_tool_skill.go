package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	root "github.com/underpass-ai/AXLR/domain"
)

var skillSelector = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func (u HostToolUseCase) readSkill(ctx context.Context, arguments root.JSONValue) (any, error) {
	if u.Skills == nil {
		return nil, errors.New("installed plugin skills are unavailable")
	}
	args, err := decodeHostArguments(arguments, "plugin", "catalog", "skill", "path", "offset_bytes", "limit_bytes")
	if err != nil {
		return nil, err
	}
	reconcileSkillOrigin(args)
	_, hasPlugin := args["plugin"]
	_, hasCatalog := args["catalog"]
	if !hasPlugin && !hasCatalog {
		return nil, errors.New("name either plugin, for a plugin's or the console's skill, or catalog, for a catalog skill; give the other one as an empty string")
	}
	var plugin, catalog, skill string
	if hasCatalog {
		if err := json.Unmarshal(args["catalog"], &catalog); err != nil || !catalogName.MatchString(catalog) {
			if !hasPlugin {
				return nil, errors.New("catalog must be the catalog of an installed skill")
			}
			hasCatalog = false
		}
	}
	if hasPlugin {
		if err := json.Unmarshal(args["plugin"], &plugin); err != nil || !skillSelector.MatchString(plugin) {
			if !hasCatalog {
				return nil, errors.New("plugin must be an installed plugin name")
			}
			hasPlugin = false
		}
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
	if !hasCatalog {
		return u.Skills.ReadSkill(ctx, plugin, skill, path, offset, limit)
	}
	var catalogErr error
	if reader, ok := u.Skills.(CatalogSkillPagePort); ok {
		page, err := reader.ReadCatalogSkill(ctx, catalog, skill, path, offset, limit)
		if err == nil || !hasPlugin {
			return page, err
		}
		catalogErr = err
	} else if !hasPlugin {
		return nil, errors.New("catalog skills are unavailable in this console")
	} else {
		catalogErr = errors.New("catalog skills are unavailable in this console")
	}
	// Both named, as a model behind a strict schema must: the catalog's skill
	// is not there, so it is the plugin's or the console's skill.
	page, err := u.Skills.ReadSkill(ctx, plugin, skill, path, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("%s is neither a skill of catalog %s (%v) nor of plugin %s (%v); for a plugin's or the console's skill give catalog as an empty string", skill, catalog, catalogErr, plugin, err)
	}
	return page, nil
}

// reconcileSkillOrigin reads a model that fills every field of the tool, as
// the Responses models behind a strict schema do: an empty or null origin is
// no origin. When both are still named, readSkill tries the catalog's skill,
// whose version the session pins, and then the plugin's or the console's.
func reconcileSkillOrigin(args map[string]json.RawMessage) {
	for _, key := range []string{"plugin", "catalog"} {
		var name *string
		if raw, ok := args[key]; ok && json.Unmarshal(raw, &name) == nil && (name == nil || *name == "") {
			delete(args, key)
		}
	}
}
