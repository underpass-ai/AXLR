#!/usr/bin/env python3
"""Great-circle and ellipsoidal distance between points: the haversine formula on a sphere and
Vincenty's inverse formula on the WGS-84 (or GRS80) ellipsoid, with initial and final bearings,
the spherical midpoint, and the destination point reached from a start, a bearing and a
distance (spherical and Vincenty's direct formula). One pair or a batch of pairs.

Input (JSON on stdin):
  operation   "inverse" (default) | "destination"
  from, to    a point: [lat, lon] or {"lat": .., "lon": ..}; each value a number, a decimal
              string or a DMS string ("50 03 59N", "005°42′53″W", "-5.7147")
  pairs       inverse batch: [{"from": point, "to": point}, ...] or [[lat1, lon1, lat2, lon2], ...]
  bearing     destination: initial bearing in degrees (number or DMS string)
  distance    destination: distance in `unit`
  unit        "km" (default) | "m" | "mi" | "nmi"
  radius_km   sphere radius for the haversine results, default 6371.0088 (IUGG mean radius)
  ellipsoid   "WGS84" (default) | "GRS80"
"""
import json
import math
import re
import sys

UNITS = {"m": 1.0, "km": 1000.0, "mi": 1609.344, "nmi": 1852.0}
ELLIPSOIDS = {"WGS84": (6378137.0, 1 / 298.257223563), "GRS80": (6378137.0, 1 / 298.257222101)}
MEAN_RADIUS_KM = 6371.0088
MAX_ITERATIONS = 200
MAX_PAIRS = 10000


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def num(x):
    """A float for display, at 12 significant digits."""
    value = float(x)
    return float(f"{value:.12g}") if math.isfinite(value) else None


ANGLE = re.compile(r"^\s*([NSEW])?\s*([+-])?\s*(\d+(?:\.\d+)?)\s*(?:[°ºd:]\s*|\s+|(?=[NSEW])|$)"
                   r"(?:(\d+(?:\.\d+)?)\s*(?:[′':]\s*|\s+|(?=[NSEW])|$))?"
                   r"(?:(\d+(?:\.\d+)?)\s*(?:[″\"])?\s*)?([NSEW])?\s*$", re.IGNORECASE)


def parse_angle(value, field, axis):
    """Degrees from a number, a decimal string or a DMS/DDM string with an optional hemisphere."""
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or an angle string")
    if isinstance(value, (int, float)):
        degrees = float(value)
        if not math.isfinite(degrees):
            fail(f"{field} is not finite")
    else:
        text = value.strip().replace("''", '"')
        match = ANGLE.match(text)
        if not text or not match:
            fail(f"{field}: cannot read angle {value!r}")
        pre, sign, d, m, s, post = match.groups()
        if pre and post:
            fail(f"{field}: two hemisphere letters in {value!r}")
        hemisphere = (pre or post or "").upper()
        if hemisphere and sign:
            fail(f"{field}: both a sign and a hemisphere in {value!r}")
        if (m is not None and "." in d) or (s is not None and m is not None and "." in m):
            fail(f"{field}: only the last component may have decimals in {value!r}")
        minutes = float(m) if m is not None else 0.0
        seconds = float(s) if s is not None else 0.0
        if s is not None and m is None:
            fail(f"{field}: cannot read angle {value!r}")
        if minutes >= 60 or seconds >= 60:
            fail(f"{field}: minutes and seconds must be below 60 in {value!r}")
        degrees = float(d) + minutes / 60 + seconds / 3600
        if sign == "-" or hemisphere in ("S", "W"):
            degrees = -degrees
        if hemisphere and axis == "bearing":
            fail(f"{field}: a bearing takes no hemisphere letter")
        if hemisphere in ("N", "S") and axis == "lon" or hemisphere in ("E", "W") and axis == "lat":
            fail(f"{field}: hemisphere {hemisphere} does not belong to a {'longitude' if axis == 'lon' else 'latitude'}")
    if axis == "lat" and abs(degrees) > 90:
        fail(f"{field}: latitude must be between -90 and 90")
    if axis == "lon" and abs(degrees) > 180:
        fail(f"{field}: longitude must be between -180 and 180")
    return degrees


