#!/usr/bin/env python3
"""Render reviewable 100x32 ANSI concept screens; no AXLR runtime state is changed."""

from pathlib import Path

WIDTH, HEIGHT = 100, 32
OUT = Path(__file__).with_name("assets")

THEMES = {
    "ink": dict(bg="#11131C", surface="#1B2030", raised="#252C40", user="#203C48", memory="#342B4D", fg="#E8EDF7", muted="#9BA6BC", accent="#7DD3FC", select="#364765", good="#6EE7B7", warn="#FBBF72", border="#526078"),
    "aurora": dict(bg="#091D29", surface="#112B38", raised="#173B48", user="#174B50", memory="#25465E", fg="#E4F9F4", muted="#A7C7C6", accent="#5EEAD4", select="#22636A", good="#A3E635", warn="#F9C66B", border="#32717C"),
    "paper": dict(bg="#F7F3E9", surface="#FFFDF7", raised="#EBE4D8", user="#E3EFE9", memory="#EEE7F2", fg="#263342", muted="#627083", accent="#315AA7", select="#CEDDFA", good="#187B61", warn="#A9542C", border="#A7B1BA"),
    "phosphor": dict(bg="#08120B", surface="#102015", raised="#18321D", user="#16321A", memory="#26321B", fg="#D8F2CE", muted="#83A984", accent="#87E66C", select="#23502A", good="#B7F173", warn="#E4C66D", border="#477D4B"),
}


class Screen:
    def __init__(self, theme):
        self.t = THEMES[theme]
        self.cells = [[(" ", self.t["fg"], self.t["bg"], False) for _ in range(WIDTH)] for _ in range(HEIGHT)]

    def fill(self, x, y, w, h, color):
        for yy in range(max(0, y), min(HEIGHT, y + h)):
            for xx in range(max(0, x), min(WIDTH, x + w)):
                ch, fg, _, bold = self.cells[yy][xx]
                self.cells[yy][xx] = (ch, fg, self.t[color], bold)

    def put(self, x, y, text, fg="fg", bg=None, bold=False):
        for i, ch in enumerate(text):
            xx = x + i
            if 0 <= y < HEIGHT and 0 <= xx < WIDTH:
                _, _, old_bg, _ = self.cells[y][xx]
                self.cells[y][xx] = (ch, self.t[fg], self.t[bg] if bg else old_bg, bold)

    def rule(self, x, y, w, fg="border"):
        self.put(x, y, "─" * w, fg)

    def box(self, x, y, w, h, surface="surface", border="border"):
        self.fill(x, y, w, h, surface)
        self.put(x, y, "╭" + "─" * (w - 2) + "╮", border)
        for yy in range(y + 1, y + h - 1):
            self.put(x, yy, "│", border)
            self.put(x + w - 1, yy, "│", border)
        self.put(x, y + h - 1, "╰" + "─" * (w - 2) + "╯", border)

    def render(self):
        lines = []
        for row in self.cells:
            parts, prev = [], None
            for ch, fg, bg, bold in row:
                style = (fg, bg, bold)
                if style != prev:
                    r, g, b = (int(fg[i:i+2], 16) for i in (1, 3, 5))
                    br, bg_, bb = (int(bg[i:i+2], 16) for i in (1, 3, 5))
                    parts.append(f"\x1b[0;{'1;' if bold else ''}38;2;{r};{g};{b};48;2;{br};{bg_};{bb}m")
                    prev = style
                parts.append(ch)
            lines.append("".join(parts) + "\x1b[0m")
        return "\n".join(lines) + "\n"


def chrome(s, title, crumb):
    s.fill(0, 0, WIDTH, 3, "surface")
    s.put(2, 0, "◆ AXLR", "accent", bold=True)
    s.put(11, 0, title, bold=True)
    s.put(78, 0, "● Connected", "good")
    s.put(2, 1, crumb, "muted")
    s.rule(0, 2, WIDTH)


