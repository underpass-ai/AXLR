# seat_allocation

Allocates seats among parties from their votes with four methods used in real
legislatures: D'Hondt (Spain, Portugal, Belgium, Finland, Netherlands), Sainte-Laguë (Germany,
Norway, New Zealand), modified Sainte-Laguë (first divisor 1.4; Sweden) and Hare largest
remainder (Hong Kong, Colombia's old system). An optional threshold in percent of valid votes
excludes parties below it before allocation. Arithmetic is exact (`fractions.Fraction`), so a
tie is a real tie; ties go to the larger vote count and then to the party name, and are
reported in `ties`.

Limits: one district per call (call it per district for a national result); no overhang,
levelling or apparentement; blank and null votes must already be excluded from `votes`.

Method references: Gallagher & Mitchell, *The Politics of Electoral Systems* (2005), appendix A.