def parse_point(value, field):
    if isinstance(value, dict):
        extra = set(value) - {"lat", "lon"}
        if extra or "lat" not in value or "lon" not in value:
            fail(f"{field} must be {{\"lat\": .., \"lon\": ..}}")
        lat, lon = value["lat"], value["lon"]
    elif isinstance(value, list) and len(value) == 2:
        lat, lon = value
    else:
        fail(f"{field} must be [lat, lon] or {{\"lat\": .., \"lon\": ..}}")
    return parse_angle(lat, f"{field}.lat", "lat"), parse_angle(lon, f"{field}.lon", "lon")


def wrap180(lon):
    lon = math.fmod(lon + 540.0, 360.0) - 180.0
    return 180.0 if lon == -180.0 else lon


def dms(degrees, axis=None, places=2):
    """306°52′05.37″, or 37°39′10.15610″S with a hemisphere for a latitude or longitude."""
    hemisphere = ""
    if axis == "lat":
        hemisphere = "N" if degrees >= 0 else "S"
    elif axis == "lon":
        hemisphere = "E" if degrees >= 0 else "W"
    value = abs(degrees) if axis else degrees % 360.0
    scale = 10 ** places
    total = round(value * 3600 * scale)
    d, rest = divmod(total, 3600 * scale)
    m, s = divmod(rest, 60 * scale)
    if axis is None:
        d %= 360
    width = {"lat": 2, "lon": 3}.get(axis, 3)
    sec = f"{s // scale:02d}" + (f".{s % scale:0{places}d}" if places else "")
    return f"{d:0{width}d}°{m:02d}′{sec}″{hemisphere}"


def point_out(lat, lon):
    lat, lon = (0.0 if abs(lat) < 1e-12 else lat), (0.0 if abs(lon) < 1e-12 else lon)
    return {"lat": num(lat), "lon": num(lon), "dms": f"{dms(lat, 'lat')} {dms(lon, 'lon')}"}


def bearing_out(degrees):
    degrees %= 360.0
    return {"degrees": num(degrees), "dms": dms(degrees)}


def haversine(lat1, lon1, lat2, lon2, radius_m):
    p1, p2 = math.radians(lat1), math.radians(lat2)
    dp, dl = p2 - p1, math.radians(lon2 - lon1)
    h = math.sin(dp / 2) ** 2 + math.cos(p1) * math.cos(p2) * math.sin(dl / 2) ** 2
    central = 2 * math.atan2(math.sqrt(h), math.sqrt(max(0.0, 1 - h)))
    return radius_m * central


def sphere_bearing(lat1, lon1, lat2, lon2):
    p1, p2, dl = math.radians(lat1), math.radians(lat2), math.radians(lon2 - lon1)
    y = math.sin(dl) * math.cos(p2)
    x = math.cos(p1) * math.sin(p2) - math.sin(p1) * math.cos(p2) * math.cos(dl)
    return math.degrees(math.atan2(y, x)) % 360.0


def midpoint(lat1, lon1, lat2, lon2):
    p1, p2, l1, dl = math.radians(lat1), math.radians(lat2), math.radians(lon1), math.radians(lon2 - lon1)
    bx, by = math.cos(p2) * math.cos(dl), math.cos(p2) * math.sin(dl)
    lat = math.atan2(math.sin(p1) + math.sin(p2), math.sqrt((math.cos(p1) + bx) ** 2 + by ** 2))
    lon = l1 + math.atan2(by, math.cos(p1) + bx)
    return math.degrees(lat), wrap180(math.degrees(lon))


def sphere_destination(lat1, lon1, bearing, distance_m, radius_m):
    p1, l1, theta, delta = math.radians(lat1), math.radians(lon1), math.radians(bearing), distance_m / radius_m
    p2 = math.asin(max(-1.0, min(1.0, math.sin(p1) * math.cos(delta) + math.cos(p1) * math.sin(delta) * math.cos(theta))))
    l2 = l1 + math.atan2(math.sin(theta) * math.sin(delta) * math.cos(p1), math.cos(delta) - math.sin(p1) * math.sin(p2))
    return math.degrees(p2), wrap180(math.degrees(l2))


