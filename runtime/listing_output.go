package runtime

import (
	"encoding/json"
	"math"

	"github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/dto"
)

func (m ResponseMapper) searchOutput(v domain.SearchResult) dto.SearchOutput {
	out := dto.SearchOutput{Matches: make([]dto.SearchMatchOutput, 0, len(v.Matches)), FilesScanned: v.FilesScanned, FilesSkipped: v.FilesSkipped, LimitReached: v.LimitReached}
	sizes := make([]int, 0, len(v.Matches))
	for _, match := range v.Matches {
		item := dto.SearchMatchOutput{Path: match.Path, Line: match.Line, Column: match.Column, Text: match.Text, Before: match.Before, After: match.After, Truncated: match.Truncated}
		out.Matches = append(out.Matches, item)
		sizes = append(sizes, encodedBytes(item))
	}
	kept := m.fit(encodedBytes(dto.SearchOutput{Matches: []dto.SearchMatchOutput{}, FilesScanned: v.FilesScanned, FilesSkipped: v.FilesSkipped, NextOffset: math.MaxInt, Truncated: true, LimitReached: v.LimitReached}), sizes)
	out.Matches = out.Matches[:kept]
	out.NextOffset = nextOffset(v.Offset, kept, len(sizes), v.More)
	out.Truncated = out.NextOffset != 0 || v.LimitReached
	return out
}

func (m ResponseMapper) listOutput(v domain.ListResult) dto.ListOutput {
	out := dto.ListOutput{Entries: make([]dto.ListEntryOutput, 0, len(v.Entries)), LimitReached: v.LimitReached}
	sizes := make([]int, 0, len(v.Entries))
	for _, entry := range v.Entries {
		item := dto.ListEntryOutput{Path: entry.Path, Type: entry.Type}
		if entry.Type == domain.EntryFile {
			size := entry.Size
			item.Size = &size
		}
		out.Entries = append(out.Entries, item)
		sizes = append(sizes, encodedBytes(item))
	}
	kept := m.fit(encodedBytes(dto.ListOutput{Entries: []dto.ListEntryOutput{}, NextOffset: math.MaxInt, Truncated: true, LimitReached: v.LimitReached}), sizes)
	out.Entries = out.Entries[:kept]
	out.NextOffset = nextOffset(v.Offset, kept, len(sizes), v.More)
	out.Truncated = out.NextOffset != 0 || v.LimitReached
	return out
}

// fit is how many leading items, of the given encoded sizes, fit in
// ListingBytes after the page's envelope of base bytes; one always does, so
// paging never stalls on an item larger than the limit.
func (m ResponseMapper) fit(base int, sizes []int) int {
	if m.ListingBytes <= 0 {
		return len(sizes)
	}
	total := base
	for i, size := range sizes {
		total += size + 1 // the separating comma
		if total > m.ListingBytes && i > 0 {
			return i
		}
	}
	return len(sizes)
}

// nextOffset is where the following page starts, or 0 when none follows.
func nextOffset(offset, kept, found int, more bool) int {
	if kept < found || more {
		return offset + kept
	}
	return 0
}

func encodedBytes(v any) int {
	b, _ := json.Marshal(v)
	return len(b)
}
