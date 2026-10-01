package storage

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
)

func TestModelFavoritesToggleAndPreserveOtherSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"ink","future_key":{"x":1}}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := NewUserSettingsStore(path, DefaultUserSettings())
	if err != nil {
		t.Fatal(err)
	}
	favorites := store.ModelFavorites()
	ctx := context.Background()
	for _, id := range []root.ModelID{"anthropic/claude-opus-4.5", "z-ai/glm-5.3-flash"} {
		if _, err := favorites.Toggle(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	got, err := favorites.Toggle(ctx, "anthropic/claude-opus-4.5")
	if err != nil || !reflect.DeepEqual(got, []root.ModelID{"z-ai/glm-5.3-flash"}) {
		t.Fatalf("toggle = %v, %v", got, err)
	}
	loaded, err := favorites.Load(ctx)
	if err != nil || !reflect.DeepEqual(loaded, got) {
		t.Fatalf("load = %v, %v", loaded, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"future_key"`) || !strings.Contains(string(data), `"theme": "ink"`) && !strings.Contains(string(data), `"theme":"ink"`) {
		t.Fatalf("other settings lost: %s", data)
	}
	if _, err := favorites.Toggle(ctx, "  "); err == nil {
		t.Fatal("invalid model accepted")
	}
}
