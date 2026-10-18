# eu_vat_validate

Offline check of EU VAT identification numbers, the ones printed on intra-community invoices.
For each prefix it applies that state's format and check algorithm:

| Prefix | Check |
|:--|:--|
| AT | U + 8 digits, Luhn-like with offset 4 |
| BE | 10 digits, 97 − (first 8 mod 97) |
| BG | 9 digits weighted mod 11, or 10 digits as a personal, foreigner or other number |
| CY | 8 digits + letter from a weighted sum mod 26 |
| CZ | 8 digits mod 11 (entities), 9 or 10 digit birth numbers, 9 digits starting with 6 |
| DE, HR | ISO 7064 MOD 11,10 |
| DK | weights 2 7 6 5 4 3 2 1, mod 11 = 0 |
| EE | weights 3 7 1, mod 10 |
| EL | powers of two, mod 11 mod 10 (GR is read as EL) |
| ES | DNI, NIE, K/L/M or entity NIF rules |
| FI, PL, PT, SI | weighted sums mod 11 |
| FR | key = (12 + 3 × (SIREN mod 97)) mod 97, SIREN by Luhn |
| HU | weights 9 7 3 1, mod 10 |
| IE | weighted mod 23 letter, old and new formats |
| IT | Luhn plus a valid office code |
| LT | 9 or 12 digits, weights 1–9 cycling, mod 11 |
| LU | first 6 digits mod 89 |
| LV | entity weights mod 11 = 3, or personal codes |
| MT | weights mod 37 = 0 |
| NL | 9 digits + B + 2, the old mod 11 or the 2020 mod 97 over "NL…" |
| RO | weights 7 5 3 2 1 7 5 3 2, ×10 mod 11 |
| SE | 10 digits by Luhn + 01 |
| SK | divisible by 11, third digit 2, 3, 4, 7, 8 or 9 |
| XI | the UK's weighted mod 97 (and the 55 offset), GD and HA numbers |

The tests run python-stdnum's documented example for every state, and the same numbers with
one check digit changed.

Limits: a well-formed number may still be unregistered or revoked; the European Commission's
VIES service answers that, over the network. GB numbers are refused as EU VAT numbers since
2021.