def reduced(lat, f):
    tan_u = (1 - f) * math.tan(math.radians(lat))
    cos_u = 1 / math.sqrt(1 + tan_u * tan_u)
    return tan_u * cos_u, cos_u


def vincenty_inverse(lat1, lon1, lat2, lon2, a, f):
    """Vincenty (1975) inverse formula. Returns a dict; converged False near antipodal points."""
    b = a * (1 - f)
    big_l = math.radians(wrap180(lon2 - lon1))
    sin_u1, cos_u1 = reduced(lat1, f)
    sin_u2, cos_u2 = reduced(lat2, f)
    lam = big_l
    for iteration in range(1, MAX_ITERATIONS + 1):
        sin_l, cos_l = math.sin(lam), math.cos(lam)
        sin_sigma = math.hypot(cos_u2 * sin_l, cos_u1 * sin_u2 - sin_u1 * cos_u2 * cos_l)
        if sin_sigma == 0:
            return {"converged": True, "iterations": iteration, "distance_m": 0.0, "initial": None, "final": None}
        cos_sigma = sin_u1 * sin_u2 + cos_u1 * cos_u2 * cos_l
        sigma = math.atan2(sin_sigma, cos_sigma)
        sin_alpha = cos_u1 * cos_u2 * sin_l / sin_sigma
        cos2_alpha = 1 - sin_alpha * sin_alpha
        cos_2sm = cos_sigma - 2 * sin_u1 * sin_u2 / cos2_alpha if cos2_alpha != 0 else 0.0
        c = f / 16 * cos2_alpha * (4 + f * (4 - 3 * cos2_alpha))
        previous = lam
        lam = big_l + (1 - c) * f * sin_alpha * (sigma + c * sin_sigma * (cos_2sm + c * cos_sigma * (-1 + 2 * cos_2sm ** 2)))
        if abs(lam) > math.pi:
            return {"converged": False, "iterations": iteration, "reason": "lambda exceeded pi (near-antipodal points)"}
        if abs(lam - previous) < 1e-12:
            break
    else:
        return {"converged": False, "iterations": MAX_ITERATIONS, "reason": f"no convergence in {MAX_ITERATIONS} iterations (near-antipodal points)"}
    u2 = cos2_alpha * (a * a - b * b) / (b * b)
    big_a = 1 + u2 / 16384 * (4096 + u2 * (-768 + u2 * (320 - 175 * u2)))
    big_b = u2 / 1024 * (256 + u2 * (-128 + u2 * (74 - 47 * u2)))
    d_sigma = big_b * sin_sigma * (cos_2sm + big_b / 4 * (cos_sigma * (-1 + 2 * cos_2sm ** 2)
                                                         - big_b / 6 * cos_2sm * (-3 + 4 * sin_sigma ** 2) * (-3 + 4 * cos_2sm ** 2)))
    s = b * big_a * (sigma - d_sigma)
    alpha1 = math.degrees(math.atan2(cos_u2 * sin_l, cos_u1 * sin_u2 - sin_u1 * cos_u2 * cos_l))
    alpha2 = math.degrees(math.atan2(cos_u1 * sin_l, -sin_u1 * cos_u2 + cos_u1 * sin_u2 * cos_l))
    return {"converged": True, "iterations": iteration, "distance_m": s, "initial": alpha1 % 360, "final": alpha2 % 360}


