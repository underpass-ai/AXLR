#!/usr/bin/env python3
"""Write catalog/tools/language/es_verb_conjugate/irregulares.json: the irregular Spanish verbs
the tool bundles, as the features that set each one apart from the regular -ar/-er/-ir models.

Reference: Real Academia Española and Asociación de Academias de la Lengua Española,
  Nueva gramática de la lengua española (2009), chapter 4 (la conjugación regular e irregular)
  and its appendix of conjugation models; Ortografía de la lengua española (2010) for the
  monosyllables written without accent (rio, riais, hui, guion); and the conjugation tables of
  the Diccionario de la lengua española (https://dle.rae.es/, "Conjugación"). The tables
  below were transcribed by hand on 18 Oct 2026 from those paradigms; the DLE site answers
  scripts with a browser challenge, so nothing here was fetched. No network is used.

Each entry lists only what differs from the regular model; the tool derives every other form:
  stem    vowel change in stressed stems: e>ie, o>ue, u>ue, i>ie (-ar, -er and -ir) and, for
          -ir verbs, e>ie/i (sentir), o>ue/u (dormir), e>i (pedir), where the second vowel
          is also used in the unstressed forms that carry it (sintió, durmamos, pidiendo)
  yo      irregular first person of the present (tengo); the present subjunctive is built
          on its stem (tenga) unless subj_stem says otherwise
  pret    strong preterite stem (tuv-: tuve, tuvo, tuvieron), also the imperfect and future
          subjunctive (tuviera, tuviese, tuviere)
  fut     future and conditional stem (tendr-)
  imp_tu  irregular tú imperative (ten); imp_vos, imp_nosotros likewise
  part, ger   irregular participle and gerund
  accent  stressed weak vowel written with an accent (envío, actúo, reúno, prohíbo)
  c_z     -cer verb with z, not zc, before a and o (cuezo)
  forms   whole tenses given as six forms (yo, tú, él, nosotros, vosotros, ellos)
  vos_pres    voseo present indicative when it is not the infinitive's -ás/-és/-ís
  like    derived verb: the base verb's features with this verb's prefix (mantener < tener)
  note    remarks shown with the result
Spelling changes before e or a/o (c-qu, g-gu, z-c, gu-gü, c-z, g-j, gu-g, qu-c), -zc- after a
vowel in -cer/-cir, y in -uir verbs and between vowels (leyó, cayendo) are rules in the tool,
not entries here.

Usage: python3 tools/catalog/data/es_verb_conjugate.py
"""
import json
import pathlib

GENERATED = "2026-10-18"

STRONG = "pretérito fuerte"

