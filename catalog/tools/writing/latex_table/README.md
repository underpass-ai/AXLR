# latex_table

Converts CSV text into a LaTeX `tabular`.

- **Parsing**: Python's `csv` module (quoted cells, doubled quotes), any one-character
  `delimiter`; empty lines are skipped and short rows are padded with empty cells (with a
  `warning`).
- **Escaping**: the reserved characters listed in the LaTeX2e unofficial reference manual
  (§23.1, *Printing special characters*) are written so they print as themselves: `\#`, `\$`,
  `\%`, `\&`, `\{`, `\}`, `\_`, and for the last three `\~{}`, `\^{}` and `\textbackslash{}`.
  The caption is escaped too; `escape: false` passes cells through (for cells that already
  hold LaTeX).
- **Alignment**: a column whose non-empty body cells are all numbers (`-1`, `2.50`, `.5`,
  `1e3`) is `r`, any other `l`; `align` sets it by hand (`l`, `c`, `r`, `S`).
- **Rules**: `\hline` above, below the header and at the end, or with `booktabs: true`
  `\toprule`, `\midrule` and `\bottomrule`.
- **siunitx**: with `siunitx: true` numeric columns become `S` columns (aligned on the
  decimal point); the header cell and any non-numeric cell of an `S` column are wrapped in
  braces so siunitx does not parse them.
- **Float**: with a `caption` or `label`, the tabular goes inside
  `\begin{table}[htbp] \centering \caption{…} \label{…}`.

`packages` lists what the preamble needs (`booktabs`, `siunitx`).

The tests check the reference manual's list of special characters and their printed forms,
and a small booktabs table checked line by line by hand.

Limits: no multi-column or multi-row cells, no `longtable` for tables longer than a page, no
column widths (`p{}`); Unicode characters are left as they are (fine with LuaLaTeX, XeLaTeX
or `inputenc` UTF-8).
