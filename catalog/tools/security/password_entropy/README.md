# password_entropy

Estimates how hard a password is to guess. **The password is never written to the output**,
nor any piece of it: each detected pattern is reported by its type, start position, length and
cost in bits. Error messages do not quote it either.

## Method

The password is normalised to NFKC and counted in Unicode code points, as NIST SP 800-63B asks.

- **Pool bound**: `length × log2(pool)`, where the pool adds 26 for lower-case letters, 26 for
  upper-case, 10 for digits, 32 for ASCII symbols, 1 for the space and 100 for any other
  Unicode character, for each class present (the formula of NIST SP 800-63 v1.0.2 appendix A
  for randomly chosen passwords). It is an upper bound: it assumes every character was random.
- **Estimate**: the pool bound minus what each detected pattern saves an attacker, choosing
  non-overlapping patterns greedily by saving (an approximation of zxcvbn's approach):

| Pattern | Cost in bits |
|:--|:--|
| common password (4–32 characters, anywhere inside) | log2(rank) + 1, + 1 if the case differs from all lower-case |
| the same with l33t substitutions undone (`@`→a, `0`→o, `1`→i or l, `$`→s…) | + 1 more |
| context word (from `context`, split at `@ . _ -`, 4+ characters) | log2(words + 1) + 1 |
| keyboard run of 4+ (QWERTY, QWERTZ, AZERTY rows and columns), either direction | log2(47 × 2) + log2(length) |
| alphabetic or numeric sequence of 4+, either direction | log2(36 × 2) + log2(length) |
| one character 3+ times | log2(its pool) + log2(count) |
| repeated block (`abcabc`) | block length × log2(its pool) + log2(repeats) |
| year 1900–2099 | log2(200) |
| date of 8 digits (YYYYMMDD, DDMMYYYY, MMDDYYYY), of 6, or with separators | log2(366 × 200), log2(366 × 100), + 2 |

A password that is itself in the list is capped at log2(rank) + 1.

- **Crack time**: the average number of guesses, 2^(bits − 1), divided by each rate: 100 per hour
  (throttled online login), 10 per second (unthrottled), 10⁴ per second (offline, slow hash
  such as bcrypt or Argon2), 10¹⁰ per second (offline, fast hash on GPUs), plus any rates in
  `guesses_per_second`.
- **Rating**: under 28 bits very weak, under 36 weak, under 60 fair, under 80 strong, else very
  strong.
- **Advice** after NIST SP 800-63B: at least 8 characters (15 when the password is the only
  factor, revision 4), reject blocklisted, repetitive, sequential and context-specific
  values, no composition rules or forced periodic changes, allow 64+ characters and pasting.

The blocklist `common_passwords.txt` holds the first 2000 entries (4+ characters) of SecLists'
`10k-most-common.txt` (MIT License) with their rank, written by
`tools/catalog/data/password_entropy.py` on 2026-10-18; the output names it in `blocklist`.

## Tests

The pool formula by hand: 12 lower-case letters = 12 × log2 26 = 56.4 bits, 8 printable ASCII
characters = 8 × log2 94 = 52.44 bits, 10 digits = 33.22 bits; crack time 2^55.405 / 10¹⁰ =
4.77 × 10⁶ s; the rank-1 entry of the list; l33t, keyboard run, repeat, date, year and context
patterns; and the usage errors. A separate check confirmed that no 5-character piece of 200
random passwords appears in the output.

## Limits

No English dictionary or name lists: a passphrase of common words ("correct horse battery
staple") is overestimated, and so is a dictionary word with mangling the list does not hold.
Patterns are searched in the first 256 characters. The estimate is a heuristic, not a measure
of how the password was chosen; a random password from a generator is as strong as its pool
bound. No breach-corpus lookup (that needs the network).
