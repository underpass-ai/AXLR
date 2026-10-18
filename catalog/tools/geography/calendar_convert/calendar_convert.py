#!/usr/bin/env python3
"""Convert a date between calendars through R.D. fixed day numbers (Dershowitz & Reingold,
Calendrical Calculations): proleptic Gregorian, Julian, ISO week date, Julian day (number and
JD with a fraction), Hebrew (arithmetic) and Islamic (tabular, arithmetic), plus the weekday.

Input (JSON on stdin):
  from     "gregorian" | "julian" | "iso_week" | "jd" | "jdn" | "rd" | "hebrew" | "islamic"
  date     "YYYY-MM-DD" (gregorian, julian) or "YYYY-Www-D" (iso_week); or year/month/day,
           year/week/weekday as integers. Years are astronomical (0 = 1 BCE, -1 = 2 BCE).
  year, month, day   integers; hebrew and islamic month may also be a name
  week, weekday      iso_week
  jd       Julian date, number or decimal string (from "jd"); jdn integer; rd integer
  time     "HH:MM[:SS]" universal time of day, optional (adds the fraction to jd)
  molad    true: also give the molad (mean conjunction) of the Hebrew month
"""
import json
import re
import sys
from fractions import Fraction


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


WEEKDAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"]
LIMIT_YEARS = 1000000


# ---- Gregorian (D&R 2.2) -------------------------------------------------------------------
def gregorian_leap(year):
    return year % 4 == 0 and year % 400 not in (100, 200, 300)


