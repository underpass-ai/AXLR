package main

import (
	"bytes"
	"encoding/json"
	"github.com/underpass-ai/AXLR"
	"testing"
)

func TestWorkerEmitsOnlyOneJSONResponse(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--root", t.TempDir(), "--profile", "trusted-local"}, bytes.NewBufferString(`{"protocol_version":1,"request_id":"x","tool":"exec","arguments":{"program":"/bin/sh","args":["-c","printf child-output"]}}`), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code %d stderr %q", code, stderr.String())
	}
	var result axlr.Response
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || result.Output == nil {
		t.Fatalf("%+v", result)
	}
	if bytes.Contains(stdout.Bytes(), []byte("child-output\n")) {
		t.Fatalf("unframed child output: %q", stdout.String())
	}
}

func TestWorkerRejectsInvalidRequestWithExitTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--root", t.TempDir(), "--profile", "trusted-local"}, bytes.NewBufferString(`{}`), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code %d", code)
	}
	var result axlr.Response
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "rejected" {
		t.Fatalf("%+v", result)
	}
}

func TestWorkerAcceptsExplicitChildEnvironment(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--root", t.TempDir(), "--profile", "trusted-local", "--env", "AXLR_FLAG=visible"}, bytes.NewBufferString(`{"protocol_version":1,"request_id":"x","tool":"exec","arguments":{"program":"/bin/sh","args":["-c","printf %s \"$AXLR_FLAG\""]}}`), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code %d stderr %s", code, stderr.String())
	}
	var response struct {
		Output axlr.ExecOutput `json:"output"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Output.Stdout != "visible" {
		t.Fatalf("%+v", response)
	}
}

func TestWorkerRejectsMissingProfile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", t.TempDir()}, bytes.NewBuffer(nil), &stdout, &stderr); code != 1 {
		t.Fatalf("code %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected protocol output %q", stdout.String())
	}
}
