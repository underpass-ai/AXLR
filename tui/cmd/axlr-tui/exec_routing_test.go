package main

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// routedHosts answers like a stopped local server on 127.0.0.1:18080 when
// down, and like a model everywhere else; it records the hosts asked for
// completions, and refuses OpenRouter's catalog.
func routedHosts(t *testing.T, localDown bool) *[]string {
	t.Helper()
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	var mu sync.Mutex
	var hosts []string
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Body != nil {
			_, _ = io.ReadAll(r.Body)
		}
		if strings.HasSuffix(r.URL.Path, "/models") {
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
		}
		mu.Lock()
		hosts = append(hosts, r.URL.Host)
		mu.Unlock()
		if r.URL.Host == "127.0.0.1:18080" && localDown {
			return nil, errors.New("dial tcp 127.0.0.1:18080: connect: connection refused")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + answerChunk + "\n\ndata: [DONE]\n\n")), Header: make(http.Header)}, nil
	})
	return &hosts
}

const routingSettings = `{"local_models":[{"id":"qwen-local","url":"http://127.0.0.1:18080/v1","context_tokens":32768}]}`

// cost sends the request to the local model; a stopped local server
// hands it, once, to the remote candidate, and the --json lines, the
// receipt and stderr say so; with remote providers forbidden the run ends
// failed without a request off the host.
func TestExecRoutesItsRequestsAmongItsCandidates(t *testing.T) {
	env := cliEnv(t)
	writeSettings(t, env, routingSettings)
	routing := writePolicy(t, `{"routing": {"candidates": ["qwen-local", "anthropic/claude-haiku-5.5"], "objective": "cost"}}`)
	hosts := routedHosts(t, false)
	code, stdout, stderr := execRun(t, env, "--root", t.TempDir(), "--exec", "say done", "--routing", routing, "--json")
	if code != execCompleted || strings.Join(*hosts, ",") != "127.0.0.1:18080" || !strings.Contains(stderr, "routing cost among qwen-local, anthropic/claude-haiku-5.5") {
		t.Fatalf("code=%d hosts=%v stderr=%s", code, *hosts, stderr)
	}
	lines := execLines(t, stdout)
	routed := []string{}
	for _, line := range lines {
		if line.Type == "model.routed" {
			routed = append(routed, line.Route+":"+line.Model+"<"+line.From)
		}
	}
	if strings.Join(routed, ",") != "ranked:qwen-local<" {
		t.Fatalf("routed lines %v", routed)
	}
	receipt := lines[len(lines)-2].Receipt
	if receipt == nil || receipt.Routing == nil || receipt.Routing.Objective != "cost" || !receipt.Routing.AllowRemoteProviders || len(receipt.Requests) != 1 || receipt.Requests[0].Model != "qwen-local" || receipt.Requests[0].Route != "ranked" {
		t.Fatalf("receipt %+v", receipt)
	}

	// The local server is down: the remote candidate takes the request.
	env = cliEnv(t)
	writeSettings(t, env, routingSettings)
	hosts = routedHosts(t, true)
	code, stdout, stderr = execRun(t, env, "--root", t.TempDir(), "--exec", "say done", "--routing", routing, "--json")
	if code != execCompleted || strings.Join(*hosts, ",") != "127.0.0.1:18080,openrouter.ai" || !strings.Contains(stderr, "qwen-local failed") {
		t.Fatalf("code=%d hosts=%v stderr=%s", code, *hosts, stderr)
	}
	lines = execLines(t, stdout)
	routed = routed[:0]
	for _, line := range lines {
		if line.Type == "model.routed" {
			routed = append(routed, line.Route+":"+line.Model+"<"+line.From)
			if line.Route == "fallback" && !strings.Contains(line.Error, "127.0.0.1:18080 transport failure") {
				t.Fatalf("the fallback line does not say why: %+v", line)
			}
		}
	}
	if strings.Join(routed, ",") != "ranked:qwen-local<,fallback:anthropic/claude-haiku-5.5<qwen-local" {
		t.Fatalf("routed lines %v", routed)
	}
	receipt = lines[len(lines)-2].Receipt
	if len(receipt.Requests) != 2 || receipt.Requests[0].Status != "failed" || receipt.Requests[1].Model != "anthropic/claude-haiku-5.5" || receipt.Requests[1].Route != "fallback" || receipt.Requests[1].Status != "completed" {
		t.Fatalf("receipt requests %+v", receipt.Requests)
	}
	if result := execResult(t, stdout); result.Status != "completed" || result.Usage.Requests != 2 {
		t.Fatalf("result %+v", result)
	}

	// Remote providers forbidden: the failed local request ends the run.
	env = cliEnv(t)
	writeSettings(t, env, routingSettings)
	strict := writePolicy(t, `{"candidates": ["qwen-local", "anthropic/claude-haiku-5.5"], "objective": "cost", "allow_remote_providers": false}`)
	hosts = routedHosts(t, true)
	code, stdout, stderr = execRun(t, env, "--root", t.TempDir(), "--exec", "say done", "--routing", strict, "--json")
	if code != execFailed || strings.Join(*hosts, ",") != "127.0.0.1:18080" || !strings.Contains(stderr, "remote providers not allowed") {
		t.Fatalf("code=%d hosts=%v stderr=%s", code, *hosts, stderr)
	}
	result := execResult(t, stdout)
	if result.Status != "interrupted" || !strings.Contains(result.Error, "no candidate model") {
		t.Fatalf("result %+v", result)
	}
}

// The settings' routing applies to a run that brings none; --routing
// replaces it; a routing file the contract does not describe stops the
// run before any request.
func TestExecTakesTheSettingsRoutingAndRefusesAnInvalidOne(t *testing.T) {
	env := cliEnv(t)
	writeSettings(t, env, `{"local_models":[{"id":"qwen-local","url":"http://127.0.0.1:18080/v1","context_tokens":32768}],"routing":{"candidates":["qwen-local"],"objective":"quality-first","allow_remote_providers":false}}`)
	hosts := routedHosts(t, false)
	code, _, stderr := execRun(t, env, "--root", t.TempDir(), "--exec", "say done")
	if code != execCompleted || strings.Join(*hosts, ",") != "127.0.0.1:18080" || !strings.Contains(stderr, "routing quality-first among qwen-local; remote providers not allowed") {
		t.Fatalf("code=%d hosts=%v stderr=%s", code, *hosts, stderr)
	}
	for name, raw := range map[string]string{
		"unknown field": `{"candidates": ["qwen-local"], "objective": "cost", "routingPolicy": "cost-optimized"}`,
		"no objective":  `{"candidates": ["qwen-local"]}`,
		"no candidate":  `{"candidates": [], "objective": "cost"}`,
	} {
		*hosts = nil
		code, _, stderr := execRun(t, env, "--root", t.TempDir(), "--exec", "say done", "--routing", writePolicy(t, raw))
		if code != execFailed || len(*hosts) != 0 || !strings.Contains(stderr, "--routing") {
			t.Fatalf("%s: code=%d hosts=%v stderr=%s", name, code, *hosts, stderr)
		}
	}
	if code, _, stderr := execRun(t, env, "--root", t.TempDir(), "--routing", writePolicy(t, `{"candidates":["a"],"objective":"cost"}`)); code == 0 || !strings.Contains(stderr, "--routing") {
		t.Fatalf("--routing without --exec: code=%d stderr=%s", code, stderr)
	}
}
