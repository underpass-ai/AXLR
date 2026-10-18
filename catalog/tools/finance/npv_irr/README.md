# npv_irr

Capital-budgeting arithmetic for one series of cash flows, period 0 first (outflows negative).

- **NPV** at `rate` (a fraction, `0.08`, or a percent string, `"8%"`), in `Decimal` at 40
  digits, rounded to six places in the output.
- **IRR**: every rate between −99 % and 10 000 % where the NPV crosses zero, found on a grid and
  refined by Newton's method with a bisection fallback. When there is exactly one, `irr` is it;
  when the flows change sign more than once and several rates qualify, `irr` is `null`, all of
  them are in `irr_candidates`, and a note says to use NPV or MIRR.
- **MIRR**: positive flows compounded to the end at `reinvest_rate`, negative flows discounted to
  the start at `finance_rate` (both default to `rate`).
- **Payback** and **discounted payback**: the fractional period where the cumulative flow turns
  non-negative, interpolating within the period; `null` if it never does.
- **Profitability index**: present value of the inflows over that of the outflows.

The tests are the examples of numpy-financial's documentation (IRR 0.2809, NPV −0.00848 at
0.281) and of Excel's IRR (8.66 %) and MIRR (12.61 %) help pages.

Limits: periods are equal and flows fall at their ends; no XNPV/XIRR with dates.
