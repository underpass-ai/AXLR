# loan_amortization

Instalment and full schedule of a loan, in `Decimal`. The period rate is
`i = annual_rate / payments_per_year` (a nominal rate, as banks quote it); the term is
`periods` or `years × payments_per_year`.

- **French** (constant payment): `payment = P·i / (1 − (1 + i)^−n)`, rounded to the cent
  (`P / n` at a zero rate). Each period's interest is `balance × i` rounded to the cent and the
  principal is the rest of the payment.
- **German** (constant principal): `P / n` of principal each period, rounded to the cent, plus
  the period's interest, so the payment falls over time.
- **American** (interest only): interest each period, the whole principal with the last one.

In every system the last payment pays off whatever balance is left, so the cent roundings never
leave a residue and the totals add up exactly.

**Early repayments** (`early_repayments`) are paid right after the regular payment of their
`period`. `reduce_term` keeps the instalment (French) or the principal step (German) and the
loan ends sooner; `reduce_payment` keeps the end date and recomputes the instalment or the step
over the remaining periods. In the American system both modes only lower the balance and the
interest; the rest of the principal is still due at the end. With early repayments the output
also gives the interest and periods of the same loan without them and the interest saved.

The tests check the instalment against published examples: the standard mortgage of 100,000 at
5 % over 30 years (536.82 a month), Excel's PMT help example (10,000 at 8 %/12 over 10 months,
1,037.03) with its schedule checked by hand, and numpy-financial's `pmt` example (200,000 at
7.5 % over 15 years, 1,854.02). The German and American schedules are checked by hand.

Limits: fixed rate only; no fees, insurance, grace periods or day-count conventions (every
period is `1 / payments_per_year` of a year); early-repayment penalties are not applied.
