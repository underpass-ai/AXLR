package typesafe_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/adapters/typesafe"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// The live comparison of docs/jev-v2-integration.md: three arms over the
// same lab cases, each through the real TypeSafe adapter, in a shuffled
// order per case and repeat. baseline is AXLR's legacy questions as the
// lab exported them from AXLR (data/axlr-v2/native.json) with the legacy
// floors; python-v2 is the pinned Python candidate's request (the oracle
// export); go-v2 is the Go port's, built from a recorded session through
// the evidence extractor and the compiler. Opt-in and billed: it runs only
// with every variable below, never in CI.
//
//	AXLR_JEV_V2_LIVE        output directory, which must not exist
//	AXLR_JEV_V2_LAB         jev-judge-lab checkout at the oracle commit
//	AXLR_JEV_V2_ORACLE      scripts/jev/export_v2_oracle.py --lab-corpus output
//	AXLR_JEV_V2_BUDGET_USD  conservative reservation cap for the run
//	AXLR_JEV_V2_MAX_CALLS   hard cap on decisions sent
//	AXLR_JEV_V2_SPLIT       test (default), development or regression
//	AXLR_JEV_V2_REPEATS     1 to 3 (default 3)
//
// The key resolves as AXLR resolves it: TYPESAFE_API_KEY, else the key
// setup saved in axlr/credentials.json. It is never written anywhere.

const (
	livePricePerToken = 0.042 / 1_000_000 // the lab's TypeSafe estimate
	liveWorkers       = 2
	liveSeed          = 20261019
)

// Data the lab froze before its test (data/axlr-v2/freeze.json).
var liveFrozen = map[string]string{
	"data/axlr-v2/cases.jsonl":      "af83604fbadcef2fde4daf695fd1ac88e25c649927249cfe507079e344c395b7",
	"data/axlr-v2/regression.jsonl": "de955200aaf57f3f7e1c4bf83cd2f97a27d1d9c567c0bf8233b590bc53a31988",
	"data/axlr-v2/native.json":      "50d8df5266697b2eba40403fd20bc86e14f56d1e261ba7152ec6ee0b4475405a",
	"data/axlr-v1/native.json":      "8d65165a4781f2cd567819b36cb9a7fdc9b3a04cfd96f8a4d1279d0c882f15bd",
}

type liveCase struct {
	ID       string                   `json:"id"`
	Episode  string                   `json:"episode"`
	Split    string                   `json:"split"`
	Family   string                   `json:"family"`
	Gold     any                      `json:"gold"`
	Dossier  application.JudgeDossier `json:"dossier"`
	baseline application.CompiledJudgeQuestion
	python   application.CompiledJudgeQuestion
}

type liveNative struct {
	Question application.JudgementQuestion `json:"question"`
	Mapping  map[string]string             `json:"mapping"`
}

