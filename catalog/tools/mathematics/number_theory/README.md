# number_theory

Exact integer arithmetic on numbers up to 200 digits. Inputs may be JSON integers or decimal
strings; integer results are always decimal strings, so no JSON parser rounds them.

- `factor`: trial division by the primes below 10000, then Pollard's rho in Brent's form with
  f(x) = x² + c, x₀ = 2 and c = 1, 2, … (fixed seeds, so the run is reproducible). The search has
  a 3-second budget; a composite it cannot split is returned in `unfactored` with
  `complete: false` rather than looping. Factors of up to about 20 digits come out quickly
  (2⁶⁷ − 1 in milliseconds); two 20-digit primes multiplied together usually do not.
- `is_prime`: strong-pseudoprime (Miller–Rabin) tests. Below 3 317 044 064 679 887 385 961 981
  the first 13 prime bases (2 to 41) make the answer deterministic (Sorenson and Webster); above
  it the first 25 primes (2 to 97) are used and a "prime" answer has `certainty: "probable"`.
- `gcd`, `lcm` of a list; `egcd` returns Bézout coefficients with a·x + b·y = gcd.
- `mod_inverse` (null with the gcd when none exists), `mod_pow` (a negative exponent uses the
  inverse).
- `totient` and `divisors` (count, sum σ and, up to 10000 of them, the list) from the full
  factorisation; they fail if the factorisation is incomplete.
- `crt`: merges congruences pairwise with the extended Euclidean algorithm, so moduli need not be
  coprime; incompatible systems return `solution: null` with the conflicting pair.
- `next_prime` (smallest prime > n), `prev_prime` (largest prime < n).

The tests check historical and published values: Cole's 1903 factorisation 2⁶⁷ − 1 =
193707721 × 761838257287, Euler's 2³² + 1 = 641 × 6700417, the strong pseudoprime
318665857834031151167461 to every base from 2 to 37 (caught by 41), the Carmichael number 561,
φ(36) = 12, the Sunzi Suanjing remainder problem (23 mod 105), and the textbook examples
240·(−9) + 46·47 = 2 and 4¹³ mod 497 = 445.

Limits: no quadratic sieve or ECM, so hard semiprimes stay unfactored; primes above 3.3 × 10²⁴
are probable, not proven.
