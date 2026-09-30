# AXLR TUI · propuesta visual e interacción

30 de septiembre de 2026. Alcance: diseño y prototipos revisables; no cambia la TUI productiva. Checkout observado: `codex/tui-mcp-controls` en `af03e7eb`.

## Evidencia de la interfaz actual

Capturé una sesión real de `axlr-tui` en una PTY de 100 × 32 celdas con raíz y estado de prueba bajo `/tmp`. Abrí `/model`, `/mcp` y `/plugin` y escribí un borrador sin enviarlo. Los ficheros `.ans` de `assets/current-*` son la captura de la PTY; los PNG muestran esas celdas renderizadas con Hack Nerd Font Mono. La consulta de `/model` cargó el catálogo, sin iniciar una generación.

| Paso | Captura | Estado | Hallazgo |
| --- | --- | --- | --- |
| 1. Redactar | [Compositor actual](assets/current-composer.png) | Funciona, pobre jerarquía | Tres líneas fijas, acciones en texto plano y mucho vacío. Foco, borrador y acción principal apenas se distinguen. |
| 2. Elegir modelo | [Selector actual](assets/current-model.png) | Denso | El nombre, ID, contexto y precios compiten en una sola línea; buena parte se trunca. No hay detalle ni filtros visibles. |
| 3. Inspeccionar MCP | [Panel actual](assets/current-mcp.png) | Navegable | Servidores, estado, política y herramientas forman una lista larga, sin búsqueda ni agrupación. |
| 4. Revisar plugin | [Panel actual](assets/current-plugin.png) | Navegable | Repite casi la misma vista que MCP; el cambio de aprobación queda reducido a la tecla `A`, sin explicar el alcance. |

La captura demuestra legibilidad y jerarquía de esas vistas, no accesibilidad completa. Faltan pruebas de teclado, lector de pantalla, contraste del terminal del usuario y latencia bajo streaming real.

## Propuesta de composición

Las siete imágenes siguientes son **conceptos estáticos**, no capturas de una implementación de Bubble Tea. [El generador ANSI](prototype.py) permite reproducirlos a 100 × 32 y el PNG permite revisarlos sin abrir un terminal.

| Theme | Muestra | Intención |
| --- | --- | --- |
| Ink | [Console](assets/concept-ink.png) | Contraste nocturno equilibrado; propuesta inicial. |
| Aurora | [Console](assets/concept-aurora.png) | Cian y verde sobre azul petróleo; actividad y conexiones. |
| Paper | [Console](assets/concept-paper.png) | Lectura clara, fondo cálido y acentos tinta. |
| Phosphor | [Console](assets/concept-phosphor.png) | Verde sobrio, densidad alta y menor decoración. |

Vistas desarrolladas con Ink: [modelo](assets/concept-model.png), [MCP](assets/concept-mcp.png), [plugin](assets/concept-plugin.png), [selector y previsualización de themes](assets/concept-theme-picker.png) y [storyboard de actividad](assets/concept-activity.png). La cifra de modelos y sus nombres en el concepto son datos ilustrativos; la implementación mostrará el catálogo real. Para regenerar las pantallas ANSI: `python3 docs/design/2026-09-30-tui/prototype.py`.

La estructura común tiene cabecera de sesión, cuerpo con viewport, barra de actividad vinculada al trabajo real, compositor y ayuda contextual. Los selectores usan una lista a la izquierda y detalle a la derecha cuando hay 80 columnas o más. Entre 50 y 79 columnas, detalle y lista se alternan con `Tab`; a 50 × 15 se mantienen selección, acción y error visibles. El cuerpo recibe el espacio sobrante. El compositor crece de dos a ocho líneas útiles y vuelve a contraerse cuando se vacía.

### Tokens y perfiles

