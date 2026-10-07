# Copiar, pegar y ratón en la consola (2026-10-08)

Análisis de la calidad de vida del ratón y del portapapeles en `axlr-tui`,
y de lo que se entrega en la rama `feat/mouse-copy-paste`.

## Cómo estaba

- `View()` fija `AltScreen` y `MouseMode = CellMotion`. El terminal envía
  cada clic y arrastre a AXLR; la selección nativa deja de funcionar salvo
  con Shift (Alt en VS Code). La ayuda y la documentación no lo decían.
- Sólo los clics sobre zonas marcadas (`bubblezone`) hacían algo:
  botones, pistas del pie, filas de sesiones, modelos, cambios y paleta.
  Un clic o arrastre sobre la conversación no hacía nada;
  `MouseMotionMsg` y `MouseReleaseMsg` se descartaban.
- No había ninguna forma de copiar desde la aplicación: ni comando, ni
  OSC 52, ni escritura al portapapeles. La única ruta era la selección
  nativa con Shift, que copia el margen, las etiquetas y los cortes de
  línea tal como se pintan.
- El `textarea` de Bubbles trae `ctrl+shift+c` para copiar su propia
  selección mediante `atotto/clipboard`, pero el terminal se queda ese
  atajo casi siempre y el paquete necesita `xclip`/`xsel`/`wl-copy`.
- Pegar funcionaba: el pegado entre corchetes está activo, `PasteMsg`
  llega al compositor y a los campos de búsqueda, catálogo y plugins, y
  conserva los saltos de línea sin enviar. Faltaba un test que lo fijara.
- Clic central: con el seguimiento de ratón activo el terminal no pega la
  selección primaria; hace falta Shift+clic central. OSC 52 de lectura
  está desactivado en casi todos los terminales, así que no se puede
  pegar desde AXLR con el botón central.

## Qué se entrega

- Arrastrar sobre la conversación selecciona celdas en coordenadas de
  contenido (línea visual y columna), de modo que la rueda puede
  desplazar durante el arrastre. Al soltar se copia el texto plano,
  línea visual por línea visual, sin margen ni colores, y el pie confirma
  «Copiadas N líneas» hasta la siguiente tecla.
- Doble clic selecciona la palabra, triple clic la fila; Shift+clic
  extiende la selección existente. Esc la descarta antes de cancelar
  trabajo. Un clic sin arrastre no copia nada.
- `/copy` (alias `/copiar`) y la acción de la paleta **Copiar última
  respuesta** (`Y`) copian la última respuesta tal como la escribió el
  modelo, con su Markdown, o la selección si existe.
- Escritura del portapapeles por tres vías a la vez: OSC 52 al
  portapapeles del sistema (funciona por SSH y en tmux con
  `set-clipboard on`), OSC 52 a la selección primaria (Shift+clic central
  en X11) y, como respaldo, las herramientas del anfitrión mediante
  `atotto/clipboard`. Ninguna informa de si el terminal lo aceptó.
- Ayuda en la aplicación (F1, versión larga y corta), catálogo EN/ES y
  `docs/console.md` explican el arrastre, Shift+arrastrar para la
  selección nativa, `/copy` y cómo pegar.

## Límites conocidos

- Con `NO_COLOR` la vista elimina todo el ANSI, así que la selección no se
  pinta mientras se arrastra; la copia funciona igual y el pie lo confirma.
- Con la búsqueda (`Ctrl+F`) abierta los clics sobre la conversación se
  descartan como en cualquier overlay; «buscar y copiar la coincidencia»
  pide cerrar la búsqueda antes de arrastrar.
- Esc descarta primero una selección terminada y sólo después cancela el
  paso en curso; durante el streaming no se da, porque cada fragmento
  nuevo limpia la selección terminada.

## Qué se analizó y no se cambió

- Clic en el compositor para colocar el cursor: `textarea` no expone un
  ajuste de fila; habría que iterar `CursorUp/Down` leyendo `Cursor().Y`
  con el ajuste de línea de por medio. Vale la pena como entrega aparte.
- El nombre del modelo y el espacio de trabajo en la cabecera no tienen
  zona; un clic podría abrir `/model`.
- El selector de tema y las filas de `/mcp` y `/plugin` sólo responden a
  la rueda; las filas de herramientas del transcript no se despliegan.
- Pegar con el botón central desde AXLR: imposible sin lectura OSC 52.
- Conmutar el modo de ratón (`/mouse off`) para recuperar la selección
  nativa en terminales sin Shift+arrastrar: innecesario mientras el
  arrastre de AXLR copie, y perdería la rueda.