def footer(s, left, right="F1 help  ·  Esc close"):
    s.rule(0, 30, WIDTH)
    s.fill(0, 31, WIDTH, 1, "surface")
    s.put(2, 31, left, "muted")
    s.put(WIDTH - len(right) - 2, 31, right, "muted")


def main(theme):
    s = Screen(theme)
    chrome(s, "Console", "workspace  /underpass-runtime       session  14  ·  openai/gpt-4.1")
    s.put(2, 3, "TRANSCRIPT", "accent", bold=True)
    s.put(19, 3, "ACTIVITY", "muted")
    s.put(85, 3, "Ctrl+P Actions", "muted")
    s.put(2, 5, "TODAY  ·  15:42", "muted")
    s.box(2, 7, 96, 5, "user")
    s.put(5, 8, "YOU", "accent", bold=True)
    s.put(5, 9, "Revisa la sesión y dime qué ha cambiado en la política de contexto.")
    s.box(2, 13, 96, 8, "surface")
    s.put(5, 14, "AXLR", "accent", bold=True)
    s.put(5, 16, "El historial completo sigue guardado. La petición al modelo usa una proyección")
    s.put(5, 17, "acotada y permite recuperar originales por páginas cuando hacen falta.")
    s.put(5, 19, "Ver origen  ↗", "muted")
    s.fill(4, 21, 92, 2, "memory")
    s.put(6, 21, "◇ MEMORIA  kmp_wake", "accent", bold=True)
    s.put(28, 21, "project:AXLR  ·  16 herramientas  ·  0,46 s", "muted")
    s.put(4, 23, "◌ Preparando herramientas", "warn", bold=True)
    s.put(31, 23, "kmp_wake  ·  actividad real  ·  00:02", "muted")
    s.box(2, 25, 96, 5, "raised", "accent")
    s.put(5, 25, " MENSAJE  ·  borrador guardado ", "accent", bold=True)
    s.put(5, 27, "Pregunta por los cambios de hoy, o usa /model, /mcp, /plugin…", "muted")
    s.put(5, 28, "Enter enviar    Shift+Enter nueva línea", "muted")
    s.put(80, 28, "[ Enviar ↗ ]", "accent", bold=True)
    footer(s, "● Listo  ·  historial privado intacto", "Tab actividad  ·  F1 ayuda")
    return s


def model():
    s = Screen("ink")
    chrome(s, "/model", "Elegir modelo predeterminado  ·  selección aplicada a futuras peticiones")
    s.box(2, 4, 96, 3, "raised")
    s.put(5, 5, "⌕ Buscar por nombre, proveedor o ID…", "muted")
    s.put(70, 5, "[ Todos ] [ Herramientas ✓ ]", "accent")
    s.put(3, 8, "MODELOS  428", "muted", bold=True)
    s.put(63, 8, "DETALLE", "muted", bold=True)
    s.box(2, 9, 56, 19)
    s.box(60, 9, 38, 19, "raised")
    rows = [
        ("Claude Sonnet 4.6", "anthropic/claude-sonnet-4.6   1M ctx", True),
        ("GPT-4.1", "openai/gpt-4.1               1M ctx", False),
        ("Gemini 2.5 Pro", "google/gemini-2.5-pro        1M ctx", False),
        ("Aion 3.5", "aion-labs/aion-3.5        262k ctx", False),
    ]
    for n, (name, detail, selected) in enumerate(rows):
        y = 11 + n * 4
        if selected:
            s.fill(4, y - 1, 52, 3, "select")
        s.put(5, y, ("› " if selected else "  ") + name, "accent" if selected else "fg", bold=selected)
        s.put(7, y + 1, detail, "muted")
    s.put(63, 11, "Claude Sonnet 4.6", "accent", bold=True)
    s.put(63, 13, "anthropic/claude-sonnet-4.6", "muted")
    s.rule(63, 15, 31)
    s.put(63, 17, "Contexto       1.000.000 tokens")
    s.put(63, 18, "Entrada        $3 / 1M tokens")
    s.put(63, 19, "Salida         $15 / 1M tokens")
    s.put(63, 20, "Herramientas   Sí", "good")
    s.fill(63, 24, 32, 2, "select")
    s.put(65, 24, "Enter  Usar como predeterminado", "accent", bold=True)
    footer(s, "↑↓ mover  ·  / buscar  ·  Tab detalle", "Esc volver")
    return s