| Theme | Fondo / superficie / elevada | Texto / atenuado / acento | Selección / borde | Estado correcto / aviso | Bordes, iconos y espacio |
| --- | --- | --- | --- | --- | --- |
| Ink | `#11131C` / `#1B2030` / `#252C40` | `#E8EDF7` / `#9BA6BC` / `#7DD3FC` | `#364765` / `#526078` | `#6EE7B7` / `#FBBF72` | Redondeados de una celda; rombo para memoria; 2 celdas de margen. |
| Aurora | `#091D29` / `#112B38` / `#173B48` | `#E4F9F4` / `#A7C7C6` / `#5EEAD4` | `#22636A` / `#32717C` | `#A3E635` / `#F9C66B` | Bordes finos; puntos para conexión; espacios de 1–2 celdas. |
| Paper | `#F7F3E9` / `#FFFDF7` / `#EBE4D8` | `#263342` / `#627083` / `#315AA7` | `#CEDDFA` / `#A7B1BA` | `#187B61` / `#A9542C` | Bordes suaves; textos explícitos; 2 celdas de margen. |
| Phosphor | `#08120B` / `#102015` / `#18321D` | `#D8F2CE` / `#83A984` / `#87E66C` | `#23502A` / `#477D4B` | `#B7F173` / `#E4C66D` | Separadores simples; pocos recuadros; 1 celda de margen. |

