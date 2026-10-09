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
	if id.Kind == ToolKindHost && id.LocalOperation == HostOperationRemember {
		// axlr_remember is a kmp_write_memory: refused where that is.
		id = MemoryWriteIdentity
	}
	if (m == ModeIncident || m.ForgesPullRequest() || m == ModePlan || m == ModeTask) && id.Kind == ToolKindPlugin && id.Plugin.PluginID == "kmp" && id.Plugin.ToolName == "kmp_write_memory" {
		if m == ModePlan || m == ModeTask {
			return VerdictDeny, "plan and task modes: the console records the plan, each hand-back and each sync in KMP; do not write project memory yourself"
		}
		// Seen on 2 Oct 2026: the model recorded an unapproved draft. Only
		// the approved postmortem reaches memory, written by the console;
		// repair records its cause and outcome the same way.
		if m == ModeRepair {
			return VerdictDeny, "repair mode: the console records the diagnosed cause and the repair outcome in KMP; pass connect_to through axlr_step_done instead of writing project memory yourself"
		}
		if m == ModeImprove {
			return VerdictDeny, "improve mode: the console records the improvement outcome in KMP; do not write project memory yourself"
		}
		return VerdictDeny, "incident mode: the console records the approved postmortem in KMP after the person approves it; do not write project memory yourself"
	}
	if m == "" || m == ModeNormal || m.StartsCeremony() || m == ModeTask || id.Kind != ToolKindLocal {
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

// MemoryWriteIdentity is KMP's kmp_write_memory, which axlr_remember
// performs.
var MemoryWriteIdentity = ToolIdentity{Kind: ToolKindPlugin, Plugin: axlr.PluginRef{PluginID: "kmp", ToolName: "kmp_write_memory"}}

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
