# invoice_calc

Invoice arithmetic in `Decimal`, never binary floating point. Per line: `quantity × unit_price`
rounded to the minor unit, then the line discount, then the global discount, each rounded. Lines
are grouped by VAT rate (and equivalence surcharge), and VAT is computed once per rate on the
sum of its bases, as Spanish invoicing rules (Real Decreto 1619/2012) and most EU practice
require. Withholding (`withholding_percent`, such as IRPF 15 % for a Spanish professional) is
computed on the total tax base and subtracted from the total.

Numbers may be given as decimal strings (`"19.99"`) to keep cents exact; JSON numbers are read
through their shortest decimal text, so `0.1` is exactly one tenth.

Limits: rounding is half up, per line and per rate; some jurisdictions round only the total,
which can differ by a cent. No currency conversion; `currency` is a label.