En todos hay tokens adicionales para usuario, asistente, memoria, error, foco y hover. El color nunca es la única pista: las etiquetas `Conectado`, `Error`, `Manual`, `Auto`, `Preparando`, `Ejecutando` acompañan al indicador. Ofrecer `NO_COLOR`, alto contraste y movimiento reducido como variantes de preferencias. La vista previa del selector de themes debe aplicar temporalmente el theme al selector y al fondo; `Enter` confirma y `Esc` restaura el anterior. Guardar `{version, theme_id, icon_profile, reduce_motion}` en un fichero privado de preferencias con escritura atómica. Si el ID falta o el fichero está dañado, volver al theme automático y avisar sin bloquear la sesión. El modo automático usa `tea.RequestBackgroundColor` y permite override explícito. [Bubble Tea v2](https://github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md) y [Lip Gloss v2](https://github.com/charmbracelet/lipgloss/blob/main/UPGRADE_GUIDE_V2.md) documentan esa detección y el render de color.

### /model, /mcp y /plugin

Una `CatalogShell` compone búsqueda, chips de filtro, lista, panel de detalle, estado y ayuda. No comparte las acciones de dominio: `/model` selecciona un modelo y lo persiste como predeterminado, `/mcp` inspecciona servidores externos y herramientas, `/plugin` revisa y cambia la política de un plugin exacto.

| Vista | Lista y filtros | Detalle y acción | Estados |
| --- | --- | --- | --- |
| `/model` | Buscar por nombre, ID y proveedor; filtros de proveedor, herramientas y contexto; opción «solo compatibles» con razón del filtro. | Nombre e ID completos, contexto, precio de entrada/salida y capacidades; `Enter` selecciona y guarda el predeterminado. Si el guardado falla, mostrar advertencia sin deshacer el modelo de la sesión, como ocurre hoy. | Cargando: spinner y cuenta si se conoce; error: mensaje y `R Reintentar`; vacío: «sin modelos compatibles» o «sin resultados» con limpiar filtros. |
| `/mcp` | Servidores con nombre, propósito, salud y cantidad de herramientas; buscar también por herramienta; filtros conectados/error. | Descripción, conexión, inventario filtrable, herramienta seleccionada y nombre exacto; `R` actualiza. Sin acciones de servidor inventadas. | Conectando, conectado, error de descubrimiento y sin servidores con instrucción de configuración. |
| `/plugin` | Identidad exacta, propósito, modo de aprobación, herramientas permitidas; filtro manual/auto. | Política actual y alcance del manifiesto; `Enter` abre elección Manual/Automática con explicación y confirmación; solo se persiste el ID exacto. | Guardando, guardado, error de persistencia, plugin sin herramientas, plugin no disponible. |

El foco queda visible en búsqueda, lista o detalle. `Tab` cambia región, flechas recorren, `Esc` cierra el nivel actual, `F1` explica teclas; el borrador del chat permanece intacto al abrir/cerrar cualquier selector. Las acciones en ratón usan zonas delimitadas y conservan un equivalente de teclado.

### Compositor

Reusar `textarea.Model` v2 como editor, con marco y cabecera que muestran foco y estado del borrador. Placeholder contextual corto, sin simular contenido. Altura: mínimo 2 líneas de texto, máximo 8 o un tercio del terminal, lo que sea menor; al superar el máximo aparece scroll interno y una posición visible. `Enter` envía; `Shift+Enter` agrega nueva línea. La línea de ayuda y el botón Enviar indican la acción, y el botón cambia a `Cancelar` durante un turno. Al abrir overlays, el foco pasa a su control y vuelve al editor al cerrarlos. Mantener el borrador actual hasta confirmación de envío; tras error, dejar el texto recuperable. El borrador persistido de la sesión sigue usando el mecanismo existente.

### Actividad y movimiento

La animación se deriva de eventos observables. No se muestra texto privado de razonamiento ni porcentaje cuando no existe progreso medible.

| Evento real | Texto de estado | Animación |
| --- | --- | --- |
| Inicio de petición, antes de respuesta | Conectando con el modelo · tiempo transcurrido | Pulso de tres puntos, hasta el primer evento de proveedor. |
| `ProviderReasoning` | Modelo razonando · tiempo transcurrido | Trama de puntos; solo indica actividad recibida. |
| `ProviderToolCall` | Preparando herramientas · tiempo transcurrido | Iconos que avanzan una celda. |
| `EventToolExecutionStarted` | Ejecutando `plugin/herramienta` o herramienta local | Spinner junto al nombre exacto y tiempo de ejecución; si es KMP, usar tratamiento de memoria. |
| `EventTextDelta` / contenido | Respondiendo | Cursor discreto junto al texto; no spinner de espera. |
| Final, cancelación o error | Resultado explícito | Detener ticks de inmediato; conservar duración real. |

Usar `spinner.Model` con un único reloj activo, en torno a 8 fotogramas por segundo, solo cuando la vista muestra actividad. El cronómetro actual de 1 s sigue dando la duración. No introducir ticks por cada fila ni forzar recomposición del transcript en cada frame. El render de 16 ms del streaming conserva prioridad; entrada y scroll no esperan al frame. En modo de movimiento reducido, mostrar un símbolo fijo y actualizar solo el tiempo cada segundo. `progress.Model` queda reservado para procesos con total conocido; nunca para la espera al proveedor. El estado de ejecución y la política exacta de aprobación ya persistidos deben seguir siendo fuente de verdad.

## Componentes y coste

| Pieza | Uso propuesto | Coste / límite |
| --- | --- | --- |
| `bubbles/list` v2 | Selección y filtro de `/model`, `/mcp`, `/plugin`; delegado propio para filas de dos líneas. | Ya es dependencia transitiva; sustituye paginación y búsqueda manuales. Validar que su filtro no altere IDs seleccionados. |
| `bubbles/viewport` v2 | Transcript y detalle de herramientas, con scroll independiente. | Ya usado; conservar posición al redimensionar y al actualizar texto. |
| `bubbles/textarea` v2 | Compositor adaptable y foco. | Ya usado; ajustar estilos y altura sin cambiar semántica Enter/Shift+Enter. |
| `bubbles/help` + `key` v2 | Ayuda según foco y ancho. | Ya disponibles; una única tabla de atajos compartida por vista. |
| `bubbles/spinner` + `progress` v2 | Feedback solo durante trabajo real y progreso medible. | Ya disponibles; limitar ticks para no degradar streaming. |
| `bubbles/table` v2 | Tabla de precios en ancho grande, si se lee mejor que pares etiqueta/valor. | No imponer en 50–79 columnas; ahí usar detalle apilado. |
| `lipgloss/v2` | Tokens, superficies y composición adaptable. | Ya usado; verificar recortes ANSI y perfiles de color. |
| BubbleZone v2 | Acciones de ratón en botones, pestañas y filas. | Ya usado. [Su README](https://github.com/lrstanley/bubblezone) advierte de incompatibilidad posible con el canvas/compositor v2; mantener `zone.Scan` solo en la raíz y evitar canvas hasta probarlo. |
| `huh/v2` | Candidato para un futuro editor de configuración MCP. | Compatible con Bubble Tea v2, pero añade dependencias y otra gestión de foco; no aporta suficiente a estos tres selectores. [Fuente](https://github.com/charmbracelet/huh). |
| `glamour/v2` | Candidato para Markdown de respuestas completas. | Añade parser y coste de render; posponer hasta medir streaming y sanitización. [Fuente](https://github.com/charmbracelet/glamour). |

[Bubbles](https://github.com/charmbracelet/bubbles/blob/main/README.md) documenta listas con filtro, ayuda, spinner, textarea, tabla, viewport y progress. No hay que actualizar versiones como parte del diseño: el módulo ya fija Bubble Tea `v2.0.10`, Bubbles `v2.2.1`, Lip Gloss `v2.0.6` y BubbleZone `v2.0.0`.

## Tipografía e iconos

La tipografía la elige el **emulador de terminal**. AXLR puede seleccionar colores, bordes, texto e iconos, y en Bubble Tea v2 puede solicitar o declarar ciertos colores del terminal; no puede imponer una familia Nerd Font desde Go. `FontFamily` de VHS solo cambia el renderizador usado para estas muestras. Por eso el selector de themes ofrece un **perfil de iconos** `Seguro` (por defecto), `Nerd Mono` opcional y `ASCII` para terminales limitados; no ofrece un selector de fuente ficticio. [FAQ de Nerd Fonts](https://github.com/ryanoasis/nerd-fonts/wiki/FAQ-and-Troubleshooting) recomienda la variante Mono para cuadrículas de terminal.

Probé la misma [muestra ANSI](assets/font-sample.ans) con cuatro familias Nerd Mono: [Hack](assets/font-hack.png), [JetBrainsMono](assets/font-jetbrainsmono.png), [FiraCode](assets/font-firacode.png) e [Iosevka](assets/font-iosevka.png). Hack y JetBrainsMono preservaron mejor la cuadrícula en este renderizador. FiraCode e Iosevka mostraron espaciado y bordes incorrectos en VHS con esta configuración; eso **no prueba** que fallen en todos los terminales. La medición con `ansi.StringWidth` dio una celda para `◆ ● ◇ ✓ ⚠ ⌕ ↗  󰊢 󰈙 󰘳`; con Pillow, los cuatro TTF Mono dieron un avance igual al de `M` para esos glifos. Aun así, el terminal puede usar fallback y métricas distintas; por eso el perfil seguro es el predeterminado. El perfil ASCII traduce, por ejemplo, memoria a `[MEM]`, correcto a `[OK]` y espera a `[WAIT]`. Los iconos nunca sustituyen al texto de estado.

## Plan de implementación

Cada etapa produce un cambio revisable con comparación PTY a 100 × 32, 80 × 24 y 50 × 15, tests de teclado/resize pertinentes, `go vet`, suite `-race` y cobertura agregada superior al 80 % por módulo. Mantener arquitectura hexagonal y tipos de dominio para preferencias/estados; la capa terminal consume DTOs, sin desplazar decisiones de aprobación o contexto al estilo visual.

1. **Base visual y preferencias.** Tokens de theme, detección claro/oscuro, `NO_COLOR`, perfil de glifos, selector con preview/rollback y almacenamiento privado atómico. Verificar contraste, degradación ANSI 16/256, recuperación de preferencias dañadas y `Esc`.
2. **Compositor y layout.** Altura adaptable, marco de foco, borrador visible, placeholder y ayuda contextual. Preservar Enter/Shift+Enter, pegado, cancelación, recuperación de sesión y scroll del transcript. Medir que el cuerpo no salta con cada pulsación.
3. **Catálogos.** Componente `CatalogShell` y lista v2; implementar `/model`, después `/mcp`, después `/plugin` con búsqueda, filtros, detalle, carga/error/vacío y acciones exactas. Probar modelo predeterminado y política manual/auto con fallos de persistencia y estado de plugins reales.
4. **Actividad.** Vincular animaciones a `ProviderPhase` y eventos de ejecución; reducir movimiento, apagar ticks al finalizar, acotar repintados. Repetir fixture de streaming y PTY real para detectar regresiones de input, scroll y latencia; no usar llamadas al proveedor para pruebas rutinarias.
5. **Pulido y entrega.** Capturas comparativas de todos los themes, fuentes y tamaños; navegación completa con teclado/ratón, contraste y fallback sin Nerd Font; documentación de controles y preferencias. Revisar la etapa antes de integrar en PR #8 o en una rama dedicada.

### Criterios de aceptación transversales

- `/model` muestra nombre e ID completos en detalle y nunca trunca silenciosamente los precios de la selección; elegir persiste el predeterminado.
- `/mcp` diferencia conexión e inventario, y `/plugin` expone el alcance de una política antes de cambiarla. No cambia aprobación sin una acción explícita.
- El editor conserva el borrador durante overlays, errores y restauración; Enter envía y Shift+Enter inserta salto.
- La actividad mostrada corresponde a eventos reales, no revela razonamiento privado y se detiene al cancelar o terminar.
- Sin Nerd Font ni color, todas las acciones y estados siguen siendo comprensibles; ningún glifo cambia el ancho de las filas o rompe el ratón.
- Streaming, entrada, scroll, política de contexto y recuperación de originales mantienen sus garantías actuales.
