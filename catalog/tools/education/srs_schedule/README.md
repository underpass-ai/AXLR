# srs_schedule

SuperMemo 2, the algorithm behind most flashcard programs (Anki's scheduler descends from it).
After each review the card's quality `q` (0–5) updates its easiness factor

    EF' = EF + (0.1 − (5 − q)·(0.08 + (5 − q)·0.02)),  never below 1.3

and its interval: 1 day after the first successful review, 6 after the second, then the
previous interval times EF', rounded up to whole days. A grade below 3 is a lapse: the
repetitions start again (interval 1) and EF is left as it was, as the original description
says. EF is computed with fractions, so 2.5 − 0.14 is exactly 2.36.

Give one review (`quality` plus the card's `repetitions`, `easiness`, `interval`, and an
optional `review_date` for the next date), or a whole `reviews` history to replay from a new
card.

Limits: SM-2 only. FSRS, which fits a memory model to a person's history, needs the `fsrs`
package and is not part of this tool. SuperMemo's advice to repeat the same day every card
graded below 4 is left to the caller.
