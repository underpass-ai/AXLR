# expense_split

Splits shared expenses and says who pays whom to settle.

## Shares

Each expense is paid by one person and shared among `among` (default: everyone):

- equally;
- by `weights` (relative, such as nights stayed: `{"A": 1, "B": 2}`);
- or by exact `shares` that must add up to the amount.

Amounts are exact decimals in the currency's minor unit (`decimals`, default 2), never binary
floating point. When a split does not divide into whole cents, each person first gets the floor
of their exact share and the cents left over go one each to the largest fractional remainders,
ties to the person listed earlier in `among` (the largest-remainder method), so 100.00 among
three is 33.34, 33.33, 33.33 and the shares always add up to the amount.

A person's balance is what they paid minus their share, plus `payments` they have already made
to others and minus those received. Balances always add up to zero.

## Settlement

The minimum number of transfers is the number of people with a non-zero balance minus the
largest number of groups those balances can be split into that each add up to zero. For up to
15 open balances the tool finds that partition exactly (a search over subsets), then settles
each group by paying the largest creditor from the largest debtor, which closes a group of k
people in k − 1 transfers. Above 15 open balances it settles everyone in one group by the same
greedy rule: at most n − 1 transfers, not always the minimum. The output names the method used.

## Tests

A hand-checked trip in the style of Splitwise (A pays 90 for dinner, B pays 30 for a taxi, both
shared by three: balances +50, −10, −40, settled by C → A 40 and B → A 10), the cent rounding of
100.00 among three, weights, a case where the exact search needs 4 transfers and greedy pairing
alone needs 5 (worked out by hand), a payment already made, and the error cases.

## Limits

One currency per run; no conversion. Ties in the settlement follow the order of `participants`.
