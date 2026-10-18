# cashflow_forecast

Month-by-month cash projection in `Decimal`. Each item has a signed `amount` (positive comes
in, negative goes out) and a `frequency`:

- `monthly`: every month from its `start`;
- `quarterly`: its `start` month and every third month after it;
- `yearly`: its `start` month each year;
- `once`: only its `start` month.

`start` defaults to the first month of the forecast; an item that started earlier keeps its
rhythm (a quarterly item from January falls in April, July, October). `end` is inclusive.
`growth_pct` compounds once for every full year since the item's start, so a 3 % rent of 1,000
from January 2026 is 1,030 from January 2027 and 1,060.90 from January 2028. Each occurrence is
rounded to the cent. `day` is accepted so that items copied from a calendar validate, but the
forecast is monthly and ignores it.

For every month the output gives the opening balance, inflows, outflows (as a positive
amount), net flow and closing balance; then the totals, the closing balance, the minimum
closing balance and its month, the first month whose closing balance is below zero and how many
months are. With `detail: true` each month also lists the amount of each item that fell in it.

The tests are worked by hand: a six-month budget with monthly, quarterly, yearly and one-off
items, the yearly growth steps, an item that ends, and the quarterly rhythm of an item that
began before the forecast.

Limits: no intra-month timing (a month's flows are netted before the balance is checked), no
interest on the balance, no business-day or end-of-month rules, at most 600 months.
