# lesson_plan_template

A lesson plan skeleton for a subject, level and duration, in English or Spanish, returned as
structured stages and objectives and as a Markdown plan with an objectives list, a sequence
table and a materials placeholder.

**Stages** follow the presentation-practice-production model, framed by a warm-up, an
assessment and a closure: `warm_up`, `presentation`, `practice`, `production`, `assessment`,
`closure`, with default weights 10, 25, 25, 20, 10 and 10 (`stage_weights` changes them; a
weight of 0 leaves a stage out). Whole minutes are allocated by the largest-remainder (Hamilton)
method: each stage's exact share `duration × weight / Σ weights` (shown as `exact_share`) is
floored, and the minutes left over go to the largest fractional remainders, ties to the earlier
stage, so the stages always add up to `duration_minutes`. Each stage has its start and end
minute and a short activity prompt.

**Objectives** read "By the end of the lesson, students will be able to *verb* *content*" ("Al
final de la sesión, el alumnado será capaz de …"). Given `objectives` (level and content, and an
optional verb), the verb comes from the level's list, taking the next verb when a level repeats.
Without them, `objective_count` objectives (default 3) are drafted at levels suited to `level`
(primary: Remember, Understand, Apply; secondary: Understand, Apply, Analyze; upper secondary:
Apply, Analyze, Evaluate; university: Analyze, Evaluate, Create; adult: Apply, Analyze, Create)
with a `[content about …]` placeholder to complete. The verb lists put the cognitive-process
verbs of the revised taxonomy (Anderson & Krathwohl 2001, Table 5.1: identify/recall, explain,
classify, compare, interpret, use, implement, differentiate, organize, critique, check, design,
produce, plan, generate) first, then common objective verbs.

The tests check two allocations worked by hand (50 minutes: 5, 13, 12, 10, 5, 5, the spare minute
going to the first of the two 12.5 shares; 55 minutes: 6, 14, 14, 11, 5, 5), custom weights
(60 minutes at 1:2:3:1 is 9, 17, 26, 8), the taxonomy verbs chosen for a primary class, and the
Spanish text.

Limits: a skeleton, not a lesson: the activity prompts are generic; the level-to-Bloom mapping
is a starting point, not a rule; no curriculum standards, differentiation or timing within a
stage.