// liveReceipt is one decision as the run records it.
type liveReceipt struct {
	Prediction    any                `json:"prediction"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Model         string             `json:"model,omitempty"`
	RequestSHA256 string             `json:"request_sha256,omitempty"`
	Attempts      int                `json:"attempts"`
	HTTPStatus    []int              `json:"http_status,omitempty"`
	InputTokens   *int               `json:"billed_input_tokens"`
	CostEstimated *float64           `json:"cost_usd_estimated"`
	CostReported  *float64           `json:"cost_usd_reported"`
	LatencyMS     float64            `json:"latency_ms"`
	Error         string             `json:"error,omitempty"`
	ErrorStage    string             `json:"error_stage,omitempty"`
}

type liveRow struct {
	ID       string                 `json:"id"`
	Episode  string                 `json:"episode"`
	Split    string                 `json:"split"`
	Family   string                 `json:"family"`
	Gold     any                    `json:"gold"`
	Repeat   int                    `json:"repeat"`
	Order    []string               `json:"order"`
	SameWire bool                   `json:"go_request_equals_python"`
	Arms     map[string]liveReceipt `json:"arms"`
}

var liveArms = []string{"baseline", "python-v2", "go-v2"}

func TestJudgeV2LiveComparison(t *testing.T) {
	out := os.Getenv("AXLR_JEV_V2_LIVE")
	if out == "" {
		t.Skip("AXLR_JEV_V2_LIVE not set: the live comparison is opt-in and billed")
	}
	lab, oraclePath := os.Getenv("AXLR_JEV_V2_LAB"), os.Getenv("AXLR_JEV_V2_ORACLE")
	budget, err := strconv.ParseFloat(os.Getenv("AXLR_JEV_V2_BUDGET_USD"), 64)
	if err != nil || !(budget > 0) || math.IsInf(budget, 0) {
		t.Fatal("AXLR_JEV_V2_BUDGET_USD must be a positive amount")
	}
	maxCalls, err := strconv.Atoi(os.Getenv("AXLR_JEV_V2_MAX_CALLS"))
	if err != nil || maxCalls <= 0 {
		t.Fatal("AXLR_JEV_V2_MAX_CALLS must be a positive count")
	}
	split := envOr("AXLR_JEV_V2_SPLIT", "test")
	repeats, err := strconv.Atoi(envOr("AXLR_JEV_V2_REPEATS", "3"))
	if err != nil || repeats < 1 || repeats > 3 {
		t.Fatal("AXLR_JEV_V2_REPEATS must be 1 to 3")
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatalf("%s exists: receipts are never overwritten", out)
	}
	hashes := map[string]string{}
	for name, want := range liveFrozen {
		got := fileSHA256(t, filepath.Join(lab, name))
		if got != want {
			t.Fatalf("%s is not the frozen lab data", name)
		}
		hashes[name] = got
	}
	hashes["oracle_export"] = fileSHA256(t, oraclePath)
	key, source := liveKey(t)
	cases := liveCases(t, lab, oraclePath, split)
	planned := len(cases) * repeats * len(liveArms)
	if planned > maxCalls {
		t.Fatalf("%d decisions planned, AXLR_JEV_V2_MAX_CALLS allows %d", planned, maxCalls)
	}
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	commit, dirty := gitState(t)
	manifest := map[string]any{
		"created_at": time.Now().UTC().Format(time.RFC3339), "axlr_commit": commit, "axlr_dirty": dirty,
		"oracle": application.JudgeV2Oracle, "policy": application.JudgeV2Version, "provider": "typesafe", "endpoint": typesafe.Endpoint,
		"model": typesafe.DefaultModel, "credential_source": source, "split": split, "repeats": repeats, "cases": len(cases),
		"episodes": countEpisodes(cases), "arms": liveArms, "budget_usd": budget, "max_calls": maxCalls, "workers": liveWorkers,
		"seed": liveSeed, "hashes": hashes, "cache": false,
		"retries": "the adapter's own: up to 2 retries on HTTP 429, each counted as an attempt; the lab's transport retries none",
		"price":   "estimate from billed input tokens at 0.042 USD per million; a missing token count is an unknown charge, not zero",
	}
	writeJSON(t, filepath.Join(out, "manifest.json"), manifest)

	type job struct {
		c      *liveCase
		repeat int
	}
	var jobs []job
	for i := range cases {
		for r := 0; r < repeats; r++ {
			jobs = append(jobs, job{&cases[i], r})
		}
	}
	rand.New(rand.NewSource(liveSeed)).Shuffle(len(jobs), func(i, j int) { jobs[i], jobs[j] = jobs[j], jobs[i] })
	runner := &liveRunner{t: t, key: key, budget: budget, maxCalls: maxCalls}
	receipts, err := os.OpenFile(filepath.Join(out, "receipts.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer receipts.Close()
	var mu sync.Mutex
	var rows []liveRow
	queue := make(chan job)
	var wg sync.WaitGroup
	for w := 0; w < liveWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				row := runner.run(j.c, j.repeat)
				line, _ := json.Marshal(row)
				mu.Lock()
				rows = append(rows, row)
				_, _ = receipts.Write(append(line, '\n'))
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		queue <- j
	}
	close(queue)
	wg.Wait()
	summary := liveSummary(rows)
	summary["receipts_sha256"] = fileSHA256(t, filepath.Join(out, "receipts.jsonl"))
	summary["reserved_usd"] = runner.reserved
	writeJSON(t, filepath.Join(out, "summary.json"), summary)
	text, _ := json.MarshalIndent(summary, "", "  ")
	t.Logf("%s", text)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// liveKey resolves Jev's key as run.go does and names only its source.
func liveKey(t *testing.T) (string, string) {
	t.Helper()
	if key := strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY")); key != "" {
		return key, "environment:TYPESAFE_API_KEY"
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(base) {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	credentials, err := storage.CredentialsStore{Path: filepath.Join(base, "axlr", "credentials.json")}.Load()
	if err != nil || credentials.TypeSafeAPIKey == "" {
		t.Fatal("no TypeSafe key: set TYPESAFE_API_KEY or run AXLR's setup")
	}
	return credentials.TypeSafeAPIKey, "axlr:credentials.json"
}

func gitState(t *testing.T) (string, bool) {
	t.Helper()
	head, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown", true
	}
	status, _ := exec.Command("git", "status", "--porcelain", "--untracked-files=no").Output()
	return strings.TrimSpace(string(head)), len(bytes.TrimSpace(status)) > 0
}

func liveCases(t *testing.T, lab, oraclePath, split string) []liveCase {
	t.Helper()
	dataset, nativePath := "data/axlr-v2/cases.jsonl", "data/axlr-v2/native.json"
	if split == "regression" {
		dataset, nativePath = "data/axlr-v2/regression.jsonl", "data/axlr-v1/native.json"
	}
	var native map[string]liveNative
	raw, err := os.ReadFile(filepath.Join(lab, nativePath))
	if err != nil || json.Unmarshal(raw, &native) != nil {
		t.Fatalf("baseline questions: %v", err)
	}
	oracle := map[string]oracleRow{}
	for _, row := range readOracle(t, oraclePath) {
		oracle[row.ID] = row
	}
	raw, err = os.ReadFile(filepath.Join(lab, dataset))
	if err != nil {
		t.Fatal(err)
	}
	var cases []liveCase
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var c liveCase
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatal(err)
		}
		if c.Split != split {
			continue
		}
		n, ok := native[c.ID]
		o, found := oracle[c.ID]
		if !ok || !found || o.Question == nil {
			t.Fatalf("%s lacks its baseline or oracle question", c.ID)
		}
		c.baseline = application.CompiledJudgeQuestion{Family: c.Family, Question: n.Question, ByID: n.Mapping}
		c.python = application.CompiledJudgeQuestion{Family: c.Family, Question: *o.Question, ByID: o.Mapping}
		cases = append(cases, c)
	}
	if len(cases) == 0 {
		t.Fatalf("no %s cases", split)
	}
	return cases
}

func countEpisodes(cases []liveCase) int {
	seen := map[string]bool{}
	for _, c := range cases {
		seen[c.Episode] = true
	}
	return len(seen)
}

// goQuestion builds the Go arm's question as the console does: a turn of
// final, gate, sentence or kind is recorded as a session and read by the
// evidence extractor; sentence keeps the lab's candidates (the console's
// own extraction is measured offline); supersede compiles the recalled
// lines it is given.
func goQuestion(c *liveCase) (application.CompiledJudgeQuestion, error) {
	dossier := c.Dossier
	if c.Family != application.JudgeFamilySupersede {
		s, err := liveSession(dossier)
		if err != nil {
			return application.CompiledJudgeQuestion{}, err
		}
		evidence, err := application.JudgeTurnEvidenceOf(s.Messages(), 0, c.Family == application.JudgeFamilyFinal)
		if err != nil {
			return application.CompiledJudgeQuestion{}, err
		}
		turn := evidence.Dossier()
		turn.Sentences = dossier.Sentences
		dossier = turn
	}
	return application.CompileJudgeV2(c.Family, dossier)
}

func liveSession(d application.JudgeDossier) (domain.Session, error) {
	workspace, err := os.MkdirTemp("", "axlr-jev-live-")
	if err != nil {
		return domain.Session{}, err
	}
	defer os.RemoveAll(workspace)
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(workspace), "test/model")
	if err != nil {
		return s, err
	}
	schema, _ := root.NewJSONObject([]byte(`{"type":"object"}`))
	id, _ := domain.NewLocalToolIdentity("exec")
	if err := s.BeginTurn(root.Text(d.Prompt), []domain.AvailableTool{{Definition: root.ToolDefinition{Name: "read", Parameters: schema}, Identity: id}}); err != nil {
		return s, err
	}
	var calls []root.ToolCall
	for i, tool := range d.Tools {
		raw, _ := json.Marshal(tool.Arguments)
		args, err := root.NewJSONObject(raw)
		if err != nil || tool.Result == nil {
			return s, errors.New("a dossier call cannot be recorded")
		}
		calls = append(calls, root.ToolCall{ID: root.ToolCallID(strconv.Itoa(i + 1)), Name: root.ToolName(tool.Name), Arguments: args})
	}
	if len(calls) > 0 {
		if err := s.CompleteAssistantWithin(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: calls}}, 100); err != nil {
			return s, err
		}
		for i, tool := range d.Tools {
			if err := s.RecordToolOutcome(calls[i].ID, domain.DecisionAutoApprove, domain.ToolOutcome{Content: root.Text(*tool.Result)}); err != nil {
				return s, err
			}
		}
	}
	return s, s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: root.Text(d.Answer)}})
}

type liveRunner struct {
	t        *testing.T
	key      string
	budget   float64
	maxCalls int
	mu       sync.Mutex
	reserved float64
	calls    int
}

func (r *liveRunner) run(c *liveCase, repeat int) liveRow {
	order := append([]string(nil), liveArms...)
	rand.New(rand.NewSource(int64(liveSeed+repeat*7919+len(c.ID)*31+int(c.ID[len(c.ID)-1])))).Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	row := liveRow{ID: c.ID, Episode: c.Episode, Split: c.Split, Family: c.Family, Gold: c.Gold, Repeat: repeat, Order: order, Arms: map[string]liveReceipt{}}
	goCompiled, goErr := goQuestion(c)
	row.SameWire = goErr == nil && goCompiled.Question.State == c.python.Question.State && goCompiled.Question.Question == c.python.Question.Question && strings.Join(goCompiled.Question.Options, "\x00") == strings.Join(c.python.Question.Options, "\x00")
	for _, arm := range order {
		switch arm {
		case "baseline":
			row.Arms[arm] = r.decide(c.baseline, false, c.Family)
		case "python-v2":
			row.Arms[arm] = r.decide(c.python, true, c.Family)
		case "go-v2":
			if goErr != nil {
				row.Arms[arm] = liveReceipt{Error: goErr.Error(), ErrorStage: "compile"}
				continue
			}
			row.Arms[arm] = r.decide(goCompiled, true, c.Family)
		}
	}
	return row
}

// decide sends one question and applies the answer: strict validation and
// the lab's evaluate for the v2 arms; the adapter's checks and the legacy
// floors (sentence 0.4, supersede 0.5) for the baseline.
func (r *liveRunner) decide(compiled application.CompiledJudgeQuestion, strict bool, family string) liveReceipt {
	q := compiled.Question
	size := len(q.State) + len(q.Question) + 1024
	for _, option := range q.Options {
		size += len(option) + 8
	}
	reservation := float64(size) * livePricePerToken
	r.mu.Lock()
	if r.calls >= r.maxCalls || r.reserved+reservation > r.budget {
		r.mu.Unlock()
		return liveReceipt{Error: "run budget or call cap exhausted", ErrorStage: "budget"}
	}
	r.calls++
	r.reserved += reservation
	r.mu.Unlock()
	capture := &liveTransport{base: http.DefaultTransport}
	client, err := typesafe.New(typesafe.Config{APIKey: r.key, HTTPClient: &http.Client{Transport: capture}})
	if err != nil {
		return liveReceipt{Error: err.Error(), ErrorStage: "client"}
	}
	started := time.Now()
	verdict, err := client.Judge(context.Background(), q)
	receipt := capture.receipt()
	receipt.LatencyMS = float64(time.Since(started).Microseconds()) / 1000
	receipt.Model = verdict.Model
	if err != nil {
		receipt.Error, receipt.ErrorStage = err.Error(), "provider"
		return receipt
	}
	if strict {
		if err := application.ValidateJudgementVerdict(q, verdict, typesafe.DefaultModel); err != nil {
			receipt.Error, receipt.ErrorStage = err.Error(), "validation"
			return receipt
		}
	}
	receipt.Probabilities = verdict.Probabilities
	switch {
	case verdict.Yes != nil:
		receipt.Prediction = *verdict.Yes >= 0.5
		receipt.Probabilities = map[string]float64{"true": *verdict.Yes, "false": 1 - *verdict.Yes}
	case family == application.JudgeFamilyKind:
		receipt.Prediction = verdict.Choice
	case strict:
		value, abstained, err := compiled.Choice(verdict)
		if err != nil {
			receipt.Error, receipt.ErrorStage = err.Error(), "mapping"
			return receipt
		}
		receipt.Prediction = value
		if abstained {
			receipt.Prediction = "none"
		}
	default:
		prediction, ok := compiled.ByID[verdict.Choice]
		if !ok {
			prediction = verdict.Choice
		}
		floor := map[string]float64{application.JudgeFamilySentence: 0.4, application.JudgeFamilySupersede: 0.5}[family]
		if verdict.Probabilities[verdict.Choice] < floor {
			prediction = "none"
		}
		receipt.Prediction = prediction
	}
	return receipt
}

// liveTransport records what crossed the wire for one decision: every
// attempt, the body's lab hash and the billed tokens. The key stays in
// the request header it was put in.
type liveTransport struct {
	base     http.RoundTripper
	mu       sync.Mutex
	attempts int
	statuses []int
	digest   string
	tokens   *int
	cost     *float64
}

func (l *liveTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	l.mu.Lock()
	l.attempts++
	l.digest = wireDigest(body)
	l.mu.Unlock()
	resp, err := l.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 256<<10+1))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(data))
	l.mu.Lock()
	l.statuses = append(l.statuses, resp.StatusCode)
	var usage struct {
		Usage struct {
			InputTokens *int     `json:"input_tokens"`
			Cost        *float64 `json:"cost"`
		} `json:"usage"`
	}
	if readErr == nil && json.Unmarshal(data, &usage) == nil {
		if usage.Usage.InputTokens != nil && *usage.Usage.InputTokens >= 0 {
			tokens := *usage.Usage.InputTokens
			l.tokens = &tokens
		}
		if usage.Usage.Cost != nil && *usage.Usage.Cost >= 0 && !math.IsInf(*usage.Usage.Cost, 0) {
			cost := *usage.Usage.Cost
			l.cost = &cost
		}
	}
	l.mu.Unlock()
	return resp, nil
}

func (l *liveTransport) receipt() liveReceipt {
	l.mu.Lock()
	defer l.mu.Unlock()
	receipt := liveReceipt{RequestSHA256: l.digest, Attempts: l.attempts, HTTPStatus: l.statuses, InputTokens: l.tokens, CostReported: l.cost}
	if l.tokens != nil {
		estimate := float64(*l.tokens) * livePricePerToken
		receipt.CostEstimated = &estimate
	}
	return receipt
}

// liveSummary scores every arm by family and overall: accuracy with
// errors counted as misses, coverage, accuracy among decisions,
// replacements, cost and latency. Repeats and the two members of an
// episode are not independent samples; the counts say how many there are.
func liveSummary(rows []liveRow) map[string]any {
	families := map[string]bool{"all": true}
	for _, row := range rows {
		families[row.Family] = true
	}
	arms := map[string]any{}
	for _, arm := range liveArms {
		byFamily := map[string]any{}
		for family := range families {
			var latencies []float64
			n, correct, decided, errs, attempts, unknownCost := 0, 0, 0, map[string]int{}, 0, 0
			tokens, usd := 0, 0.0
			replacement := map[string]int{}
			cases, episodes := map[string]bool{}, map[string]bool{}
			for _, row := range rows {
				if family != "all" && row.Family != family {
					continue
				}
				receipt := row.Arms[arm]
				n++
				cases[row.ID], episodes[row.Episode] = true, true
				attempts += receipt.Attempts
				if receipt.Attempts > 0 {
					latencies = append(latencies, receipt.LatencyMS)
					if receipt.InputTokens == nil {
						unknownCost++
					} else {
						tokens += *receipt.InputTokens
						usd += *receipt.CostEstimated
					}
				}
				if receipt.Error != "" {
					errs[receipt.ErrorStage]++
				} else {
					decided++
					if fmt.Sprint(receipt.Prediction) == fmt.Sprint(row.Gold) {
						correct++
					}
				}
				if row.Family == application.JudgeFamilySupersede {
					positive := fmt.Sprint(row.Gold) != "none"
					p := fmt.Sprint(receipt.Prediction)
					switch {
					case positive:
						replacement["positives"]++
						switch {
						case receipt.Error != "":
							replacement["positive_errors"]++
						case p == fmt.Sprint(row.Gold):
							replacement["recovered"]++
						case p == "none":
							replacement["missed"]++
						default:
							replacement["wrong_target"]++
						}
					default:
						replacement["negatives"]++
						switch {
						case receipt.Error != "":
							replacement["negative_errors"]++
						case p != "none":
							replacement["false_replacements"]++
						}
					}
				}
			}
			if n == 0 {
				continue
			}
			sort.Float64s(latencies)
			entry := map[string]any{
				"decisions": n, "unique_cases": len(cases), "episodes": len(episodes), "correct": correct,
				"accuracy_with_errors": ratio(correct, n), "coverage": ratio(decided, n), "accuracy_when_decided": ratio(correct, decided),
				"errors_by_stage": errs, "attempts": attempts, "retries": attempts - len(latencies),
				"billed_input_tokens": tokens, "usd_estimated": usd, "usd_per_1000_decisions": 1000 * usd / math.Max(1, float64(len(latencies)-unknownCost)),
				"unknown_charges": unknownCost, "latency_ms_p50": quantile(latencies, 0.5), "latency_ms_p95": quantile(latencies, 0.95),
			}
			if len(replacement) > 0 {
				entry["supersede"] = replacement
			}
			byFamily[family] = entry
		}
		arms[arm] = byFamily
	}
	same := 0
	for _, row := range rows {
		if row.SameWire {
			same++
		}
	}
	return map[string]any{"arms": arms, "rows": len(rows), "go_request_equals_python": same}
}

func ratio(a, b int) any {
	if b == 0 {
		return nil
	}
	return float64(a) / float64(b)
}

func quantile(sorted []float64, q float64) any {
	if len(sorted) == 0 {
		return nil
	}
	return sorted[int(math.Ceil(q*float64(len(sorted))))-1]
}
