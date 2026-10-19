package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

// The questions' hashes as the pinned oracle computes them (sha256 of the
// UTF-8 bytes of axlr.PROMPTS[family], supersede from axlr_v2.SUPERSEDE).
func TestJudgeV2QuestionsMatchTheOracle(t *testing.T) {
	want := map[string]string{
		JudgeFamilyFinal:     "7465be6eb5aba6fa8974b8f2718e3de0264b41c714fea9fe6fcf3c21c48d3fc4",
		JudgeFamilyGate:      "79d087405305d761d4caa106bb18b39a3ed47c5561ff4d53206d490e13f5e28b",
		JudgeFamilySentence:  "a8377abe0271cbbc9f591ac8b28f57c24c2084e7892ca2ec3834d32ac9cd998d",
		JudgeFamilyKind:      "ac91b119789351a5e03d93cda72f546cf124652e6d36bb1593ea30b26716d2ef",
		JudgeFamilySupersede: "30efdca1598594be5b265bb25df8ad06699f99feb0cf83f3eab725b6eac09475",
	}
	if len(judgeV2Questions) != len(want) {
		t.Fatalf("%d families", len(judgeV2Questions))
	}
	for family, hash := range want {
		sum := sha256.Sum256([]byte(judgeV2Questions[family]))
		if hex.EncodeToString(sum[:]) != hash {
			t.Errorf("%s question differs from the oracle", family)
		}
		if len(judgeV2Questions[family]) > MaxJudgementQuestionBytes {
			t.Errorf("%s question over the bound", family)
		}
	}
}

// oracleRow is one line of scripts/jev/export_v2_oracle.py's output.
type oracleRow struct {
	ID          string             `json:"id"`
	Family      string             `json:"family"`
	Dossier     JudgeDossier       `json:"dossier"`
	Question    *JudgementQuestion `json:"question"`
	Mapping     map[string]string  `json:"mapping"`
	Error       string             `json:"error"`
	GoOnlyError string             `json:"go_only_error"`
	GoDiverges  string             `json:"go_divergence"`
	Version     string             `json:"version"`
}

func readOracle(t *testing.T, path string) []oracleRow {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []oracleRow
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var row oracleRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	return rows
}

// The Go compiler builds the oracle's question for every dossier: the
// question, the options in order, the state byte for byte and the ID map;
// where the oracle refuses a dossier, so does Go. The public fixture is
// synthetic; AXLR_JEV_V2_ORACLE names the export of the private lab
// corpus (docs/jev-v2-integration.md).
func TestJudgeV2CompilesAsTheOracle(t *testing.T) {
	paths := []string{"testdata/jev_v2/synthetic.jsonl"}
	if private := os.Getenv("AXLR_JEV_V2_ORACLE"); private != "" {
		paths = append(paths, private)
	}
	for _, path := range paths {
		rows := readOracle(t, path)
		compared, refused, diverged := 0, 0, 0
		for _, row := range rows {
			if row.Version != JudgeV2Version {
				t.Fatalf("%s: oracle version %q", row.ID, row.Version)
			}
			compiled, err := CompileJudgeV2(row.Family, row.Dossier)
			if row.Error != "" || row.GoOnlyError != "" {
				if err == nil {
					t.Errorf("%s: compiled what the oracle refuses (%s%s)", row.ID, row.Error, row.GoOnlyError)
				}
				refused++
				continue
			}
			if err != nil {
				t.Errorf("%s: %v", row.ID, err)
				continue
			}
			want := *row.Question
			if row.GoDiverges != "" {
				// A documented divergence: the oracle's state would lose
				// evidence; Go's keeps it and carries no secret.
				if compiled.Question.Question != want.Question || compiled.Question.State == want.State || secretPattern.MatchString(compiled.Question.State) {
					t.Errorf("%s: expected divergence (%s) not shown", row.ID, row.GoDiverges)
				}
				diverged++
				continue
			}
			if compiled.Question.Question != want.Question || len(compiled.Question.Options) != len(want.Options) || (len(want.Options) > 0 && !reflect.DeepEqual(compiled.Question.Options, want.Options)) {
				t.Errorf("%s: question or options differ:\n%q\n%q", row.ID, compiled.Question.Options, want.Options)
			}
			if compiled.Question.State != want.State {
				t.Errorf("%s: state differs:\ngo     %s\noracle %s", row.ID, compiled.Question.State, want.State)
			}
			var got, expected any
			if json.Unmarshal([]byte(compiled.Question.State), &got) != nil || json.Unmarshal([]byte(want.State), &expected) != nil || !reflect.DeepEqual(got, expected) {
				t.Errorf("%s: decoded states differ", row.ID)
			}
			if len(row.Mapping) == 0 {
				row.Mapping = map[string]string{}
			}
			if !reflect.DeepEqual(compiled.ByID, row.Mapping) {
				t.Errorf("%s: mapping %v, oracle %v", row.ID, compiled.ByID, row.Mapping)
			}
			compared++
		}
		t.Logf("%s: %d questions identical, %d refused by both, %d documented divergences", path, compared, refused, diverged)
		if compared == 0 {
			t.Fatalf("%s compared nothing", path)
		}
	}
}

