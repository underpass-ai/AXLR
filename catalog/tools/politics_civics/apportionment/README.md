# apportionment

Shares a fixed number of seats among districts (states, provinces, constituencies) in
proportion to their populations. Every district first receives `min_seats`; the rest are
awarded one at a time.

Divisor methods give the next seat to the district with the highest priority value
`population / d(s)`, where `s` is the seats it already holds:

| `method` | `d(s)` | also known as |
|---|---|---|
| `huntington_hill` (default) | `√(s (s + 1))` | equal proportions; the US House since 1941 |
| `webster` | `s + ½` | Sainte-Laguë, major fractions |
| `jefferson` | `s + 1` | D'Hondt, greatest divisors |
| `adams` | `s` | smallest divisors |

`d(0) = 0` for Huntington-Hill and Adams, so they give every district at least one seat
whatever `min_seats` says (the US House rule of 2 U.S.C. 2a). Priorities are compared exactly
(Huntington-Hill by the square of the priority value), so a tie is a real tie; it goes to the
larger population, then the name, and a tie for the last seat is reported. Each award lists
its priority value (Huntington-Hill rounded to 0.001) and `next_priority` is the first value
that missed out.

`hamilton` (largest remainder, Vinton) computes each district's quota `population × seats /
total`, gives the whole part, and hands the remaining seats to the largest fractional parts.
With `min_seats`, districts whose whole quota is below the minimum are raised to it and the
remaining seats are reapportioned among the others, repeating until none falls short
(`raised_to_minimum`).

Tests: Wikipedia's *Huntington–Hill method* example (100 000 / 80 000 / 30 000, 8 seats, one
each first: 4, 3, 1 with priorities 70 711 … 28 868), the Alabama paradox of Wikipedia's
*Largest remainder method* (25 seats give D and E three each, 26 give them two), Wikipedia's
*Sainte-Laguë method* example (Webster 3, 3, 1, 1; D'Hondt four to A and none to D), and Adams
and the minimum-seat rules by hand.

Limits: no maximum seats per district, no overhang; populations must be whole numbers.
