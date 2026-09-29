package domain

import (
	"bytes"
	"testing"
)

func TestPluginIDRejectsInvalidNames(t *testing.T) {
	for _, input := range []string{"", "1plugin", "bad.name", "bad space", "ñ"} {
		if _, err := NewPluginID(input); err == nil {
			t.Errorf("accepted plugin ID %q", input)
		}
	}
	if id, err := NewPluginID("search_2"); err != nil || id.String() != "search_2" {
		t.Fatalf("valid ID: %q, %v", id, err)
	}
}

func TestPluginToolNameRejectsInvalidNames(t *testing.T) {
	for _, input := range []string{"", "bad/name", "bad space", string(bytes.Repeat([]byte{'x'}, 129))} {
		if _, err := NewPluginToolName(input); err == nil {
			t.Errorf("accepted tool name %q", input)
		}
	}
	if name, err := NewPluginToolName("find.v2"); err != nil || name.String() != "find.v2" {
		t.Fatalf("valid name: %q, %v", name, err)
	}
}

func TestJSONValueOwnsValidatedBytes(t *testing.T) {
	input := []byte(`{"query":"a"}`)
	value, err := NewJSONObject(input)
	if err != nil {
		t.Fatal(err)
	}
	input[10] = 'b'
	first := value.Bytes()
	first[10] = 'c'
	if got := string(value.Bytes()); got != `{"query":"a"}` {
		t.Fatalf("JSON was mutated: %s", got)
	}
	for _, bad := range [][]byte{[]byte(`[]`), []byte(`null`), []byte(`{"a":`)} {
		if _, err := NewJSONObject(bad); err == nil {
			t.Errorf("accepted arguments %s", bad)
		}
	}
	if _, err := NewJSONValue([]byte(`{"a":1} trailing`)); err == nil {
		t.Fatal("accepted multiple JSON values")
	}
}
