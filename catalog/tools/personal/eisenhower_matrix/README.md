# eisenhower_matrix

Places each task in one of the four quadrants of the Eisenhower (urgent/important) matrix and
turns them into a plan.

| | Urgent | Not urgent |
|:--|:--|:--|
| Important | 1 Do first | 2 Schedule |
| Not important | 3 Delegate | 4 Eliminate |

**Urgency** of a task, in this order: the boolean `urgent` if given; otherwise an `urgency` score
(1–5) at or above `thresholds.urgency` (default 4), and a lower score is still urgent when the
task is due within the window; otherwise its `due` date: urgent when it falls within
`thresholds.due_within_days` (default 2) of `today`, or is overdue; otherwise not urgent.
**Importance**: the boolean `important`, else an `importance` score at or above
`thresholds.importance` (default 3), else not important. Each task says `why` it landed where it
did, with its days left and an `overdue` flag. Days are counted from the `today` in the input,
never from the system clock, so the answer is reproducible; `today` is required when any task
has a due date.

**The plan** lists quadrant 1, then 2, then 3 (quadrant 4 only with `include_eliminate`); within a
quadrant, earliest due date first (tasks without one last), then smallest `effort` first (quick
wins), then higher importance score, then input order. The `markdown` field has the 2×2 table,
the numbered plan and the dropped tasks.

The tests check a hand-sorted list of eight tasks (scores, flags and due dates around
2026-10-18), effort ordering, overdue tasks, the due-date override of a low urgency score and the
Markdown.

Limits: no calendars or capacity planning (it does not check that the scheduled work fits the
time available), no dependencies between tasks, no recurring tasks.
