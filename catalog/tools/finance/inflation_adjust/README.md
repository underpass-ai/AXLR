# inflation_adjust

Moves an amount between two years or two months with a consumer price index:
`adjusted = amount × index(to) / index(from)`, in `Decimal`, cents rounded half up. It also
returns the factor, the cumulative inflation in percent and the annualised rate
`factor^(1/years) − 1`. `to` may be earlier than `from` (deflating).

Bundled series (`cpi.json`; `list_series: true` lists them with their coverage). Every answer
carries the series' agency, id, base, source URL and fetch date in `source`:

| Series | What | Coverage |
|:--|:--|:--|
| `us_cpi_u` | BLS CPI-U, U.S. city average, all items, not seasonally adjusted (`CUUR0000SA0`), 1982-84=100. Annual values are BLS's own annual averages (period M13) | years 1913–2025, months from 1913-01 |
| `ea_hicp` | Eurostat HICP, all items, euro area (changing composition), 2015=100: annual average index `prc_hicp_aind` (`INX_A_AVG`) and monthly index `prc_hicp_midx` (`I15`) | 1996–2025 (Eurostat's dataset ends there) |
| `es_ipc` | INE IPC, national general index, base 2025 (`IPC290751`), monthly. INE's API gives months only, so the annual value is the mean of the twelve months rounded to 3 decimals, for complete years | years 2002–2025, months from 2002-01 |

Or give `index` with your own values keyed by year or `YYYY-MM` (another country, a regional
or sector index, a newer release).

The file is written by `tools/catalog/data/inflation_adjust.py`, which fetched it on
2026-10-18 from the BLS public API v1 (POST, ten years per request, `annualaverage`), the
Eurostat dissemination API and INE's `wstempus` API. BLS's flat file (`download.bls.gov`)
answers 403 to scripts, so the API is used instead. Spain comes from INE, not the Eurostat
HICP fallback. All three sources were fetched; none is missing. Rerun the generator for newer
data (that is a new version of the tool).

The tests check the bundled data against figures the agencies published: BLS's CPI-U annual
averages of 258.811 for 2020 and 292.655 for 2022 and its 8.0 % annual-average rise in 2022,
Eurostat's 8.4 % euro-area annual average inflation for 2022, INE's 3.5 % average rise in 2023,
and the base-year average of 100 for INE's base 2025.

Limits: no seasonal adjustment and no chain-linking across series; a month the agency did not
publish (CPI-U for October 2025) is an error rather than an interpolation; Eurostat's euro-area
series uses the composition of each year, not a fixed set of countries.
