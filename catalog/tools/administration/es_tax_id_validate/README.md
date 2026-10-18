# es_tax_id_validate

Checks the Spanish tax identification number (NIF) in its forms:

| Form | Shape | Check |
|:--|:--|:--|
| DNI (citizen) | 8 digits + letter | letter = `TRWAGMYFPDXBNJZSQVHLCKE`[number mod 23] |
| NIE (foreign national) | X, Y or Z + 7 digits + letter | X, Y, Z become 0, 1, 2, then as a DNI |
| Special NIF | K, L or M + 7 digits + letter | as a DNI on the 7 digits |
| Entity NIF (former CIF) | entity letter + 2-digit province + 5-digit serial + check | digits in odd positions doubled and their digits summed, plus the even ones; check = (10 − sum mod 10) mod 10, as a digit or as `JABCDEFGHI`[check] |

For entities the letter decides the check's form: P, Q, R, S, N and W carry a letter; A, B,
E and H a digit; the rest either. The output names the entity type (A *sociedad anónima*, B
*sociedad de responsabilidad limitada*, G *asociación o fundación*, Q *organismo público*…)
and, when the check fails, the character that would be right.

Short DNIs (fewer than 8 digits) are padded with zeros, and spaces, dots, dashes and an `ES`
VAT prefix are ignored.

Limits: a valid check character does not mean the number was ever issued or is still in use;
only the tax agency can say that.
