package terminal

import (
	"fmt"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
)

func TestUnsupportedPlatformExplainsWhereUpdatesWork(t *testing.T) {
	for locale, want := range map[Locale]string{English: "update them by hand", Spanish: "actualízalos a mano"} {
		got := engineUpdateContent(nil, fmt.Errorf("check: %w", application.ErrEngineUpdatePlatform), Theme{Locale: locale, Monochrome: true})
		if !strings.Contains(got, want) || !strings.Contains(got, "Linux") || strings.Contains(got, "no official") {
			t.Fatalf("%v: %q", locale, got)
		}
	}
}
