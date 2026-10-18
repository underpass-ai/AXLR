#!/usr/bin/env python3
"""Sunrise, sunset, solar noon and civil, nautical and astronomical twilight for a date and a
place, by the NOAA solar calculator algorithm (Meeus' low-accuracy solar position), with the
Sun's declination and the equation of time.

Input (JSON on stdin):
  date        "YYYY-MM-DD", the local calendar date
  latitude    degrees, north positive, -90..90
  longitude   degrees, east positive, -180..180
  timezone    IANA zone name ("Europe/Madrid") or "UTC"     } one of these, default UTC
  utc_offset  "+02:00" / "-03:30" or hours as a number       }
  rounding    "second" (default) or "minute" for the reported times
  datetime    optional ISO instant ("1992-10-13T00:00:00Z"): also give the Sun's position then
"""
import datetime as dt
import json
import math
import re
import sys


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


J2000 = dt.datetime(2000, 1, 1, 12, tzinfo=dt.timezone.utc)
DEPRESSIONS = {"sunrise_sunset": 0.833, "civil": 6.0, "nautical": 12.0, "astronomical": 18.0}


def jd_of(moment):
    return 2451545.0 + (moment - J2000).total_seconds() / 86400.0


def moment_of(jd):
    return J2000 + dt.timedelta(days=jd - 2451545.0)


def sun(jd):
    """NOAA spreadsheet: declination (deg), equation of time (min), apparent longitude (deg)."""
    t = (jd - 2451545.0) / 36525.0
    l0 = (280.46646 + t * (36000.76983 + t * 0.0003032)) % 360.0
    m = 357.52911 + t * (35999.05029 - 0.0001537 * t)
    e = 0.016708634 - t * (0.000042037 + 0.0000001267 * t)
    mr = math.radians(m)
    c = (math.sin(mr) * (1.914602 - t * (0.004817 + 0.000014 * t)) + math.sin(2 * mr) * (0.019993 - 0.000101 * t)
         + math.sin(3 * mr) * 0.000289)
    true_long = l0 + c
    omega = math.radians(125.04 - 1934.136 * t)
    app_long = true_long - 0.00569 - 0.00478 * math.sin(omega)
    mean_obliq = 23.0 + (26.0 + (21.448 - t * (46.815 + t * (0.00059 - t * 0.001813))) / 60.0) / 60.0
    obliq = math.radians(mean_obliq + 0.00256 * math.cos(omega))
    decl = math.degrees(math.asin(math.sin(obliq) * math.sin(math.radians(app_long))))
    y = math.tan(obliq / 2) ** 2
    l0r = math.radians(l0)
    eot = 4 * math.degrees(y * math.sin(2 * l0r) - 2 * e * math.sin(mr) + 4 * e * y * math.sin(mr) * math.cos(2 * l0r)
                           - 0.5 * y * y * math.sin(4 * l0r) - 1.25 * e * e * math.sin(2 * mr))
    return {"declination": decl, "eot": eot, "apparent_longitude": app_long % 360.0,
            "obliquity": math.degrees(obliq), "true_longitude": true_long % 360.0}


def transit(day_jd0, longitude, guess):
    """Solar noon (JD) for the mean solar day starting at JD day_jd0 (0h UT)."""
    t = guess
    for _ in range(4):
        t = day_jd0 + (720.0 - 4.0 * longitude - sun(t)["eot"]) / 1440.0
    return t


def hour_angle(latitude, decl, depression):
    """Hour angle (deg) of the Sun at altitude -depression, or 'above'/'below' if never reached."""
    lat, d = math.radians(latitude), math.radians(decl)
    num = math.sin(math.radians(-depression)) - math.sin(lat) * math.sin(d)
    den = math.cos(lat) * math.cos(d)
    if abs(den) < 1e-12:
        return "above" if num < 0 else "below"
    cos_h = num / den
    if cos_h < -1:
        return "above"
    if cos_h > 1:
        return "below"
    return math.degrees(math.acos(cos_h))


def event(day_jd0, latitude, longitude, depression, sign, noon):
    """Rising (sign -1) or setting (sign +1) time (JD), iterated at the event, or a status."""
    t = noon
    for _ in range(5):
        info = sun(t)
        h = hour_angle(latitude, info["declination"], depression)
        if isinstance(h, str):
            return h
        t = day_jd0 + (720.0 - 4.0 * longitude - info["eot"] + sign * 4.0 * h) / 1440.0
    return t


