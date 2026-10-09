package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/dto"
)

func strictJSON(data []byte, dst any) error {
	if err := validateJSONText(data); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	start, err := d.Token()
	if err != nil {
		return err
	}
	if start != json.Delim('{') {
		return errors.New("expected JSON object")
	}
	typ := reflect.TypeOf(dst)
	if typ.Kind() != reflect.Pointer || typ.Elem().Kind() != reflect.Struct {
		return errors.New("invalid DTO target")
	}
	allowed := make(map[string]bool)
	for i := 0; i < typ.Elem().NumField(); i++ {
		tag := strings.Split(typ.Elem().Field(i).Tag.Get("json"), ",")[0]
		if tag != "" && tag != "-" {
			allowed[tag] = true
		}
	}
	seen := make(map[string]bool)
	for d.More() {
		keyToken, err := d.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("invalid JSON key")
		}
		if !allowed[key] {
			return fmt.Errorf("unknown field %q", key)
		}
		if seen[key] {
			return fmt.Errorf("duplicate field %q", key)
		}
		seen[key] = true
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return err
		}
	}
	if _, err := d.Token(); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON documents")
		}
		return err
	}
	return json.Unmarshal(data, dst)
}

// validateJSONText prevents encoding/json from silently replacing malformed Unicode.
func validateJSONText(data []byte) error {
	if !utf8.Valid(data) {
		return errors.New("JSON contains invalid UTF-8")
	}
	inString := false
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '"':
			inString = !inString
		case '\\':
			if !inString {
				continue
			}
			i++
			if i >= len(data) {
				return errors.New("invalid JSON escape")
			}
			if data[i] != 'u' {
				continue
			}
			if i+4 >= len(data) {
				return errors.New("invalid Unicode escape")
			}
			value, ok := hex4(data[i+1 : i+5])
			if !ok {
				return errors.New("invalid Unicode escape")
			}
			if value >= 0xdc00 && value <= 0xdfff {
				return errors.New("unpaired low surrogate")
			}
			if value >= 0xd800 && value <= 0xdbff {
				if i+10 >= len(data) || data[i+5] != '\\' || data[i+6] != 'u' {
					return errors.New("unpaired high surrogate")
				}
				low, ok := hex4(data[i+7 : i+11])
				if !ok || low < 0xdc00 || low > 0xdfff {
					return errors.New("unpaired high surrogate")
				}
				i += 10
			} else {
				i += 4
			}
		}
	}
	return nil
}

func hex4(data []byte) (uint16, bool) {
	var value uint16
	for _, c := range data {
		value <<= 4
		switch {
		case c >= '0' && c <= '9':
			value += uint16(c - '0')
		case c >= 'a' && c <= 'f':
			value += uint16(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			value += uint16(c - 'A' + 10)
		default:
			return 0, false
		}
	}
	return value, true
}

func Decode(r io.Reader) (dto.Request, error) {
	var req dto.Request
	b, err := io.ReadAll(io.LimitReader(r, MaxRequestBytes+1))
	if err != nil {
		return req, err
	}
	if len(b) > MaxRequestBytes {
		return req, errors.New("request exceeds 4 MiB")
	}
	if err := strictJSON(b, &req); err != nil {
		return req, err
	}
	_, err = (RequestMapper{MaxReadBytes: hardFileBytes, MaxFileBytes: hardFileBytes, MaxOutputBytes: hardFileBytes, MaxTimeout: HardTimeout}).Map(req)
	return req, err
}
func MarshalResponse(r dto.Response) ([]byte, error) {
	if r.RequestID != "" {
		if _, err := domain.NewRequestID(r.RequestID); err != nil {
			r.RequestID = ""
		}
	}
	switch r.Tool {
	case "", "read", "write", "edit", "exec", "search", "list", "plugins.list", "plugins.call":
	default:
		r.Tool = ""
	}
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	if len(b) <= MaxResponseBytes {
		return b, nil
	}
	r.Status = "failed"
	r.Output = nil
	r.Error = &dto.Failure{Code: "response_too_large", Message: "response exceeded 4 MiB; operation may have had effects"}
	b, err = json.Marshal(r)
	if err != nil || len(b) > MaxResponseBytes {
		return nil, errors.New("unable to bound response")
	}
	return b, nil
}
func ProtocolRejection(err error) dto.Response {
	now := time.Now().UTC()
	return dto.Response{ProtocolVersion: ProtocolVersion, Status: "rejected", StartedAt: now, FinishedAt: now, Error: &dto.Failure{Code: "invalid_request", Message: err.Error()}}
}
