#!/usr/bin/env python3
"""Write catalog/tools/administration/holiday_table/holidays.json: public holidays for ES, PT,
FR, DE (nationwide), IT, GB-ENG (England and Wales) and US (federal), years 2024-2032, COMPUTED
here from each holiday's legal rule (fixed date, Easter offset, nth weekday, substitution) rather
than copied from a list. Each rule records its legal source. Rules read 18 Oct 2026; a change in
the law, or a one-off holiday, is a new run of this generator and a new version of the tool.

Easter Sunday: the anonymous Gregorian algorithm (Meeus/Jones/Butcher).

Usage: python3 tools/catalog/data/holiday_table.py
"""
import datetime as dt
import json
import pathlib

YEARS = range(2024, 2033)  # the years the tool answers
COMPUTED = range(2023, 2034)  # one more on each side, for observed days that cross the year
GENERATED = "2026-10-18"


def easter(year):
    a = year % 19
    b, c = divmod(year, 100)
    d, e = divmod(b, 4)
    f = (b + 8) // 25
    g = (b - f + 1) // 3
    h = (19 * a + b - d - g + 15) % 30
    i, k = divmod(c, 4)
    l = (32 + 2 * e + 2 * i - h - k) % 7
    m = (a + 11 * h + 22 * l) // 451
    month, day = divmod(h + l - 7 * m + 114, 31)
    return dt.date(year, month, day + 1)


assert easter(2024) == dt.date(2024, 3, 31) and easter(2026) == dt.date(2026, 4, 5) and easter(2027) == dt.date(2027, 3, 28)


def nth_weekday(year, month, weekday, n):
    """n-th (1-based) ISO weekday of a month; n = -1 is the last one."""
    if n > 0:
        first = dt.date(year, month, 1)
        return first + dt.timedelta(days=(weekday - first.isoweekday()) % 7 + 7 * (n - 1))
    nxt = dt.date(year + (month == 12), month % 12 + 1, 1)
    last = nxt - dt.timedelta(days=1)
    return last - dt.timedelta(days=(last.isoweekday() - weekday) % 7)


def fixed(month, day):
    return lambda y: dt.date(y, month, day)


def from_easter(offset):
    return lambda y: easter(y) + dt.timedelta(days=offset)


BOE_ET = "Estatuto de los Trabajadores (RDLeg 2/2015) art. 37.2 and Real Decreto 2001/1983 art. 45; the yearly list is the BOE resolution of the Dirección General de Trabajo: https://www.boe.es/buscar/act.php?id=BOE-A-2015-11430"
PT_CT = "Código do Trabalho (Lei 7/2009) art. 234, as amended by Lei 8/2016 (restored Corpus Christi, 5 Oct, 1 Nov, 1 Dec)"
FR_CT = "Code du travail art. L3133-1: https://www.legifrance.gouv.fr/codes/article_lc/LEGIARTI000033020517"
FR_AM = "Code du travail art. L3134-13 (Alsace-Moselle local law)"
DE_LAND = "Feiertagsgesetze of all 16 Länder (holiday law is state law)"
DE_EV = "Einigungsvertrag art. 2(2) (federal law): 3 October is the Day of German Unity"
IT_LAW = "Legge 27 maggio 1949 n. 260 as amended by Legge 54/1977 and DPR 792/1985"
IT_FRAN = "Legge 8 ottobre 2025 n. 151 (GU n. 236 of 10 Oct 2025), amending Legge 260/1949, in force from 2026"
UK_ACT = "Banking and Financial Dealings Act 1971, Schedule 1, para 1 (England and Wales); substitute days and any one-off change by royal proclamation: https://www.gov.uk/bank-holidays"
US_LAW = "5 U.S.C. 6103(a): https://www.law.cornell.edu/uscode/text/5/6103"
US_OBS = "observed day: 5 U.S.C. 6103(b) and Executive Order 11582 (Saturday to the preceding Friday, Sunday to the following Monday): https://www.opm.gov/policy-data-oversight/pay-leave/federal-holidays/"