def mcp():
    s = Screen("ink")
    chrome(s, "/mcp", "Servidores MCP externos  ·  inventario y salud")
    s.box(2, 4, 96, 3, "raised")
    s.put(5, 5, "⌕ Buscar servidor o herramienta…", "muted")
    s.put(69, 5, "[ Todos ] [ Con error ]", "accent")
    s.put(3, 8, "SERVIDORES  2", "muted", bold=True)
    s.put(44, 8, "HERRAMIENTAS  ·  KMP", "muted", bold=True)
    s.box(2, 9, 38, 19)
    s.box(42, 9, 56, 19, "raised")
    s.fill(4, 11, 34, 5, "select")
    s.put(6, 12, "› ● KMP", "good", bold=True)
    s.put(8, 13, "Memoria  ·  16 herramientas", "muted")
    s.put(8, 14, "Conectado  ·  aprobación auto", "muted")
    s.put(6, 18, "  ● MADE", "good", bold=True)
    s.put(8, 19, "Ceremonias  ·  31 herramientas", "muted")
    s.put(8, 20, "Conectado  ·  aprobación auto", "muted")
    s.put(45, 11, "KMP", "accent", bold=True)
    s.put(45, 12, "Memoria grafo temporal de Underpass", "muted")
    s.rule(45, 14, 49)
    for i, name in enumerate(["kmp_wake        Recuperar contexto", "kmp_ask         Consultar evidencia", "kmp_inspect     Abrir una referencia", "kmp_trace       Seguir la prueba", "kmp_write_memory Guardar una decisión"]):
        s.put(46, 16 + i * 2, "◇ " + name, "fg" if i else "accent")
    s.put(45, 26, "R actualizar  ·  Enter ver herramienta", "muted")
    footer(s, "↑↓ servidor  ·  / buscar  ·  Tab herramientas", "Esc volver")
    return s


def plugin():
    s = Screen("ink")
    chrome(s, "/plugin", "Plugins registrados  ·  permisos por identidad exacta")
    s.box(2, 4, 96, 3, "raised")
    s.put(5, 5, "⌕ Buscar plugin…", "muted")
    s.put(70, 5, "[ Todos ] [ Manual ] [ Auto ]", "accent")
    s.put(3, 8, "PLUGINS  2", "muted", bold=True)
    s.put(44, 8, "PERMISOS  ·  KMP", "muted", bold=True)
    s.box(2, 9, 38, 19)
    s.box(42, 9, 56, 19, "raised")
    s.fill(4, 11, 34, 5, "select")
    s.put(6, 12, "› ◇ KMP", "accent", bold=True)
    s.put(8, 13, "Memoria  ·  auto", "good")
    s.put(8, 14, "16 herramientas permitidas", "muted")
    s.put(6, 18, "  ◆ MADE", "fg", bold=True)
    s.put(8, 19, "Ceremonias  ·  auto", "good")
    s.put(8, 20, "31 herramientas permitidas", "muted")
    s.put(45, 11, "KMP  ·  memoria", "accent", bold=True)
    s.put(45, 13, "Identidad exacta   kmp", "muted")
    s.put(45, 14, "Manifiesto        16 herramientas", "muted")
    s.rule(45, 16, 49)
    s.put(45, 18, "Política actual", "muted")
    s.fill(45, 20, 23, 3, "select")
    s.put(47, 21, "● Automática", "good", bold=True)
    s.put(70, 21, "○ Manual", "muted")
    s.put(45, 24, "Solo herramientas permitidas del plugin.", "muted")
    s.put(45, 25, "Cambiar exige confirmar el alcance.", "warn")
    footer(s, "↑↓ plugin  ·  / buscar  ·  Enter cambiar política", "Esc volver")
    return s


