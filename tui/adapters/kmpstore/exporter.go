package kmpstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

// Exporter writes an about of a store as the checkout's bundle.
type Exporter struct{ CLI CLI }

// Export writes the store's events under one about as the checkout's
// bundle (/memory save, decision I3 of the lifecycle integration design),
// which a live engine on the store does not block. It returns the bundle's
// path and how many events it holds.
//
// The committed bundle may hold other abouts (a teammate's, written by
// another host): KMP 1.0 refuses, with exit 2, an export that would not
// keep every event of an existing .kmp/memory.jsonl, and until then the
// file was replaced with the one about alone. So the console first brings
// each of the bundle's abouts into the store (kmp-mcp import --from, one
// about at a time: KMP refuses a whole import when the store is ahead on
// one of them, which is the session's own about after any new memory),
// then exports them with the session's about. A refused catch-up is kept:
// when the export is refused as well, both reach the person as KMP wrote
// them, and nothing is written. It never passes --overwrite-committed.
func (e Exporter) Export(ctx context.Context, selection Selection, about string, env []string) (string, int, error) {
	if e.CLI.Binary == "" {
		return "", 0, errNoBinary
	}
	if selection.TopLevel == "" {
		return "", 0, errors.New("the workspace is not a git checkout: the bundle lives at <checkout>/" + BundlePath)
	}
	if strings.TrimSpace(about) == "" {
		return "", 0, errors.New("an about is required")
	}
	bundle := filepath.Join(selection.TopLevel, filepath.FromSlash(BundlePath))
	if err := os.MkdirAll(filepath.Dir(bundle), 0o755); err != nil {
		return "", 0, err
	}
	store := WithStore(env, selection.Path)
	abouts := []string{about}
	// An unreadable header is KMP's to judge: the export below keeps
	// every event of the file or is refused.
	committed, _ := bundleAbouts(bundle)
	var behind []string
	for _, name := range committed {
		if !slices.Contains(abouts, name) {
			abouts = append(abouts, name)
		}
		if output, err := e.CLI.run(ctx, []string{"import", "--from", bundle, "--about", name}, store); err != nil {
			behind = append(behind, name+": "+refusalText(output, err))
		}
	}
	args := []string{"export", bundle}
	for _, name := range abouts {
		args = append(args, "--about", name)
	}
	output, err := e.CLI.run(ctx, args, store)
	if err != nil {
		refused := fmt.Sprintf("KMP refused to write %s; nothing was written: %s", BundlePath, refusalText(output, err))
		if len(behind) > 0 {
			refused += " · The store could not take in what the committed bundle holds of " + strings.Join(behind, " · ")
		}
		return "", 0, errors.New(refused)
	}
	return bundle, exportedEvents(output), nil
}

// refusalText is what kmp-mcp said when a verb failed, whole up to 2 KiB
// (its refusal names what would be lost and the ways out); the command's
// error when it said nothing.
func refusalText(output []byte, err error) string {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return err.Error()
	}
	if len(text) > 2048 {
		cut := 2048
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut] + "…"
	}
	return text
}

// exportedEvents reads event_count from the JSON line kmp-mcp export
// prints last; -1 when it printed none.
func exportedEvents(output []byte) int {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if !bytes.HasPrefix(line, []byte("{")) {
			continue
		}
		var report struct {
			EventCount int `json:"event_count"`
		}
		if json.Unmarshal(line, &report) == nil {
			return report.EventCount
		}
	}
	return -1
}