# rule id: (local name, English name, scope, regions or None, date function, source, first year)
COUNTRIES = {
    "ES": {
        "name": "Spain", "language": "es",
        "note": "National holidays common to all of Spain. Each autonomous community may replace some of them (RD 2001/1983 art. 45.3), adds its own and moves a holiday falling on Sunday to Monday; two local holidays per municipality. Check the yearly BOE resolution and the regional and municipal calendars.",
        "rules": {
            "anio_nuevo": ("Año Nuevo", "New Year's Day", "national", None, fixed(1, 1), BOE_ET, None),
            "epifania": ("Epifanía del Señor", "Epiphany", "national", None, fixed(1, 6), BOE_ET, None),
            "jueves_santo": ("Jueves Santo", "Holy Thursday", "regional", None, from_easter(-3), BOE_ET + " (Holy Thursday is a holiday in most communities; the list varies each year)", None),
            "viernes_santo": ("Viernes Santo", "Good Friday", "national", None, from_easter(-2), BOE_ET, None),
            "lunes_pascua": ("Lunes de Pascua", "Easter Monday", "regional", None, from_easter(1), BOE_ET + " (Easter Monday is a holiday in a few communities; the list varies each year)", None),
            "trabajo": ("Fiesta del Trabajo", "Labour Day", "national", None, fixed(5, 1), BOE_ET, None),
            "asuncion": ("Asunción de la Virgen", "Assumption Day", "national", None, fixed(8, 15), BOE_ET, None),
            "fiesta_nacional": ("Fiesta Nacional de España", "National Day of Spain", "national", None, fixed(10, 12), BOE_ET, None),
            "todos_santos": ("Todos los Santos", "All Saints' Day", "national", None, fixed(11, 1), BOE_ET, None),
            "constitucion": ("Día de la Constitución Española", "Constitution Day", "national", None, fixed(12, 6), BOE_ET, None),
            "inmaculada": ("Inmaculada Concepción", "Immaculate Conception", "national", None, fixed(12, 8), BOE_ET, None),
            "navidad": ("Natividad del Señor", "Christmas Day", "national", None, fixed(12, 25), BOE_ET, None),
        },
    },
    "PT": {
        "name": "Portugal", "language": "pt",
        "note": "Mandatory national holidays. Carnival (Easter minus 47 days) is optional (Código do Trabalho art. 235) and each municipality has its own holiday; the autonomous regions of the Azores and Madeira add theirs.",
        "rules": {
            "ano_novo": ("Dia de Ano Novo", "New Year's Day", "national", None, fixed(1, 1), PT_CT, None),
            "sexta_santa": ("Sexta-Feira Santa", "Good Friday", "national", None, from_easter(-2), PT_CT, None),
            "pascoa": ("Domingo de Páscoa", "Easter Sunday", "national", None, from_easter(0), PT_CT, None),
            "liberdade": ("Dia da Liberdade", "Freedom Day", "national", None, fixed(4, 25), PT_CT, None),
            "trabalhador": ("Dia do Trabalhador", "Labour Day", "national", None, fixed(5, 1), PT_CT, None),
            "corpo_deus": ("Corpo de Deus", "Corpus Christi", "national", None, from_easter(60), PT_CT, None),
            "portugal": ("Dia de Portugal, de Camões e das Comunidades Portuguesas", "Portugal Day", "national", None, fixed(6, 10), PT_CT, None),
            "assuncao": ("Assunção de Nossa Senhora", "Assumption Day", "national", None, fixed(8, 15), PT_CT, None),
            "republica": ("Implantação da República", "Republic Day", "national", None, fixed(10, 5), PT_CT, None),
            "todos_santos": ("Dia de Todos os Santos", "All Saints' Day", "national", None, fixed(11, 1), PT_CT, None),
            "independencia": ("Restauração da Independência", "Restoration of Independence", "national", None, fixed(12, 1), PT_CT, None),
            "imaculada": ("Imaculada Conceição", "Immaculate Conception", "national", None, fixed(12, 8), PT_CT, None),
            "natal": ("Natal", "Christmas Day", "national", None, fixed(12, 25), PT_CT, None),
        },
    },
    "FR": {
        "name": "France", "language": "fr",
        "note": "Jours fériés of the Code du travail. Only 1 May is a paid day off by law for every employee; others depend on the collective agreement. Whit Monday may be the journée de solidarité. Overseas departments add the abolition of slavery day; Alsace-Moselle (FR-57, FR-67, FR-68) adds Good Friday and 26 December.",
        "rules": {
            "jour_an": ("Jour de l'an", "New Year's Day", "national", None, fixed(1, 1), FR_CT, None),
            "vendredi_saint": ("Vendredi saint", "Good Friday", "regional", ["FR-57", "FR-67", "FR-68"], from_easter(-2), FR_AM, None),
            "lundi_paques": ("Lundi de Pâques", "Easter Monday", "national", None, from_easter(1), FR_CT, None),
            "fete_travail": ("Fête du Travail", "Labour Day", "national", None, fixed(5, 1), FR_CT, None),
            "victoire_1945": ("Victoire 1945", "Victory in Europe Day", "national", None, fixed(5, 8), FR_CT, None),
            "ascension": ("Ascension", "Ascension Day", "national", None, from_easter(39), FR_CT, None),
            "lundi_pentecote": ("Lundi de Pentecôte", "Whit Monday", "national", None, from_easter(50), FR_CT, None),
            "fete_nationale": ("Fête nationale", "Bastille Day", "national", None, fixed(7, 14), FR_CT, None),
            "assomption": ("Assomption", "Assumption Day", "national", None, fixed(8, 15), FR_CT, None),
            "toussaint": ("Toussaint", "All Saints' Day", "national", None, fixed(11, 1), FR_CT, None),
            "armistice": ("Armistice 1918", "Armistice Day", "national", None, fixed(11, 11), FR_CT, None),
            "noel": ("Noël", "Christmas Day", "national", None, fixed(12, 25), FR_CT, None),
            "saint_etienne": ("Saint-Étienne", "St Stephen's Day", "regional", ["FR-57", "FR-67", "FR-68"], fixed(12, 26), FR_AM, None),
        },
    },
    "DE": {
        "name": "Germany", "language": "de",
        "note": "Only the nine holidays observed in every Land. Each Land adds its own (Epiphany, Corpus Christi, Reformation Day, All Saints' Day, Women's Day, Repentance Day and others) and some local ones; those are not in this table.",
        "rules": {
            "neujahr": ("Neujahr", "New Year's Day", "national", None, fixed(1, 1), DE_LAND, None),
            "karfreitag": ("Karfreitag", "Good Friday", "national", None, from_easter(-2), DE_LAND, None),
            "ostermontag": ("Ostermontag", "Easter Monday", "national", None, from_easter(1), DE_LAND, None),
            "tag_der_arbeit": ("Tag der Arbeit", "Labour Day", "national", None, fixed(5, 1), DE_LAND, None),
            "himmelfahrt": ("Christi Himmelfahrt", "Ascension Day", "national", None, from_easter(39), DE_LAND, None),
            "pfingstmontag": ("Pfingstmontag", "Whit Monday", "national", None, from_easter(50), DE_LAND, None),
            "einheit": ("Tag der Deutschen Einheit", "Day of German Unity", "national", None, fixed(10, 3), DE_EV, None),
            "weihnachten_1": ("1. Weihnachtstag", "Christmas Day", "national", None, fixed(12, 25), DE_LAND, None),
            "weihnachten_2": ("2. Weihnachtstag", "St Stephen's Day", "national", None, fixed(12, 26), DE_LAND, None),
        },
    },
    "IT": {
        "name": "Italy", "language": "it",
        "note": "National holidays. Each comune adds its patron saint's day (e.g. Rome 29 June, Milan 7 December); Bolzano adds Whit Monday.",
        "rules": {
            "capodanno": ("Capodanno", "New Year's Day", "national", None, fixed(1, 1), IT_LAW, None),
            "epifania": ("Epifania", "Epiphany", "national", None, fixed(1, 6), IT_LAW, None),
            "pasqua": ("Pasqua", "Easter Sunday", "national", None, from_easter(0), IT_LAW, None),
            "pasquetta": ("Lunedì dell'Angelo", "Easter Monday", "national", None, from_easter(1), IT_LAW, None),
            "liberazione": ("Festa della Liberazione", "Liberation Day", "national", None, fixed(4, 25), IT_LAW, None),
            "lavoro": ("Festa del Lavoro", "Labour Day", "national", None, fixed(5, 1), IT_LAW, None),
            "repubblica": ("Festa della Repubblica", "Republic Day", "national", None, fixed(6, 2), IT_LAW, None),
            "ferragosto": ("Ferragosto", "Assumption Day", "national", None, fixed(8, 15), IT_LAW, None),
            "san_francesco": ("San Francesco d'Assisi e Santa Caterina da Siena", "St Francis of Assisi", "national", None, fixed(10, 4), IT_FRAN, 2026),
            "ognissanti": ("Ognissanti", "All Saints' Day", "national", None, fixed(11, 1), IT_LAW, None),
            "immacolata": ("Immacolata Concezione", "Immaculate Conception", "national", None, fixed(12, 8), IT_LAW, None),
            "natale": ("Natale", "Christmas Day", "national", None, fixed(12, 25), IT_LAW, None),
            "santo_stefano": ("Santo Stefano", "St Stephen's Day", "national", None, fixed(12, 26), IT_LAW, None),
        },
    },
    "GB-ENG": {
        "name": "United Kingdom: England and Wales", "language": "en",
        "note": "Bank holidays of England and Wales with substitute days. Scotland and Northern Ireland have different lists. One-off bank holidays (coronations, jubilees, a moved Early May bank holiday) are made by royal proclamation and must be added.",
        "rules": {
            "new_year": ("New Year's Day", "New Year's Day", "national", None, fixed(1, 1), UK_ACT, None),
            "good_friday": ("Good Friday", "Good Friday", "national", None, from_easter(-2), UK_ACT + " (Good Friday is a common-law holiday)", None),
            "easter_monday": ("Easter Monday", "Easter Monday", "national", None, from_easter(1), UK_ACT, None),
            "early_may": ("Early May bank holiday", "Early May bank holiday", "national", None, lambda y: nth_weekday(y, 5, 1, 1), UK_ACT, None),
            "spring": ("Spring bank holiday", "Spring bank holiday", "national", None, lambda y: nth_weekday(y, 5, 1, -1), UK_ACT, None),
            "summer": ("Summer bank holiday", "Summer bank holiday", "national", None, lambda y: nth_weekday(y, 8, 1, -1), UK_ACT, None),
            "christmas": ("Christmas Day", "Christmas Day", "national", None, fixed(12, 25), UK_ACT + " (Christmas Day is a common-law holiday)", None),
            "boxing_day": ("Boxing Day", "Boxing Day", "national", None, fixed(12, 26), UK_ACT, None),
        },
    },
    "US": {
        "name": "United States (federal)", "language": "en",
        "note": "Federal holidays for federal employees, with the observed day when the holiday falls on a weekend. States, cities and private employers set their own lists; Inauguration Day (Washington DC area, every four years) is not included.",
        "rules": {
            "new_year": ("New Year's Day", "New Year's Day", "national", None, fixed(1, 1), US_LAW, None),
            "mlk": ("Birthday of Martin Luther King, Jr.", "Birthday of Martin Luther King, Jr.", "national", None, lambda y: nth_weekday(y, 1, 1, 3), US_LAW + " (third Monday in January)", None),
            "washington": ("Washington's Birthday", "Washington's Birthday", "national", None, lambda y: nth_weekday(y, 2, 1, 3), US_LAW + " (third Monday in February)", None),
            "memorial": ("Memorial Day", "Memorial Day", "national", None, lambda y: nth_weekday(y, 5, 1, -1), US_LAW + " (last Monday in May)", None),
            "juneteenth": ("Juneteenth National Independence Day", "Juneteenth National Independence Day", "national", None, fixed(6, 19), US_LAW, None),
            "independence": ("Independence Day", "Independence Day", "national", None, fixed(7, 4), US_LAW, None),
            "labor": ("Labor Day", "Labor Day", "national", None, lambda y: nth_weekday(y, 9, 1, 1), US_LAW + " (first Monday in September)", None),
            "columbus": ("Columbus Day", "Columbus Day", "national", None, lambda y: nth_weekday(y, 10, 1, 2), US_LAW + " (second Monday in October)", None),
            "veterans": ("Veterans Day", "Veterans Day", "national", None, fixed(11, 11), US_LAW, None),
            "thanksgiving": ("Thanksgiving Day", "Thanksgiving Day", "national", None, lambda y: nth_weekday(y, 11, 4, 4), US_LAW + " (fourth Thursday in November)", None),
            "christmas": ("Christmas Day", "Christmas Day", "national", None, fixed(12, 25), US_LAW, None),
        },
    },
}


