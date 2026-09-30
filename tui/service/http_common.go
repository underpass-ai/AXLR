package service

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
)

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{16,128}$`)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, request string, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": request}})
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	contentType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
		writeError(w, requestID(r), http.StatusUnsupportedMediaType, "unsupported_media_type", "application/json is required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			writeError(w, requestID(r), http.StatusRequestEntityTooLarge, "body_too_large", "request body exceeds 4 MiB")
		} else {
			writeError(w, requestID(r), http.StatusBadRequest, "invalid_json", "invalid JSON request")
		}
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		writeError(w, requestID(r), http.StatusBadRequest, "invalid_json", "trailing JSON data")
		return false
	}
	return true
}

func requireRole(w http.ResponseWriter, r *http.Request, role string) bool {
	if principal(r).Can(role) {
		return true
	}
	writeError(w, requestID(r), http.StatusForbidden, "forbidden", "insufficient role")
	return false
}

func requireKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := r.Header.Get("Idempotency-Key")
	if !keyPattern.MatchString(key) {
		writeError(w, requestID(r), http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key must be 16–128 safe ASCII characters")
		return "", false
	}
	return key, true
}
