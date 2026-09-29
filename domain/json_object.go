package domain

import (
	"bytes"
	"encoding/json"
	"errors"
)

type JSONObject struct{ raw []byte }

func NewJSONObject(raw []byte) (JSONObject, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return JSONObject{}, errors.New("expected JSON object")
	}
	return JSONObject{raw: append([]byte(nil), trimmed...)}, nil
}

func (o JSONObject) Bytes() []byte { return append([]byte(nil), o.raw...) }

func (o JSONObject) MarshalJSON() ([]byte, error) {
	if len(o.raw) == 0 {
		return nil, errors.New("empty JSON object")
	}
	return o.Bytes(), nil
}

func (o JSONObject) valid() bool { return len(o.raw) > 0 }