def vincenty_direct(lat1, lon1, bearing, s, a, f):
    """Vincenty (1975) direct formula: the point s metres from (lat1, lon1) on initial bearing."""
    b = a * (1 - f)
    alpha1 = math.radians(bearing)
    sin_a1, cos_a1 = math.sin(alpha1), math.cos(alpha1)
    sin_u1, cos_u1 = reduced(lat1, f)
    sigma1 = math.atan2(sin_u1 / cos_u1, cos_a1)
    sin_alpha = cos_u1 * sin_a1
    cos2_alpha = 1 - sin_alpha * sin_alpha
    u2 = cos2_alpha * (a * a - b * b) / (b * b)
    big_a = 1 + u2 / 16384 * (4096 + u2 * (-768 + u2 * (320 - 175 * u2)))
    big_b = u2 / 1024 * (256 + u2 * (-128 + u2 * (74 - 47 * u2)))
    sigma = s / (b * big_a)
    for _ in range(MAX_ITERATIONS):
        cos_2sm = math.cos(2 * sigma1 + sigma)
        sin_sigma, cos_sigma = math.sin(sigma), math.cos(sigma)
        d_sigma = big_b * sin_sigma * (cos_2sm + big_b / 4 * (cos_sigma * (-1 + 2 * cos_2sm ** 2)
                                                             - big_b / 6 * cos_2sm * (-3 + 4 * sin_sigma ** 2) * (-3 + 4 * cos_2sm ** 2)))
        previous = sigma
        sigma = s / (b * big_a) + d_sigma
        if abs(sigma - previous) < 1e-12:
            break
    cos_2sm = math.cos(2 * sigma1 + sigma)
    sin_sigma, cos_sigma = math.sin(sigma), math.cos(sigma)
    x = sin_u1 * sin_sigma - cos_u1 * cos_sigma * cos_a1
    lat2 = math.atan2(sin_u1 * cos_sigma + cos_u1 * sin_sigma * cos_a1, (1 - f) * math.hypot(sin_alpha, x))
    lam = math.atan2(sin_sigma * sin_a1, cos_u1 * cos_sigma - sin_u1 * sin_sigma * cos_a1)
    c = f / 16 * cos2_alpha * (4 + f * (4 - 3 * cos2_alpha))
    big_l = lam - (1 - c) * f * sin_alpha * (sigma + c * sin_sigma * (cos_2sm + c * cos_sigma * (-1 + 2 * cos_2sm ** 2)))
    final = math.degrees(math.atan2(sin_alpha, -x)) % 360
    return math.degrees(lat2), wrap180(lon1 + math.degrees(big_l)), final


def inverse(lat1, lon1, lat2, lon2, unit, radius_m, a, f):
    factor = UNITS[unit]
    hav_m = haversine(lat1, lon1, lat2, lon2, radius_m)
    sphere = {"distance": num(hav_m / factor), "distance_m": num(hav_m)}
    same = lat1 == lat2 and wrap180(lon1 - lon2) == 0
    if not same:
        initial = sphere_bearing(lat1, lon1, lat2, lon2)
        final = (sphere_bearing(lat2, lon2, lat1, lon1) + 180.0) % 360.0
        sphere["initial_bearing"] = bearing_out(initial)
        sphere["final_bearing"] = bearing_out(final)
    sphere["midpoint"] = point_out(*midpoint(lat1, lon1, lat2, lon2))
    v = vincenty_inverse(lat1, lon1, lat2, lon2, a, f)
    ellipsoid = {"converged": v["converged"], "iterations": v["iterations"]}
    if v["converged"]:
        ellipsoid["distance"] = num(v["distance_m"] / factor)
        ellipsoid["distance_m"] = num(v["distance_m"])
        if v["initial"] is not None:
            ellipsoid["initial_bearing"] = bearing_out(v["initial"])
            ellipsoid["final_bearing"] = bearing_out(v["final"])
            ellipsoid["reverse_azimuth"] = bearing_out(v["final"] + 180.0)
        ellipsoid["difference_from_haversine_pct"] = num((hav_m - v["distance_m"]) / v["distance_m"] * 100) if v["distance_m"] else 0.0
    else:
        ellipsoid["reason"] = v["reason"]
    return {"from": point_out(lat1, lon1), "to": point_out(lat2, lon2), "haversine": sphere, "vincenty": ellipsoid,
            "more_exact": "vincenty" if v["converged"] else "haversine (Vincenty did not converge)"}