def elevation_azimuth(jd, latitude, longitude):
    info = sun(jd)
    minutes = ((jd + 0.5) % 1.0) * 1440.0
    true_solar = (minutes + info["eot"] + 4.0 * longitude) % 1440.0
    ha = true_solar / 4.0 - 180.0
    lat, d, har = math.radians(latitude), math.radians(info["declination"]), math.radians(ha)
    cos_zen = max(-1.0, min(1.0, math.sin(lat) * math.sin(d) + math.cos(lat) * math.cos(d) * math.cos(har)))
    zenith = math.acos(cos_zen)
    elevation = 90.0 - math.degrees(zenith)
    denom = math.cos(lat) * math.sin(zenith)
    if abs(denom) < 1e-12:
        azimuth = 180.0 if latitude > 0 else 0.0
    else:
        cos_az = max(-1.0, min(1.0, (math.sin(lat) * math.cos(zenith) - math.sin(d)) / denom))
        az = math.degrees(math.acos(cos_az))
        azimuth = (az + 180.0) % 360.0 if ha > 0 else (540.0 - az) % 360.0
    return info, elevation, azimuth


def number(request, field, low, high):
    value = request.get(field)
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        fail(f"{field} must be a number")
    if not low <= value <= high or value != value:
        fail(f"{field} must be between {low} and {high}")
    return float(value)


def offset_zone(value):
    if isinstance(value, bool):
        fail("utc_offset must be \"+HH:MM\" or a number of hours")
    if isinstance(value, (int, float)):
        minutes = round(value * 60)
    else:
        match = re.fullmatch(r"(?:UTC)?([+-])(\d{1,2})(?::?(\d{2}))?", str(value).strip())
        if not match:
            fail("utc_offset must be \"+HH:MM\" or a number of hours")
        minutes = (int(match.group(2)) * 60 + int(match.group(3) or 0)) * (1 if match.group(1) == "+" else -1)
    if abs(minutes) > 18 * 60:
        fail("utc_offset must be within ±18 hours")
    return dt.timezone(dt.timedelta(minutes=minutes)), value


def zone_of(request):
    if "timezone" in request and "utc_offset" in request:
        fail("give timezone or utc_offset, not both")
    if "utc_offset" in request:
        zone, _ = offset_zone(request["utc_offset"])
        return zone, str(zone)
    name = request.get("timezone", "UTC")
    if not isinstance(name, str) or not name:
        fail("timezone must be an IANA zone name such as Europe/Madrid")
    if name.upper() in ("UTC", "Z", "GMT", "ETC/UTC", "ETC/GMT"):
        return dt.timezone.utc, "UTC"
    try:
        import zoneinfo
        return zoneinfo.ZoneInfo(name), name
    except Exception as error:  # ZoneInfoNotFoundError, ValueError on a malformed key, missing tzdata
        fail(f"unknown time zone {name!r} ({type(error).__name__}); use an IANA name or utc_offset")


def parse_instant(text):
    if not isinstance(text, str):
        fail("datetime must be an ISO 8601 string")
    value = text.strip()
    if value.endswith(("Z", "z")):
        value = value[:-1] + "+00:00"
    try:
        moment = dt.datetime.fromisoformat(value)
    except ValueError:
        fail("datetime must be ISO 8601, e.g. 1992-10-13T00:00:00Z")
    if moment.tzinfo is None:
        moment = moment.replace(tzinfo=dt.timezone.utc)
    return moment.astimezone(dt.timezone.utc)


