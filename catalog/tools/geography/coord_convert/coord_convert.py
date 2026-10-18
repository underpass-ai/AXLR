#!/usr/bin/env python3
"""Convert geographic coordinates between notations: decimal degrees, degrees-minutes-seconds
and degrees-decimal-minutes (parsing and formatting), UTM on WGS-84 (transverse Mercator by
Krueger's series to sixth order in n, as in Karney 2011 and GeographicLib) with zone and
latitude band including the Norway and Svalbard exceptions, MGRS, and geohash.

Input (JSON on stdin):
  operation   "all" (default) | "format" | "to_utm" | "from_utm" | "to_mgrs" | "from_mgrs" |
              "geohash_encode" | "geohash_decode"
  lat, lon    numbers, decimal strings or DMS/DDM strings ("40°26′46″N", "079 58 56W", "-5.6")
  position    instead of lat and lon, one string holding both ("40 26 46N 79 58 56W",
              "E44d24 N33d20", "33.3, 44.4"); without hemisphere letters the order is lat, lon
  zone        1..60: from_utm; to_utm to force a zone other than the standard one
  hemisphere  "N" | "S" (from_utm; or give band)
  band        latitude band letter C..X (from_utm; checked against the result)
  easting, northing   metres (from_utm)
  mgrs        MGRS string (from_mgrs), e.g. "38SMB4488"
  mgrs_digits digits per coordinate 0..5 for to_mgrs/all, default 5 (1 m)
  corner      from_mgrs: "center" (default, as GeoConvert) or "southwest" of the square
  geohash     geohash string (geohash_decode)
  geohash_length  1..12 for geohash_encode/all, default 9
  seconds_decimals  0..6, DMS seconds decimals, default 2; DDM minutes get two more
"""
import json
import math
import re
import sys

A_WGS84 = 6378137.0
F_WGS84 = 1 / 298.257223563
K0 = 0.9996
FALSE_EASTING = 500000.0
FALSE_NORTHING_SOUTH = 10000000.0
BANDS = "CDEFGHJKLMNPQRSTUVWX"
GEOHASH = "0123456789bcdefghjkmnpqrstuvwxyz"
MGRS_COLUMNS = ("STUVWXYZ", "ABCDEFGH", "JKLMNPQR")   # zone % 3 == 0, 1, 2
MGRS_ROWS = "ABCDEFGHJKLMNPQRSTUV"
OPERATIONS = ("all", "format", "to_utm", "from_utm", "to_mgrs", "from_mgrs", "geohash_encode", "geohash_decode")


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def num(x, digits=12):
    value = float(x)
    if value == 0:
        return 0.0
    return float(f"{value:.{digits}g}") if math.isfinite(value) else None


# ---------------------------------------------------------------- angles: parsing and formatting

ANGLE = re.compile(r"^\s*([NSEW])?\s*([+-])?\s*(\d+(?:\.\d+)?)\s*(?:[°ºd:]\s*|\s+|(?=[NSEW])|$)"
                   r"(?:(\d+(?:\.\d+)?)\s*(?:[′':]\s*|\s+|(?=[NSEW])|$))?"
                   r"(?:(\d+(?:\.\d+)?)\s*(?:[″\"])?\s*)?([NSEW])?\s*$", re.IGNORECASE)


def read_angle(text):
    """(degrees, hemisphere letter or '') from a DMS/DDM/decimal string; ValueError if unreadable."""
    text = text.strip().replace("''", '"').replace("’", "'").replace("”", '"')
    match = ANGLE.match(text)
    if not text or not match:
        raise ValueError(f"cannot read angle {text!r}")
    pre, sign, d, m, s, post = match.groups()
    if pre and post:
        raise ValueError(f"two hemisphere letters in {text!r}")
    hemisphere = (pre or post or "").upper()
    if hemisphere and sign:
        raise ValueError(f"both a sign and a hemisphere in {text!r}")
    if s is not None and m is None:
        raise ValueError(f"cannot read angle {text!r}")
    if (m is not None and "." in d) or (s is not None and "." in m):
        raise ValueError(f"only the last component may have decimals in {text!r}")
    minutes = float(m) if m is not None else 0.0
    seconds = float(s) if s is not None else 0.0
    if minutes >= 60 or seconds >= 60:
        raise ValueError(f"minutes and seconds must be below 60 in {text!r}")
    degrees = float(d) + minutes / 60 + seconds / 3600
    if sign == "-" or hemisphere in ("S", "W"):
        degrees = -degrees
    return degrees, hemisphere