def number(value, field):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number")
    try:
        x = float(value)
    except ValueError:
        fail(f"{field} must be a number")
    if not math.isfinite(x):
        fail(f"{field} must be finite")
    return x


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    allowed = {"operation", "from", "to", "pairs", "bearing", "distance", "unit", "radius_km", "ellipsoid"}
    unknown = sorted(set(request) - allowed)
    if unknown:
        fail(f"unknown field(s): {', '.join(unknown)}")
    operation = request.get("operation", "inverse")
    unit = request.get("unit", "km")
    if unit not in UNITS:
        fail("unit must be one of km, m, mi, nmi")
    radius_km = number(request.get("radius_km", MEAN_RADIUS_KM), "radius_km")
    if radius_km <= 0:
        fail("radius_km must be positive")
    radius_m = radius_km * 1000
    name = request.get("ellipsoid", "WGS84")
    if name not in ELLIPSOIDS:
        fail("ellipsoid must be WGS84 or GRS80")
    a, f = ELLIPSOIDS[name]
    common = {"operation": operation, "unit": unit, "radius_km": num(radius_km), "ellipsoid": name}
    if operation == "inverse":
        if "pairs" in request:
            pairs = request["pairs"]
            if not isinstance(pairs, list) or not pairs or len(pairs) > MAX_PAIRS:
                fail(f"pairs must be a list of 1 to {MAX_PAIRS} pairs")
            results = []
            for i, pair in enumerate(pairs):
                if isinstance(pair, list) and len(pair) == 4:
                    p, q = parse_point(pair[:2], f"pairs[{i}].from"), parse_point(pair[2:], f"pairs[{i}].to")
                elif isinstance(pair, dict) and set(pair) == {"from", "to"}:
                    p, q = parse_point(pair["from"], f"pairs[{i}].from"), parse_point(pair["to"], f"pairs[{i}].to")
                else:
                    fail(f"pairs[{i}] must be {{\"from\": point, \"to\": point}} or [lat1, lon1, lat2, lon2]")
                results.append(inverse(*p, *q, unit, radius_m, a, f))
            print(json.dumps({**common, "count": len(results), "results": results}, ensure_ascii=False))
            return
        if "from" not in request or "to" not in request:
            fail("inverse needs from and to, or pairs")
        p, q = parse_point(request["from"], "from"), parse_point(request["to"], "to")
        print(json.dumps({**common, **inverse(*p, *q, unit, radius_m, a, f)}, ensure_ascii=False))
    elif operation == "destination":
        if "from" not in request or "bearing" not in request or "distance" not in request:
            fail("destination needs from, bearing and distance")
        lat1, lon1 = parse_point(request["from"], "from")
        bearing = request["bearing"]
        if isinstance(bearing, str):
            bearing = parse_angle(bearing, "bearing", "bearing")
        else:
            bearing = number(bearing, "bearing")
        distance = number(request["distance"], "distance")
        if distance < 0:
            fail("distance must not be negative")
        distance_m = distance * UNITS[unit]
        slat, slon = sphere_destination(lat1, lon1, bearing, distance_m, radius_m)
        sphere_final = (sphere_bearing(slat, slon, lat1, lon1) + 180.0) % 360.0 if distance_m else bearing % 360
        elat, elon, efinal = vincenty_direct(lat1, lon1, bearing, distance_m, a, f)
        print(json.dumps({**common, "from": point_out(lat1, lon1), "bearing": bearing_out(bearing), "distance": num(distance),
                          "distance_m": num(distance_m),
                          "haversine": {"destination": point_out(slat, slon), "final_bearing": bearing_out(sphere_final)},
                          "vincenty": {"destination": point_out(elat, elon), "final_bearing": bearing_out(efinal),
                                       "reverse_azimuth": bearing_out(efinal + 180.0)},
                          "more_exact": "vincenty"}, ensure_ascii=False))
    else:
        fail("operation must be inverse or destination")


if __name__ == "__main__":
    main()
