package codex

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestPluginCatalogListsPackagesAndInstallsExactSelector(t *testing.T) {
	var calls [][]string
	catalog := PluginCatalog{Run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		switch args[1] {
		case "list":
			return []byte(`{"installed":[{"pluginId":"made@made","name":"made","marketplaceName":"made","version":"0.8.0","installed":true,"enabled":true},{"pluginId":"kmp@underpass","name":"kmp","marketplaceName":"underpass","version":"0.24.0","installed":true,"enabled":true}],"available":[{"pluginId":"sample@market","name":"sample","marketplaceName":"market","version":"1.2.3","installed":false}]}`), nil
		case "add":
			return []byte(`{}`), nil
		case "marketplace":
			return []byte(`{}`), nil
		}
		return nil, errors.New("unexpected command")
	}}
	installed, err := catalog.List(context.Background(), false)
	if err != nil || len(installed) != 2 || installed[0].ID != "kmp@underpass" || installed[1].ID != "made@made" {
		t.Fatalf("installed: %+v %v", installed, err)
	}
	all, err := catalog.List(context.Background(), true)
	if err != nil || len(all) != 3 || all[2].ID != "sample@market" {
		t.Fatalf("available: %+v %v", all, err)
	}
	if err := catalog.Install(context.Background(), "sample@market"); err != nil {
		t.Fatal(err)
	}
	if err := catalog.AddMarketplace(context.Background(), "https://github.com/example/plugins"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, [][]string{{"plugin", "list", "--json"}, {"plugin", "list", "--available", "--json"}, {"plugin", "add", "sample@market", "--json"}, {"plugin", "marketplace", "add", "https://github.com/example/plugins", "--json"}}) {
		t.Fatalf("unexpected calls: %v", calls)
	}
	if err := catalog.Install(context.Background(), "--config=evil@market"); err == nil || len(calls) != 4 {
		t.Fatal("invalid selector reached CLI")
	}
	if err := catalog.AddMarketplace(context.Background(), "--config=evil"); err == nil || len(calls) != 4 {
		t.Fatal("invalid marketplace reached CLI")
	}
}
