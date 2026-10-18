#!/usr/bin/env python3
"""Lesson plan skeleton: stages with minutes allocated in proportion to weights (largest
remainder, so they add up to the duration) and objectives written with verbs of the revised
Bloom taxonomy (Anderson & Krathwohl 2001), in English or Spanish.

Input (JSON on stdin):
  subject           string (required)
  topic             string, optional
  level             "primary" | "secondary" | "upper_secondary" | "university" | "adult"   default secondary
  duration_minutes  integer 10-480 (required)
  language          "en" | "es"   default en
  objectives        optional [{"level": "analyze", "content": "..."}] or strings with a Bloom level prefix
  objective_count   when objectives are absent: how many to draft (1-6, default 3)
  stage_weights     optional {"warm_up": 10, "presentation": 25, "practice": 25, "production": 20,
                    "assessment": 10, "closure": 10} (the default); a stage with weight 0 is left out
"""
import json
import sys
from fractions import Fraction

LEVELS = ["remember", "understand", "apply", "analyze", "evaluate", "create"]
LEVEL_NAMES = {"en": ["Remember", "Understand", "Apply", "Analyze", "Evaluate", "Create"],
               "es": ["Recordar", "Comprender", "Aplicar", "Analizar", "Evaluar", "Crear"]}
# Verbs per level: the taxonomy's own process verbs first (Anderson & Krathwohl 2001, Table 5.1),
# then common objective verbs; compiled for this tool.
VERBS = {
    "en": {"remember": ["identify", "recall", "list", "define", "name"],
           "understand": ["explain", "summarize", "classify", "compare", "interpret"],
           "apply": ["use", "solve", "implement", "calculate", "demonstrate"],
           "analyze": ["differentiate", "organize", "distinguish", "examine", "outline"],
           "evaluate": ["judge", "critique", "justify", "assess", "check"],
           "create": ["design", "produce", "plan", "compose", "generate"]},
    "es": {"remember": ["identificar", "recordar", "enumerar", "definir", "nombrar"],
           "understand": ["explicar", "resumir", "clasificar", "comparar", "interpretar"],
           "apply": ["usar", "resolver", "implementar", "calcular", "demostrar"],
           "analyze": ["diferenciar", "organizar", "distinguir", "examinar", "esquematizar"],
           "evaluate": ["juzgar", "criticar", "justificar", "valorar", "comprobar"],
           "create": ["diseñar", "producir", "planificar", "componer", "generar"]},
}
DEFAULT_LEVELS = {"primary": ["remember", "understand", "apply"], "secondary": ["understand", "apply", "analyze"],
                  "upper_secondary": ["apply", "analyze", "evaluate"], "university": ["analyze", "evaluate", "create"],
                  "adult": ["apply", "analyze", "create"]}
