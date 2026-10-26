package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// SkillRequest is a pin as an execution asks for it: the catalog may be
// left out when one catalog's install has the name.
type SkillRequest struct {
	Catalog, Name, Version string
}

// ResolveSkillPins turns an execution's requested pins into the session's:
// each one installed, at the version asked, from the catalog named or the
// only catalog whose install has it, with the installed bundle's digest. A
// pin this host cannot serve wraps domain.ErrSkillsUnavailable: an
// execution never installs.
func ResolveSkillPins(ctx context.Context, installed InstalledSkillsPort, requests []SkillRequest) (domain.ExecutionSkills, error) {
	skills := domain.ExecutionSkills{Pins: []domain.SkillPin{}}
	if len(requests) == 0 {
		return skills, nil
	}
	if installed == nil {
		return domain.ExecutionSkills{}, fmt.Errorf("%w: catalog skills are not enabled", domain.ErrSkillsUnavailable)
	}
	have, err := installed.List(ctx)
	if err != nil {
		return domain.ExecutionSkills{}, err
	}
	for _, request := range requests {
		var matches []InstalledSkill
		catalogs := map[string]bool{}
		for _, skill := range have {
			if skill.Name != request.Name || (request.Catalog != "" && skill.Catalog != request.Catalog) {
				continue
			}
			catalogs[skill.Catalog] = true
			if skill.Version == request.Version {
				matches = append(matches, skill)
			}
		}
		switch {
		case len(matches) == 1:
		case len(matches) > 1:
			names := make([]string, 0, len(matches))
			for _, skill := range matches {
				names = append(names, skill.Catalog)
			}
			return domain.ExecutionSkills{}, fmt.Errorf("%w: %s %s is installed from catalogs %s; name the catalog", domain.ErrSkillsUnavailable, request.Name, request.Version, strings.Join(names, ", "))
		case len(catalogs) > 0:
			return domain.ExecutionSkills{}, fmt.Errorf("%w: %s %s is not installed (installed: %s)", domain.ErrSkillsUnavailable, request.Name, request.Version, installedVersions(have, request))
		default:
			return domain.ExecutionSkills{}, fmt.Errorf("%w: %s is not installed", domain.ErrSkillsUnavailable, request.Name)
		}
		skill := matches[0]
		skills.Pins = append(skills.Pins, domain.SkillPin{Catalog: skill.Catalog, Name: skill.Name, Version: skill.Version, BundleSHA256: skill.BundleSHA256})
	}
	// A skill pinned twice is an invalid request, not an unavailable
	// skill: the error does not wrap domain.ErrSkillsUnavailable.
	if err := skills.Validate(); err != nil {
		return domain.ExecutionSkills{}, err
	}
	return skills, nil
}

func installedVersions(have []InstalledSkill, request SkillRequest) string {
	var versions []string
	for _, skill := range have {
		if skill.Name == request.Name && (request.Catalog == "" || skill.Catalog == request.Catalog) {
			versions = append(versions, skill.Version)
		}
	}
	sort.Slice(versions, func(i, j int) bool { return newerVersion(versions[j], versions[i]) })
	return strings.Join(versions, ", ")
}

type sessionSkillsKey struct{}

// WithSessionSkills carries the session's pinned skills to the skill index
// and to axlr_skill, so one console or service serves sessions pinned to
// different versions; a session without pins carries nothing and reads the
// newest installed.
func WithSessionSkills(ctx context.Context, s domain.Session) context.Context {
	skills, has := s.Skills()
	if !has {
		return ctx
	}
	return context.WithValue(ctx, sessionSkillsKey{}, skills)
}

// SessionSkillsOf is the pinned skills of the session being served, if it
// has them.
func SessionSkillsOf(ctx context.Context) (domain.ExecutionSkills, bool) {
	skills, ok := ctx.Value(sessionSkillsKey{}).(domain.ExecutionSkills)
	return skills, ok
}

// SessionSkillReader reads the catalog skills the session in the context
// may read: exactly its pins, or the newest installed version of each.
type SessionSkillReader struct {
	Installed InstalledSkillsPort
}

// Visible lists the session's catalog skills, by catalog then name. A pin
// whose version was removed since is left out of the list; reading it says
// why.
func (r SessionSkillReader) Visible(ctx context.Context) ([]InstalledSkill, error) {
	if r.Installed == nil {
		return nil, nil
	}
	installed, err := r.Installed.List(ctx)
	if err != nil {
		return nil, err
	}
	skills, pinned := SessionSkillsOf(ctx)
	if !pinned {
		return NewestInstalled(installed), nil
	}
	var visible []InstalledSkill
	for _, pin := range skills.Pins {
		for _, skill := range installed {
			if skill.Catalog == pin.Catalog && skill.Name == pin.Name && skill.Version == pin.Version {
				visible = append(visible, skill)
			}
		}
	}
	sortInstalled(visible)
	return visible, nil
}

// Read returns one file of the session's version of a catalog skill.
func (r SessionSkillReader) Read(ctx context.Context, catalog, name, file string) (InstalledSkill, []byte, error) {
	if r.Installed == nil {
		return InstalledSkill{}, nil, errors.New("catalog skills are not enabled in this console")
	}
	if skills, pinned := SessionSkillsOf(ctx); pinned {
		pin, has := skills.Pin(catalog, name)
		if !has {
			return InstalledSkill{}, nil, fmt.Errorf("this execution's skills do not include %s of catalog %s", name, catalog)
		}
		visible, err := r.Visible(ctx)
		if err != nil {
			return InstalledSkill{}, nil, err
		}
		for _, skill := range visible {
			if skill.Catalog == catalog && skill.Name == name {
				data, err := r.Installed.Read(ctx, catalog, name, skill.Version, file)
				return skill, data, err
			}
		}
		return InstalledSkill{}, nil, fmt.Errorf("%s %s of catalog %s, which this execution pins, is no longer installed", name, pin.Version, catalog)
	}
	visible, err := r.Visible(ctx)
	if err != nil {
		return InstalledSkill{}, nil, err
	}
	for _, skill := range visible {
		if skill.Catalog == catalog && skill.Name == name {
			data, err := r.Installed.Read(ctx, catalog, name, skill.Version, file)
			return skill, data, err
		}
	}
	return InstalledSkill{}, nil, fmt.Errorf("no skill %s is installed from catalog %s", name, catalog)
}

// BuiltinToolNames are the tools every console offers by name: the local
// tools and the host tools. A skill's tools are checked against them, the
// workspace's forged and catalog tools and nothing else: an MCP tool is
// marked missing when no list names it.
func BuiltinToolNames() []string {
	names := []string{"local_read", "local_write", "local_edit", "local_exec", "local_search", "local_list"}
	for _, tool := range HostTools() {
		names = append(names, string(tool.Definition.Name))
	}
	for _, name := range []string{"axlr_judge", "axlr_remember", "axlr_recall", "axlr_logs", "axlr_catalog", "axlr_install_tool", "axlr_forge_tool", "axlr_run_tool", "axlr_remove_tool", "axlr_publish_tool", "axlr_deliver"} {
		names = append(names, name)
	}
	return names
}