// IDs follow the received order, map to whole texts and refs, and none is
// an abstention; candidates the question cannot hold are refused, never
// hidden.
func TestJudgeV2IdentityAndBounds(t *testing.T) {
	prefix := strings.Repeat("Las exportaciones usan una codificación única, ", 6)
	compiled, err := CompileJudgeV2(JudgeFamilySentence, JudgeDossier{Prompt: "p", Answer: "a", Sentences: []string{prefix + "UTF-8", prefix + "UTF-16"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(compiled.Question.Options, []string{"s1", "s2", "none"}) || compiled.ByID["s1"] != prefix+"UTF-8" || compiled.ByID["s2"] != prefix+"UTF-16" {
		t.Fatalf("%+v", compiled)
	}
	if value, abstained, err := compiled.Choice(JudgementVerdict{Choice: "none"}); value != "" || !abstained || err != nil {
		t.Fatalf("none: %q %v %v", value, abstained, err)
	}
	if _, _, err := compiled.Choice(JudgementVerdict{Choice: "s9"}); err == nil {
		t.Fatal("an ID outside the mapping applied")
	}
	ref := "project:x:entry:decision:" + strings.Repeat("r", 220)
	compiled, err = CompileJudgeV2(JudgeFamilySupersede, JudgeDossier{NewMemory: "n", EarlierMemories: []string{"project:x:entry:decision:b beta", ref + " alfa"}})
	if err != nil {
		t.Fatal(err)
	}
	if compiled.ByID["m1"] != "project:x:entry:decision:b" || compiled.ByID["m2"] != ref || len(compiled.ByID) != 2 {
		t.Fatalf("%v", compiled.ByID)
	}
	if _, err := CompileJudgeV2(JudgeFamilySupersede, JudgeDossier{EarlierMemories: []string{"a uno", "a dos"}}); !errors.Is(err, ErrJudgeInvalidMemory) {
		t.Fatalf("repeated ref: %v", err)
	}
	if _, err := CompileJudgeV2(JudgeFamilySupersede, JudgeDossier{}); !errors.Is(err, ErrJudgeNoCandidates) {
		t.Fatalf("no memories: %v", err)
	}
	if _, err := CompileJudgeV2(JudgeFamilySupersede, JudgeDossier{EarlierMemories: make([]string, 16)}); !errors.Is(err, ErrJudgeTooManyCandidates) {
		t.Fatalf("16 memories: %v", err)
	}
	if _, err := CompileJudgeV2(JudgeFamilySentence, JudgeDossier{Sentences: make([]string, 16)}); !errors.Is(err, ErrJudgeTooManyCandidates) {
		t.Fatalf("16 sentences: %v", err)
	}
	if _, err := CompileJudgeV2(JudgeFamilyGate, JudgeDossier{Answer: strings.Repeat("a", MaxJudgementStateBytes)}); !errors.Is(err, ErrJudgeEvidenceTooLarge) {
		t.Fatalf("over the bound: %v", err)
	}
	if _, err := CompileJudgeV2("verify", JudgeDossier{}); err == nil {
		t.Fatal("unknown family compiled")
	}
	kind, err := CompileJudgeV2(JudgeFamilyKind, JudgeDossier{Prompt: "Prefiero UTC\n\n[AXLR · recall] x", Answer: "Las fechas van en UTC"})
	if err != nil || !reflect.DeepEqual(kind.Question.Options, settleKinds) || kind.Question.State != `{"answer":"Las fechas van en UTC","person_request":"Prefiero UTC"}` {
		t.Fatalf("kind %+v %v", kind, err)
	}
}

// Secrets are redacted value by value, so the JSON and the fields beside
// them survive; numbers keep their text.
func TestJudgeV2StateRedactsValuesAndKeepsNumbers(t *testing.T) {
	result := `{"status":"completed","output":{"stdout":"API_KEY=sk-abcdefghijklmnop","exit_code":0},"error":null}`
	tool := JudgeToolEvidence{Name: "local_exec", Result: &result}
	if err := json.Unmarshal([]byte(`{"name":"local_exec","arguments":{"env":{"A":"MY_TOKEN=abc \"q\"","n":1.0,"big":12345678901234567890,"e":1e5},"next":"kept"},"result":null}`), &tool); err != nil {
		t.Fatal(err)
	}
	tool.Result = &result
	compiled, err := CompileJudgeV2(JudgeFamilyGate, JudgeDossier{Prompt: "p", Answer: "Bearer abcdefghijkl ok", Tools: []JudgeToolEvidence{tool}})
	if err != nil {
		t.Fatal(err)
	}
	state := compiled.Question.State
	var decoded map[string]any
	if err := json.Unmarshal([]byte(state), &decoded); err != nil {
		t.Fatalf("state is not JSON: %v", err)
	}
	for _, secret := range []string{"sk-abcdefghijklmnop", "MY_TOKEN=abc", "Bearer abcdefghijkl"} {
		if strings.Contains(state, secret) {
			t.Errorf("secret %q kept: %s", secret, state)
		}
	}
	// Python would print 1e5 as 100000.0: the one byte difference a lab
	// dossier with an exponent would show, structurally the same number.
	for _, kept := range []string{`"next":"kept"`, `"n":1.0`, `"big":12345678901234567890`, `"e":1e5`, `[redacted] \"q\"`, `"answer":"[redacted] ok"`, `\"exit_code\":0`, `\"stdout\":\"[redacted]\"`} {
		if !strings.Contains(state, kept) {
			t.Errorf("%s lost: %s", kept, state)
		}
	}
	if !strings.Contains(unescapeLineSeparators([]byte(`"a b\\u2029c\\ "`)), "a b\\\\u2029c\\\\ ") {
		t.Fatal("line separators")
	}
}