EXTRA_LEVELS = ["remember", "understand", "apply", "analyze", "evaluate", "create"]
STAGES = ["warm_up", "presentation", "practice", "production", "assessment", "closure"]
DEFAULT_WEIGHTS = {"warm_up": 10, "presentation": 25, "practice": 25, "production": 20, "assessment": 10, "closure": 10}
TEXT = {
    "en": {
        "titles": {"warm_up": "Warm-up", "presentation": "Presentation", "practice": "Controlled practice",
                   "production": "Production", "assessment": "Assessment", "closure": "Closure"},
        "activities": {
            "warm_up": "Activate prior knowledge of {topic}: a question, image or short quiz; share the objectives.",
            "presentation": "Introduce the new content on {topic} with examples; check understanding with questions.",
            "practice": "Guided exercises with feedback, individually or in pairs.",
            "production": "Open task where students apply {topic} with more autonomy (project, problem, discussion).",
            "assessment": "Check the objectives ({objectives}): exit ticket, short quiz or observation with a rubric.",
            "closure": "Summarise the key points, answer questions and preview the next lesson."},
        "objective": "By the end of the lesson, students will be able to {verb} {content}.",
        "heading": "Lesson plan", "level": "Level", "duration": "Duration", "objectives": "Objectives",
        "sequence": "Sequence", "stage": "Stage", "minutes": "Minutes", "time": "Time", "activities_h": "Activities",
        "materials": "Materials", "materials_text": "[list materials and resources]", "content": "[content]", "about": "[content about {topic}]",
        "levels": {"primary": "Primary", "secondary": "Secondary", "upper_secondary": "Upper secondary",
                   "university": "University", "adult": "Adult education"},
    },
    "es": {
        "titles": {"warm_up": "Activación", "presentation": "Presentación", "practice": "Práctica guiada",
                   "production": "Producción", "assessment": "Evaluación", "closure": "Cierre"},
        "activities": {
            "warm_up": "Activar conocimientos previos sobre {topic}: una pregunta, imagen o cuestionario breve; compartir los objetivos.",
            "presentation": "Presentar el contenido nuevo sobre {topic} con ejemplos; comprobar la comprensión con preguntas.",
            "practice": "Ejercicios guiados con retroalimentación, individualmente o por parejas.",
            "production": "Tarea abierta en la que el alumnado aplica {topic} con más autonomía (proyecto, problema, debate).",
            "assessment": "Comprobar los objetivos ({objectives}): ticket de salida, cuestionario breve u observación con rúbrica.",
            "closure": "Resumir las ideas clave, resolver dudas y anticipar la próxima sesión."},
        "objective": "Al final de la sesión, el alumnado será capaz de {verb} {content}.",
        "heading": "Plan de clase", "level": "Nivel", "duration": "Duración", "objectives": "Objetivos",
        "sequence": "Secuencia", "stage": "Fase", "minutes": "Minutos", "time": "Tiempo", "activities_h": "Actividades",
        "materials": "Materiales", "materials_text": "[enumerar materiales y recursos]", "content": "[contenido]", "about": "[contenido sobre {topic}]",
        "levels": {"primary": "Primaria", "secondary": "Secundaria", "upper_secondary": "Bachillerato",
                   "university": "Universidad", "adult": "Educación de adultos"},
    },
}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def allocate(duration, weights):
    """Largest-remainder (Hamilton) allocation of whole minutes; ties go to the earlier stage."""
    total = sum(weights.values())
    exact = {s: Fraction(duration) * w / total for s, w in weights.items()}
    minutes = {s: int(v) for s, v in exact.items()}
    left = duration - sum(minutes.values())
    order = sorted(weights, key=lambda s: (-(exact[s] - minutes[s]), STAGES.index(s)))
    for s in order[:left]:
        minutes[s] += 1
    return minutes, exact


