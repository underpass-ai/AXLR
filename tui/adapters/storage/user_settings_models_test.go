package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func loadSettingsText(t *testing.T, data string) (*UserSettingsStore, UserSettings, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := NewUserSettingsStore(path, DefaultUserSettings())
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.Load(context.Background())
	return store, settings, err
}

func sameJSON(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var actual, expected any
	if err := json.Unmarshal(got, &actual); err != nil {
		t.Fatalf("invalid JSON %s: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}

// The options load per exact model id, and saving another setting keeps
// them, like any other section.
func TestUserSettingsLoadModelOptionsAndKeepThemWhenSelectorsSave(t *testing.T) {
	store, settings, err := loadSettingsText(t, `{"model":"z-ai/glm-5.3-flash","future":{"kept":true},"models":{
	 "z-ai/glm-5.3-flash":{"provider":{"sort":"throughput","ignore":["openinference"],"max_price":{"completion":0.6},"allow_fallbacks":false},"reasoning":{"effort":"low","exclude":true},"max_tokens":8192},
	 "anthropic/claude-haiku-5.5":{"reasoning":{"max_tokens":2048}},
	 "openai/gpt-4o":{"provider":{"sort":{"by":"latency","partition":"none"},"data_collection":"deny","zdr":true,"quantizations":["fp8","bf16"],"preferred_min_throughput":50,"preferred_max_latency":2,"order":["openai","azure"],"only":["openai","azure"],"require_parameters":true}}}}`)
	if err != nil {
		t.Fatal(err)
	}
	check := func(settings UserSettings) {
		t.Helper()
		options := settings.ModelRequestOptions()
		if len(options) != 3 {
			t.Fatalf("options = %+v", options)
		}
		glm := options["z-ai/glm-5.3-flash"]
		sameJSON(t, glm.Provider, `{"sort":"throughput","ignore":["openinference"],"max_price":{"completion":0.6},"allow_fallbacks":false}`)
		sameJSON(t, glm.Reasoning, `{"effort":"low","exclude":true}`)
		if glm.MaxTokens != 8192 {
			t.Fatalf("glm = %+v", glm)
		}
		haiku := options["anthropic/claude-haiku-5.5"]
		sameJSON(t, haiku.Reasoning, `{"max_tokens":2048}`)
		if haiku.Provider != nil || haiku.MaxTokens != 0 {
			t.Fatalf("haiku = %+v", haiku)
		}
	}
	check(settings)
	if _, duplicated := settings.Extra["models"]; duplicated {
		t.Fatal("models is kept a second time among unknown keys")
	}
	if err := store.ModelPreference().Save(context.Background(), "openai/gpt-4o"); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load(context.Background())
	if err != nil || saved.Model != "openai/gpt-4o" {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	sameJSON(t, saved.Extra["future"], `{"kept":true}`)
	check(saved)
}

func TestUserSettingsWithoutModelOptionsHaveNone(t *testing.T) {
	store, settings, err := loadSettingsText(t, `{"model":"z-ai/glm-5.3-flash"}`)
	if err != nil || len(settings.ModelRequestOptions()) != 0 {
		t.Fatalf("options = %+v, %v", settings.ModelRequestOptions(), err)
	}
	if err := store.ModelPreference().Save(context.Background(), "openai/gpt-4o"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.Path)
	if err != nil || strings.Contains(string(data), "models") {
		t.Fatalf("saved %s, %v", data, err)
	}
}

// Every refusal names what is wrong, down to the key, so a misspelt
// option stops the launch instead of leaving the routing as it was.
func TestUserSettingsRejectInvalidModelOptions(t *testing.T) {
	for name, tc := range map[string]struct{ options, want string }{
		"not an object":            {`"fast"`, "invalid settings.json JSON"},
		"entry not an object":      {`{"z-ai/glm":5}`, "settings models z-ai/glm must be a JSON object"},
		"invalid id":               {`{" ":{}}`, "settings models has an invalid model id"},
		"unknown entry key":        {`{"z-ai/glm":{"providr":{}}}`, `settings models z-ai/glm has unknown key "providr"`},
		"case variant":             {`{"z-ai/glm":{"Provider":{}}}`, `has unknown key "Provider"`},
		"provider not an object":   {`{"z-ai/glm":{"provider":"inferencenet"}}`, "settings models z-ai/glm provider must be a JSON object"},
		"unknown provider key":     {`{"z-ai/glm":{"provider":{"sortt":"price"}}}`, `settings models z-ai/glm provider has unknown key "sortt"`},
		"null value":               {`{"z-ai/glm":{"provider":{"sort":null}}}`, "provider.sort must not be null"},
		"null provider":            {`{"z-ai/glm":{"provider":null}}`, "z-ai/glm provider must not be null"},
		"sort":                     {`{"z-ai/glm":{"provider":{"sort":"cheapest"}}}`, "provider.sort must be price, throughput or latency, or an object"},
		"sort number":              {`{"z-ai/glm":{"provider":{"sort":1}}}`, "provider.sort must be price"},
		"sort by":                  {`{"z-ai/glm":{"provider":{"sort":{"by":"cost"}}}}`, "provider.sort.by must be price, throughput or latency"},
		"sort without by":          {`{"z-ai/glm":{"provider":{"sort":{"partition":"none"}}}}`, "provider.sort needs by"},
		"sort unknown key":         {`{"z-ai/glm":{"provider":{"sort":{"by":"price","direction":"asc"}}}}`, `provider.sort has unknown key "direction"`},
		"sort partition":           {`{"z-ai/glm":{"provider":{"sort":{"by":"price","partition":""}}}}`, "provider.sort.partition must be a name"},
		"data collection":          {`{"z-ai/glm":{"provider":{"data_collection":"maybe"}}}`, "provider.data_collection must be allow or deny"},
		"quantization":             {`{"z-ai/glm":{"provider":{"quantizations":["int3"]}}}`, "provider.quantizations accepts only int4"},
		"no quantizations":         {`{"z-ai/glm":{"provider":{"quantizations":[]}}}`, "provider.quantizations must be a non-empty list"},
		"empty slug":               {`{"z-ai/glm":{"provider":{"order":[""]}}}`, "provider.order has an empty or invalid provider slug"},
		"control in slug":          {`{"z-ai/glm":{"provider":{"ignore":["open\u0007"]}}}`, "provider.ignore has an empty or invalid provider slug"},
		"padded slug":              {`{"z-ai/glm":{"provider":{"only":[" relace"]}}}`, "provider.only has an empty or invalid provider slug"},
		"empty list":               {`{"z-ai/glm":{"provider":{"only":[]}}}`, "provider.only must be a list of 1 to 64 provider slugs"},
		"slug not a string":        {`{"z-ai/glm":{"provider":{"order":[1]}}}`, "provider.order must be a list"},
		"fallbacks type":           {`{"z-ai/glm":{"provider":{"allow_fallbacks":"no"}}}`, "provider.allow_fallbacks must be true or false"},
		"zdr type":                 {`{"z-ai/glm":{"provider":{"zdr":1}}}`, "provider.zdr must be true or false"},
		"negative price":           {`{"z-ai/glm":{"provider":{"max_price":{"completion":-1}}}}`, "provider.max_price.completion must be a number of at least 0"},
		"price as text":            {`{"z-ai/glm":{"provider":{"max_price":{"prompt":"1"}}}}`, "provider.max_price.prompt must be a number"},
		"unknown price":            {`{"z-ai/glm":{"provider":{"max_price":{"output":1}}}}`, `provider.max_price has unknown key "output"`},
		"throughput type":          {`{"z-ai/glm":{"provider":{"preferred_min_throughput":"fast"}}}`, "provider.preferred_min_throughput must be a number"},
		"negative latency":         {`{"z-ai/glm":{"provider":{"preferred_max_latency":-2}}}`, "provider.preferred_max_latency must be a number of at least 0"},
		"effort":                   {`{"z-ai/glm":{"reasoning":{"effort":"extreme"}}}`, "reasoning.effort must be max, xhigh, high, medium, low, minimal or none"},
		"effort and budget":        {`{"z-ai/glm":{"reasoning":{"effort":"low","max_tokens":100}}}`, "reasoning sets both effort and max_tokens"},
		"reasoning budget":         {`{"z-ai/glm":{"reasoning":{"max_tokens":0}}}`, "reasoning.max_tokens must be a whole number between 1 and 1000000"},
		"reasoning unknown":        {`{"z-ai/glm":{"reasoning":{"budget":100}}}`, `reasoning has unknown key "budget"`},
		"reasoning switch":         {`{"z-ai/glm":{"reasoning":{"enabled":"yes"}}}`, "reasoning.enabled must be true or false"},
		"max tokens zero":          {`{"z-ai/glm":{"max_tokens":0}}`, "settings models z-ai/glm max_tokens must be a whole number between 1 and 1000000"},
		"max tokens too large":     {`{"z-ai/glm":{"max_tokens":1000001}}`, "max_tokens must be a whole number"},
		"max tokens fraction":      {`{"z-ai/glm":{"max_tokens":1.5}}`, "max_tokens must be a whole number"},
		"first error in id order":  {`{"b/model":{"max_tokens":0},"a/model":{"x":1}}`, `settings models a/model has unknown key "x"`},
		"reasoning not an object":  {`{"z-ai/glm":{"reasoning":true}}`, "reasoning must be a JSON object"},
		"max price not an object":  {`{"z-ai/glm":{"provider":{"max_price":1}}}`, "provider.max_price must be a JSON object"},
		"sort object not complete": {`{"z-ai/glm":{"provider":{"sort":{}}}}`, "provider.sort needs by"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := loadSettingsText(t, `{"models":`+tc.options+`}`)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

// A local server receives none of these fields, so an entry for a local
// model is refused instead of being ignored.
func TestUserSettingsRejectModelOptionsForLocalModels(t *testing.T) {
	_, _, err := loadSettingsText(t, `{"local_models":[{"id":"local/qwen","url":"http://127.0.0.1:8080/v1","context_tokens":65536}],"models":{"local/qwen":{"max_tokens":512}}}`)
	if err == nil || !strings.Contains(err.Error(), "settings models local/qwen is a local model") {
		t.Fatalf("error = %v", err)
	}
}

func TestUserSettingsBoundTheNumberOfModelOptions(t *testing.T) {
	entries := make([]string, 0, maxModelOptions+1)
	for i := range maxModelOptions + 1 {
		entries = append(entries, fmt.Sprintf(`"vendor/model-%d":{"max_tokens":100}`, i))
	}
	if _, _, err := loadSettingsText(t, `{"models":{`+strings.Join(entries[:maxModelOptions], ",")+`}}`); err != nil {
		t.Fatalf("%d entries refused: %v", maxModelOptions, err)
	}
	if _, _, err := loadSettingsText(t, `{"models":{`+strings.Join(entries, ",")+`}}`); err == nil || !strings.Contains(err.Error(), "more than 64 models") {
		t.Fatalf("error = %v", err)
	}
}
