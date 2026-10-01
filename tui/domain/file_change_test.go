package domain

import (
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
)

func TestFileChangeRejectsInvalidOrPartialEvidence(t *testing.T) {
	for _, c := range []FileChange{
		{Path: "../outside"},
		{Path: "file", Unavailable: "unknown"},
		{Path: "file", Unavailable: "too_large", Before: "partial"},
		{Path: "file", Created: true, Before: "not new"},
		{Path: "file", After: root.Text(strings.Repeat("x", MaxChangePreviewBytes+1))},
		{Path: "file", Before: "binary\x00"},
	} {
		if c.Validate() == nil {
			t.Fatalf("accepted invalid review evidence: %+v", c)
		}
	}
	for _, c := range []FileChange{{Path: "file", Created: true}, {Path: "file", Before: "old", After: "new"}, {Path: "file", Unavailable: "too_large"}} {
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}
