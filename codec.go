package axlr

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"
)

func strictJSON(data []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON documents")
		}
		return err
	}
	return nil
}

func Decode(r io.Reader) (Request, error) {
	var req Request
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
	_, err = (RequestMapper{MaxReadBytes: hardFileBytes, MaxFileBytes: hardFileBytes, MaxOutputBytes: hardFileBytes, MaxTimeout: hardTimeout}).Map(req)
	return req, err
}
func MarshalResponse(r Response) ([]byte, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	if len(b) <= MaxResponseBytes {
		return b, nil
	}
	r.Status = "failed"
	r.Output = nil
	r.Error = &Failure{Code: "response_too_large", Message: "response exceeded 4 MiB; operation may have had effects"}
	return json.Marshal(r)
}
func ProtocolRejection(err error) Response {
	now := time.Now().UTC()
	return Response{ProtocolVersion: ProtocolVersion, Status: "rejected", StartedAt: now, FinishedAt: now, Error: &Failure{Code: "invalid_request", Message: err.Error()}}
}
