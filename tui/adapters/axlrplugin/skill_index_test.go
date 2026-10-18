package axlrplugin

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillDescriptionReadsEveryScalarForm(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"---\nname: a\ndescription: Plain text, with commas\n---\n", "Plain text, with commas"},
		{"---\ndescription: \"Use for \\\"interrogate\\\", \\\"tear this apart\\\".\"\n---\n", `Use for "interrogate", "tear this apart".`},
		{"---\ndescription: 'It''s quoted'\n---\n", "It's quoted"},
		{"---\ndescription: >-\n  Folded over\n  two lines\nname: b\n---\n", "Folded over two lines"},
		{"---\ndescription: |\n  Literal\n\n  block\n---\n", "Literal block"},
		{"\ufeff---\ndescription: After a byte order mark\n---\n", "After a byte order mark"},
		{"---\nname: none\n---\ndescription: in the body\n", ""},
		{"# No front matter\ndescription: x\n", ""},
		{"---\nmetadata:\n  description: nested\n---\n", ""},
	} {
		if got := skillDescription(c.text); got != c.want {
			t.Errorf("skillDescription(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// installSkills writes a package with count skills whose descriptions are
// size bytes long.
func installSkills(t *testing.T, root, id string, count, size int) {
	t.Helper()
	dir := filepath.Join(root, "plugins", "installed", id)
	mustWrite(t, filepath.Join(dir, ".codex-plugin", "plugin.json"), `{"name":"`+id+`","version":"1.0.0","description":"Package `+id+`","skills":"./skills/"}`)
	for i := range count {
		description := strings.Repeat("word ", size/5)
		mustWrite(t, filepath.Join(dir, "skills", fmt.Sprintf("skill-%02d", i), "SKILL.md"), "---\ndescription: "+description+"\n---\nBody.\n")
	}
}

// One clip length applies to every package: the longest under the cap.
func TestSkillIndexShortensDescriptionsToStayUnderTheCap(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		count int
		clip  int
	}{{10, 120}, {110, 80}, {160, 48}, {400, 0}} {
		root := t.TempDir()
		installSkills(t, root, "big", c.count, 400)
		installSkills(t, root, "small", 2, 400)
		catalog := &Catalog{Root: root}
		guidance, err := catalog.Guidance(ctx)
		if err != nil {
			t.Fatal(err)
		}
		index := guidance[strings.Index(guidance, "\n- "):]
		if len(index) > skillIndexBytes+1 {
			t.Fatalf("%d skills: index of %d bytes", c.count, len(index))
		}
		small := guidance[strings.Index(guidance, "\n- small"):]
		if c.clip == 0 {
			if strings.Contains(guidance, "(word") || !strings.Contains(small, "Skills: skill-00, skill-01.") {
				t.Fatalf("%d skills: expected names only: %q", c.count, small)
			}
			continue
		}
		want := "skill-00 (" + clipText(strings.Repeat("word ", 80), c.clip) + "); "
		if !strings.Contains(small, want) {
			t.Fatalf("%d skills: want clip %d, got %q", c.count, c.clip, small)
		}
		again, err := catalog.Guidance(ctx)
		if err != nil || again != guidance {
			t.Fatal("the index changed between two requests")
		}
	}
}

// When names alone exceed the cap, the packages that fit are kept whole.
func TestSkillIndexKeepsWholePackagesWhenNamesOverflow(t *testing.T) {
	root := t.TempDir()
	installSkills(t, root, "alpha", 2, 40)
	installSkills(t, root, "beta", 1200, 40)
	guidance, err := (&Catalog{Root: root}).Guidance(context.Background())
	if err != nil || !strings.Contains(guidance, "- alpha: Package alpha. Skills: skill-00, skill-01.") || strings.Contains(guidance, "beta") {
		t.Fatalf("guidance: %q %v", guidance, err)
	}
}