VERBS = {
    # ---- fully or largely irregular
    "ser": {"forms": {"presente": ["soy", "eres", "es", "somos", "sois", "son"],
                      "imperfecto": ["era", "eras", "era", "éramos", "erais", "eran"],
                      "preterito": ["fui", "fuiste", "fue", "fuimos", "fuisteis", "fueron"],
                      "subj_presente": ["sea", "seas", "sea", "seamos", "seáis", "sean"]},
            "vos_pres": "sos", "imp_tu": "sé", "imp_vos": "sé", "model": "ser (irregular)"},
    "estar": {"forms": {"presente": ["estoy", "estás", "está", "estamos", "estáis", "están"],
                        "subj_presente": ["esté", "estés", "esté", "estemos", "estéis", "estén"]},
              "pret": "estuv", "imp_tu": "está", "model": "estar (irregular)"},
    "haber": {"forms": {"presente": ["he", "has", "ha", "hemos", "habéis", "han"],
                        "subj_presente": ["haya", "hayas", "haya", "hayamos", "hayáis", "hayan"]},
              "pret": "hub", "fut": "habr", "vos_pres": "has", "imp_tu": None, "imp_vos": None, "imp_vosotros": None,
              "note": "auxiliary verb; impersonal hay in the present (3rd person singular of the existential use); no imperative in use",
              "model": "haber (irregular)"},
    "ir": {"forms": {"presente": ["voy", "vas", "va", "vamos", "vais", "van"],
                     "imperfecto": ["iba", "ibas", "iba", "íbamos", "ibais", "iban"],
                     "preterito": ["fui", "fuiste", "fue", "fuimos", "fuisteis", "fueron"],
                     "subj_presente": ["vaya", "vayas", "vaya", "vayamos", "vayáis", "vayan"]},
           "vos_pres": "vas", "imp_tu": "ve", "imp_vos": None, "imp_nosotros": "vamos", "ger": "yendo",
           "note": "the voseo imperative of ir is not used; andá (from andar) takes its place", "model": "ir (irregular)"},
    "dar": {"forms": {"presente": ["doy", "das", "da", "damos", "dais", "dan"],
                      "preterito": ["di", "diste", "dio", "dimos", "disteis", "dieron"],
                      "subj_presente": ["dé", "des", "dé", "demos", "deis", "den"]},
            "vos_pres": "das", "imp_vos": "da", "model": "dar (irregular)"},
    "ver": {"forms": {"presente": ["veo", "ves", "ve", "vemos", "veis", "ven"],
                      "imperfecto": ["veía", "veías", "veía", "veíamos", "veíais", "veían"],
                      "preterito": ["vi", "viste", "vio", "vimos", "visteis", "vieron"]},
            "yo": "veo", "vos_pres": "ves", "imp_vos": "ve", "part": "visto", "model": "ver (irregular)"},
    "saber": {"yo": "sé", "subj_stem": "sep", "pret": "sup", "fut": "sabr", "model": "saber (irregular)"},
    "caber": {"yo": "quepo", "pret": "cup", "fut": "cabr", "model": "caber (irregular)"},
    "tener": {"stem": "e>ie", "yo": "tengo", "pret": "tuv", "fut": "tendr", "imp_tu": "ten", "model": "tener (irregular)"},
    "venir": {"stem": "e>ie", "yo": "vengo", "pret": "vin", "fut": "vendr", "imp_tu": "ven", "ger": "viniendo", "model": "venir (irregular)"},
    "poner": {"yo": "pongo", "pret": "pus", "fut": "pondr", "imp_tu": "pon", "part": "puesto", "model": "poner (irregular)"},
    "hacer": {"yo": "hago", "pret": "hic", "fut": "har", "imp_tu": "haz", "part": "hecho", "model": "hacer (irregular)"},
    "decir": {"stem": "e>i", "yo": "digo", "pret": "dij", "fut": "dir", "imp_tu": "di", "part": "dicho", "model": "decir (irregular)"},
    "poder": {"stem": "o>ue", "pret": "pud", "fut": "podr", "ger": "pudiendo", "model": "poder (irregular)"},
    "querer": {"stem": "e>ie", "pret": "quis", "fut": "querr", "model": "querer (irregular)"},
    "salir": {"yo": "salgo", "fut": "saldr", "imp_tu": "sal", "model": "salir (irregular)"},
    "valer": {"yo": "valgo", "fut": "valdr", "model": "valer (irregular)"},
    "traer": {"yo": "traigo", "pret": "traj", "model": "traer (irregular)"},
    "caer": {"yo": "caigo", "model": "caer (irregular)"},
    "oír": {"forms": {"presente": ["oigo", "oyes", "oye", "oímos", "oís", "oyen"]}, "yo": "oigo", "model": "oír (irregular)"},
    "andar": {"pret": "anduv", "model": "andar (" + STRONG + ")"},
    "conducir": {"pret": "conduj", "model": "conducir (-zc- and " + STRONG + ")"},
    "producir": {"like": "conducir"},
    "traducir": {"like": "conducir"},
    "reducir": {"like": "conducir"},
    "introducir": {"like": "conducir"},
    "deducir": {"like": "conducir"},
    "satisfacer": {"yo": "satisfago", "pret": "satisfic", "fut": "satisfar", "imp_tu": "satisfaz", "part": "satisfecho",
                   "note": "tú imperative also satisface", "model": "hacer (irregular)"},
    "reír": {"forms": {"presente": ["río", "ríes", "ríe", "reímos", "reís", "ríen"],
                       "subj_presente": ["ría", "rías", "ría", "riamos", "riais", "rían"],
                       "preterito": ["reí", "reíste", "rio", "reímos", "reísteis", "rieron"]},
             "ger": "riendo", "model": "reír (e>i, hiatus)",
             "note": "rio and riais are written without accent (Ortografía 2010: monosyllables)"},
    "sonreír": {"forms": {"presente": ["sonrío", "sonríes", "sonríe", "sonreímos", "sonreís", "sonríen"],
                          "subj_presente": ["sonría", "sonrías", "sonría", "sonriamos", "sonriáis", "sonrían"],
                          "preterito": ["sonreí", "sonreíste", "sonrió", "sonreímos", "sonreísteis", "sonrieron"]},
                "ger": "sonriendo", "model": "reír (e>i, hiatus)"},
    "freír": {"forms": {"presente": ["frío", "fríes", "fríe", "freímos", "freís", "fríen"],
                        "subj_presente": ["fría", "frías", "fría", "friamos", "friais", "frían"],
                        "preterito": ["freí", "freíste", "frio", "freímos", "freísteis", "frieron"]},
              "ger": "friendo", "part": "frito", "part_alt": "freído", "model": "reír (e>i, hiatus)"},
    "huir": {"forms": {"presente": ["huyo", "huyes", "huye", "huimos", "huis", "huyen"],
                       "preterito": ["hui", "huiste", "huyó", "huimos", "huisteis", "huyeron"]},
             "vos_pres": "huis", "imp_vos": "hui", "model": "construir (-uir: y)",
             "note": "hui, huis are written without accent (Ortografía 2010: monosyllables)"},
    # ---- derived verbs: base features with the prefix; monosyllabic imperatives gain an accent
    "mantener": {"like": "tener", "imp_tu": "mantén"},
    "obtener": {"like": "tener", "imp_tu": "obtén"},
    "contener": {"like": "tener", "imp_tu": "contén"},
    "detener": {"like": "tener", "imp_tu": "detén"},
    "sostener": {"like": "tener", "imp_tu": "sostén"},
    "retener": {"like": "tener", "imp_tu": "retén"},
    "componer": {"like": "poner", "imp_tu": "compón"},
    "proponer": {"like": "poner", "imp_tu": "propón"},
    "suponer": {"like": "poner", "imp_tu": "supón"},
    "disponer": {"like": "poner", "imp_tu": "dispón"},
    "exponer": {"like": "poner", "imp_tu": "expón"},
    "imponer": {"like": "poner", "imp_tu": "impón"},
    "oponer": {"like": "poner", "imp_tu": "opón"},
    "convenir": {"like": "venir", "imp_tu": "convén"},
    "prevenir": {"like": "venir", "imp_tu": "prevén"},
    "intervenir": {"like": "venir", "imp_tu": "intervén"},
    "deshacer": {"like": "hacer"},
    "atraer": {"like": "traer"},
    "distraer": {"like": "traer"},
    "contraer": {"like": "traer"},
    "extraer": {"like": "traer"},
    "sobresalir": {"like": "salir"},
    # ---- e > ie
    "pensar": {"stem": "e>ie"}, "empezar": {"stem": "e>ie"}, "comenzar": {"stem": "e>ie"}, "cerrar": {"stem": "e>ie"},
    "despertar": {"stem": "e>ie"}, "sentar": {"stem": "e>ie"}, "recomendar": {"stem": "e>ie"}, "negar": {"stem": "e>ie"},
    "calentar": {"stem": "e>ie"}, "gobernar": {"stem": "e>ie"}, "atravesar": {"stem": "e>ie"}, "nevar": {"stem": "e>ie", "note": "impersonal (weather)"},
    "entender": {"stem": "e>ie"}, "perder": {"stem": "e>ie"}, "defender": {"stem": "e>ie"}, "encender": {"stem": "e>ie"},
    "atender": {"stem": "e>ie"}, "extender": {"stem": "e>ie"},
    # ---- o > ue, u > ue
    "contar": {"stem": "o>ue"}, "encontrar": {"stem": "o>ue"}, "recordar": {"stem": "o>ue"}, "mostrar": {"stem": "o>ue"},
    "probar": {"stem": "o>ue"}, "costar": {"stem": "o>ue"}, "volar": {"stem": "o>ue"}, "soñar": {"stem": "o>ue"},
    "almorzar": {"stem": "o>ue"}, "acostar": {"stem": "o>ue"}, "colgar": {"stem": "o>ue"}, "aprobar": {"stem": "o>ue"},
    "sonar": {"stem": "o>ue"}, "rogar": {"stem": "o>ue"},
    "jugar": {"stem": "u>ue"},
    "volver": {"stem": "o>ue", "part": "vuelto"}, "devolver": {"stem": "o>ue", "part": "devuelto"},
    "envolver": {"stem": "o>ue", "part": "envuelto"}, "resolver": {"stem": "o>ue", "part": "resuelto"},
    "mover": {"stem": "o>ue"}, "llover": {"stem": "o>ue", "note": "impersonal (weather)"}, "doler": {"stem": "o>ue"},
    "oler": {"stem": "o>ue", "note": "ue at the start of a word is written hue (huelo)"}, "soler": {"stem": "o>ue", "note": "defective: no future, conditional or imperative in use"},
    "torcer": {"stem": "o>ue"}, "cocer": {"stem": "o>ue", "c_z": True}, "mecer": {"c_z": True},
    # ---- -ir: e > ie / i, o > ue / u, e > i, i > ie
    "sentir": {"stem": "e>ie/i"}, "preferir": {"stem": "e>ie/i"}, "mentir": {"stem": "e>ie/i"}, "convertir": {"stem": "e>ie/i"},
    "divertir": {"stem": "e>ie/i"}, "sugerir": {"stem": "e>ie/i"}, "advertir": {"stem": "e>ie/i"}, "herir": {"stem": "e>ie/i"},
    "requerir": {"stem": "e>ie/i"}, "referir": {"stem": "e>ie/i"}, "consentir": {"stem": "e>ie/i"}, "hervir": {"stem": "e>ie/i"},
    "dormir": {"stem": "o>ue/u"}, "morir": {"stem": "o>ue/u", "part": "muerto"},
    "pedir": {"stem": "e>i"}, "servir": {"stem": "e>i"}, "repetir": {"stem": "e>i"}, "seguir": {"stem": "e>i"},
    "conseguir": {"stem": "e>i"}, "perseguir": {"stem": "e>i"}, "vestir": {"stem": "e>i"}, "medir": {"stem": "e>i"},
    "elegir": {"stem": "e>i"}, "corregir": {"stem": "e>i"}, "despedir": {"stem": "e>i"}, "impedir": {"stem": "e>i"},
    "competir": {"stem": "e>i"}, "reñir": {"stem": "e>i"}, "teñir": {"stem": "e>i"},
    "adquirir": {"stem": "i>ie"},
    # ---- -uir (y) and vowel stems (y between vowels): rules in the tool, listed for reference
    "construir": {"model": "construir (-uir: y)"}, "destruir": {"model": "construir (-uir: y)"},
    "incluir": {"model": "construir (-uir: y)"}, "concluir": {"model": "construir (-uir: y)"},
    "contribuir": {"model": "construir (-uir: y)"}, "distribuir": {"model": "construir (-uir: y)"},
    "leer": {"model": "leer (y between vowels)"}, "creer": {"model": "leer (y between vowels)"},
    "poseer": {"model": "leer (y between vowels)"},
    "proveer": {"model": "leer (y between vowels)", "part": "provisto", "part_alt": "proveído"},
    # ---- -zc- (rule in the tool)
    "conocer": {"model": "agradecer (-zc-)"}, "reconocer": {"model": "agradecer (-zc-)"},
    "parecer": {"model": "agradecer (-zc-)"}, "aparecer": {"model": "agradecer (-zc-)"},
    "ofrecer": {"model": "agradecer (-zc-)"}, "nacer": {"model": "agradecer (-zc-)"},
    "crecer": {"model": "agradecer (-zc-)"}, "agradecer": {"model": "agradecer (-zc-)"},
    "merecer": {"model": "agradecer (-zc-)"}, "establecer": {"model": "agradecer (-zc-)"},
    "pertenecer": {"model": "agradecer (-zc-)"}, "lucir": {"model": "lucir (-zc-)"},
    # ---- written accent on the stressed weak vowel
    "enviar": {"accent": True}, "confiar": {"accent": True}, "variar": {"accent": True}, "enfriar": {"accent": True},
    "actuar": {"accent": True}, "continuar": {"accent": True}, "graduar": {"accent": True}, "situar": {"accent": True},
    "evaluar": {"accent": True}, "reunir": {"accent": True}, "prohibir": {"accent": True}, "aislar": {"accent": True},
    # ---- irregular participles only
    "escribir": {"part": "escrito"}, "describir": {"part": "descrito"}, "inscribir": {"part": "inscrito"},
    "abrir": {"part": "abierto"}, "cubrir": {"part": "cubierto"}, "descubrir": {"part": "descubierto"},
    "romper": {"part": "roto"}, "imprimir": {"part": "impreso", "part_alt": "imprimido"},
}


def main():
    out = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/language/es_verb_conjugate/irregulares.json"
    payload = {
        "generated": GENERATED,
        "generator": "tools/catalog/data/es_verb_conjugate.py",
        "source": "RAE and ASALE, Nueva gramática de la lengua española (2009), ch. 4 and appendix of conjugation models; "
                  "Ortografía de la lengua española (2010); DLE conjugation tables (https://dle.rae.es/). Transcribed by hand.",
        "persons": ["yo", "tú", "él", "nosotros", "vosotros", "ellos"],
        "verbs": dict(sorted(VERBS.items())),
    }
    out.write_text(json.dumps(payload, indent=1, ensure_ascii=False) + "\n")
    print(f"{len(VERBS)} verbs -> {out}")


if __name__ == "__main__":
    main()
