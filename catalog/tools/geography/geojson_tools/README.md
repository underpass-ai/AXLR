# geojson_tools

Measures and checks GeoJSON (RFC 7946) passed inline, as an object in `geojson` or as a JSON
string in `text`: a Geometry (any of the seven types, GeometryCollection included), a Feature
or a FeatureCollection. Positions are `[lon, lat]` in degrees, GeoJSON order.

| `operation` | Result |
|:--|:--|
| `summary` (default) | everything below, plus `per_feature` bbox and area for a FeatureCollection |
| `bbox` | `[min lon, min lat, max lon, max lat]` |
| `centroid` | area-weighted centroid of the polygons; else length-weighted of the lines; else the mean of the points |
| `area` | spherical km² and m², planar square degrees (exact) |
| `perimeter` | polygon rings (holes included) and line lengths, spherical km and planar degrees |
| `contains` | `point` or `points`: `inside`, `on_boundary`, and for a collection the indices of the features that cover each point |
| `validate` | `valid`, counts and the list of `issues` with a path for each |

Method:

- **Spherical area** is the spherical excess of each ring on a sphere of R = 6371.0088 km (the
  IUGG mean radius R₁; `radius_km` changes it), with great-circle edges. Each edge contributes
  the excess of the quadrangle it forms with the equator and the meridians of its ends,
  `tan(E/2) = tan(Δλ/2)·sin((φ₁+φ₂)/2)/cos((φ₂−φ₁)/2)`, computed as
  `2·atan2(tan(Δλ/2)(t₁+t₂), 1+t₁t₂)` with `t = tan(φ/2)`; the ring's area is R²·|ΣE| and holes
  are subtracted. A ring whose longitudes wind once round a pole (ΣΔλ = ±360°) takes its
  interior on its left, the RFC 7946 counter-clockwise rule, so its area is R²(2π − ΣE). This
  is a sphere: it is not the ellipsoidal (WGS-84) area, which differs by up to about 0.5 %.
- **Planar** measures treat lon/lat as plane coordinates: shoelace area and centroid summed
  exactly over fractions (exterior rings add and holes subtract whatever their winding), returned
  also as exact fractions; perimeter as the sum of Euclidean segment lengths in degrees.
  Spherical perimeters and lengths sum great-circle (haversine) edge lengths.
- **Point in polygon**: even-odd ray casting in the lon/lat plane, exact over fractions; a point
  on an edge or vertex is `on_boundary` and not `inside`; a point in a hole is outside.
- **Validation** (errors): unknown types, missing members, non-numeric positions, longitude
  outside −180..180 or latitude outside −90..90, rings that are not closed or have fewer than four
  positions, LineStrings of fewer than two positions, rings that cross themselves (checked exactly
  on rings up to 1000 vertices). Warnings: wrong winding (RFC 7946 §3.1.6 asks for counter-clockwise
  exteriors and clockwise holes, but says parsers should not reject other polygons), holes not
  inside their exterior, zero-area rings, positions with more than three members, Features
  without `properties`. Any operation other than `validate` refuses input with errors (exit 2).

The tests check the spherical area against figures computed independently: the 1°×1° cell at
the equator is 12 364.03 km² (L'Huilier's theorem on two triangles gives the same; the hand
formula R²·Δλ·(sin φ₂ − sin φ₁) = 12 363.72 km² is the cell bounded by the parallel at 1°N, and
the great-circle top edge bulges poleward to enclose 0.31 km² more); the octant triangle
(0,0)–(90,0)–(0,90) is exactly 4πR²/8; the four-vertex ring at 80°N matches L'Huilier on the
four pole triangles. The planar results are checked by hand: unit square, a 4×4 square with a
1×1 hole (centroid 61/30), an L-shape (centroid 5/6) and a MultiPolygon (centroid 13/6).

Limits: no antimeridian splitting (an edge is taken the short way round for spherical area but
the planar measures, bbox and point-in-polygon do not wrap); planar centroid and containment are
in lon/lat, which distorts away from the equator; heights are ignored; at most 100000 positions.
