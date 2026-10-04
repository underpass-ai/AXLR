package domain

import (
	"encoding/json"
	"path"
	"path/filepath"
	"strings"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// ModeVerdict is what a work mode allows for one resolved tool call.
type ModeVerdict int

const (
	VerdictAllow ModeVerdict = iota
	// VerdictAsk requires the user's decision even when autonomy is on.
	VerdictAsk
	VerdictDeny
)

var documentExtensions = map[string]bool{".md": true, ".mdx": true, ".txt": true, ".rst": true}

// Judge applies the mode to a resolved call. Plugin and host tools are not
// workspace changes and stay under their own approval policy.
func (m WorkMode) Judge(id ToolIdentity, arguments axlr.JSONValue) (ModeVerdict, string) {
	if m == ModeIncident && id.Kind == ToolKindPlugin && id.Plugin.PluginID == "kmp" && id.Plugin.ToolName == "kmp_write_memory" {
		// Seen on 2 Oct 2026: the model recorded an unapproved draft. Only
		// the approved postmortem reaches memory, written by the console.
		return VerdictDeny, "incident mode: the console records the approved postmortem in KMP after the person approves it; do not write project memory yourself"
	}
	if m == "" || m == ModeNormal || m.StartsCeremony() || id.Kind != ToolKindLocal {
		return VerdictAllow, ""
	}
	switch id.LocalOperation {
	case "exec":
		// The host cannot see what a command does, so a human approves each
		// one. Code piped through stdin hides below the approval card's fold,
		// which a model used on 2 Oct 2026 to rewrite a file in writer mode;
		// restricted modes therefore refuse it outright.
		var command struct {
			Stdin string `json:"stdin"`
		}
		if json.Unmarshal(arguments.Bytes(), &command) == nil && command.Stdin != "" {
			return VerdictDeny, string(m) + " mode refuses commands that read stdin; put the command in args where the user can review it"
		}
		return VerdictAsk, ""
	case "write", "edit":
		if m == ModeReview {
			return VerdictDeny, "review mode does not change files"
		}
		var target struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(arguments.Bytes(), &target) != nil || !documentPath(target.Path) {
			return VerdictDeny, string(m) + " mode writes only documents: .md, .mdx, .txt, .rst or files under docs/"
		}
	}
	return VerdictAllow, ""
}

// HidesWriteTools reports modes whose model never sees write tools at all.
func (m WorkMode) HidesWriteTools() bool { return m == ModeReview }

func documentPath(raw string) bool {
	relative, err := axlr.NewRelativePath(raw)
	if err != nil {
		return false
	}
	clean := path.Clean(filepath.ToSlash(string(relative)))
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return false
	}
	return documentExtensions[strings.ToLower(path.Ext(clean))] || strings.HasPrefix(clean, "docs/")
}
