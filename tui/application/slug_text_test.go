package application

import (
	"regexp"
	"testing"
)

// A plan's id keeps the words of a Spanish brief: "Añade en buildinfo…"
// named its plan "a-ade-en-adc4da" on 12 October 2026.
func TestPlanSlugSpellsAccentedLettersInASCII(t *testing.T) {
	slug := planSlug("Añade en buildinfo IsDev()")
	if !regexp.MustCompile(`^anade-en-buildinfo-[0-9a-f]{6}$`).MatchString(slug) {
		t.Fatalf("slug %q", slug)
	}
	for brief, want := range map[string]string{"Façade Straße Œuvre": "Facade Strasse OEuvre", "Ünïcödé ŁÓDŹ": "Unicode LODZ", "日本 ok": "日本 ok"} {
		if got := foldToASCII(brief); got != want {
			t.Fatalf("fold %q = %q, want %q", brief, got, want)
		}
	}
}