def observed_us(day):
    if day.isoweekday() == 6:
        return day - dt.timedelta(days=1)
    if day.isoweekday() == 7:
        return day + dt.timedelta(days=1)
    return None


def substitutes_uk(days):
    """days: {rule: date}. A bank holiday on a weekend gets the next weekday that is not
    already a bank holiday (Christmas Day before Boxing Day)."""
    taken = {d for d in days.values() if d.isoweekday() < 6}
    out = {}
    for rule in sorted(days, key=lambda r: days[r]):
        day = days[rule]
        if day.isoweekday() < 6:
            continue
        sub = day + dt.timedelta(days=1)
        while sub.isoweekday() > 5 or sub in taken:
            sub += dt.timedelta(days=1)
        taken.add(sub)
        out[rule] = sub
    return out


def build(code, country):
    years = {}
    for year in COMPUTED:
        days = {}
        for rule, (_, _, _, _, when, _, first) in country["rules"].items():
            if first is None or year >= first:
                days[rule] = when(year)
        observed = {}
        if code == "US":
            observed = {r: o for r, d in days.items() if (o := observed_us(d))}
        elif code == "GB-ENG":
            observed = substitutes_uk(days)
        years[str(year)] = [[d.isoformat(), observed[r].isoformat() if r in observed else None, r]
                            for r, d in sorted(days.items(), key=lambda kv: kv[1])]
    rules = {rule: {"name": local, "name_en": en, "scope": scope, "regions": regions, "source": source,
                    **({"from_year": first} if first else {})}
             for rule, (local, en, scope, regions, _, source, first) in country["rules"].items()}
    observed_rule = {"US": US_OBS, "GB-ENG": "substitute day: the next weekday that is not already a bank holiday, " + UK_ACT}.get(code)
    return {"name": country["name"], "language": country["language"], "note": country["note"],
            "observed_rule": observed_rule, "rules": rules, "years": years}


