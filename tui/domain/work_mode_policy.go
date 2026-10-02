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
	if m == "" || m == ModeNormal || id.Kind != ToolKindLocal {
		return VerdictAllow, ""
	}
	switch id.LocalOperation {
	case "exec":
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
