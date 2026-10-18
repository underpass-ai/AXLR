# seat_allocation_droop

Allocates seats among parties by the largest remainder method with one of four quotas, where
`V` is the valid votes of the parties that pass the threshold and `S` the seats:

| `quota` | formula |
|---|---|
| `droop` (default) | `floor(V / (S + 1)) + 1`, the integer Droop quota |
| `hagenbach_bischoff` | `V / (S + 1)`, exact |
| `imperiali` | `V / (S + 2)`, exact |
| `hare` | `V / S`, exact, for comparison |

Each party's votes divided by the quota give its quotient; the whole part is its automatic
seats and the rest of the seats go one each to the largest remainders. Arithmetic is exact
(`fractions.Fraction`); quotients and remainders print rounded to six decimals. A tie at the
remainder cut goes to the larger vote count, then the party name, and is listed in `ties`.

A small quota can hand out more whole seats than exist (Imperiali can; Hagenbach-Bischoff only
when quotients are exact integers). No single rule governs that case, so the tool states the one
it applies: with `on_overallocation: "smallest_remainders"` it takes one seat each back from the
seat-holding parties with the smallest remainders and reports them in `overallocation`; with
`"error"` it exits with a usage error.

An optional `threshold` (percent of all valid votes) removes parties before the quota is
computed, so the quota is based on the votes of the parties still in the count.

Tests: the six-party example of Wikipedia's *Largest remainder method* article (Hare quota, 11
seats: 5, 2, 2, 1, 1, 0 with remainder seats to Whites, Reds and Blues), and the same votes by
hand with the Droop quota for 10 seats (9091; Blues lose their seat) and Imperiali for 11 seats
(every seat filled by whole quotas: 6, 2, 2, 1, 0, 0).

Limits: one district per call; no list alliances or national compensation; blank and null
votes must already be excluded from `votes`. For divisor methods (D'Hondt, Sainte-Laguë) use
`seat_allocation`.
