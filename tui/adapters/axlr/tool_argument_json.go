package axlr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
)

// strictToolJSON preserves all numbers and rejects duplicate keys with
// different values at every depth. The schema validator and executor
// therefore see the same argument. A key repeated with the same value says
// one thing and is kept once: a local model behind llama.cpp sent
// "stdin":"" three times and "args" twice in one local_exec, refused for
// it, and spent a request sending it again (wrx80, 20 November 2026).
func strictToolJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := readToolJSON(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("extra JSON data")
	}
	return value, nil
}

func readToolJSON(decoder *json.Decoder, depth int) (any, error) {
	if depth > 128 {
		return nil, errors.New("JSON nesting exceeds 128 levels")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		object := map[string]any{}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := token.(string)
			if !ok {
				return nil, errors.New("JSON object key must be a string")
			}
			value, err := readToolJSON(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			if existing, exists := object[key]; exists {
				if !reflect.DeepEqual(existing, value) {
					return nil, fmt.Errorf("duplicate JSON key %q with different values", key)
				}
				continue
			}
			object[key] = value
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return object, nil
	case json.Delim('['):
		array := []any{}
		for decoder.More() {
			value, err := readToolJSON(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return array, nil
	case json.Delim('}'), json.Delim(']'):
		return nil, errors.New("unexpected JSON delimiter")
	default:
		return token, nil
	}
}

// canonicalArguments is raw with every key once: the runtime and an MCP
// server decode strictly and refuse a repeated key, which strictToolJSON
// admits when its values are the same. Arguments it refuses are left as
// they are, for the receiver to refuse.
func canonicalArguments(raw []byte) []byte {
	value, err := strictToolJSON(raw)
	if err != nil {
		return raw
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return encoded
}
