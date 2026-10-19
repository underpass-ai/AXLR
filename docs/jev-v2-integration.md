# Jev's automatic decisions under AXLR v2 (opt-in)

AXLR asks TypeSafe Jev five automatic questions: whether a final answer completes the request (the [final check](console.md#jev-an-external-judge-off-by-default)) and, after a turn with results, whether it settled something durable (gate), which sentence states it (sentence), what kind of memory it is (kind) and which recalled memory it replaces (supersede). `jev.judge_policy` selects how those questions are asked. `legacy`, the default, keeps the questions of v0.11.0. `axlr-v2` asks them as the JEV lab's AXLR v2 candidate does. `axlr_judge`, the model's own question, is the same under both.

Nothing changes for an installation that does not set the field. Issue [#148](https://github.com/underpass-ai/AXLR/issues/148) holds the full specification.

## Turn it on, and back off

```json
"jev": {
  "tool": true,
  "final_check": true,
  "final_threshold": 0.5,
  "model": "jev-1.13.0",
  "timeout_ms": 20000,
  "judge_policy": "axlr-v2",
  "v2_supersede": false
}
```

- `judge_policy`: empty or `legacy` asks the legacy questions; `axlr-v2` asks the lab's. Any other value stops the launch (`settings jev.judge_policy must be legacy or axlr-v2, not "…"`), with no silent fallback.
- `v2_supersede` (default `false`): under `axlr-v2`, a settled memory is recorded but the supersede question is not sent and no `supersedes` link is written. `true` sends it; it is for evaluation until the [activation criteria](#activation-of-supersede-pending) are met. `true` under any other policy stops the launch (`settings jev.v2_supersede needs jev.judge_policy axlr-v2`).
- A policy turns nothing on. `tool` and `final_check` still decide whether Jev is asked at all, and the settlement still needs Jev and KMP. The setup assistant writes no policy.
- `/axlr-info` names a policy other than legacy (`axlr-v2 decisions`, and `supersede (evaluation)` when the switch is on).

To roll back, remove `judge_policy` or set it to `"legacy"`, remove `v2_supersede`, and restart the console. The key and the model do not change: `TYPESAFE_API_KEY` wins over the key the setup saved in `credentials.json`, as before.

## Provenance

The port copies revision **`axlr-accuracy-r4`** of [underpass-ai/jev-judge-lab](https://github.com/underpass-ai/jev-judge-lab) (private) at commit `64fe6d992ab4b2097c9eecf869dcec24c45b3d88`: the questions of `final`, `gate`, `sentence` and `kind` from `src/jev_judge/axlr.py` (revision r1.1, unchanged in v2), the question of `supersede` from `src/jev_judge/axlr_v2.py`, the state layout and options of `make_request`, the decision rules of `evaluate` and the strict checks of `transport.validate`. It does not take the later v5 candidate, its batch of questions, its two stages or its thresholds. The lab's [v5 decision](https://github.com/underpass-ai/jev-judge-lab/blob/64fe6d992ab4b2097c9eecf869dcec24c45b3d88/docs/axlr-v5-results.md) was not to integrate it.

The code names its source: `JudgeV2Version` and `JudgeV2Oracle` in [judge_policy_v2.go](../tui/application/judge_policy_v2.go), and `TestJudgeV2QuestionsMatchTheOracle` pins the sha256 of each question. Translating or rewording a question makes a new policy, which needs its own revision and benchmark.

## What each decision reads and how its answer applies

The evidence is the turn as it happened, read from the transcript by `JudgeTurnEvidenceOf` ([judge_evidence.go](../tui/application/judge_evidence.go)). It includes the request as the person typed it, without the console's `[AXLR …]` notes; the answer that reached the person (for the final check, the transcript's last message); and every tool call of the turn, reads included, with its arguments as JSON and the whole content of its result, envelope and exit code included. An absent result is `null`, an empty one `""`, and nothing is inferred from a call alone. The turn ends at the person's next message, as in the turn record. The evidence is never stored: not in `LastRecord`, the logs or memory. The turn record keeps its 500-byte excerpts.

| Decision | State | Options | Applied |
|:--|:--|:--|:--|
| final | `person_request`, `answer`, `tool_results` | yes/no | below `final_threshold` (default 0.5) the doubt goes back to the model once, as in legacy |
| gate | same | yes/no | durable at 0.5 or more |
| sentence | same and `candidates` (`id`, `text`) | `s1`…`sn`, `none` | the whole sentence the ID stands for, or nothing on `none` |
| kind | `person_request`, and the settled sentence as `answer` | the seven kinds | the kind chosen |
| supersede | `new_memory`, `earlier_memories` (`id`, `ref`, `text`) | `m1`…`mn`, `none` | a `supersedes` link to the exact ref, or none |

IDs follow the order received, never the content. An ID maps to one whole sentence or one exact ref, never to a label cut to 200 bytes. `none` is an abstention and never reaches KMP. The state is serialized as the lab serializes it: keys sorted, compact, no HTML escaping, `U+2028` and `U+2029` written as themselves. Every string value is redacted before serialization, never the serialized document. A state over 24 KiB is not cut: the decision is not asked, and the omission is logged (the settlement) or counted in the diagnostics trace (the final check).

Every answer is validated against the question sent before it applies (`ValidateJudgementVerdict`, [judge_verdict_validation.go](../tui/application/judge_verdict_validation.go)). The pinned model must be the one that answered. A yes/no answer needs a finite probability in [0, 1]. A choice needs one of the options offered, exactly one probability per option, each finite in [0, 1], summing to 1 within 0.02, and highest on the choice within 0.001, plus a confidence in [0, 1]. Confidence is never read as the probability of being right. A choice applies as `evaluate` applies it, without the legacy floors (0.4 for the sentence, 0.5 for supersede); keeping a floor would be a different arm with its own measurements.

## What the session keeps

The final check keeps its guards: only a completed request's final answer, never during a live ceremony, once per request, never after the person denied or cancelled a call, never without calls left, at most one extra turn, written to the person. A failed, invalid or omitted judgement lets the answer stand.

The settlement keeps settle's guards (Jev and KMP configured, a turn with results, no memory written by the model, a non-empty answer, not interrupted) and its single 20-second timeout, idempotency key, context, step and action evidence, and routing of preferences and feedback to the person's about. A failure or negative of the gate, the sentence or the kind records nothing. Nothing falls back to the legacy questions. The sentence candidates come from the whole answer, with the legacy extractor's own bounds, and the sentence is stored redacted. Entries settled under v2 carry the label `judge_policy: axlr-accuracy-r4`. `RecordSettled` and KMP's protocol are unchanged.

Supersede, when its switch is on, is asked only when every condition holds:

- the memory goes to the project's about, since the recall exposes project memories (a preference or feedback goes to the person's about);
- no recalled line is cut (the console bounds each to 300 bytes with `…`): a clause lost to the cut cannot be shown to be carried forward;
- there are 1 to 15 recalled memories, with no ref repeated.

The link names only a ref that was offered. An abstention, a failure or an answer outside the offer records the memory without a link.

The TypeSafe adapter now refuses a `null` probability under every policy. Decoded into a number, it read as 0 and passed for an answer covering its option.

## Where the port differs from the lab

| Difference | Effect |
|:--|:--|
| No candidates, more than 15 memories or a repeated ref: Go asks nothing | The lab would send one option, or offer one ref under two IDs |
| A JSON tool result that holds a secret is redacted value by value | The lab redacts the result as text, and its `KEY=\S+` swallows the fields after the secret, the exit code included. Identical when no secret matches |
| Argument numbers keep their literal (`1e5`) | Python reprints floats (`100000.0`); structurally the same number. No lab case has one |
| Sentence candidates come from the console's extractor | The lab supplies them. The extractor drops sentences under 20 bytes: 1 of the 16 lab sentence cases lists one |
| Recalled lines read `ref (kind): text`, at most 8 lines of 300 bytes, from KMP's wake | The lab measured `ref text`. A line from the wake is KMP's summary of the entry, whose completeness the console cannot verify; for entries the console settled, the summary is the stored text |
| The adapter retries HTTP 429 twice | The lab's transport retries none. The live run below saw no 429 |
| The final check uses `final_threshold` | The lab decides at 0.5, the default |

## Parity with the oracle

Offline, no network. [export_v2_oracle.py](../scripts/jev/export_v2_oracle.py) runs the pinned lab candidate and writes its questions, mappings and recorded answers.

- **Questions.** All 166 lab dossiers (the 96 v2 cases and 70 regression cases, all five families) compile in Go to the oracle's question, options in order, state bytes and ID map (`TestJudgeV2CompilesAsTheOracle`).
- **From a session.** Recorded as session turns, 44/44 final, 24/24 gate and 15/16 sentence dossiers produce the oracle's question through the transcript, the extractor and settle (`TestJudgeV2SessionQuestionsMatchTheOracle`). The sixteenth differs only by the extractor's 20-byte floor.
- **Wire and decisions.** All 358 TypeSafe answers the lab recorded for v2 replay through the real adapter: the Go body hashes to the body the lab sent, byte for byte, and the decision equals the lab's (`TestJudgeV2ReplaysTheLabsAnswers`).
- **Public fixture.** The public synthetic fixture ([testdata/jev_v2](../tui/application/testdata/jev_v2/synthetic.jsonl), generated by [synthetic_v2_dossiers.py](../scripts/jev/synthetic_v2_dossiers.py)) runs in CI. Of its 22 dossiers, 13 are identical (HTML characters, `U+2028`, quotes, control characters, emoji, two sentences with a long common prefix, a 251-byte ref, permuted memories, a state of exactly 24,576 bytes), 8 are refused by both (no candidates, 16 options, no text, one byte over the bound) and 1 is the documented redaction difference.

The lab corpus and its export are private. Set `AXLR_JEV_V2_ORACLE` to the export to run the private checks.

## Live comparison

Run on 19 October 2026 by `TestJudgeV2LiveComparison` ([jev_v2_live_test.go](../tui/adapters/typesafe/jev_v2_live_test.go)) at AXLR `1c4c500` (clean), against TypeSafe `jev-1.13.0` with the key from the environment. The run used the lab's frozen test split: 60 cases in 30 contrastive episodes, three repeats and three arms, 540 decisions in a shuffled order per case and repeat, two workers, no cache, a $0.25 reservation cap and a 540-call cap. The arms all went through the real Go adapter:

- **baseline** is AXLR's legacy questions as the lab exported them from AXLR's own code, with the legacy floors;
- **python-v2** is the pinned Python candidate's request;
- **go-v2** is the Go port's request, built from a recorded session through the extractor and the compiler.

The go-v2 request was byte-identical to python-v2's in 180 of 180 cases, so differences between those two arms are Jev's variation on the same request.

| Arm | Correct, errors included | Coverage | Supersede: false replacements / negatives | Valid replacements recovered / positives | USD per 1,000 decisions | p50 / p95 ms |
|:--|--:|--:|--:|--:|--:|--:|
| baseline | 106/180 (58.89%) | 100% | 30/48 | 48/48 | 0.02063 | 217 / 269 |
| python-v2 | 172/180 (95.56%) | 100% | 5/48 | 48/48 | 0.02589 | 218 / 261 |
| go-v2 | 171/180 (95.00%) | 100% | 6/48 | 48/48 | 0.02589 | 215 / 285 |

By family, go-v2 scored final 36/36, gate 21/24, sentence 12/12, kind 12/12 and supersede 90/96; python-v2 matched it except supersede (91/96). Accuracy when a decision was made equals the totals: there were no transport or validation errors, no retries and no unknown charges. The three arms used 310,374 billed input tokens, about $0.013 at the lab's 0.042 USD per million; the cost is estimated from billed tokens, since TypeSafe reported no charge. Latencies are descriptive (two workers from one machine), not a production commitment.

The baseline reproduces the lab's published AXLR baseline exactly (106/180, $0.02063 per 1,000), and v2's cost matches the lab's ($0.02589). Paired by episode (bootstrap, 10,000 resamples, 95%), the gains were:

- go-v2 over baseline: +36.1 points [+23.3, +47.8], and +25.0 [+10.4, +38.5] on supersede;
- go-v2 over python-v2: −0.6 [−2.2, +1.1], the same requests.

go-v2 resolved both members of 26 of the 30 episodes in every repeat. Repeats and the two members of an episode are not independent samples. The cases were written from AXLR's code by the lab's author. They are not user traces, not independent human gold and not tests of the whole loop (agent → judge → second answer, or a real memory write). Every lab collection has been consumed: this run is regression and parity, not a fresh holdout, and tuning a question, a threshold or a policy needs new reserved episodes frozen before the run.

Per turn, a settled turn under v2 asks gate, sentence and kind, three sequential decisions of about 6.4 × 10⁻⁵ USD and about 0.63 s at the median when the gate passes, plus supersede when its switch is on. The final check is one decision of about 2.2 × 10⁻⁵ USD.

The receipts, manifest and summary are in the private lab checkout under `results/axlr-go-port/` (not pushed). They hold the cases' text and are not published here.

## Coverage lost to the 24 KiB bound

v2 sends the whole evidence or nothing. Over the 95 sessions recorded on the development machine (`TestJudgeV2CoverageOfRecordedSessions`, offline, counts only), the bound left these decisions unasked:

| Decision | Unasked | State size p50 / p90 |
|:--|--:|--:|
| Final check | 12 of 168 (7.1%) | 4,229 / 14,735 bytes |
| Settlement gate | 10 of 77 (13.0%) | 7,932 / 16,714 bytes |

Legacy asks those turns with bounded excerpts. Splitting evidence across questions was not evaluated and is not done.

## Known failures of v2

A faithful port reproduces them; they are debts of the policy, not of the port. In the lab's own runs and in this one, v2:

- replaces a memory when a new retry count omits a still-valid idempotency requirement;
- replaces a policy restated with an added comment that adopts no change (a regression from v1);
- replaces a memory when a partial withdrawal omits another obligation (intermittent);
- misses the explicit global CRLF → LF change in the lab's history (0 of 6), which v1 caught;
- replaces a memory with a diagram or mention that does not keep an operating obligation, and with a signature withdrawal that drops deduplication (0 of 6 right each in the lab's regression);
- accepts as durable a parser fix whose write and test failed (gate; the question is r1.1's, unchanged).

## Activation of supersede: pending

This change ships `v2_supersede` off. The port proves parity, not that supersession is safe. Before the switch can be on by default, each of the following is required:

- a disposition for each supersede regression above, CRLF → LF and the omitted obligations included; a changed question is a new revision with an independent benchmark, never called v2;
- no false retirement observed on a reserved activation set, published with its positive coverage and size (zero observed does not prove zero risk);
- a way to give the question the whole text of each recalled memory, since the wake's lines are summaries;
- a real test on an isolated rehearsal store: write, replace validly, keep the memory on abstention, and repeat with the same idempotency key without a duplicate;
- cost and latency compared under the same budget and load.

## Reproduce

Run the offline checks in CI and locally:

```bash
go test ./tui/application ./tui/adapters/typesafe ./tui/adapters/storage ./tui/adapters/ceremonyhost ./tui/cmd/axlr-tui -count=1
```

The private parity checks need a lab checkout at the oracle commit:

```bash
python3 scripts/jev/export_v2_oracle.py --lab ../jev-judge-lab --lab-corpus --receipts --output /private/oracle.jsonl
AXLR_JEV_V2_ORACLE=/private/oracle.jsonl go -C tui test ./application ./adapters/typesafe -run 'JudgeV2' -count=1 -v
```

The live comparison is billed and opt-in. It refuses to overwrite receipts or to plan more decisions than the call cap:

```bash
AXLR_JEV_V2_LIVE=../jev-judge-lab/results/axlr-go-port/test-<date> AXLR_JEV_V2_LAB=../jev-judge-lab \
AXLR_JEV_V2_ORACLE=/private/oracle.jsonl AXLR_JEV_V2_BUDGET_USD=0.25 AXLR_JEV_V2_MAX_CALLS=540 \
go -C tui test ./adapters/typesafe -run TestJudgeV2LiveComparison -count=1 -timeout 15m -v
```

To measure coverage against the bound, point `AXLR_JEV_V2_SESSIONS` at a copy of a session store; loading may migrate files:

```bash
AXLR_JEV_V2_SESSIONS=/copy/of/sessions go -C tui test ./adapters/storage -run JudgeV2Coverage -count=1 -v
```

No Python runs in AXLR: the scripts above are development tools against the lab checkout. No credentials were migrated, no `-latest` model is used, and OpenRouter is not involved.
