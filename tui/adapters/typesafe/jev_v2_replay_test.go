package typesafe_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/adapters/typesafe"
	"github.com/underpass-ai/AXLR/tui/application"
)

// oracleReceipt is one answer the lab recorded for its v2 arm: the hash of
// the exact body it sent, the typed answer and the decision it derived.
type oracleReceipt struct {
	Source     string                     `json:"source"`
	Model      string                     `json:"model"`
	Hash       string                     `json:"request_sha256"`
	Answers    map[string]json.RawMessage `json:"answers"`
	Error      string                     `json:"error"`
	Prediction any                        `json:"prediction"`
}

type oracleRow struct {
	ID       string                         `json:"id"`
	Family   string                         `json:"family"`
	Dossier  application.JudgeDossier       `json:"dossier"`
	Error    string                         `json:"error"`
	Receipts []oracleReceipt                `json:"receipts"`
	Question *application.JudgementQuestion `json:"question"`
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

// wireDigest is the lab's request hash: the provider's name and the exact
// body bytes.
func wireDigest(body []byte) string {
	sum := sha256.Sum256(append([]byte("kmp-wire-v1\x00typesafe\x00"), body...))
	return hex.EncodeToString(sum[:])
}

type replayTransport struct {
	t      *testing.T
	digest string
	reply  []byte
	calls  int
}

func (r *replayTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.calls++
	body, err := io.ReadAll(req.Body)
	if err != nil {
		r.t.Fatal(err)
	}
	if got := wireDigest(body); got != r.digest {
		r.t.Errorf("wire body differs from the lab's: %s, lab %s", got, r.digest)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(r.reply))), Request: req}, nil
}

// decide applies a validated verdict as the lab's evaluate does: a yes/no
// probability at 0.5, a choice through the question's ID map.
func decide(compiled application.CompiledJudgeQuestion, verdict application.JudgementVerdict) (any, error) {
	if verdict.Yes != nil {
		return *verdict.Yes >= 0.5, nil
	}
	if compiled.Family == application.JudgeFamilyKind {
		return verdict.Choice, nil
	}
	value, abstained, err := compiled.Choice(verdict)
	if abstained {
		return "none", err
	}
	return value, err
}

// Every v2 answer the lab recorded from TypeSafe goes through the Go port
// offline: the Go compiler's question, the real adapter's body (equal byte
// for byte to the body the lab sent, by its hash), the adapter's decoding,
// the strict validation and the decision, which must equal the lab's.
// AXLR_JEV_V2_ORACLE names the private export with receipts
// (scripts/jev/export_v2_oracle.py --receipts).
func TestJudgeV2ReplaysTheLabsAnswers(t *testing.T) {
	path := os.Getenv("AXLR_JEV_V2_ORACLE")
	if path == "" {
		t.Skip("AXLR_JEV_V2_ORACLE not set: the lab's receipts are private")
	}
	replayed, families := 0, map[string]int{}
	for _, row := range readOracle(t, path) {
		if len(row.Receipts) == 0 || row.Error != "" {
			continue
		}
		compiled, err := application.CompileJudgeV2(row.Family, row.Dossier)
		if err != nil {
			t.Fatalf("%s: %v", row.ID, err)
		}
		for _, receipt := range row.Receipts {
			if receipt.Error != "" {
				continue
			}
			reply, _ := json.Marshal(map[string]any{"model": receipt.Model, "answers": receipt.Answers})
			transport := &replayTransport{t: t, digest: receipt.Hash, reply: reply}
			client, err := typesafe.New(typesafe.Config{APIKey: "local-fixture-only", Model: receipt.Model, HTTPClient: &http.Client{Transport: transport}})
			if err != nil {
				t.Fatal(err)
			}
			verdict, err := client.Judge(context.Background(), compiled.Question)
			if err != nil {
				t.Fatalf("%s: %v", row.ID, err)
			}
			if err := application.ValidateJudgementVerdict(compiled.Question, verdict, receipt.Model); err != nil {
				t.Fatalf("%s: %v", row.ID, err)
			}
			got, err := decide(compiled, verdict)
			if err != nil {
				t.Fatalf("%s: %v", row.ID, err)
			}
			if transport.calls != 1 || got != receipt.Prediction {
				t.Errorf("%s (%s): decision %v, lab %v", row.ID, receipt.Source, got, receipt.Prediction)
			}
			replayed++
			families[row.Family]++
		}
	}
	t.Logf("%d lab answers replayed with equal wire bytes and decisions: %v", replayed, families)
	if replayed == 0 {
		t.Fatal("nothing replayed")
	}
}