def theme_picker():
    s = Screen("ink")
    chrome(s, "/theme", "Vista previa temporal  ·  Enter guardar  ·  Esc restaurar")
    s.put(3, 5, "APARIENCIA", "muted", bold=True)
    s.box(2, 6, 39, 21)
    options = [
        ("Ink", "Noche equilibrada", True),
        ("Aurora", "Cian y verde sobre azul", False),
        ("Paper", "Lectura clara", False),
        ("Phosphor", "Verde compacto", False),
    ]
    for i, (name, detail, selected) in enumerate(options):
        y = 8 + i * 4
        if selected:
            s.fill(4, y - 1, 35, 3, "select")
        s.put(6, y, ("› " if selected else "  ") + name, "accent" if selected else "fg", bold=selected)
        s.put(8, y + 1, detail, "muted")
    s.put(4, 24, "Iconos: Seguro  ·  Nerd Mono  ·  ASCII", "muted")
    s.put(44, 5, "PREVISUALIZACIÓN  ·  INK", "muted", bold=True)
    s.box(43, 6, 55, 21, "raised")
    s.fill(46, 9, 49, 4, "user")
    s.put(48, 10, "YOU", "accent", bold=True)
    s.put(48, 11, "Muéstrame las decisiones de hoy")
    s.fill(46, 14, 49, 6, "surface")
    s.put(48, 15, "AXLR", "accent", bold=True)
    s.put(48, 17, "He recuperado project:AXLR.")
    s.fill(46, 21, 49, 2, "memory")
    s.put(48, 21, "◇ MEMORIA  kmp_wake  ·  correcto", "good")
    s.put(46, 25, "● Conectado    ⚠ Aviso    › Selección", "muted")
    footer(s, "↑↓ probar  ·  Tab perfil de iconos", "Enter guardar  ·  Esc restaurar")
    return s


def activity():
    s = Screen("ink")
    chrome(s, "Estados de actividad", "Storyboard  ·  cada estado empieza y acaba con un evento real")
    states = [
        ("1  CONEXIÓN", "EventStreamStart", "◌  Conectando con el modelo  ·  00:01", "warn"),
        ("2  RAZONAMIENTO", "ProviderReasoning", "✦  Modelo razonando  ·  00:06", "accent"),
        ("3  PREPARACIÓN", "ProviderToolCall", "◇  Preparando herramientas  ·  00:02", "accent"),
        ("4  EJECUCIÓN", "EventToolExecutionStarted", "●  Ejecutando kmp_wake  ·  00:04", "good"),
    ]
    for i, (title, event, label, color) in enumerate(states):
        y = 4 + i * 6
        s.box(2, y, 96, 5, "surface")
        s.put(5, y + 1, title, color, bold=True)
        s.put(43, y + 1, event, "muted")
        s.put(5, y + 3, label, color)
        s.put(75, y + 3, "Esc cancelar", "muted")
    footer(s, "Movimiento reducido: glifo fijo; duración cada 1 s", "Sin porcentaje inventado")
    return s


def main_run():
    OUT.mkdir(parents=True, exist_ok=True)
    for name in THEMES:
        (OUT / f"concept-{name}.ans").write_text(main(name).render())
    for name, factory in (("model", model), ("mcp", mcp), ("plugin", plugin), ("theme-picker", theme_picker), ("activity", activity)):
        (OUT / f"concept-{name}.ans").write_text(factory().render())


if __name__ == "__main__":
    main_run()
