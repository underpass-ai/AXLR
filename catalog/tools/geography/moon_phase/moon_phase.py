#!/usr/bin/env python3
"""Moon phase at an instant: phase angle, illuminated fraction, age since the last new Moon,
phase name, and the next new Moon, first quarter, full Moon and last quarter (Meeus,
Astronomical Algorithms, chapters 48 and 49); or every principal phase of a year.

Input (JSON on stdin):
  operation  "at" (default) | "year"
  datetime   ISO 8601 instant, UTC ("2026-10-18T00:00:00Z"; an offset is converted; naive = UTC)
  year       integer (operation year)
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
PHASES = ["New Moon", "First Quarter", "Full Moon", "Last Quarter"]
SYNODIC = 29.530588861


def sind(x):
    return math.sin(math.radians(x))


def cosd(x):
    return math.cos(math.radians(x))


# ---- Delta T: Espenak and Meeus polynomials (NASA Five Millennium Canon of Solar Eclipses) ----
def delta_t(year):
    y = year
    if y < -500 or y >= 2150:
        u = (y - 1820) / 100
        return -20 + 32 * u * u
    if y < 500:
        u = y / 100
        return (10583.6 - 1014.41 * u + 33.78311 * u ** 2 - 5.952053 * u ** 3 - 0.1798452 * u ** 4
                + 0.022174192 * u ** 5 + 0.0090316521 * u ** 6)
    if y < 1600:
        u = (y - 1000) / 100
        return (1574.2 - 556.01 * u + 71.23472 * u ** 2 + 0.319781 * u ** 3 - 0.8503463 * u ** 4
                - 0.005050998 * u ** 5 + 0.0083572073 * u ** 6)
    if y < 1700:
        t = y - 1600
        return 120 - 0.9808 * t - 0.01532 * t ** 2 + t ** 3 / 7129
    if y < 1800:
        t = y - 1700
        return 8.83 + 0.1603 * t - 0.0059285 * t ** 2 + 0.00013336 * t ** 3 - t ** 4 / 1174000
    if y < 1860:
        t = y - 1800
        return (13.72 - 0.332447 * t + 0.0068612 * t ** 2 + 0.0041116 * t ** 3 - 0.00037436 * t ** 4
                + 0.0000121272 * t ** 5 - 0.0000001699 * t ** 6 + 0.000000000875 * t ** 7)
    if y < 1900:
        t = y - 1860
        return (7.62 + 0.5737 * t - 0.251754 * t ** 2 + 0.01680668 * t ** 3 - 0.0004473624 * t ** 4
                + t ** 5 / 233174)
    if y < 1920:
        t = y - 1900
        return -2.79 + 1.494119 * t - 0.0598939 * t ** 2 + 0.0061966 * t ** 3 - 0.000197 * t ** 4
    if y < 1941:
        t = y - 1920
        return 21.20 + 0.84493 * t - 0.076100 * t ** 2 + 0.0020936 * t ** 3
    if y < 1961:
        t = y - 1950
        return 29.07 + 0.407 * t - t ** 2 / 233 + t ** 3 / 2547
    if y < 1986:
        t = y - 1975
        return 45.45 + 1.067 * t - t ** 2 / 260 - t ** 3 / 718
    if y < 2005:
        t = y - 2000
        return (63.86 + 0.3345 * t - 0.060374 * t ** 2 + 0.0017275 * t ** 3 + 0.000651814 * t ** 4
                + 0.00002373599 * t ** 5)
    if y < 2050:
        t = y - 2000
        return 62.92 + 0.32217 * t + 0.005589 * t ** 2
    return -20 + 32 * ((y - 1820) / 100) ** 2 - 0.5628 * (2150 - y)


def decimal_year(jd):
    return 2000.0 + (jd - 2451545.0) / 365.25


def jd_of(moment):
    return 2451545.0 + (moment - J2000).total_seconds() / 86400.0


def moment_of(jd):
    return J2000 + dt.timedelta(days=jd - 2451545.0)


def ut_to_td(jd_ut):
    return jd_ut + delta_t(decimal_year(jd_ut)) / 86400.0


def td_to_ut(jde):
    return jde - delta_t(decimal_year(jde)) / 86400.0


# ---- Meeus chapter 49: true phases ------------------------------------------------------------
NEW = [(-0.40720, 0, "Mp"), (0.17241, 1, "M"), (0.01608, 0, "2Mp"), (0.01039, 0, "2F"), (0.00739, 1, "Mp-M"),
       (-0.00514, 1, "Mp+M"), (0.00208, 2, "2M"), (-0.00111, 0, "Mp-2F"), (-0.00057, 0, "Mp+2F"),
       (0.00056, 1, "2Mp+M"), (-0.00042, 0, "3Mp"), (0.00042, 1, "M+2F"), (0.00038, 1, "M-2F"),
       (-0.00024, 1, "2Mp-M"), (-0.00017, 0, "O"), (-0.00007, 0, "Mp+2M"), (0.00004, 0, "2Mp-2F"),
       (0.00004, 0, "3M"), (0.00003, 0, "Mp+M-2F"), (0.00003, 0, "2Mp+2F"), (-0.00003, 0, "Mp+M+2F"),
       (0.00003, 0, "Mp-M+2F"), (-0.00002, 0, "Mp-M-2F"), (-0.00002, 0, "3Mp+M"), (0.00002, 0, "4Mp")]
FULL = [(-0.40614, 0, "Mp"), (0.17302, 1, "M"), (0.01614, 0, "2Mp"), (0.01043, 0, "2F"), (0.00734, 1, "Mp-M"),
        (-0.00515, 1, "Mp+M"), (0.00209, 2, "2M"), (-0.00111, 0, "Mp-2F"), (-0.00057, 0, "Mp+2F"),
        (0.00056, 1, "2Mp+M"), (-0.00042, 0, "3Mp"), (0.00042, 1, "M+2F"), (0.00038, 1, "M-2F"),
        (-0.00024, 1, "2Mp-M"), (-0.00017, 0, "O"), (-0.00007, 0, "Mp+2M"), (0.00004, 0, "2Mp-2F"),
        (0.00004, 0, "3M"), (0.00003, 0, "Mp+M-2F"), (0.00003, 0, "2Mp+2F"), (-0.00003, 0, "Mp+M+2F"),
        (0.00003, 0, "Mp-M+2F"), (-0.00002, 0, "Mp-M-2F"), (-0.00002, 0, "3Mp+M"), (0.00002, 0, "4Mp")]
QUARTER = [(-0.62801, 0, "Mp"), (0.17172, 1, "M"), (-0.01183, 1, "Mp+M"), (0.00862, 0, "2Mp"), (0.00804, 0, "2F"),
           (0.00454, 1, "Mp-M"), (0.00204, 2, "2M"), (-0.00180, 0, "Mp-2F"), (-0.00070, 0, "Mp+2F"),
           (-0.00040, 0, "3Mp"), (-0.00034, 1, "2Mp-M"), (0.00032, 1, "M+2F"), (0.00032, 1, "M-2F"),
           (-0.00028, 2, "Mp+2M"), (0.00027, 1, "2Mp+M"), (-0.00017, 0, "O"), (-0.00005, 0, "Mp-M-2F"),
           (0.00004, 0, "2Mp+2F"), (-0.00004, 0, "Mp+M+2F"), (0.00004, 0, "Mp-2M"), (0.00003, 0, "Mp+M-2F"),
           (0.00003, 0, "3M"), (0.00002, 0, "2Mp-2F"), (0.00002, 0, "Mp-M+2F"), (-0.00002, 0, "3Mp+M")]
PLANETARY = [(0.000325, 299.77, 0.107408), (0.000165, 251.88, 0.016321), (0.000164, 251.83, 26.651886),
             (0.000126, 349.42, 36.412478), (0.000110, 84.66, 18.206239), (0.000062, 141.74, 53.303771),
             (0.000060, 207.14, 2.453732), (0.000056, 154.84, 7.306860), (0.000047, 34.52, 27.261239),
             (0.000042, 207.19, 0.121824), (0.000040, 291.34, 1.844379), (0.000037, 161.72, 24.198154),
             (0.000035, 239.56, 25.513099), (0.000023, 331.55, 3.592518)]
TERM = re.compile(r"([+-]?)(\d?)(Mp|M|F|O)")


def argument(text, values):
    total = 0.0
    for sign, count, name in TERM.findall(text):
        total += (-1 if sign == "-" else 1) * int(count or 1) * values[name]
    return total


def true_phase(k):
    """JDE (TD) of the phase with lunation number k (integer + 0, .25, .5, .75), Meeus 49."""
    t = k / 1236.85
    jde = (2451550.09766 + SYNODIC * k + 0.00015437 * t ** 2 - 0.000000150 * t ** 3
           + 0.00000000073 * t ** 4)
    e = 1 - 0.002516 * t - 0.0000074 * t ** 2
    values = {"M": 2.5534 + 29.10535670 * k - 0.0000014 * t ** 2 - 0.00000011 * t ** 3,
              "Mp": 201.5643 + 385.81693528 * k + 0.0107582 * t ** 2 + 0.00001238 * t ** 3 - 0.000000058 * t ** 4,
              "F": 160.7108 + 390.67050284 * k - 0.0016118 * t ** 2 - 0.00000227 * t ** 3 + 0.000000011 * t ** 4,
              "O": 124.7746 - 1.56375588 * k + 0.0020672 * t ** 2 + 0.00000215 * t ** 3}
    fraction = round((k % 1) * 4) % 4
    table = (NEW, QUARTER, FULL, QUARTER)[fraction]
    correction = sum(c * e ** p * sind(argument(a, values)) for c, p, a in table)
    if fraction in (1, 3):
        m, mp, f = values["M"], values["Mp"], values["F"]
        w = (0.00306 - 0.00038 * e * cosd(m) + 0.00026 * cosd(mp) - 0.00002 * cosd(mp - m)
             + 0.00002 * cosd(mp + m) + 0.00002 * cosd(2 * f))
        correction += w if fraction == 1 else -w
    a1 = 299.77 + 0.107408 * k - 0.009173 * t ** 2
    planetary = PLANETARY[0][0] * sind(a1) + sum(c * sind(a0 + rate * k) for c, a0, rate in PLANETARY[1:])
    return jde + correction + planetary, fraction


# ---- Meeus chapter 48, formula 48.4 (lower accuracy) ----------------------------------------
def illumination(jde):
    t = (jde - 2451545.0) / 36525.0
    d = 297.8501921 + 445267.1114034 * t - 0.0018819 * t ** 2 + t ** 3 / 545868 - t ** 4 / 113065000
    m = 357.5291092 + 35999.0502909 * t - 0.0001536 * t ** 2 + t ** 3 / 24490000
    mp = 134.9633964 + 477198.8675055 * t + 0.0087414 * t ** 2 + t ** 3 / 69699 - t ** 4 / 14712000
    i = (180 - d - 6.289 * sind(mp) + 2.100 * sind(m) - 1.274 * sind(2 * d - mp) - 0.658 * sind(2 * d)
         - 0.214 * sind(2 * mp) - 0.110 * sind(d)) % 360.0
    phase_angle = i if i <= 180 else 360 - i
    return phase_angle, (1 + cosd(i)) / 2, d % 360.0


def phase_record(k):
    jde, fraction = true_phase(k)
    ut = moment_of(td_to_ut(jde))
    td = moment_of(jde)
    minute = (ut + dt.timedelta(seconds=30)).replace(second=0, microsecond=0)
    return {"phase": PHASES[fraction], "utc": iso(ut), "utc_minute": minute.strftime("%Y-%m-%dT%H:%MZ"),
            "td": iso(td)[:-1] + " TD", "jde": round(jde, 5), "lunation_k": k}


def iso(moment):
    moment = (moment + dt.timedelta(microseconds=500000)).replace(microsecond=0)
    return moment.strftime("%Y-%m-%dT%H:%M:%SZ")


def parse_instant(text):
    if not isinstance(text, str):
        fail("datetime must be an ISO 8601 string")
    value = text.strip()
    if value.endswith(("Z", "z")):
        value = value[:-1] + "+00:00"
    try:
        moment = dt.datetime.fromisoformat(value)
    except ValueError:
        fail("datetime must be ISO 8601, e.g. 2026-10-18T00:00:00Z")
    if moment.tzinfo is None:
        moment = moment.replace(tzinfo=dt.timezone.utc)
    moment = moment.astimezone(dt.timezone.utc)
    if not 1000 <= moment.year <= 3000:
        fail("datetime must be between the years 1000 and 3000")
    return moment


def first_k_before(jde):
    """A quarter-lunation number whose phase falls safely before jde."""
    return math.floor((decimal_year(jde) - 2000) * 12.3685 * 4) / 4 - 1


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    unknown = sorted(set(request) - {"operation", "datetime", "year"})
    if unknown:
        fail(f"unknown field(s): {', '.join(unknown)}")
    operation = request.get("operation", "at")
    note = ("true phases by Meeus ch. 49 (TD) converted to UTC with the Espenak-Meeus delta T polynomial; "
            "illuminated fraction and phase angle by Meeus' lower-accuracy formula 48.4")
    if operation == "year":
        year = request.get("year")
        if isinstance(year, bool) or not isinstance(year, int) or not 1000 <= year <= 3000:
            fail("year must be an integer between 1000 and 3000")
        start = jd_of(dt.datetime(year, 1, 1, tzinfo=dt.timezone.utc))
        end = jd_of(dt.datetime(year + 1, 1, 1, tzinfo=dt.timezone.utc))
        k = first_k_before(ut_to_td(start))
        phases = []
        while True:
            jde, _ = true_phase(k)
            ut = td_to_ut(jde)
            if ut >= end:
                break
            if ut >= start:
                phases.append(phase_record(k))
            k += 0.25
        print(json.dumps({"operation": "year", "year": year, "count": len(phases), "phases": phases,
                          "delta_t_seconds": round(delta_t(year + 0.5), 1), "method": note}))
        return
    if operation != "at":
        fail("operation must be at or year")
    if "datetime" not in request:
        fail("datetime is required for operation at")
    moment = parse_instant(request["datetime"])
    jd_ut = jd_of(moment)
    jde = ut_to_td(jd_ut)
    k = first_k_before(jde)
    previous_new = None
    upcoming = {}
    while len(upcoming) < 4:
        when, fraction = true_phase(k)
        if when <= jde:
            if fraction == 0:
                previous_new = k
        elif PHASES[fraction] not in upcoming:
            upcoming[PHASES[fraction]] = k
        k += 0.25
    phase_angle, fraction_lit, elongation = illumination(jde)
    prev_jde, _ = true_phase(previous_new)
    age = jde - prev_jde
    # principal phase within 12 hours of the instant; otherwise the stretch between two of them
    nearest = None
    for kk in [previous_new + q / 4 for q in range(5)]:
        when, fraction = true_phase(kk)
        if abs(when - jde) <= 0.5:
            nearest = PHASES[fraction]
    if nearest:
        name = nearest
    else:
        quarter = min(int(age / SYNODIC * 4), 3)
        for q in range(1, 4):
            if true_phase(previous_new + q / 4)[0] > jde:
                quarter = q - 1
                break
        else:
            quarter = 3
        name = ["Waxing Crescent", "Waxing Gibbous", "Waning Gibbous", "Waning Crescent"][quarter]
    result = {
        "operation": "at", "datetime": iso(moment), "jde": round(jde, 6),
        "delta_t_seconds": round(delta_t(decimal_year(jd_ut)), 1),
        "phase_name": name, "waxing": age < true_phase(previous_new + 0.5)[0] - prev_jde,
        "phase_angle_deg": round(phase_angle, 4), "illuminated_fraction": round(fraction_lit, 4),
        "mean_elongation_deg": round(elongation, 4), "age_days": round(age, 4),
        "previous_new_moon": phase_record(previous_new),
        "next": [phase_record(kk) for kk in sorted(upcoming.values())],
        "method": note,
    }
    print(json.dumps(result))


if __name__ == "__main__":
    main()