def main():
    out = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/administration/holiday_table/holidays.json"
    payload = {"generated": GENERATED, "generator": "tools/catalog/data/holiday_table.py",
               "method": "computed from each rule: fixed dates, Easter offsets (anonymous Gregorian algorithm), nth weekdays and the US and UK substitution rules",
               "years": [YEARS.start, YEARS.stop - 1],
               "entry": ["date", "observed (substitute day, or null)", "rule"],
               "countries": {code: build(code, c) for code, c in COUNTRIES.items()}}
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(compact(payload) + "\n")
    print(f"{out}: {len(COUNTRIES)} countries, {out.stat().st_size} bytes")


def compact(payload):
    """JSON with each year's entries one per line."""
    head = {k: v for k, v in payload.items() if k != "countries"}
    parts = ["{"]
    for k, v in head.items():
        parts.append(f" {json.dumps(k)}: {json.dumps(v, ensure_ascii=False)},")
    parts.append(' "countries": {')
    codes = list(payload["countries"])
    for ci, code in enumerate(codes):
        c = payload["countries"][code]
        parts.append(f"  {json.dumps(code)}: {{")
        for k in ("name", "language", "note", "observed_rule"):
            parts.append(f"   {json.dumps(k)}: {json.dumps(c[k], ensure_ascii=False)},")
        parts.append('   "rules": {')
        rules = list(c["rules"].items())
        for ri, (rule, r) in enumerate(rules):
            parts.append(f"    {json.dumps(rule)}: {json.dumps(r, ensure_ascii=False)}" + ("," if ri < len(rules) - 1 else ""))
        parts.append("   },")
        parts.append('   "years": {')
        years = list(c["years"].items())
        for yi, (year, entries) in enumerate(years):
            parts.append(f"    {json.dumps(year)}: [")
            for ei, e in enumerate(entries):
                parts.append(f"     {json.dumps(e)}" + ("," if ei < len(entries) - 1 else ""))
            parts.append("    ]" + ("," if yi < len(years) - 1 else ""))
        parts.append("   }")
        parts.append("  }" + ("," if ci < len(codes) - 1 else ""))
    parts.append(" }")
    parts.append("}")
    text = "\n".join(parts)
    assert json.loads(text) == json.loads(json.dumps(payload))
    return text


if __name__ == "__main__":
    main()