def check_range(degrees, field, axis):
    if axis == "lat" and abs(degrees) > 90:
        fail(f"{field}: latitude must be between -90 and 90")
    if axis == "lon" and abs(degrees) > 180:
        fail(f"{field}: longitude must be between -180 and 180")
    return degrees


def parse_angle(value, field, axis):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or an angle string")
    if isinstance(value, (int, float)):
        if not math.isfinite(value):
            fail(f"{field} is not finite")
        return check_range(float(value), field, axis)
    try:
        degrees, hemisphere = read_angle(value)
    except ValueError as error:
        fail(f"{field}: {error}")
    if hemisphere in ("N", "S") and axis == "lon" or hemisphere in ("E", "W") and axis == "lat":
        fail(f"{field}: hemisphere {hemisphere} does not belong to a {'longitude' if axis == 'lon' else 'latitude'}")
    return check_range(degrees, field, axis)


def parse_position(text):
    """Split one string into a latitude and a longitude, by a comma or the one whitespace split
    at which both halves read as angles; hemisphere letters decide which is which."""
    if not isinstance(text, str) or not text.strip():
        fail("position must be a non-empty string")
    if "," in text:
        parts = text.split(",")
        if len(parts) != 2:
            fail("position: use one comma between the two coordinates")
        candidates = [(parts[0], parts[1])]
    else:
        tokens = text.split()
        candidates = [(" ".join(tokens[:i]), " ".join(tokens[i:])) for i in range(1, len(tokens))]
    readings = []
    for left, right in candidates:
        try:
            readings.append((read_angle(left), read_angle(right)))
        except ValueError:
            continue
    if not readings:
        fail(f"position: cannot read two coordinates in {text!r}")
    if len(readings) > 1:
        fail(f"position: {text!r} is ambiguous; separate the coordinates with a comma or use hemisphere letters")
    (a, ha), (b, hb) = readings[0]
    axes = {"N": "lat", "S": "lat", "E": "lon", "W": "lon", "": None}
    xa, xb = axes[ha], axes[hb]
    if xa == "lon" or xb == "lat":
        a, b, xa, xb = b, a, xb, xa
    if (xa or "lat") != "lat" or (xb or "lon") != "lon":
        fail(f"position: {text!r} needs one latitude and one longitude")
    return check_range(a, "position latitude", "lat"), check_range(b, "position longitude", "lon")


def sexagesimal(value, places, with_seconds):
    """Integer degrees, minutes and (seconds or decimal minutes) of |value|, rounded with carry."""
    scale = 10 ** places
    unit = 3600 if with_seconds else 60
    total = round(abs(value) * unit * scale)
    d, rest = divmod(total, unit * scale)
    if with_seconds:
        m, s = divmod(rest, 60 * scale)
        return d, m, s, scale
    return d, rest, None, scale


def fmt_dms(value, axis, places):
    d, m, s, scale = sexagesimal(value, places, True)
    hemisphere = ("N" if value >= 0 else "S") if axis == "lat" else ("E" if value >= 0 else "W")
    sec = f"{s // scale:02d}" + (f".{s % scale:0{places}d}" if places else "")
    return f"{d:0{2 if axis == 'lat' else 3}d}°{m:02d}′{sec}″{hemisphere}"


def fmt_ddm(value, axis, places):
    d, m, _, scale = sexagesimal(value, places, False)
    hemisphere = ("N" if value >= 0 else "S") if axis == "lat" else ("E" if value >= 0 else "W")
    minutes = f"{m // scale:02d}" + (f".{m % scale:0{places}d}" if places else "")
    return f"{d:0{2 if axis == 'lat' else 3}d}°{minutes}′{hemisphere}"


def formats(lat, lon, places):
    return {"lat": num(lat), "lon": num(lon),
            "decimal": f"{abs(lat):.{places + 4}f}°{'N' if lat >= 0 else 'S'} {abs(lon):.{places + 4}f}°{'E' if lon >= 0 else 'W'}",
            "dms": f"{fmt_dms(lat, 'lat', places)} {fmt_dms(lon, 'lon', places)}",
            "ddm": f"{fmt_ddm(lat, 'lat', places + 2)} {fmt_ddm(lon, 'lon', places + 2)}"}


