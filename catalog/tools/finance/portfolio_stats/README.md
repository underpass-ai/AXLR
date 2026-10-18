# portfolio_stats

Return and risk statistics of a small price table, given inline as CSV: a date column (the
first, or `date_column`) and one column per asset, oldest row first. With `kind: "returns"` the
cells are simple returns per period instead of prices, and the wealth index starts at 1 on a
row labelled `start`.

For every asset, and for a portfolio when `weights` are given:

- **total return** from the first to the last value, and the **annualised return**
  `(1 + total)^(periods_per_year / n) − 1` (geometric);
- **mean return** per period and annualised (`× periods_per_year`), and the **volatility**, the
  sample standard deviation (n − 1) of the returns, annualised by `√periods_per_year`;
  `return_type` chooses simple returns `p₁/p₀ − 1` or log returns `ln(p₁/p₀)`;
- **Sharpe ratio** `(mean − rf/periods_per_year) / sd × √periods_per_year`, with
  `risk_free_rate` annual;
- **maximum drawdown**, the deepest fall of the wealth index from a running peak, with the
  peak, the trough and the first date the peak is regained (`null` if never).

The portfolio is rebalanced to its weights every period (its return is the weighted sum of
the assets' simple returns); weights are normalised to sum 1. The **correlation** matrix
(Pearson) covers every asset and the portfolio, and with `benchmark` each series' **beta** is
`cov(r, r_b) / var(r_b)`.

Arithmetic is in binary floating point (roots and logarithms), rounded to ten decimals. The
tests use a five-day table worked by hand: A's 25 % drawdown from 120 to 90 regained at 135, the
returns 0.35 and 0.32, the 50/50 portfolio's 1.15 × 0.925 × 137/132 × 87/70 − 1, the benchmark's
beta of 1, the portfolio's beta as the weighted mean of the assets' betas, and the log-return
mean ln(1.1)/3.

Limits: no missing values (every cell must be a number), no dividends or splits beyond what the
prices include, no buy-and-hold drift of the weights, rows are not re-sorted.