def fixed_from_gregorian(year, month, day):
    adj = 0 if month <= 2 else (-1 if gregorian_leap(year) else -2)
    return (365 * (year - 1) + (year - 1) // 4 - (year - 1) // 100 + (year - 1) // 400
            + (367 * month - 362) // 12 + adj + day)


def gregorian_year_from_fixed(date):
    d0 = date - 1
    n400, d1 = divmod(d0, 146097)
    n100, d2 = divmod(d1, 36524)
    n4, d3 = divmod(d2, 1461)
    n1 = d3 // 365
    year = 400 * n400 + 100 * n100 + 4 * n4 + n1
    return year if n100 == 4 or n1 == 4 else year + 1


def gregorian_from_fixed(date):
    year = gregorian_year_from_fixed(date)
    prior = date - fixed_from_gregorian(year, 1, 1)
    correction = 0 if date < fixed_from_gregorian(year, 3, 1) else (1 if gregorian_leap(year) else 2)
    month = (12 * (prior + correction) + 373) // 367
    return year, month, date - fixed_from_gregorian(year, month, 1) + 1


def gregorian_month_days(year, month):
    if month == 2:
        return 29 if gregorian_leap(year) else 28
    return 30 if month in (4, 6, 9, 11) else 31


# ---- Julian (D&R 3.1), astronomical years outside, D&R's no-year-zero inside ---------------
JULIAN_EPOCH = fixed_from_gregorian(0, 12, 30)


def julian_leap_astro(year):
    return year % 4 == 0


def fixed_from_julian(year, month, day):
    # D&R write y' = y + 1 for their BCE years, which is the astronomical year itself
    y = year
    adj = 0 if month <= 2 else (-1 if julian_leap_astro(year) else -2)
    return JULIAN_EPOCH - 1 + 365 * (y - 1) + (y - 1) // 4 + (367 * month - 362) // 12 + adj + day


def julian_from_fixed(date):
    year = (4 * (date - JULIAN_EPOCH) + 1464) // 1461  # D&R's approx, already astronomical
    prior = date - fixed_from_julian(year, 1, 1)
    correction = 0 if date < fixed_from_julian(year, 3, 1) else (1 if julian_leap_astro(year) else 2)
    month = (12 * (prior + correction) + 373) // 367
    return year, month, date - fixed_from_julian(year, month, 1) + 1


def julian_month_days(year, month):
    if month == 2:
        return 29 if julian_leap_astro(year) else 28
    return 30 if month in (4, 6, 9, 11) else 31


# ---- ISO week date (D&R 5.1) ----------------------------------------------------------------
def kday_on_or_before(k, date):
    return date - (date - k) % 7


def fixed_from_iso(year, week, day):
    # nth-kday(week, sunday, gregorian(year - 1, 12, 28)) + day
    return 7 * week + kday_on_or_before(0, fixed_from_gregorian(year - 1, 12, 28) - 1) + day


def iso_from_fixed(date):
    approx = gregorian_year_from_fixed(date - 3)
    year = approx + 1 if date >= fixed_from_iso(approx + 1, 1, 1) else approx
    week = (date - fixed_from_iso(year, 1, 1)) // 7 + 1
    day = date % 7 or 7
    return year, week, day


def iso_weeks_in_year(year):
    return (fixed_from_iso(year + 1, 1, 1) - fixed_from_iso(year, 1, 1)) // 7


# ---- Hebrew (D&R 8) ----------------------------------------------------------------------------
HEBREW_EPOCH = fixed_from_julian(-3760, 10, 7)  # 7 October 3761 BCE (Julian), a Monday
TISHREI, ADAR, ADAR_II = 7, 12, 13
HEBREW_NAMES = {1: "Nisan", 2: "Iyyar", 3: "Sivan", 4: "Tammuz", 5: "Av", 6: "Elul", 7: "Tishrei",
                8: "Marheshvan", 9: "Kislev", 10: "Tevet", 11: "Shevat", 12: "Adar", 13: "Adar II"}
HEBREW_ALIASES = {"nisan": 1, "nissan": 1, "iyyar": 2, "iyar": 2, "sivan": 3, "tammuz": 4, "tamuz": 4, "av": 5,
                  "ab": 5, "elul": 6, "tishrei": 7, "tishri": 7, "marheshvan": 8, "marcheshvan": 8, "heshvan": 8,
                  "cheshvan": 8, "kislev": 9, "tevet": 10, "teveth": 10, "shevat": 11, "shvat": 11,
                  "adar i": "adar1", "adar 1": "adar1", "adar alef": "adar1", "adar ii": 13,
                  "adar 2": 13, "adar bet": 13, "adar sheni": 13, "veadar": 13, "adar": "adar"}


def hebrew_leap(year):
    return (7 * year + 1) % 19 < 7


def hebrew_last_month(year):
    return ADAR_II if hebrew_leap(year) else ADAR


def hebrew_elapsed_days(year):
    months = (235 * year - 234) // 19
    parts = 12084 + 13753 * months
    days = 29 * months + parts // 25920
    return days + 1 if (3 * (days + 1)) % 7 < 3 else days


def hebrew_year_length_correction(year):
    ny0, ny1, ny2 = hebrew_elapsed_days(year - 1), hebrew_elapsed_days(year), hebrew_elapsed_days(year + 1)
    if ny2 - ny1 == 356:
        return 2
    if ny1 - ny0 == 382:
        return 1
    return 0


def hebrew_new_year(year):
    return HEBREW_EPOCH + hebrew_elapsed_days(year) + hebrew_year_length_correction(year)


def hebrew_year_days(year):
    return hebrew_new_year(year + 1) - hebrew_new_year(year)


def hebrew_month_days(year, month):
    length = hebrew_year_days(year)
    if month in (2, 4, 6, 10, 13) or (month == ADAR and not hebrew_leap(year)) \
            or (month == 8 and length % 10 != 5) or (month == 9 and length % 10 == 3):
        return 29
    return 30


def fixed_from_hebrew(year, month, day):
    total = hebrew_new_year(year) + day - 1
    if month < TISHREI:
        total += sum(hebrew_month_days(year, m) for m in range(TISHREI, hebrew_last_month(year) + 1))
        total += sum(hebrew_month_days(year, m) for m in range(1, month))
    else:
        total += sum(hebrew_month_days(year, m) for m in range(TISHREI, month))
    return total


def hebrew_from_fixed(date):
    approx = (date - HEBREW_EPOCH) * 98496 // 35975351 + 1
    year = approx - 1  # D&R: the largest y >= approx - 1 whose new year is on or before date
    while hebrew_new_year(year + 1) <= date:
        year += 1
    start = TISHREI if date < fixed_from_hebrew(year, 1, 1) else 1
    month = start
    while date > fixed_from_hebrew(year, month, hebrew_month_days(year, month)):
        month += 1
    return year, month, date - fixed_from_hebrew(year, month, 1) + 1


def hebrew_month_name(year, month):
    if month == ADAR and hebrew_leap(year):
        return "Adar I"
    return HEBREW_NAMES[month]


def molad(year, month):
    """Moment (R.D. with fraction) of the mean conjunction, D&R 8.4."""
    y = year + 1 if month < TISHREI else year
    months_elapsed = month - TISHREI + (235 * y - 234) // 19
    return HEBREW_EPOCH - Fraction(876, 25920) + months_elapsed * (29 + Fraction(1, 2) + Fraction(793, 25920))


def molad_report(year, month):
    moment = molad(year, month)
    shifted = moment + Fraction(1, 4)  # the traditional day begins at 6 pm of the civil day before
    day = shifted.numerator // shifted.denominator
    parts = (shifted - day) * 25920
    hours, chalakim = divmod(int(parts), 1080)
    civil = moment - (moment.numerator // moment.denominator)
    seconds = round(civil * 86400)
    return {"weekday": WEEKDAYS[day % 7], "hours": hours, "parts": chalakim,
            "text": f"{WEEKDAYS[day % 7]}, {hours} hours and {chalakim} parts",
            "gregorian_ut": f"{iso_gregorian(moment.numerator // moment.denominator)} "
                     f"{seconds // 3600:02d}:{seconds // 60 % 60:02d}:{seconds % 60:02d}",
            "reckoning": "Jerusalem mean time; hours counted from 6 pm; 1080 parts to the hour"}


# ---- Islamic, arithmetic (D&R 7.1) -------------------------------------------------------------
ISLAMIC_EPOCH = fixed_from_julian(622, 7, 16)
ISLAMIC_NAMES = ["Muharram", "Safar", "Rabi' al-Awwal", "Rabi' al-Thani", "Jumada al-Ula", "Jumada al-Akhira",
                 "Rajab", "Sha'ban", "Ramadan", "Shawwal", "Dhu al-Qa'da", "Dhu al-Hijja"]


def normal_name(text):
    return re.sub(r"[^a-z0-9 ]", "", text.lower().replace("-", " ").replace("_", " ")).replace("  ", " ").strip()


ISLAMIC_ALIASES = {normal_name(n): i + 1 for i, n in enumerate(ISLAMIC_NAMES)}
ISLAMIC_ALIASES.update({"rabi i": 3, "rabi ii": 4, "rabi al akhir": 4, "jumada i": 5, "jumada ii": 6,
                        "jumada al awwal": 5, "jumada al thani": 6, "shaban": 8, "ramadhan": 9,
                        "dhu al qadah": 11, "dhul qadah": 11, "dhu al hijjah": 12, "dhul hijjah": 12})


def islamic_leap(year):
    return (14 + 11 * year) % 30 < 11


def fixed_from_islamic(year, month, day):
    return (day + 29 * (month - 1) + (6 * month - 1) // 11 + (year - 1) * 354 + (3 + 11 * year) // 30
            + ISLAMIC_EPOCH - 1)


def islamic_from_fixed(date):
    year = (30 * (date - ISLAMIC_EPOCH) + 10646) // 10631
    prior = date - fixed_from_islamic(year, 1, 1)
    month = (11 * prior + 330) // 325
    return year, month, date - fixed_from_islamic(year, month, 1) + 1


def islamic_month_days(year, month):
    if month == 12:
        return 30 if islamic_leap(year) else 29
    return 30 if month % 2 == 1 else 29


# ---- input --------------------------------------------------------------------------------------
def integer(request, field, low=None, high=None):
    value = request.get(field)
    if isinstance(value, bool) or not isinstance(value, int):
        fail(f"{field} must be an integer")
    if (low is not None and value < low) or (high is not None and value > high):
        fail(f"{field} must be between {low} and {high}")
    return value


def ymd(request, calendar):
    text = request.get("date")
    if text is not None:
        match = re.fullmatch(r"(-?\d{1,7})-(\d{2})-(\d{2})", text) if isinstance(text, str) else None
        if not match:
            fail(f"date must be YYYY-MM-DD for {calendar} (astronomical year, may be negative)")
        return int(match.group(1)), int(match.group(2)), int(match.group(3))
    year = integer(request, "year", -LIMIT_YEARS, LIMIT_YEARS)
    return year, request.get("month"), integer(request, "day")


def check_year(year):
    if not -LIMIT_YEARS <= year <= LIMIT_YEARS:
        fail(f"year must be between {-LIMIT_YEARS} and {LIMIT_YEARS}")


def month_number(value, maximum):
    if isinstance(value, bool) or not isinstance(value, int) or not 1 <= value <= maximum:
        fail(f"month must be an integer 1-{maximum} or a month name")
    return value


def parse_time(request):
    text = request.get("time")
    if text is None:
        return Fraction(0)
    match = re.fullmatch(r"(\d{2}):(\d{2})(?::(\d{2})(\.\d+)?)?", text) if isinstance(text, str) else None
    if not match:
        fail("time must be HH:MM or HH:MM:SS (universal time)")
    h, m, s = int(match.group(1)), int(match.group(2)), int(match.group(3) or 0)
    if h > 23 or m > 59 or s > 59:
        fail("time is out of range")
    return Fraction(h * 3600 + m * 60 + s, 86400) + Fraction(match.group(4) or "0") / 86400


def to_fraction(value, field):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    try:
        return Fraction(str(value).strip())
    except (ValueError, ZeroDivisionError):
        fail(f"{field} must be a number or a decimal string")


def read_rd(request):
    source = request.get("from")
    fraction = parse_time(request)
    if source == "gregorian":
        year, month, day = ymd(request, "gregorian")
        check_year(year)
        month = month_number(month, 12)
        if not 1 <= day <= gregorian_month_days(year, month):
            fail(f"day must be 1-{gregorian_month_days(year, month)} in that Gregorian month")
        return fixed_from_gregorian(year, month, day), fraction
    if source == "julian":
        year, month, day = ymd(request, "julian")
        check_year(year)
        month = month_number(month, 12)
        if not 1 <= day <= julian_month_days(year, month):
            fail(f"day must be 1-{julian_month_days(year, month)} in that Julian month")
        return fixed_from_julian(year, month, day), fraction
    if source == "iso_week":
        text = request.get("date")
        if text is not None:
            match = re.fullmatch(r"(-?\d{1,7})-?W(\d{2})-?([1-7])", text) if isinstance(text, str) else None
            if not match:
                fail("date must be YYYY-Www-D for iso_week, e.g. 2009-W01-1")
            year, week, day = (int(g) for g in match.groups())
        else:
            year, week, day = integer(request, "year"), integer(request, "week"), integer(request, "weekday", 1, 7)
        check_year(year)
        if not 1 <= week <= iso_weeks_in_year(year):
            fail(f"week must be 1-{iso_weeks_in_year(year)} in ISO year {year}")
        return fixed_from_iso(year, week, day), fraction
    if source == "jd":
        if "time" in request:
            fail("time is not used with from jd; put the fraction in jd")
        moment = to_fraction(request.get("jd"), "jd") - Fraction(3442849, 2)  # JD - 1721424.5
        day = moment.numerator // moment.denominator
        if abs(day) > 400 * LIMIT_YEARS:
            fail("jd is out of range")
        return day, moment - day
    if source == "jdn":
        jdn = integer(request, "jdn", -400 * LIMIT_YEARS, 400 * LIMIT_YEARS)
        return jdn - 1721425, fraction
    if source == "rd":
        return integer(request, "rd", -400 * LIMIT_YEARS, 400 * LIMIT_YEARS), fraction
    if source == "hebrew":
        year = integer(request, "year", 1, LIMIT_YEARS)
        month = request.get("month")
        if isinstance(month, str):
            key = HEBREW_ALIASES.get(normal_name(month).replace("  ", " "))
            if key is None:
                fail(f"unknown Hebrew month name {month!r}")
            if key == "adar1":
                if not hebrew_leap(year):
                    fail(f"{year} is not a leap year: it has a single Adar")
                key = ADAR
            elif key == "adar":
                if hebrew_leap(year):
                    fail(f"{year} is a leap year: say Adar I or Adar II")
                key = ADAR
            month = key
        month = month_number(month, hebrew_last_month(year))
        day = integer(request, "day")
        if not 1 <= day <= hebrew_month_days(year, month):
            fail(f"day must be 1-{hebrew_month_days(year, month)} in {hebrew_month_name(year, month)} {year}")
        return fixed_from_hebrew(year, month, day), fraction
    if source == "islamic":
        year = integer(request, "year", -LIMIT_YEARS, LIMIT_YEARS)
        month = request.get("month")
        if isinstance(month, str):
            month = ISLAMIC_ALIASES.get(normal_name(month))
            if month is None:
                fail(f"unknown Islamic month name {request.get('month')!r}")
        month = month_number(month, 12)
        day = integer(request, "day")
        if not 1 <= day <= islamic_month_days(year, month):
            fail(f"day must be 1-{islamic_month_days(year, month)} in that Islamic month")
        return fixed_from_islamic(year, month, day), fraction
    fail("from must be one of gregorian, julian, iso_week, jd, jdn, rd, hebrew, islamic")


def year_text(year):
    return f"{year:04d}" if year >= 0 else f"-{-year:04d}"


def iso_gregorian(rd):
    y, m, d = gregorian_from_fixed(rd)
    return f"{year_text(y)}-{m:02d}-{d:02d}"


def era(year):
    return f"{year} CE" if year > 0 else f"{1 - year} BCE"


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    known = {"from", "date", "year", "month", "day", "week", "weekday", "jd", "jdn", "rd", "time", "molad"}
    unknown = sorted(set(request) - known)
    if unknown:
        fail(f"unknown field(s): {', '.join(unknown)}")
    if "molad" in request and not isinstance(request["molad"], bool):
        fail("molad must be true or false")
    rd, fraction = read_rd(request)

    gy, gm, gd = gregorian_from_fixed(rd)
    jy, jm, jdd = julian_from_fixed(rd)
    iy, iw, idd = iso_from_fixed(rd)
    jd = rd + Fraction(3442849, 2) + fraction
    seconds = round(fraction * 86400)
    result = {
        "from": request["from"],
        "rd": rd,
        "jdn": rd + 1721425,
        "jd": float(round(jd, 6)) if fraction else float(jd),
        "mjd": float(round(jd - Fraction(4800001, 2), 6)),
        "time_ut": f"{seconds // 3600:02d}:{seconds // 60 % 60:02d}:{seconds % 60:02d}",
        "weekday": {"name": WEEKDAYS[rd % 7], "iso": rd % 7 or 7},
        "gregorian": {"year": gy, "month": gm, "day": gd, "date": f"{year_text(gy)}-{gm:02d}-{gd:02d}",
                      "leap_year": gregorian_leap(gy), "era": era(gy)},
        "julian": {"year": jy, "month": jm, "day": jdd, "date": f"{year_text(jy)}-{jm:02d}-{jdd:02d}",
                   "leap_year": julian_leap_astro(jy), "era": era(jy)},
        "iso_week": {"year": iy, "week": iw, "weekday": idd, "date": f"{year_text(iy)}-W{iw:02d}-{idd}"},
    }
    if rd >= HEBREW_EPOCH:
        hy, hm, hd = hebrew_from_fixed(rd)
        result["hebrew"] = {"year": hy, "month": hm, "month_name": hebrew_month_name(hy, hm), "day": hd,
                            "date": f"{hd} {hebrew_month_name(hy, hm)} {hy}", "leap_year": hebrew_leap(hy),
                            "days_in_year": hebrew_year_days(hy),
                            "numbering": "D&R: Nisan = 1, Tishrei = 7, Adar (Adar I) = 12, Adar II = 13"}
        if request.get("molad"):
            result["hebrew"]["molad"] = molad_report(hy, hm)
    else:
        result["hebrew"] = None
        if request.get("molad"):
            fail("molad needs a date on or after the Hebrew epoch (1 Tishrei AM 1)")
    sy, sm, sd = islamic_from_fixed(rd)
    result["islamic"] = {"year": sy, "month": sm, "month_name": ISLAMIC_NAMES[sm - 1], "day": sd,
                         "date": f"{sd} {ISLAMIC_NAMES[sm - 1]} {sy} AH", "leap_year": islamic_leap(sy),
                         "scheme": "tabular (D&R arithmetic): leap years 2, 5, 7, 10, 13, 16, 18, 21, 24, 26, 29 "
                                   "of 30; epoch 16 July 622 Julian (Friday)"}
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