def clock(seconds):
    seconds = int(round(seconds))
    return f"{seconds // 3600:02d}:{seconds // 60 % 60:02d}:{seconds % 60:02d}"


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    unknown = sorted(set(request) - {"date", "latitude", "longitude", "timezone", "utc_offset", "rounding", "datetime"})
    if unknown:
        fail(f"unknown field(s): {', '.join(unknown)}")
    try:
        date = dt.date.fromisoformat(request.get("date"))
    except (TypeError, ValueError):
        fail("date must be a date YYYY-MM-DD")
    if not 2 <= date.year <= 9998:
        fail("date must be between years 2 and 9998")
    latitude = number(request, "latitude", -90, 90)
    longitude = number(request, "longitude", -180, 180)
    zone, zone_name = zone_of(request)
    rounding = request.get("rounding", "second")
    if rounding not in ("second", "minute"):
        fail("rounding must be second or minute")

    def local(jd):
        moment = moment_of(jd)
        if rounding == "minute":
            moment = (moment + dt.timedelta(seconds=30)).replace(second=0, microsecond=0)
        else:
            moment = (moment + dt.timedelta(microseconds=500000)).replace(microsecond=0)
        return moment.astimezone(zone).isoformat(timespec="minutes" if rounding == "minute" else "seconds")

    # the solar noon whose local date is the requested date (mean solar days D-1, D, D+1)
    local_noon = dt.datetime(date.year, date.month, date.day, 12, tzinfo=zone)
    target = jd_of(local_noon.astimezone(dt.timezone.utc))
    candidates = []
    for shift in (-1, 0, 1):
        day = date + dt.timedelta(shift)
        jd0 = jd_of(dt.datetime(day.year, day.month, day.day, tzinfo=dt.timezone.utc))
        noon = transit(jd0, longitude, jd0 + 0.5)
        on_date = moment_of(noon).astimezone(zone).date() == date
        candidates.append((not on_date, abs(noon - target), jd0, noon))
    _, _, day_jd0, noon = min(candidates)

    noon_info, noon_elevation, _ = elevation_azimuth(noon, latitude, longitude)
    result = {"date": date.isoformat(), "latitude": latitude, "longitude": longitude, "timezone": zone_name,
              "solar_noon": local(noon), "solar_noon_elevation_deg": round(noon_elevation, 4)}
    rise = event(day_jd0, latitude, longitude, DEPRESSIONS["sunrise_sunset"], -1, noon)
    sets = event(day_jd0, latitude, longitude, DEPRESSIONS["sunrise_sunset"], 1, noon)
    status = "normal"
    if isinstance(rise, str) and isinstance(sets, str):
        status = "polar_day" if rise == "above" or sets == "above" else "polar_night"
    elif isinstance(rise, str) or isinstance(sets, str):
        status = "sun_rises_only" if isinstance(sets, str) else "sun_sets_only"
    result["day_status"] = status
    result["sunrise"] = None if isinstance(rise, str) else local(rise)
    result["sunset"] = None if isinstance(sets, str) else local(sets)
    if status == "normal":
        result["day_length"] = clock((sets - rise) * 86400)
    elif status == "polar_day":
        result["day_length"] = "24:00:00"
    elif status == "polar_night":
        result["day_length"] = "00:00:00"
    else:
        result["day_length"] = None
    for name in ("civil", "nautical", "astronomical"):
        begin = event(day_jd0, latitude, longitude, DEPRESSIONS[name], -1, noon)
        end = event(day_jd0, latitude, longitude, DEPRESSIONS[name], 1, noon)
        entry = {"begin": None if isinstance(begin, str) else local(begin),
                 "end": None if isinstance(end, str) else local(end)}
        flags = {x for x in (begin, end) if isinstance(x, str)}
        if flags:
            entry["status"] = (f"sun_always_above_-{DEPRESSIONS[name]:g}_deg" if "above" in flags
                               else f"sun_always_below_-{DEPRESSIONS[name]:g}_deg")
        result[f"{name}_twilight"] = entry
    result["declination_deg"] = round(noon_info["declination"], 5)
    result["equation_of_time_minutes"] = round(noon_info["eot"], 4)
    if "datetime" in request:
        moment = parse_instant(request["datetime"])
        jd = jd_of(moment)
        info, elevation, azimuth = elevation_azimuth(jd, latitude, longitude)
        eot_seconds = info["eot"] * 60
        result["position"] = {
            "datetime": moment.isoformat(), "julian_day": round(jd, 6),
            "declination_deg": round(info["declination"], 6),
            "apparent_longitude_deg": round(info["apparent_longitude"], 6),
            "obliquity_deg": round(info["obliquity"], 6),
            "equation_of_time_minutes": round(info["eot"], 5),
            "equation_of_time": f"{'-' if eot_seconds < 0 else '+'}{int(abs(eot_seconds) // 60)}m "
                                f"{abs(eot_seconds) % 60:04.1f}s",
            "elevation_deg": round(elevation, 4), "azimuth_deg": round(azimuth, 4),
            "note": "geometric elevation, no refraction; azimuth clockwise from north"}
    result["method"] = ("NOAA solar calculator (Meeus low-accuracy Sun); sunrise/sunset at -0.833 deg "
                        "(refraction and solar radius, sea level); twilights at -6, -12, -18 deg")
    print(json.dumps(result))


if __name__ == "__main__":
    main()
