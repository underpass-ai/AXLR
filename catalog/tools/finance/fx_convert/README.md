# fx_convert

Converts an amount between currencies with the euro foreign exchange reference rates the
European Central Bank publishes each TARGET business day around 16:00 CET. Tier 3: one HTTPS
request per call (15 s timeout, User-Agent `AXLR-catalog-tool/1.0 (+https://axlr.underpassai.com)`,
no key) to

- `https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml` without `date` (the latest day), or
- `https://www.ecb.europa.eu/stats/eurofxref/eurofxref-hist-90d.xml` with `date` (any of the last 90 days).

Offline or cached use: put the exact XML in `response`; no connection is opened and the same
parser runs (every `<Cube time="…">` in the file, so a daily and a 90-day file both work). The
tests run that way.

## Method

The ECB quotes each currency per euro. With EUR = 1:

    converted = amount × (to per EUR) / (from per EUR)

computed in `Decimal` with 40 digits and rounded half up to `decimals` (default 2) only at the
end. `rate` (units of `to` per unit of `from`) and `inverse_rate` are shown to 10 significant
digits; `per_eur` repeats the two published rates; `method` says whether the answer is a
direct ECB rate or a cross rate through the euro. Every answer has `rate_date` and the ECB's
notice that the rates are for information only and not meant for transactions.

A `date` with no rates (weekends, 1 January, Good Friday, Easter Monday, 1 May, 25–26 December)
is an error unless `use_previous` is true, which takes the latest earlier day; `rate_date` then
differs from `requested_date`. Dates older than the 90-day file are refused: the full history is
a several-megabyte file this tool does not read.

## Tests

From the ECB files fetched on 2026-10-18 (daily: rates of 2026-10-16; 90-day file trimmed to
2026-10-09…16, cut at day boundaries, values untouched):

- 100 EUR → 112.53 USD at the published 1.1253.
- 100 USD → JPY as a cross rate, worked by hand: 1.1253 × 158 = 177.7974; 177.82 − 177.7974 =
  0.0226; 0.0226 / 1.1253 = 0.0200835…, so the rate is 158.0200835… and 100 USD = 15 802.01 JPY.
- 1000 GBP → 1185.04 EUR (1000 / 0.84385), whose inverse rate is the published 0.84385.
- 2026-10-15 at 1.1208, and Sunday 2026-10-11 falling back to Friday 2026-10-09 (1.1206).

Limits: only the currencies the ECB quotes (about 30; no RUB since March 2022); no bid/ask, no
intraday rates; a converted amount is an estimate, not a bank's or card network's rate.
