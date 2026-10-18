# stats_describe

Descriptive statistics without spreadsheet rounding. Values may be numbers or decimal strings;
sums, means, variances and the regression slope and intercept are computed over `Fraction`s and
also returned exactly (`exact`), so `[2, 4, 4, 4, 5, 5, 7, 9]` has sample variance `32/7`.

One variable (`data`): count, sum, mean, median, modes (empty when every value is unique),
min, max, range, sample and population variance and standard deviation, standard error,
coefficient of variation, quartiles and IQR (`exclusive`, the (n + 1)p rule of R type 6 and
Minitab, or `inclusive`, R type 7 and Excel's `QUARTILE.INC`), moment skewness, and the
Student-t confidence interval for the mean at `confidence` (0.95 by default). The t quantile is
computed by inverting the t distribution through the regularized incomplete beta function
(Numerical Recipes, 6.4); for 7 degrees of freedom at 95 % it is 2.3646, as in the tables.

Two variables (`x`, `y`): the description of each, Pearson's r, Spearman's ρ (average ranks for
ties), and the least-squares line with r² and the two-sided p-value of the slope. The tests use
Anscombe's first data set (slope 0.500, intercept 3.00, r 0.816).

Limits: displayed floats carry 12 significant digits; no weighting, no outlier tests.