# ---------------------------------------------------------------- transverse Mercator (Krueger series)

N3 = F_WGS84 / (2 - F_WGS84)
E2 = F_WGS84 * (2 - F_WGS84)
E = math.sqrt(E2)


def series(n):
    alpha = [0.0,
             n * (1 / 2 + n * (-2 / 3 + n * (5 / 16 + n * (41 / 180 + n * (-127 / 288 + n * 7891 / 37800))))),
             n ** 2 * (13 / 48 + n * (-3 / 5 + n * (557 / 1440 + n * (281 / 630 + n * -1983433 / 1935360)))),
             n ** 3 * (61 / 240 + n * (-103 / 140 + n * (15061 / 26880 + n * 167603 / 181440))),
             n ** 4 * (49561 / 161280 + n * (-179 / 168 + n * 6601661 / 7257600)),
             n ** 5 * (34729 / 80640 + n * -3418889 / 1995840),
             n ** 6 * (212378941 / 319334400)]
    beta = [0.0,
            n * (1 / 2 + n * (-2 / 3 + n * (37 / 96 + n * (-1 / 360 + n * (-81 / 512 + n * 96199 / 604800))))),
            n ** 2 * (1 / 48 + n * (1 / 15 + n * (-437 / 1440 + n * (46 / 105 + n * -1118711 / 3870720)))),
            n ** 3 * (17 / 480 + n * (-37 / 840 + n * (-209 / 4480 + n * 5569 / 90720))),
            n ** 4 * (4397 / 161280 + n * (-11 / 504 + n * -830251 / 7257600)),
            n ** 5 * (4583 / 161280 + n * -108847 / 3991680),
            n ** 6 * (20648693 / 638668800)]
    rectifying = A_WGS84 / (1 + n) * (1 + n ** 2 / 4 + n ** 4 / 64 + n ** 6 / 256)
    return alpha, beta, rectifying


ALPHA, BETA, RECTIFYING_RADIUS = series(N3)


def tm_forward(lat, dlon):
    """Transverse Mercator x (east) and y (north) in metres, unscaled origin at the equator on
    the central meridian, for latitude lat and longitude difference dlon (degrees)."""
    phi, lam = math.radians(lat), math.radians(dlon)
    if abs(lat) == 90:
        return 0.0, math.copysign(RECTIFYING_RADIUS * math.pi / 2, lat)
    tau = math.tan(phi)
    sigma = math.sinh(E * math.atanh(E * tau / math.sqrt(1 + tau * tau)))
    tau_c = tau * math.sqrt(1 + sigma * sigma) - sigma * math.sqrt(1 + tau * tau)
    xi_c = math.atan2(tau_c, math.cos(lam))
    eta_c = math.asinh(math.sin(lam) / math.sqrt(tau_c * tau_c + math.cos(lam) ** 2))
    xi, eta = xi_c, eta_c
    for j in range(1, 7):
        xi += ALPHA[j] * math.sin(2 * j * xi_c) * math.cosh(2 * j * eta_c)
        eta += ALPHA[j] * math.cos(2 * j * xi_c) * math.sinh(2 * j * eta_c)
    return RECTIFYING_RADIUS * eta, RECTIFYING_RADIUS * xi


def tm_inverse(x, y):
    """Latitude and longitude difference (degrees) of unscaled transverse Mercator x, y."""
    xi, eta = y / RECTIFYING_RADIUS, x / RECTIFYING_RADIUS
    xi_c, eta_c = xi, eta
    for j in range(1, 7):
        xi_c -= BETA[j] * math.sin(2 * j * xi) * math.cosh(2 * j * eta)
        eta_c -= BETA[j] * math.cos(2 * j * xi) * math.sinh(2 * j * eta)
    sinh_eta, cos_xi = math.sinh(eta_c), math.cos(xi_c)
    r = math.hypot(sinh_eta, cos_xi)
    if r == 0:
        return math.copysign(90.0, xi_c), 0.0
    tau_c = math.sin(xi_c) / r
    lam = math.atan2(sinh_eta, cos_xi)
    tau = tau_c   # Newton's method on tau'(tau) = tau_c, as in Karney 2011
    for _ in range(10):
        sigma = math.sinh(E * math.atanh(E * tau / math.sqrt(1 + tau * tau)))
        tau_i = tau * math.sqrt(1 + sigma * sigma) - sigma * math.sqrt(1 + tau * tau)
        step = (tau_c - tau_i) / math.sqrt(1 + tau_i * tau_i) * (1 + (1 - E2) * tau * tau) / ((1 - E2) * math.sqrt(1 + tau * tau))
        tau += step
        if abs(step) < 1e-14 * max(1.0, abs(tau)):
            break
    return math.degrees(math.atan(tau)), math.degrees(lam)


