# haversine

Distance and bearing between two points, or between each pair of a batch, computed twice: on a
sphere with the haversine formula and on the WGS-84 (or GRS80) ellipsoid with Vincenty's
inverse formula. Vincenty is the more exact of the two (about 0.5 mm on the ellipsoid); the
sphere errs by up to about 0.5 % because the Earth is flattened, which `difference_from_haversine_pct`
shows for each pair. `operation: "destination"` goes the other way: from a start point, an
initial bearing and a distance to the point reached, again on the sphere and by Vincenty's
direct formula.

Points are `[lat, lon]` or `{"lat", "lon"}`. Each coordinate is a number, a decimal string or a
degrees–minutes–seconds string: `"50 03 59N"`, `"005°42′53″W"`, `"40:26:46.302N"`,
`"N 40 26.767"`, `"E44d24"`, `"-5.7147"`. Hemisphere letters must fit the axis (no `W` on a
latitude) and cannot be combined with a sign; minutes and seconds must be below 60. Distances
come in `unit` (`km`, `m`, `mi` = 1609.344 m, `nmi` = 1852 m) and always also in metres;
bearings in degrees clockwise from true north and as DMS.

Method:

- **Haversine**: `h = sin²(Δφ/2) + cos φ₁ cos φ₂ sin²(Δλ/2)`, `d = 2R·atan2(√h, √(1−h))`, with
  R = 6371.0088 km (the IUGG mean radius) unless `radius_km` is given (6371 to match most web
  calculators). Initial bearing `atan2(sin Δλ cos φ₂, cos φ₁ sin φ₂ − sin φ₁ cos φ₂ cos Δλ)`;
  final bearing is the reverse initial bearing plus 180°; midpoint and destination by the
  formulas on Movable Type Scripts.
- **Vincenty (1975)** on a = 6378137 m, f = 1/298.257223563 (GRS80: 1/298.257222101): the
  inverse iterates on λ until it changes by less than 10⁻¹² rad; the direct formula iterates on
  σ the same way. Near-antipodal points, where the inverse iteration does not converge (λ leaves
  ±π, or 200 iterations pass), give `"converged": false` with the reason and `more_exact`
  falls back to the haversine figure; the tool never loops.

The tests reproduce the Geoscience Australia test case quoted on Movable Type Scripts' Vincenty
page (Flinders Peak 37°57′03.72030″S 144°25′29.52440″E to Buninyong 37°39′10.15610″S
143°55′35.38390″E: s = 54 972.271 m, forward azimuth 306°52′05.37″, reverse azimuth
127°10′25.07″, and the direct problem back to Buninyong), Movable Type Scripts' worked example
with R = 6371 km (Land's End to John o' Groats 968.9 km, initial bearing 009°07′11″, midpoint
54°21′44″N 004°31′50″W), one degree of the equator (a·π/180 = 111 319.49 m) and the first degree
of meridian (110.574 km, as in the length-of-a-degree table of Wikipedia's Latitude article).

Limits: Vincenty's inverse fails near antipodal points (Karney's algorithm in GeographicLib
does not; it is not implemented here); the midpoint is spherical only; heights are ignored;
displayed floats carry 12 significant digits; a batch holds at most 10000 pairs.
