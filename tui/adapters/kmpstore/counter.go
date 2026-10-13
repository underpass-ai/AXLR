package kmpstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
)

// Counter counts a store's memories under an about.
type Counter struct{ CLI CLI }

// Count is how many events a store holds under an about, read with
// `kmp-mcp memories --json`, which lists every store without opening it
// (safe beside a running engine). known is false when the store or the
// about is not listed: a new store, or an about nothing was recorded
// under (decision J2: "N memories" or "new project").
func (c Counter) Count(ctx context.Context, selection Selection, about string, env []string) (count int, known bool, err error) {
	output, err := c.CLI.run(ctx, []string{"memories", "--json"}, WithStore(env, selection.Path))
	if err != nil {
		return 0, false, err
	}
	var listed struct {
		Memories []struct {
			Path   string `json:"path"`
			Abouts []struct {
				About  string `json:"about"`
				Events int    `json:"events"`
			} `json:"abouts"`
		} `json:"memories"`
	}
	start := bytes.IndexByte(output, '{')
	if start < 0 {
		return 0, false, errors.New("kmp-mcp memories printed no JSON")
	}
	if err := json.Unmarshal(output[start:], &listed); err != nil {
		return 0, false, fmt.Errorf("read kmp-mcp memories: %w", err)
	}
	want := filepath.Clean(selection.Path)
	for _, store := range listed.Memories {
		if filepath.Clean(store.Path) != want {
			continue
		}
		for _, entry := range store.Abouts {
			if entry.About == about {
				return entry.Events, true, nil
			}
		}
		return 0, false, nil
	}
	return 0, false, nil
}
