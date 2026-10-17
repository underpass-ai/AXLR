# Downloadable ceremonies: state of the art and catalog

Status: proposed (16 Oct 2026). Part of the [registry design](2026-10-16-tool-registry-design.md). Survey facts were checked on 16 Oct 2026 (`gh repo view` for stars and activity, arXiv abstracts and vendor documentation); abstract-only or unverified claims are marked. The reference definition [`draft_critique_revise` 1.0](../../catalog/ceremonies/writing/draft_critique_revise-1.0.yaml) validates as publishable with MADE 0.10.0 (one warning, discussed under [what the generic loop can run](#what-the-generic-loop-can-run-today)) and [`tools/catalog/walk_ceremonies.py`](../../tools/catalog/walk_ceremonies.py) drives its four paths (accepted at once; rejected, revised, accepted; revision cannot settle, escalated; three rejections, escalated) through the installed `made-mcp` in a disposable store, including the guards that must refuse.

## What the generic loop can run today

`/ceremony <definition>` drives any published definition ([ceremonies.md](../ceremonies.md#any-published-definition-ceremony)): the session's model works each `host_callback` step with the step's own prompt, hands back fields with `axlr_step_done`, MADE evaluates the guards and the console claims the next step. The envelope a catalog ceremony must fit:

| Mechanism | Available in the generic loop | Catalog rule |
|:--|:--|:--|
| `step_status:<step>:COMPLETED` | yes | every transition carries it |
| `output_field:<step>:<field>=<value>` | yes (equality on a scalar) | the step derives any judgement into a scalar (`accepted=true`, `decision="approve"`, `severity="high"`); a numeric threshold ("score ≥ 4") is computed by the step and handed back as a boolean |
| `step_repeat_exhausted:<step>` with `repeat.max_iterations` | yes | every loop has a cap and an exit to a terminal or escalation state |
| `max_transitions` / `max_bounces` | required by MADE on any cyclic definition | every catalog definition declares both |
| `roles` | a step is claimable only by a declared role whose `allowed_actions` name it and its triggers | every catalog definition declares one role, `AXLR`, with every step and trigger; without it MADE refuses the first claim ("no ceremony role can execute step") |
| `repeat.max_iterations` | counted per entry into the state: a step re-entered after a transition starts its budget again (walked on 16 Oct 2026) | overall rounds are bounded by a field the model derives (the critic counts its rejections) and by `max_transitions` |
| `type: human` guard (`manual_approval`) | the loop waits for an approval the console cannot give | **not used**; a decision of the person is a step whose prompt says to put the question to the person in the conversation and hand back `decision`. MADE warns that a cycle has "no human-controlled state"; the warning is accepted and shown on the ceremony's page |
| `num_agents > 1` | the console works the step once (no code reads `num_agents`) | independent voices (council members, debaters, blind screeners, Delphi panellists) are sequential steps with `see_prior: false`, one per voice, and the collecting step reads them all; every voice is the session's model, since the generic loop cannot pick a model per step, so a "council" is one model in several fresh contexts until a per-step model exists. True fan-out waits for the `x-axlr.fan_out` extension |
| check commands (`check_command` exits zero) | only in the console's pinned ceremonies | the step runs the command through `local_exec` and reports `passed`; the person approves the command as usual |
| fresh-context review | only in `/incident` | approximated by a step whose prompt forbids reading the prior transcript (`see_prior: false`) |
| budget | `max_session_usd` and the 75K-token session cap apply to the whole session | no per-ceremony guard; the cap and `max_transitions` bound the work |

The [registry design](2026-10-16-tool-registry-design.md#open-mechanisms) lists the console extension (`x-axlr` step annotations for approval cards, check commands and fan-out) that would promote the "approximated" rows to real guards. Until then every catalog definition is written inside this envelope and its page says which judgements the model makes for itself.

## Survey: orchestration patterns worth packaging

### Canonical taxonomies and frameworks

| Source | Patterns | Where a person decides | Evidence of value |
|:--|:--|:--|:--|
| Anthropic, *Building effective agents* (Dec 2024) | prompt chaining, routing, parallelization (sectioning, voting), orchestrator-workers, evaluator-optimizer; autonomous agents last | checkpoints and blockers; max-iteration stops | guidance: the simplest workflow that works |
| Anthropic multi-agent research system | lead plans and saves the plan, parallel subagents, citation agent | none built in; checkpoints | +90.2 % over single Opus 4 on an internal research eval; ~15× the tokens of chat; a poor fit for most coding |
| Microsoft Agent Framework (AutoGen's successor; `microsoft/autogen` archived) | sequential, concurrent, handoff, group chat, **magentic** (task and progress ledgers, stall counter, replan with caps) | plan review (`RequirePlanSignoff`), tool approval | Magentic-One competitive on GAIA, AssistantBench, WebArena |
| LangGraph | the Anthropic five plus plan-and-execute, reflection, Reflexion, LATS, supervisor and hierarchical teams | interrupts and checkpoints | 43k stars; the supervisor package now steers to tool-based supervisors |
| CrewAI | sequential and hierarchical processes, flows | `human_input` on a task | 60k stars |
| OpenAI Agents SDK | handoffs as `transfer_to_x` tools, agents as tools, guardrails on first and last agent | guardrails | 30k stars |
| Google ADK | sequential, parallel, loop workflow agents; ADK 2.0 moves to graph-based dynamic workflows | | 22k stars |
| AWS Strands | graph, swarm, workflow, agents-as-tools (repo renamed `harness-sdk`) | | 9k stars |
| Claude Agent SDK subagents | fresh context per subagent, summary back, parallel up to 20, depth 3 | only the parent asks the person | |
| MetaGPT, ChatDev, CAMEL | SOPs as role assembly lines; chat chains with "communicative dehallucination"; two-agent role play | none | 71k / 35k / 18k stars; MetaGPT's last push Jan 2026 |
| spec-kit (GitHub), Kiro specs | constitution → specify → plan → tasks → implement, with clarify, checklist and analyze; requirements/design/tasks documents each reviewed | per phase or per document | spec-kit 141k stars, the strongest adoption signal of the survey |
| Aider architect/editor | a reasoning model describes the change, an editor model writes the edits | interactive | 85 % on aider's edit benchmark |

### Research patterns

| Pattern | Structure | Evidence |
|:--|:--|:--|
| Reflexion | act → evaluate → written reflection → retry, capped | 91 % pass@1 HumanEval vs 80 % (abstract) |
| Self-Refine | one model produces, critiques, revises | ~+20 points average across 7 tasks (abstract) |
| Tree of Thoughts, LATS | branch, evaluate, prune, backtrack; tree search with reflection | Game of 24: 74 % vs 4 %; LATS 92.7 % HumanEval (abstracts) |
| Multi-agent debate, LLM Council | N proposers, rounds of critique, a chair synthesises; Karpathy's `llm-council` ranks answers anonymously | better factuality (abstract); council 25k stars |
| Mixture of Agents | layered proposers then an aggregator | 65.1 % AlpacaEval 2.0 vs 57.5 % GPT-4o |
| STORM / Co-STORM | perspectives → simulated interviews → outline → article; Co-STORM keeps a person steering | +25 points organisation vs RAG baseline; 32k stars |
| AI Scientist, Agent Laboratory, Co-Scientist | idea → code → experiments → paper → simulated review; Agent Laboratory adds human feedback at each stage (84 % lower cost than fully autonomous); Co-Scientist runs a hypothesis tournament with debate | in-vitro confirmations for Co-Scientist |
| AgentReview | reviewers, area chair, authors | reviewer bias shifted decisions ~37 % |
| Habermas Machine (Science 2024) | statements → critiques → regenerated statements maximising group approval | preferred to human mediators by >5,000 participants (secondary sources; check the DOI 10.1126/science.adq2852) |

### Three regularities that shape the catalog

1. **Multi-agent value is strongest in breadth-first work** (research, review, consultation), weakest and most expensive in coding. Research ceremonies fan out; software ceremonies stay sequential with a fresh reviewer.
2. **Every mature framework converges on the same three controls**: an explicit plan a person signs off, loops with hard caps and stall detection, and an independent evaluator with fresh context. MADE already has the first two (`manual_approval`, `repeat`, `max_transitions`); the catalog models the third as a `see_prior: false` step.
3. **Frameworks churn** (AutoGen archived, ADK rewrote its workflows, Strands renamed): a declarative YAML catalog is a stable layer above them, and the pattern names in the index (`pattern:` in the manifest) use MADE's own catalog where it has one: `roundtable_fixed_order`, `broadcast_collect`, `group_chat`, `maker_checker`, `handoff`, `magentic`, `advisor`.

## Catalog

Tier **A** runs in the generic loop today; tier **B** runs today with the person's decision and parallel voices approximated as described above (its page says so); tier **C** waits for `x-axlr` guards (approval card, check command, fan-out) to be worth publishing. Rounds are always capped; the cap is in the definition.

### Writing and editing

| Name | Goal | States | Loop | Pattern | Tier |
|:--|:--|:--|:--|:--|:--|
| `draft_critique_revise` | polished text from a brief with named criteria | draft → critique → revise ⟲ → deliver / escalate | ≤ 3 critique rounds, ≤ 2 revision attempts each | maker_checker, Self-Refine | **A** (reference, validated and walked) |
| `editorial_board` | one draft read by fact, style, legal and audience editors, merged by a chief | draft → fact → style → legal → audience → merge → chief verdict | one pass; chief may return once | roundtable_fixed_order | A |
| `translation_backtranslation` | translation checked by a blind back-translation | translate → back-translate (`see_prior: false`) → diff → fix ⟲ → sign-off | fix ≤ 3 | maker_checker | A |
| `writers_room` | story development | premise → pitches (4 sequential voices) → selection → beat sheet → scene drafts → table read → revise | revise ≤ 2 | broadcast_collect | B |

### Research and science

| Name | Goal | States | Loop | Pattern | Tier |
|:--|:--|:--|:--|:--|:--|
| `literature_review_storm` | cited survey of a topic | perspectives → interviews (one step per perspective) → outline → person's outline decision → draft → citation check → revise ⟲ | revise ≤ 2 | STORM, broadcast_collect | B |
| `systematic_review` | screen studies against inclusion criteria, two blind screeners | protocol → search → screen A → screen B (`see_prior: false`) → conflicts → extraction → bias appraisal → synthesis | conflicts to the person | voting parallelization | B |
| `reproducibility_check` | re-run a paper's claim | claims → environment → run ⟲ → compare → discrepancy report | run ≤ 3 | Reflexion | C (check commands) |
| `peer_review_simulation` | pre-submission critique | three reviews (sequential, `see_prior: false`) → meta-review → rebuttal → revised verdict ⟲ | until scores stop moving, ≤ 2 | AgentReview, debate | B |
| `hypothesis_tournament` | rank candidate hypotheses | generate → pairwise debate ⟲ → ranking → person's pick → experiment design | debate ≤ 3 rounds | Co-Scientist, group_chat | B |

### Software

| Name | Goal | States | Loop | Pattern | Tier |
|:--|:--|:--|:--|:--|:--|
| `spec_driven_delivery` | idea to feature through reviewed specs | specify → clarify ⟲ → plan → tasks → implement (hands off to `/plan`) → verify → converge | clarify until no ambiguity, ≤ 3 | spec-kit, Kiro, orchestrator-workers | C (approval cards; `/plan` handoff) |
| `security_review_council` | threat model, injection, authorization, secrets and dependency lenses on a diff | scope → five lenses (sequential, `see_prior: false`) → consolidate → rebuttal → verdict → remediation plan | rebuttal once | broadcast_collect, council | B |
| `dependency_upgrade` | upgrade one dependency safely | inventory → changelog digest → branch → fix breakages ⟲ → tests → PR | fix ≤ 5 | Reflexion | C (check commands, PR flow of `/improve`) |
| `code_migration` | migrate in slices with parity checks | survey → strategy (person decides) → slice plan → migrate slice ⟲ → parity → cutover | per slice | plan-and-execute, waves | C |
| `documentation_audit` | find and fix documentation that diverges from code | inventory → claims → verify claims → fix → fresh review | verify per claim | evaluator-optimizer | B |
| `test_first_loop` | TDD on one behaviour | write failing test → red → implement ⟲ → green → refactor | implement ≤ 6 | TDD, Reflexion | C (check commands; `/plan` tasks already do red/green) |
| `release_readiness` | go or no-go | evidence → checklist → risk review → rollback plan → decision | none | prompt chaining with a gate | B |

### Decision-making

| Name | Goal | States | Loop | Pattern | Tier |
|:--|:--|:--|:--|:--|:--|
| `red_team_review` | attack a plan before committing to it | summary → attacks (3 voices, `see_prior: false`) → defence → severity rating → mitigations ⟲ → sign-off | until no new high finding, ≤ 3 | debate, maker_checker | A |
| `delphi_estimate` | expert-style forecast | question → five estimates (`see_prior: false`) → anonymised feedback → revise ⟲ → aggregate | until spread under the brief's bound, ≤ 3 | Delphi method (methodology source unverified), Mixture of Agents | B |
| `pre_mortem` | how the project fails | assume failure → five failure stories → cluster → rank → mitigations | none | broadcast_collect | A |
| `decision_memo_council` | options memo with a vote and a dissent record | frame → options → analysis per option → five votes (`see_prior: false`, the same model five times) → dissent → decision | vote once | LLM Council, group_chat | A |
| `advisor_checked_task` | a task the executor must clear with an advisor before and after | brief → advice → work ⟲ → advice → deliver | advice loops until no blocking advice, ≤ 3 | MADE `advisor` | A |

### Business and administration

| Name | Goal | States | Loop | Pattern | Tier |
|:--|:--|:--|:--|:--|:--|
| `rfp_response` | answer a request for proposals | requirements → compliance matrix → draft per section ⟲ → review → price check → sign-off | per section until the review passes | orchestrator-workers | B |
| `procurement_comparison` | compare vendors against the person's criteria | criteria (person decides) → gather → score per vendor → sensitivity check → recommendation | none | parallelization | B |
| `meeting_to_actions` | notes to owned, confirmed actions | extract → owners → confirm with the person → publish | until every item has an owner | prompt chaining | A |
| `okr_review` | quarterly objectives review | collect evidence → score key results → explain gaps → propose next quarter → person decides | none | prompt chaining | A |
| `incident_comms` | customer-facing communications during an incident | facts → audiences → drafts → legal read → approval → publish | legal read ≤ 2 | maker_checker | B |

### Policy and civics

| Name | Goal | States | Loop | Pattern | Tier |
|:--|:--|:--|:--|:--|:--|
| `policy_brief_stakeholders` | a brief tested against simulated stakeholders | evidence → draft → stakeholder simulation (4–6 personas, sequential) → revise ⟲ → sign-off | until objections are addressed, ≤ 3 | Generative Agents, Habermas | B |
| `legislative_impact` | impact analysis of a bill | summary → affected groups → cost and benefit per group → legal consistency → options → brief | none | prompt chaining, broadcast_collect | A |
| `consultation_synthesis` | synthesis of public comments | cluster → positions → common ground → statement → critique rounds ⟲ → final statement | until approval rises no further, ≤ 3 | Habermas Machine | B |
| `deliberative_minipublic` | a simulated citizens' assembly on one question | briefing → small-group rounds (sequential) → plenary → recommendations → minority report | rounds ≤ 3 | group_chat | B |

### Education and creative

| Name | Goal | States | Loop | Pattern | Tier |
|:--|:--|:--|:--|:--|:--|
| `lesson_design_with_simulation` | lesson plan tried on simulated students | goals → plan → three students → misconception log → revise ⟲ → exam → validation | until the students pass, ≤ 3 | Self-Refine plus simulation | B |
| `socratic_tutoring` | tutoring that asks, never tells, until the learner explains it back | diagnose → question ⟲ → learner explanation → check → next concept | per concept, ≤ 6 turns | evaluator-optimizer | A |
| `exam_generation` | an exam with validated answers | blueprint → items → blind solve (`see_prior: false`) → disagreement fix ⟲ → key | fix ≤ 2 | maker_checker | A |
| `game_design_loop` | mechanics tested by simulated playtests | pitch → rules → three playtests → findings → revise ⟲ → rulebook | revise ≤ 3 | broadcast_collect, maker_checker | B |

### Data and personal

| Name | Goal | States | Loop | Pattern | Tier |
|:--|:--|:--|:--|:--|:--|
| `analysis_plan_execute_audit` | an analysis with a blind audit | plan → execute → audit (`see_prior: false`) → fix ⟲ → report | fix ≤ 2 | plan-and-execute | B |
| `data_cleaning_gates` | cleaning with validation gates | profile → rules → apply → validate ⟲ → sign-off | until validation passes, ≤ 3 | evaluator-optimizer | C (check commands) |
| `weekly_review` | a personal weekly review | collect → review → prioritise → plan → commit | none | prompt chaining | A |

### First batch

In order of value over cost of console work: `draft_critique_revise` (done), `decision_memo_council`, `red_team_review`, `pre_mortem`, `editorial_board`, `meeting_to_actions`, `exam_generation`, `socratic_tutoring`, `legislative_impact`, `advisor_checked_task` — all tier A — then the tier B research set (`literature_review_storm`, `security_review_council`, `peer_review_simulation`). `spec_driven_delivery` is the most-adopted pattern in the survey but needs the approval card and a handoff to `/plan`, so it is the first tier-C candidate once the `x-axlr` extension exists.

## Sources

Anthropic: [Building effective agents](https://www.anthropic.com/engineering/building-effective-agents), [multi-agent research system](https://www.anthropic.com/engineering/multi-agent-research-system). Microsoft: [Agent Framework orchestrations](https://learn.microsoft.com/en-us/agent-framework/workflows/orchestrations/), [Magentic-One](https://arxiv.org/abs/2411.04468). [LangGraph workflows](https://docs.langchain.com/oss/python/langgraph/workflows-agents), [CrewAI processes](https://docs.crewai.com/en/concepts/processes), [OpenAI Agents SDK handoffs](https://openai.github.io/openai-agents-python/handoffs/), [Google ADK workflow agents](https://adk.dev/agents/workflow-agents/), [Strands multi-agent patterns](https://strandsagents.com/1.x/documentation/docs/user-guide/concepts/multi-agent/multi-agent-patterns), [Claude Code subagents](https://code.claude.com/docs/en/sub-agents), [spec-kit](https://github.com/github/spec-kit), [Kiro specs](https://kiro.dev/docs/specs/), [Aider architect mode](https://aider.chat/2024/09/26/architect.html). Papers: [Reflexion](https://arxiv.org/abs/2303.11366), [Self-Refine](https://arxiv.org/abs/2303.17651), [Tree of Thoughts](https://arxiv.org/abs/2305.10601), [LATS](https://arxiv.org/abs/2310.04406), [multi-agent debate](https://arxiv.org/abs/2305.14325), [Mixture of Agents](https://arxiv.org/abs/2406.04692), [STORM](https://arxiv.org/abs/2402.14207), [Co-STORM](https://arxiv.org/abs/2408.15232), [AI Scientist](https://arxiv.org/abs/2408.06292), [Agent Laboratory](https://arxiv.org/abs/2501.04227), [Co-Scientist](https://arxiv.org/abs/2502.18864), [Generative Agents](https://arxiv.org/abs/2304.03442), [AgentReview](https://arxiv.org/abs/2406.12708), [Habermas Machine](https://doi.org/10.1126/science.adq2852), [MetaGPT](https://arxiv.org/abs/2308.00352), [ChatDev](https://arxiv.org/abs/2307.07924), [CAMEL](https://arxiv.org/abs/2303.17760), [SWE-agent](https://arxiv.org/abs/2405.15793), [OpenHands](https://arxiv.org/abs/2407.16741), [llm-council](https://github.com/karpathy/llm-council).
