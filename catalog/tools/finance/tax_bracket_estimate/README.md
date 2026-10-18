# tax_bracket_estimate

Progressive tax on a taxable base, bracket by bracket, in `Decimal`.

Bundled tables (`list_tables: true` lists them with their sources; each answer names the table,
its year, its source and the date it was transcribed):

| Table | Brackets |
|:--|:--|
| `es_irpf_2025_state_general` | Spain, state half of the general-base scale (Ley 35/2006 art. 63.1): 9.5 % to 12,450; 12 % to 20,200; 15 % to 35,200; 18.5 % to 60,000; 22.5 % to 300,000; 24.5 % above |
| `es_irpf_2025_state_savings` | Spain, state half of the savings-base scale (art. 66.1, Ley 7/2024): 9.5 % to 6,000; 10.5 % to 50,000; 11.5 % to 200,000; 13.5 % to 300,000; 15 % above |
| `us_federal_2025_single` | IRS Rev. Proc. 2024-40: 10/12/22/24/32/35/37 % at 11,925 / 48,475 / 103,350 / 197,300 / 250,525 / 626,350 |
| `us_federal_2025_married_joint` | the same rates at 23,850 / 96,950 / 206,700 / 394,600 / 501,050 / 751,600 |

Or pass `brackets` (rates in percent, the last `up_to` `null`), for a regional Spanish scale or
any other country.

An `allowance` is either deducted from the base (`deduct_from_base`, like a standard
deduction) or, as Spain's *mínimo personal y familiar*, taxed at the same scale and the result
subtracted from the tax (`tax_on_allowance`).

The tables come from `tools/catalog/data/tax_brackets.py`, which records each legal source; a
new tax year is a new table and a new version of this tool.

Not tax advice: deductions, credits, surcharges, regional scales and special regimes are not
applied, and the output says so.
