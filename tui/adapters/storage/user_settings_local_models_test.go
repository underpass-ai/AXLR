package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUserSettingsLoadLocalModelsAndKeepThemWhenSelectorsSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	data := `{"model":"local/qwen","context_tokens":65536,"local_models":[
	 {"id":"local/qwen","name":"Qwen3.8-27B","url":"http://127.0.0.1:8080/v1/","context_tokens":65536},
	 {"id":"local/gemma","url":"http://127.0.0.1:8082/v1","model":"gemma-4-31b","context_tokens":32768,"tools":false,"stream_idle_seconds":30,"stream_max_minutes":5}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	store, _ := NewUserSettingsStore(path, DefaultUserSettings())
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.ContextTokens != 65536 || len(got.LocalModels) != 2 {
		t.Fatalf("settings = %+v", got)
	}
	qwen, gemma := got.LocalModels[0], got.LocalModels[1]
	if qwen.ChatCompletionsURL() != "http://127.0.0.1:8080/v1/chat/completions" || qwen.UpstreamModel() != "local/qwen" || !qwen.SupportsTools() || qwen.StreamIdle() != 10*time.Minute || qwen.StreamMax() != time.Hour {
		t.Fatalf("qwen = %+v", qwen)
	}
	if gemma.UpstreamModel() != "gemma-4-31b" || gemma.SupportsTools() || gemma.StreamIdle() != 30*time.Second || gemma.StreamMax() != 5*time.Minute {
		t.Fatalf("gemma = %+v", gemma)
	}
	if err := store.ModelPreference().Save(context.Background(), "local/gemma"); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"local_models"`, `"gemma-4-31b"`, `"context_tokens": 65536`, `"model": "local/gemma"`} {
		if !strings.Contains(string(saved), want) {
			t.Fatalf("saved settings lost %s: %s", want, saved)
		}
	}
}

func TestUserSettingsRejectInvalidLocalModels(t *testing.T) {
	for name, local := range map[string]string{
		"missing window":  `{"id":"local/a","url":"http://127.0.0.1:8080/v1"}`,
		"tiny window":     `{"id":"local/a","url":"http://127.0.0.1:8080/v1","context_tokens":1024}`,
		"relative url":    `{"id":"local/a","url":"127.0.0.1:8080/v1","context_tokens":8192}`,
		"url with query":  `{"id":"local/a","url":"http://127.0.0.1:8080/v1?k=1","context_tokens":8192}`,
		"url credentials": `{"id":"local/a","url":"http://u:p@127.0.0.1:8080/v1","context_tokens":8192}`,
		"bad key env":     `{"id":"local/a","url":"http://127.0.0.1:8080/v1","context_tokens":8192,"api_key_env":"MY-KEY"}`,
		"empty id":        `{"id":"","url":"http://127.0.0.1:8080/v1","context_tokens":8192}`,
		"idle too long":   `{"id":"local/a","url":"http://127.0.0.1:8080/v1","context_tokens":8192,"stream_idle_seconds":4000}`,
		"duplicate":       `{"id":"local/a","url":"http://127.0.0.1:8080/v1","context_tokens":8192},{"id":"local/a","url":"http://127.0.0.1:8081/v1","context_tokens":8192}`,
	} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(`{"local_models":[`+local+`]}`), 0600); err != nil {
			t.Fatal(err)
		}
		store, _ := NewUserSettingsStore(path, DefaultUserSettings())
		if _, err := store.Load(context.Background()); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for _, cap := range []string{"100", "-1"} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(`{"context_tokens":`+cap+`}`), 0600); err != nil {
			t.Fatal(err)
		}
		store, _ := NewUserSettingsStore(path, DefaultUserSettings())
		if _, err := store.Load(context.Background()); err == nil {
			t.Errorf("context_tokens %s accepted", cap)
		}
	}
}

func TestUserSettingsJevSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"jev":{"tool":true,"final_check":false,"final_threshold":0.6,"timeout_ms":30000}}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, _ := NewUserSettingsStore(path, DefaultUserSettings())
	got, err := store.Load(context.Background())
	if err != nil || !got.Jev.Enabled() || !got.Jev.Tool || got.Jev.FinalThreshold != 0.6 || got.Jev.TimeoutMS != 30000 {
		t.Fatalf("jev = %+v, %v", got.Jev, err)
	}
	if (&JevSettings{}).Enabled() || (*JevSettings)(nil).Enabled() {
		t.Fatal("Jev enabled without a switch")
	}
	for _, jev := range []string{`{"final_threshold":1}`, `{"final_threshold":-0.1}`, `{"timeout_ms":10}`} {
		if err := os.WriteFile(path, []byte(`{"jev":`+jev+`}`), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(context.Background()); err == nil {
			t.Errorf("jev %s accepted", jev)
		}
	}
}
