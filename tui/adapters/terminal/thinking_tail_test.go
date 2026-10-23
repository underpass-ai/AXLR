package terminal

import (
	"strings"
	"testing"
)

// The reasoning shown while the model thinks is folded to one paragraph
// and keeps its end, cut on a character boundary.
func TestThinkingTailKeepsTheEndOnOneParagraph(t *testing.T) {
	if got := thinkingTail("Plan the\n\n  page   now"); got != "Plan the page now" {
		t.Fatalf("folded %q", got)
	}
	long := strings.Repeat("é", 400) + " end"
	got := thinkingTail(long)
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, " end") || len(got) > 320+len("…") {
		t.Fatalf("cut %d bytes: %q", len(got), got[:20])
	}
	if strings.ContainsRune(got, '�') {
		t.Fatal("cut inside a character")
	}
}