def clean(value, field, required=False, limit=200):
    if value is None and not required:
        return None
    if not isinstance(value, str) or not value.strip() or len(value) > limit:
        fail(f"{field} must be a non-empty string of at most {limit} characters")
    return value.strip()


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"subject", "topic", "level", "duration_minutes", "language", "objectives",
                                   "objective_count", "stage_weights"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    subject = clean(request.get("subject"), "subject", required=True)
    topic = clean(request.get("topic"), "topic")
    level = request.get("level", "secondary")
    if not isinstance(level, str) or level not in DEFAULT_LEVELS:
        fail(f"level must be one of {', '.join(DEFAULT_LEVELS)}")
    language = request.get("language", "en")
    if not isinstance(language, str) or language not in TEXT:
        fail("language must be en or es")
    t = TEXT[language]
    duration = request.get("duration_minutes")
    if isinstance(duration, bool) or not isinstance(duration, int) or not 10 <= duration <= 480:
        fail("duration_minutes must be an integer from 10 to 480")
    weights = request.get("stage_weights", DEFAULT_WEIGHTS)
    if not isinstance(weights, dict) or set(weights) - set(STAGES):
        fail(f"stage_weights keys must be among {', '.join(STAGES)}")
    for s, w in weights.items():
        if isinstance(w, bool) or not isinstance(w, (int, float)) or w < 0:
            fail(f"stage_weights.{s} must be a number zero or more")
    weights = {s: Fraction(str(weights[s])) for s in STAGES if s in weights and weights[s] > 0}
    if not weights:
        fail("stage_weights must give at least one stage a positive weight")

    raw = request.get("objectives")
    specs = []
    if raw is not None:
        if not isinstance(raw, list) or not 1 <= len(raw) <= 12:
            fail("objectives must be a list of 1 to 12 items")
        for i, item in enumerate(raw):
            if not isinstance(item, dict) or set(item) - {"level", "content", "verb"}:
                fail(f"objectives[{i}] must be an object with level, content and optional verb")
            lv = item.get("level")
            if lv not in LEVELS:
                fail(f"objectives[{i}].level must be one of {', '.join(LEVELS)}")
            specs.append((lv, clean(item.get("content"), f"objectives[{i}].content"),
                          clean(item.get("verb"), f"objectives[{i}].verb", limit=40)))
    else:
        count = request.get("objective_count", 3)
        if isinstance(count, bool) or not isinstance(count, int) or not 1 <= count <= 6:
            fail("objective_count must be an integer from 1 to 6")
        base = DEFAULT_LEVELS[level]
        chosen = (base + [lv for lv in EXTRA_LEVELS if lv not in base])[:count]
        draft = t["about"].format(topic=topic) if topic else None
        specs = [(lv, draft, None) for lv in sorted(chosen, key=LEVELS.index)]
    used = {lv: 0 for lv in LEVELS}
    objectives = []
    for lv, content, verb in specs:
        if verb is None:
            verb = VERBS[language][lv][used[lv] % len(VERBS[language][lv])]
            used[lv] += 1
        objectives.append({"level": lv, "level_name": LEVEL_NAMES[language][LEVELS.index(lv)], "verb": verb,
                           "text": t["objective"].format(verb=verb, content=content or t["content"])})

    minutes, exact = allocate(duration, weights)
    topic_text = topic or subject
    numbers = ", ".join(str(i + 1) for i in range(len(objectives)))
    stages, clock = [], 0
    for s in STAGES:
        if s not in weights:
            continue
        stages.append({"stage": s, "title": t["titles"][s], "minutes": minutes[s], "start_minute": clock,
                       "end_minute": clock + minutes[s], "exact_share": str(exact[s]),
                       "activities": t["activities"][s].format(topic=topic_text, objectives=numbers)})
        clock += minutes[s]

    lines = [f"# {t['heading']}: {subject}" + (f" — {topic}" if topic else ""), "",
             f"**{t['level']}:** {t['levels'][level]}  ", f"**{t['duration']}:** {duration} min", "",
             f"## {t['objectives']}", ""]
    lines += [f"{i + 1}. {o['text']} *({o['level_name']})*" for i, o in enumerate(objectives)]
    lines += ["", f"## {t['sequence']}", "", f"| {t['stage']} | {t['minutes']} | {t['time']} | {t['activities_h']} |",
              "|:--|--:|:--|:--|"]
    lines += [f"| {s['title']} | {s['minutes']} | {s['start_minute']}–{s['end_minute']} | {s['activities']} |" for s in stages]
    lines += ["", f"## {t['materials']}", "", t["materials_text"], ""]
    print(json.dumps({
        "subject": subject, "topic": topic, "level": level, "language": language, "duration_minutes": duration,
        "objectives": objectives, "stages": stages, "markdown": "\n".join(lines),
        "allocation": "largest remainder over the stage weights; ties to the earlier stage",
        "source": "Bloom verbs after the revised taxonomy (Anderson & Krathwohl 2001); stages after the "
                  "presentation-practice-production lesson model",
    }, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
