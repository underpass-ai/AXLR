package typesafe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
)

func server(t *testing.T, handler func(w http.ResponseWriter, body map[string]any)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key-never-print" || r.Method != http.MethodPost {
			t.Errorf("request = %s %v", r.Method, r.Header)
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		handler(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func client(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := New(Config{APIKey: "key-never-print", Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestJudgeAsksAYesNoQuestion(t *testing.T) {
	srv := server(t, func(w http.ResponseWriter, body map[string]any) {
		q := body["questions"].(map[string]any)["q"].(map[string]any)
		if body["model"] != DefaultModel || body["state"] != "facts" || q["type"] != "noul" || q["instructions"] != "Done?" {
			t.Errorf("body = %v", body)
		}
		_, _ = io.WriteString(w, `{"model":"jev-1.13.0","answers":{"q":{"type":"noul","noul":0.25}},"usage":{"input_tokens":12}}`)
	})
	verdict, err := client(t, srv).Judge(context.Background(), application.JudgementQuestion{State: "facts", Question: "Done?"})
	if err != nil || verdict.Yes == nil || *verdict.Yes != 0.25 || verdict.Model != DefaultModel {
		t.Fatalf("verdict = %+v, %v", verdict, err)
	}
}

func TestJudgeAsksAChoice(t *testing.T) {
	srv := server(t, func(w http.ResponseWriter, body map[string]any) {
		q := body["questions"].(map[string]any)["q"].(map[string]any)
		criteria := q["criteria"].(map[string]any)
		if q["type"] != "choice" || len(criteria) != 2 {
			t.Errorf("body = %v", body)
		}
		_, _ = io.WriteString(w, `{"model":"jev-1.13.0","answers":{"q":{"type":"choice","choice":"b","probabilities":{"a":0.2,"b":0.8},"confidence":0.7}}}`)
	})
	verdict, err := client(t, srv).Judge(context.Background(), application.JudgementQuestion{State: "facts", Question: "Which?", Options: []string{"a", "b"}})
	if err != nil || verdict.Choice != "b" || verdict.Probabilities["b"] != 0.8 || *verdict.Confidence != 0.7 {
		t.Fatalf("verdict = %+v, %v", verdict, err)
	}
}

func TestJudgeRejectsAnswersThatDoNotMatchTheQuestion(t *testing.T) {
	for name, reply := range map[string]string{
		"other model":     `{"model":"jev-2","answers":{"q":{"type":"noul","noul":0.5}}}`,
		"wrong type":      `{"model":"jev-1.13.0","answers":{"q":{"type":"choice","choice":"a"}}}`,
		"out of range":    `{"model":"jev-1.13.0","answers":{"q":{"type":"noul","noul":1.5}}}`,
		"missing answer":  `{"model":"jev-1.13.0","answers":{}}`,
		"malformed":       `{"model":`,
		"extra questions": `{"model":"jev-1.13.0","answers":{"q":{"type":"noul","noul":0.5},"r":{"type":"noul","noul":0.5}}}`,
	} {
		srv := server(t, func(w http.ResponseWriter, _ map[string]any) { _, _ = io.WriteString(w, reply) })
		if _, err := client(t, srv).Judge(context.Background(), application.JudgementQuestion{State: "s", Question: "q?"}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	srv := server(t, func(w http.ResponseWriter, _ map[string]any) {
		_, _ = io.WriteString(w, `{"model":"jev-1.13.0","answers":{"q":{"type":"choice","choice":"c","probabilities":{"a":0.5,"b":0.5},"confidence":0.5}}}`)
	})
	if _, err := client(t, srv).Judge(context.Background(), application.JudgementQuestion{State: "s", Question: "q?", Options: []string{"a", "b"}}); err == nil {
		t.Error("a choice that was not offered was accepted")
	}
}

func TestJudgeRetriesRateLimitsAndNeverPrintsTheKey(t *testing.T) {
	var calls atomic.Int32
	srv := server(t, func(w http.ResponseWriter, _ map[string]any) {
		if calls.Add(1) < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, `{"model":"jev-1.13.0","answers":{"q":{"type":"noul","noul":0.9}}}`)
	})
	if _, err := client(t, srv).Judge(context.Background(), application.JudgementQuestion{State: "s", Question: "q?"}); err != nil || calls.Load() != 3 {
		t.Fatalf("calls=%d err=%v", calls.Load(), err)
	}
	denied := server(t, func(w http.ResponseWriter, _ map[string]any) { w.WriteHeader(http.StatusUnauthorized) })
	_, err := client(t, denied).Judge(context.Background(), application.JudgementQuestion{State: "s", Question: "q?"})
	if err == nil || strings.Contains(err.Error(), "key-never-print") || !strings.Contains(err.Error(), "401") {
		t.Fatalf("error = %v", err)
	}
}

func TestNewValidatesTheConfiguration(t *testing.T) {
	for name, config := range map[string]Config{
		"no key":        {},
		"latest":        {APIKey: "k", Model: "jev-latest"},
		"bad model":     {APIKey: "k", Model: "jev 1"},
		"short timeout": {APIKey: "k", Timeout: 10},
	} {
		if _, err := New(config); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
