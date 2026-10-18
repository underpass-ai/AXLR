# coord_convert

Converts a point on WGS-84 between the notations people paste into forms and maps: decimal
degrees, degrees–minutes–seconds (DMS), degrees and decimal minutes (DDM), UTM, MGRS and geohash.
`operation: "all"` (the default) takes `lat`/`lon` or `position` and returns every notation;
the other operations do one conversion each.

| `operation` | Input | Output |
|:--|:--|:--|
| `format` | `lat`, `lon` or `position` | decimal, DMS and DDM strings |
| `to_utm` | point, optional `zone` | zone, hemisphere, band, easting, northing |
| `from_utm` | `zone`, `hemisphere` or `band`, `easting`, `northing` | point |
| `to_mgrs` | point, `mgrs_digits` (0–5) | MGRS string |
| `from_mgrs` | `mgrs`, `corner` | point and its UTM |
| `geohash_encode` | point, `geohash_length` (1–12) | geohash and its error bounds |
| `geohash_decode` | `geohash` | centre, error bounds and bbox |

**Parsing.** A coordinate is a number or a string: `"40°26′46″N"`, `"40 26 46 N"`,
`"40:26:46.302N"`, `"N 40 26.767"`, `"E44d24"`, `"40°26'46.302\"N"`, `"-79.9822"`. Degree marks
`°` `º` `d` `:`, minute marks `′` `'`, second marks `″` `"` `''`; a hemisphere letter before or
after (not with a sign); only the last component may carry decimals; minutes and seconds must be
below 60; latitudes within ±90, longitudes within ±180, and N/S only on a latitude. `position`
holds both coordinates: split at a comma, or else at the single whitespace break where both
halves read as angles (`"40 26 46 79 58 56"` works; `"40 26 30 10"` is ambiguous and refused);
hemisphere letters decide which is the latitude, so GeoConvert's `"E44d24 N33d20"` reads
correctly. Formatting rounds once, at the last place, so 59.99999999° becomes 60°00′00.00″.

**UTM.** Transverse Mercator by Krüger's series in the third flattening n to sixth order with
the coefficients of Karney (2011), the method of GeographicLib: conformal latitude, the α series
forward, the β series and a Newton step on tan φ inverse; k₀ = 0.9996, false easting 500 km,
false northing 10 000 km south of the equator. The standard zone is ⌊(λ + 180)/6⌋ + 1 with the
exceptions of the grid: zone 32 covers 3–12°E between 56 and 64°N (band V, Norway), and between
72 and 84°N (band X, Svalbard) zones 31, 33, 35 and 37 cover 0–9, 9–21, 21–33 and 33–42°E.
Bands are 8° from 80°S (C to X without I and O; X spans 72–84°N). `zone` forces a neighbouring
zone up to 9° from its central meridian. Checked during development: forward then inverse
agrees to a few nanometres over 20 000 random points; the central-meridian northing matches a
numerically integrated meridian arc to 10⁻⁷ m; off the meridian it agrees with Snyder's USGS
series (Professional Paper 1395) to under a millimetre.

**MGRS.** The 100 km square letters of the WGS-84 lettering (column sets A–H, J–R, S–Z by zone
mod 3; rows A–V with a five-letter offset in even zones). The zone is written with two digits
(`04QFJ…`, as GeographicLib; `4QFJ…` is read too). Output digits are truncated, not
rounded, so the string names the square the point is in; input returns the square's centre by
default (GeographicLib's convention) or its south-west corner, and the 2000 km row cycle is
resolved with the band letter.

**Geohash.** Niemeyer's interleaved bisection, longitude first, in the base-32 alphabet without
a, i, l and o; decoding returns the centre of the cell and half its width as the error.

The tests reproduce the examples printed in GeographicLib's GeoConvert manual page
(`38SMB4488` → 33.33424 44.40363, in DMS 33°20′03.25″N 044°24′13.06″E, UTM `38n 444500 3688500`;
`E44d24 N33d20` at 1 km → `38SMB4488`) and the two examples of Wikipedia's Geohash article
(`ezs42` → 42.605, −5.603 ± 0.022; 57.64911, 10.40744 → `u4pruydqqvj`), plus the Norway and
Svalbard zones and the parsing rules.

Limits: WGS-84 only, no datum shifts; no UPS, so no UTM or MGRS beyond 80°S and 84°N; MGRS
uses the current (AA) lettering, not the old Bessel/Clarke one; geohash neighbours are not
computed.