# ---------------------------------------------------------------- UTM and MGRS

def standard_zone(lat, lon):
    if lon == 180:
        lon = -180.0
    zone = int((lon + 180) // 6) + 1
    if 56 <= lat < 64 and 3 <= lon < 12:
        zone = 32
    if 72 <= lat <= 84 and 0 <= lon < 42:
        zone = 31 if lon < 9 else 33 if lon < 21 else 35 if lon < 33 else 37
    return zone


def band_of(lat):
    if lat < -80 or lat > 84:
        return None
    return BANDS[min(19, int((lat + 80) // 8))]


def central_meridian(zone):
    return zone * 6 - 183


def to_utm(lat, lon, zone=None):
    if lat < -80 or lat > 84:
        fail("UTM covers latitudes -80 to 84; the polar regions use UPS, which this tool does not do")
    standard = standard_zone(lat, lon)
    if zone is None:
        zone = standard
    dlon = (lon - central_meridian(zone) + 540) % 360 - 180
    if abs(dlon) > 9:
        fail(f"longitude {lon} is {abs(dlon):.1f}° from the central meridian of zone {zone}; at most 9° allowed")
    x, y = tm_forward(lat, dlon)
    easting = FALSE_EASTING + K0 * x
    northing = K0 * y + (FALSE_NORTHING_SOUTH if lat < 0 else 0.0)
    return {"zone": zone, "hemisphere": "N" if lat >= 0 else "S", "band": band_of(lat),
            "easting": easting, "northing": northing, "standard_zone": standard == zone,
            "central_meridian": central_meridian(zone)}


def from_utm(zone, hemisphere, easting, northing):
    x = (easting - FALSE_EASTING) / K0
    y = (northing - (FALSE_NORTHING_SOUTH if hemisphere == "S" else 0.0)) / K0
    lat, dlon = tm_inverse(x, y)
    lon = (central_meridian(zone) + dlon + 540) % 360 - 180
    return lat, lon


def utm_out(u):
    return {"zone": u["zone"], "hemisphere": u["hemisphere"], "band": u["band"], "easting": round(u["easting"], 3),
            "northing": round(u["northing"], 3), "central_meridian": u["central_meridian"], "standard_zone": u["standard_zone"],
            "text": f"{u['zone']}{u['hemisphere'].lower()} {u['easting']:.0f} {u['northing']:.0f}"}


def to_mgrs(u, digits):
    zone, easting, northing = u["zone"], u["easting"], u["northing"]
    column = int(easting // 100000)
    if not 1 <= column <= 8:
        fail("easting outside the MGRS 100 km columns (100000 to 899999 m)")
    letters = MGRS_COLUMNS[zone % 3][column - 1]
    row = int(northing // 100000) % 20
    if zone % 2 == 0:
        row = (row + 5) % 20
    letters += MGRS_ROWS[row]
    unit = 10 ** (5 - digits)
    e = int((easting % 100000) // unit)   # truncated, not rounded, as MGRS and GeoConvert do
    n = int((northing % 100000) // unit)
    text = f"{zone:02d}{u['band']}{letters}" + (f"{e:0{digits}d}{n:0{digits}d}" if digits else "")
    return {"mgrs": text, "digits": digits, "square_m": unit}


MGRS_PATTERN = re.compile(r"^\s*(\d{1,2})\s*([C-HJ-NP-X])\s*([A-HJ-NP-Z])([A-HJ-NP-V])\s*(\d*)\s*(\d*)\s*$", re.IGNORECASE)


def from_mgrs(text, corner):
    if not isinstance(text, str):
        fail("mgrs must be a string")
    match = MGRS_PATTERN.match(text)
    if not match:
        fail(f"mgrs: cannot read {text!r} (zone, band, two 100 km letters, an even number of digits; UPS not supported)")
    zone, band, col, row, d1, d2 = match.groups()
    zone, band, col, row = int(zone), band.upper(), col.upper(), row.upper()
    digits_text = d1 + d2
    if not 1 <= zone <= 60:
        fail("mgrs: zone must be 1 to 60")
    if len(digits_text) % 2 or len(digits_text) > 10:
        fail("mgrs: the numeric part must have an even number of digits, at most 10")
    digits = len(digits_text) // 2
    columns = MGRS_COLUMNS[zone % 3]
    if col not in columns:
        fail(f"mgrs: column letter {col} does not occur in zone {zone} (letters {columns})")
    e100k = (columns.index(col) + 1) * 100000
    r = MGRS_ROWS.index(row)
    if zone % 2 == 0:
        r = (r - 5) % 20
    unit = 10 ** (5 - digits)
    offset = unit / 2 if corner == "center" else 0.0
    easting = e100k + (int(digits_text[:digits]) * unit if digits else 0) + offset
    within = (int(digits_text[digits:]) * unit if digits else 0) + offset
    band_south = -80 + 8 * BANDS.index(band)
    hemisphere = "N" if band >= "N" else "S"
    minimum = K0 * tm_forward(band_south, 0)[1] + (FALSE_NORTHING_SOUTH if hemisphere == "S" else 0.0) - 100000
    northing = r * 100000 + within
    while northing < minimum:
        northing += 2000000
    lat, lon = from_utm(zone, hemisphere, easting, northing)
    warnings = []
    if band_of(lat) != band:
        warnings.append(f"the point falls in band {band_of(lat)}, not {band}")
    return zone, hemisphere, band, easting, northing, digits, lat, lon, warnings


# ---------------------------------------------------------------- geohash

def geohash_encode(lat, lon, length):
    lat_range, lon_range = [-90.0, 90.0], [-180.0, 180.0]
    out, bits, value, even = [], 0, 0, True
    while len(out) < length:
        rng, coordinate = (lon_range, lon) if even else (lat_range, lat)
        mid = (rng[0] + rng[1]) / 2
        if coordinate >= mid:
            value = value * 2 + 1
            rng[0] = mid
        else:
            value *= 2
            rng[1] = mid
        even = not even
        bits += 1
        if bits == 5:
            out.append(GEOHASH[value])
            bits = value = 0
    return "".join(out)


def geohash_decode(text):
    if not isinstance(text, str) or not 1 <= len(text) <= 22:
        fail("geohash must be a string of 1 to 22 characters")
    lat_range, lon_range = [-90.0, 90.0], [-180.0, 180.0]
    even = True
    for ch in text.lower():
        if ch not in GEOHASH:
            fail(f"geohash: {ch!r} is not a geohash character (0-9 and b-z without a, i, l, o)")
        value = GEOHASH.index(ch)
        for shift in range(4, -1, -1):
            rng = lon_range if even else lat_range
            mid = (rng[0] + rng[1]) / 2
            if value >> shift & 1:
                rng[0] = mid
            else:
                rng[1] = mid
            even = not even
    return lat_range, lon_range


# ---------------------------------------------------------------- main

def integer(request, field, low, high, default=None):
    value = request.get(field, default)
    if isinstance(value, bool) or not isinstance(value, int) or not low <= value <= high:
        fail(f"{field} must be an integer from {low} to {high}")
    return value


def metres(request, field):
    value = request.get(field)
    if isinstance(value, str):
        try:
            value = float(value)
        except ValueError:
            fail(f"{field} must be a number of metres")
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(value):
        fail(f"{field} must be a number of metres")
    return float(value)


def latlon(request):
    if "position" in request:
        if "lat" in request or "lon" in request:
            fail("give position or lat and lon, not both")
        return parse_position(request["position"])
    if "lat" not in request or "lon" not in request:
        fail("this operation needs lat and lon (or position)")
    return parse_angle(request["lat"], "lat", "lat"), parse_angle(request["lon"], "lon", "lon")


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    allowed = {"operation", "lat", "lon", "position", "zone", "hemisphere", "band", "easting", "northing", "mgrs",
               "mgrs_digits", "corner", "geohash", "geohash_length", "seconds_decimals"}
    unknown = sorted(set(request) - allowed)
    if unknown:
        fail(f"unknown field(s): {', '.join(unknown)}")
    operation = request.get("operation", "all")
    if operation not in OPERATIONS:
        fail(f"operation must be one of {', '.join(OPERATIONS)}")
    places = integer(request, "seconds_decimals", 0, 6, 2)
    digits = integer(request, "mgrs_digits", 0, 5, 5)
    length = integer(request, "geohash_length", 1, 12, 9)
    result = {"operation": operation, "datum": "WGS84"}

    if operation in ("all", "format", "to_utm", "to_mgrs", "geohash_encode"):
        lat, lon = latlon(request)
        result.update(formats(lat, lon, places))
        zone = integer(request, "zone", 1, 60) if "zone" in request else None
        if operation in ("all", "to_utm", "to_mgrs"):
            if operation == "all" and not -80 <= lat <= 84:
                result["utm"] = None
                result["mgrs"] = None
                result["note"] = "UTM and MGRS cover latitudes -80 to 84 only (UPS not supported)"
            else:
                u = to_utm(lat, lon, zone)
                if operation != "to_mgrs":
                    result["utm"] = utm_out(u)
                if operation != "to_utm":
                    result["mgrs"] = to_mgrs(u, digits)
        if operation in ("all", "geohash_encode"):
            gh = geohash_encode(lat, lon, length)
            lat_range, lon_range = geohash_decode(gh)
            result["geohash"] = {"geohash": gh, "length": length,
                                 "lat_error": num((lat_range[1] - lat_range[0]) / 2), "lon_error": num((lon_range[1] - lon_range[0]) / 2)}
    elif operation == "from_utm":
        zone = integer(request, "zone", 1, 60)
        band = request.get("band")
        hemisphere = request.get("hemisphere")
        if band is not None:
            if not isinstance(band, str) or band.upper() not in BANDS or len(band) != 1:
                fail("band must be one letter C..X without I and O")
            band = band.upper()
            implied = "N" if band >= "N" else "S"
            if hemisphere is not None and str(hemisphere).upper() != implied:
                fail(f"band {band} is in the {'northern' if implied == 'N' else 'southern'} hemisphere")
            hemisphere = implied
        if not isinstance(hemisphere, str) or hemisphere.upper() not in ("N", "S"):
            fail("from_utm needs hemisphere N or S (or a band letter)")
        hemisphere = hemisphere.upper()
        easting, northing = metres(request, "easting"), metres(request, "northing")
        if not 0 < easting < 1000000 or not 0 <= northing <= 10000000:
            fail("easting must be between 0 and 1000000 m and northing between 0 and 10000000 m")
        lat, lon = from_utm(zone, hemisphere, easting, northing)
        result.update(formats(lat, lon, places))
        result["utm"] = {"zone": zone, "hemisphere": hemisphere, "easting": easting, "northing": northing,
                         "band": band_of(lat), "standard_zone": standard_zone(lat, lon) == zone}
        warnings = []
        if not 100000 <= easting <= 900000:
            warnings.append("easting is outside 100000..900000 m, beyond the usual extent of a zone")
        if band is not None and band_of(lat) != band:
            warnings.append(f"the point falls in band {band_of(lat)}, not {band}")
        if warnings:
            result["warnings"] = warnings
    elif operation == "from_mgrs":
        if "mgrs" not in request:
            fail("from_mgrs needs mgrs")
        corner = request.get("corner", "center")
        if corner not in ("center", "southwest"):
            fail("corner must be center or southwest")
        zone, hemisphere, band, easting, northing, mdigits, lat, lon, warnings = from_mgrs(request["mgrs"], corner)
        result.update(formats(lat, lon, places))
        result["utm"] = {"zone": zone, "hemisphere": hemisphere, "band": band, "easting": easting, "northing": northing,
                         "text": f"{zone}{hemisphere.lower()} {easting:.0f} {northing:.0f}"}
        result["mgrs"] = {"digits": mdigits, "square_m": 10 ** (5 - mdigits), "point": corner}
        if warnings:
            result["warnings"] = warnings
    else:
        if "geohash" not in request:
            fail("geohash_decode needs geohash")
        lat_range, lon_range = geohash_decode(request["geohash"])
        lat, lon = (lat_range[0] + lat_range[1]) / 2, (lon_range[0] + lon_range[1]) / 2
        result.update(formats(lat, lon, places))
        result["geohash"] = {"geohash": request["geohash"].lower(), "lat_error": num((lat_range[1] - lat_range[0]) / 2),
                             "lon_error": num((lon_range[1] - lon_range[0]) / 2),
                             "bbox": [num(lon_range[0]), num(lat_range[0]), num(lon_range[1]), num(lat_range[1])]}
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
