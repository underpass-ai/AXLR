package domain

import (
	"bytes"
	"encoding/json"
	"errors"
)

type JSONValue struct{ raw []byte }

func NewJSONValue(raw []byte) (JSONValue, error) {
	if !json.Valid(raw) {
		return JSONValue{}, errors.New("invalid JSON value")
	}
	return JSONValue{raw: append([]byte(nil), raw...)}, nil
}

func NewJSONObject(raw []byte) (JSONValue, error) {
	v, err := NewJSONValue(raw)
	if err != nil {
		return JSONValue{}, err
	}
	if !v.isObject() {
		return JSONValue{}, errors.New("plugin arguments must be a JSON object")
	}
	return v, nil
}

func (v JSONValue) Bytes() []byte { return append([]byte(nil), v.raw...) }

func (v JSONValue) isObject() bool {
	trimmed := bytes.TrimSpace(v.raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func (v JSONValue) MarshalJSON() ([]byte, error) {
	if len(v.raw) == 0 {
		return []byte("null"), nil
	}
	return v.Bytes(), nil
}
