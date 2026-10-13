# Cuestionario: tu visión del ciclo de vida AXLR + KMP + MADE

Documento de trabajo, en español, para que Tirso responda. Respondido íntegramente en el chat el 13 de octubre de 2026 en ocho rondas de cuatro preguntas; B1 y B4 se derivan de otras respuestas. Cada respuesta se convierte en una decisión del [diseño de integración del ciclo de vida](2026-10-13-lifecycle-integration-design.md); las que contradigan una recomendación del diseño la sustituyen.

**Cómo responder.** Cada pregunta tiene un identificador estable (`A1`, `C4`…). Responde en este fichero tras `R:` o en el chat con el identificador ("C4: sí, por store"). Vale "decide AXLR". Marca prioridad si quieres: `!` imprescindible, `~` deseable, `-` fuera de alcance. Donde hay opciones son ejemplos, no una lista cerrada.

Cada pregunta trae **Hoy** (lo que hace la consola ahora) y, cuando el diseño ya eligió, **Propuesta**.

---

## A. Principios y roles

**A1. Dueño de la memoria.** Hoy: el modelo escribe con `axlr_remember` o `kmp_write_memory`; la consola solo escribe el resultado terminal de una ceremonia. ¿Quién debe escribir en KMP y qué puede escribir cada uno sin pedir permiso: consola, modelo, persona, Jev?
R: Consola: ws:<sesión> (resultados, cierres, ceremonias), sin tarjeta, actor axlr. Jev: project:<x> y user:<nombre> (lo asentado, preferencias, supersedes), escrito por la consola con evidencia. Modelo: project:<x> solo sin Jev, con axlr_remember. La persona no tiene comando de escritura propio (solo /memory save y /memory forget).

**A2. Qué es "memoria principal".** ¿La consola debe poder arrancar sin KMP (degradada, avisando) o KMP es obligatorio como lo es el modelo? ¿Y MADE?
R: Obligatorios para arrancar; si caen a mitad de sesión, encolar las escrituras en disco, avisar y reintentar.

**A3. Memoria vs historia de ejecución.** Tu frase "guardar cada acción y por qué" choca con la doctrina de KMP ("decisiones, restricciones, resultados; nunca transcripts") y con mantener útil el wake del about del proyecto. ¿El journal por acción es *memoria* (vive junto a los hechos del proyecto) o *historia de ejecución* (vive en un about de sesión y se consulta por tiempo)? ¿Qué debería mostrar jamás un wake del about del proyecto procedente del journal?
R: Historia de ejecución, registrada cuando una interacción produce un resultado (fichero escrito, test, …): cualquier interacción entre persona y modelo con resultado.

**A4. Tres memorias, tres usos.** ¿KMP guarda memoria del proyecto, del usuario (preferencias, estilo), de la sesión, o las tres? ¿Con qué about cada una?
R: Tres abouts: project:<x> (proyecto), user:<nombre> (persona), ws:<sesión> (sesión).

**A5. Invariantes al final de una sesión.** Enumera lo que debe ser verdad siempre al cerrar: por ejemplo "cada fichero escrito tiene una entrada con su porqué", "cada decisión tiene evidencia", "el bundle está al día".
R: Cada turno con resultado tiene su paso MADE completado y su entrada en ws:<sesión>; cada fichero escrito tiene un why y aparece en algún turno; cada decisión del about del proyecto tiene evidencia y ref de MADE o juicio de Jev; la instancia MADE está en DONE o ABANDONED, nunca abierta sin lease. La consola lo comprueba al cerrar y lo dice si falla.

**A6. Un modelo pequeño.** ¿Qué es lo máximo que puede pedirse a Haiku 5.5 o GLM 5.3 Flash en cada turno respecto a memoria y ceremonias? Una regla por turno, una llamada, nada.
R: Un why por acción; una frase final sobre lo asentado cuando no hay Jev; un summary_en cuando escribe memoria en otro idioma. Título y about siguen permitidos (G3/C9), no exigidos.

**A7. La ejecución en el Mac.** No he visto esa transcripción. ¿Qué hizo Haiku en cada momento (arranque, primer prompt, acciones, cierre) y qué esperabas en su lugar? ¿Está disponible el fichero de sesión (`~/.local/state/axlr/sessions/<id>.json`) o el app log del Mac?
R: Tengo el app log del Mac (~/.local/state/axlr/logs/axlr.log). Pendiente de copiarlo aquí.

**A8. Dos síntomas o uno.** He separado "no hay memoria" (store global vacío, bundle no restaurado) de "el modelo tropieza con el protocolo" (carga en el prompt). ¿Lo ves como dos problemas o como uno solo?
R: Sí, dos problemas con dos arreglos: fases 1 y 5 para "no hay memoria"; 2 a 4 para "el modelo tropieza con el protocolo".

---

## B. Instalación y primer arranque

**B1. Qué instala sin preguntar.** Hoy: el asistente pregunta idioma, modelo y si instalar KMP y MADE, y la aprobación de KMP. ¿Qué debe hacerse sin preguntar y qué debe preguntarse siempre?
R: (Derivado de K5a/K5b) Sin preguntar: idioma del entorno, engines, stores, grants, definiciones, aprobaciones auto, autonomía, settings y mcp.json completos. Se pregunta: modelo (clave de OpenRouter o servidor local) y clave de TypeSafe opcional.

**B2. Dónde vive el store de KMP.** Hoy: uno global por usuario (`~/.local/share/kmp/default`) fijado con `KMP_MCP_DATA_DIR`. Propuesta: `<repo>/.kernel` por workspace git, global fuera de git, operador por encima. Alternativas: store por workspace fuera del repo; configurable en `settings.json`; preguntar al abrir. ¿Cuál es tu modelo?
R: <repo>/.kernel por workspace git; global fuera de git; un valor de operador en mcp.json gana.

**B3. Dos procesos sobre un `.kernel`.** En este checkout Claude Code tiene abierto `AXLR/.kernel` y AXLR lo abriría también. Como autor de KMP: ¿dos `kmp-mcp` escribiendo a la vez en un store embebido está soportado, tolerado o prohibido? ¿Qué hacen `store-leases` y `commit-native-bundle.lock`?
R: Soportado: dos kmp-mcp pueden escribir a la vez en un store embebido.

**B4. Consistencia con los otros hosts.** ¿AXLR debe abrir el mismo store y el mismo about que Claude Code y Codex en el mismo repo, o puede tener memoria propia?
R: (Derivado de B2/B3) Sí: AXLR abre el mismo .kernel y el mismo about que Claude Code y Codex en el mismo repo; dos writers están soportados.

**B5. Bundle `.kmp/memory.jsonl`.** Hoy: AXLR no lo lee ni lo escribe; en este repo está sin trackear. ¿Importar al abrir el workspace cuando el store está vacío? ¿Exportar al cerrar siempre, solo si `.kmp/` existe, o solo con `/memory save`? ¿Puede AXLR hacer `git add` o commit del bundle? ¿Debe trackearse `.kmp/` en AXLR?
R: Importar al abrir si el store está vacío. Sin export automático ni git.

**B6. Guide sync.** Hoy: una vez al instalar, en el store global. Con store por workspace: ¿por store al abrir, antes de arrancar el engine, o cuando `kmp_guide` lo pida? ¿Quién lo ejecuta si el store es de otro host?
R: Guide sync por store: al crearlo y cuando kmp_guide diga que falta; antes de arrancar el engine, nunca con otro engine escribiendo.

**B7. Configuración del store.** El store global tiene `typesafe.json`, `rerank`, `wake-focus`, `write-relations`, `judgement-book`, `curate`, `ask-judge` activos; `AXLR/.kernel` no tiene ninguno. ¿Quién configura un store de proyecto nuevo y con qué valores por defecto? ¿Debe AXLR copiar la configuración del store global?
R: Copiar la configuración del store global al nuevo (typesafe, rerank, wake-focus…); las configuraciones principales listas para usar.

**B8. Verificación al terminar el setup.** Propuesta: importar bundle, despertar el about y decir "N memorias para project:X" o "proyecto nuevo". ¿Qué línea quieres ver exactamente? ¿Debe el setup abrir el visor de KMP?
R: Línea de arranque: store, razón, about, N memorias o "proyecto nuevo", instancia MADE; la misma en stderr, app log y setup. El setup no abre el visor.

**B9. Versiones de los engines.** Hoy: `engines.lock.json` y los runbooks dicen KMP 0.24.0 y MADE 0.9.1; el instalador baja el último release (0.25.0 y 0.10.0). ¿Lock que manda, último release, o lock solo para `axlr-serve`?
R: Siempre el último release; el lock es solo del servicio.

**B10. MADE en la instalación.** Hoy: store por usuario, grant `axlr-default-work-v1`, aprobador, ocho definiciones publicadas. ¿Debe publicarse algo más? ¿Store de MADE por proyecto como el de KMP, o siempre por usuario?
R: Store de MADE por usuario, como hoy (no se marcó por proyecto).

**B11. Aprobación por defecto.** Hoy: `kmp` auto (si dices sí), `made` manual; con autonomía, todo sin tarjeta. ¿Qué debe correr sin tarjeta en una instalación nueva: lecturas de KMP, escrituras de KMP, lecturas de MADE, nada de MADE?
R: kmp auto y made auto; la consola nunca pide tarjeta por su contabilidad.

**B12. Instalación sin terminal interactiva.** ¿Debe `axlr-tui` con `--model` o en un pipe instalar los engines por sí solo o solo avisar?
R: Instala y configura igual, sin preguntar nada: clave del entorno o falla; engines, stores y configuración completa se crean solos; Jev solo si TYPESAFE_API_KEY existe.

---

## C. Apertura del workspace y creación de sesión

**C1. About canónico.** Hoy: `project:<nombre del directorio en minúsculas>` (desde el 10 de octubre); antes `ws:<sesión>`; KMP y tú escribís `project:AXLR`. Propuesta: about del bundle si existe, si no el nombre del directorio con su caja. Otras fuentes posibles: remote de git (`owner/name`), `AGENTS.md` o `CLAUDE.md` del repo, pregunta a la persona la primera vez. ¿Cuál es la regla?
R: About del bundle si existe; si no, nombre del directorio con su caja (project:AXLR). Se retira el paso a minúsculas.

**C2. Los duplicados actuales.** El store global tiene `project:AXLR` (30 eventos) y `project:axlr` (27); `.kernel` tiene `project:AXLR` (7). ¿Migrar con `kmp-mcp import --from … --about`, declarar `same_entity_as`, elegir uno y abandonar el otro, o despertar ambos hasta que se fundan?
R: Borrar project:AXLR y project:axlr del store global (export del resto y reimport; AXLR/.kernel no se toca). Pendiente de ejecutar con visto bueno explícito y export de respaldo previo.

**C3. Significado de `ws:`.** Lo fijaste tú para las ceremonias ("decisión 2"). ¿Qué quería decir `ws:` exactamente: sesión, workspace, ambos? ¿El journal por acción va bajo `ws:<sesión>`, bajo un prefijo nuevo (`session:`), o bajo el about del proyecto con etiquetas?
R: ws:<id de sesión> como hoy; el workspace va como etiqueta. (Confirmado en la segunda ronda: no se renombra nada.)

**C4. Identidad del agente KMP.** Propuesta: la consola registra con `kmp_guide {registration_key: axlr:<install-id>}` una vez por store, porque `agent-users.sqlite3` vive en el store. ¿La identidad debe ser por store, por instalación, por máquina, por persona? ¿Cómo debe llamarse el agente de la consola?
R: Nombre del agente axlr@<host>: una identidad por store y máquina; registration_key axlr:<install-id>, el nombre nuestro en attributes.

**C5. Contexto de sesión.** Propuesta: un contexto KMP por sesión (`context_key: axlr:session:<id>`), `context_id` inyectado por la consola en cada llamada `kmp_*` y en `axlr_remember`; tras compactación, contexto nuevo con el mismo agente. ¿De acuerdo? ¿El modelo debe saber que existe un `context_id`?
R: Un contexto por sesión; el modelo no sabe que existe un context_id (ninguna opción de visibilidad marcada: se mantiene oculto).

**C6. Varias sesiones a la vez en el mismo workspace.** Hoy: `engines.shared` comparte un proceso por lanzamiento idéntico. ¿Mismo store, mismo about, mismo agente, contextos distintos? ¿Algo que deban ver unas de otras?
R: Varias sesiones en el mismo workspace comparten store, about y agente, con contextos distintos y cada una su instancia MADE.

**C7. Worktrees y clones.** KMP comparte el store entre worktrees de un checkout. ¿Un clon bajo otro nombre de directorio es otro proyecto (otro about) o el mismo?
R: Un worktree comparte store y about con su checkout (ruta como etiqueta ws). Clones: sin respuesta, regla del directorio.

**C8. Sesiones de reparación y mejora (`/repair`, `/improve`).** Hoy usan `project:<repo en minúsculas>` del ajuste `repair.repository`. ¿Mismo about y store que la sesión que las pidió?
R: El .kernel del clon, con el about del repo reparado (project:AXLR); el bundle del clon se importa si existe.

**C9. Selección explícita de about.** Hoy: `axlr_session {about}` guarda un about elegido y gana siempre. ¿Mantener que el modelo pueda elegirlo, solo la persona, o solo instrucciones del repo?
R: El modelo puede fijar el about con axlr_session.

---

## D. Primer prompt y recuperación

**D1. Wake al empezar.** Hoy: en modo normal la consola no despierta nada; solo las ceremonias reciben un paquete. Propuesta: `kmp_wake` del about del proyecto con el primer prompt como intent, añadido al final de ese prompt como nota `[AXLR · recall]`, tope 4 KiB (1 KiB en perfil compacto). ¿Siempre? ¿Solo si hay memoria? ¿Con intent o sin él?
R: Siempre, con el primer prompt como intent, como nota [AXLR · recall] al final del prompt; en resume también ws:<sesión>.

**D2. Dónde va el paquete.** Alternativas: nota al final del prompt (mantiene estable el prefijo del system prompt), mensaje de consola aparte, system prompt. ¿Cuál?
R: Nota al final del primer prompt.

**D3. Qué secciones del wake.** Hoy (ceremonias): `current_state`, `open_loops`, `next_actions` en prosa, sin refs. ¿Con refs para que el modelo pueda enlazar? ¿Etiquetas? ¿Proyección temporal (últimos N días)?
R: Secciones como en ceremonias (current_state, open_loops, next_actions); incluye también el about user:<nombre>. Los refs no se marcaron.

**D4. Despertar en cada prompt o solo en el primero.** ¿Un wake nuevo cuando la persona cambia de tema (intent nuevo)? ¿Quién lo decide?
R: Solo el primer prompt, y se repite cuando el prompt cambia de tema (la consola detecta el cambio de intent).

**D5. Resume de sesión.** Propuesta: al reabrir (`--session`, picker), despertar también `ws:<sesión>` (su journal). ¿Qué quieres ver al reanudar: últimas acciones, ficheros tocados, decisiones, todo?
R: Al reanudar: los últimos N turnos de ws:<sesión> (ficheros, comandos, decisiones), el estado de la instancia MADE y la ceremonia dirigida viva, y el último prompt y la última respuesta completos del transcript.

**D6. Sesión sin memoria previa.** ¿Qué debe decir la nota: "proyecto nuevo", nada, o una invitación a registrar?
R: "Proyecto nuevo, nada registrado" y nada más; Jev o axlr_remember asientan lo primero que se decida.

**D7. Comparación entre abouts.** Hoy: el skill pide un `kmp_relate` tras el segundo prompt. Propuesta: fuera del arranque, `/memory compare` a demanda. ¿Tiene sitio en el arranque automático en algún caso?
R: /memory compare a demanda, nunca automático; Jev propone una comparación como nota cuando el wake muestra abouts relacionados y la persona decide.

**D8. `axlr_recall`.** Propuesta: un host tool `{question?, about?, since?, mode: ask|wake|history}` que mapea a `kmp_ask`, `kmp_wake`, `kmp_time` con el about, el contexto y un presupuesto fijos, respuesta compacta. ¿Qué verbos de KMP debe cubrir? ¿Debe existir, o basta con `kmp_*` por el bridge?
R: axlr_recall cubre ask, wake y time (no inspect ni trace), con about, contexto y presupuesto fijos y respuesta compacta.

---

## E. Cada acción del turno (el journal)

**E1. Qué es una acción.** Propuesta: prompts de la persona, efectos del modelo (`local_write`, `local_edit`, `local_exec`, `axlr_call_tool`, `axlr_run_tool`, `axlr_remember`, `axlr_step_done`), decisiones de aprobación, cambios de modo/modelo/about, pasos de ceremonia, cierre. Las lecturas (`local_read`, `local_search`, `local_list`, `axlr_tools`, `axlr_history`, `axlr_skill`, `axlr_session`) solo se cuentan. ¿Qué falta, qué sobra? ¿Las lecturas también una a una?
R: Una entrada por turno en KMP bajo ws:<sesión> con solo los resultados (ficheros, comandos con exit code, llamadas externas, memorias, decisiones y why) apuntando al paso de MADE; prompt y respuesta viven en MADE.

**E2. El porqué del modelo.** Propuesta: argumento `why` opcional (una frase, ≤ 240 caracteres) en write/edit/exec/call_tool/run_tool, mostrado en la tarjeta y guardado tal cual. Alternativas: obligatorio (la consola rechaza la llamada sin él), un `why` por turno en vez de por llamada, el modelo lo declara antes con otra tool. ¿Qué pasa cuando falta?
R: Argumento why obligatorio: la consola rechaza la llamada sin why.

**E3. El porqué de la persona.** Hoy: motivo opcional al denegar. ¿Motivo al aprobar? ¿Una nota libre que se adjunta al turno (`/why …`)? ¿El prompt ya es el porqué?
R: Motivo opcional al aprobar (tecla N en la tarjeta), además de al denegar; y una nota libre por turno con /why que va al paso MADE y a la entrada ws:.

**E4. La respuesta final del modelo.** ¿Se guarda algo de ella (primeras líneas, un resumen, su clasificación: respondió / cambió ficheros / preguntó)? KMP dice "nunca transcripts".
R: Solo en MADE: extracto de 500 bytes y answer_kind. KMP nunca recibe prosa del modelo.

**E5. Contenido de cada entrada.** Propuesta: tool, digest de argumentos (ruta y modo; línea de comando; tool del plugin), `why`, decisión y motivo, clase de resultado (`completed`, `error`, `denied`, `over_budget`); en el turno: ficheros escritos, comandos con exit code, memorias registradas, coste. ¿Argumentos completos? ¿Salida de los comandos? ¿Diffs? ¿Latencia, tokens, modelo?
R: Línea de comando, exit code y cola de salida redactada (hasta 4 KiB, como el check de ceremonia).

**E6. Kinds y verbos de KMP.** ¿`kind: turn` se concibió para esto? ¿Qué kind para cada evento (`observation`, `decision`, `feedback`, `error_path`, `success_path`)? ¿La consola debe usar `kmp_write_memory` o `kmp_ingest` (grafo exacto, coordenadas y relaciones propias)?
R: kmp_write_memory basta para la consola; kmp_ingest no.

**E7. Relaciones en el journal.** ¿Entradas planas, o la consola enlaza mecánicamente (`follows` entre acciones, `triggers` prompt→acción, `verified_by` acción→comando, `updates_state` hacia la memoria del proyecto)? ¿Con qué `why` y `evidence` escritos por la consola?
R: Entradas planas con etiquetas; sin relaciones mecánicas por ahora.

**E8. Evidencia de cada entrada.** ¿Exit code, ruta, hash del fichero, índice del mensaje en el transcript, id de la llamada? ¿Bastan como `evidence` para KMP?
R: Ref del paso MADE + ruta o comando + exit code + hash del fichero, una línea por acción, como evidence de texto.

**E9. Cuándo se escribe.** Propuesta: un `kmp_write_memory` por fin de turno con todas las entradas del turno. Alternativas: una escritura por acción (más llamadas, más fiel en tiempo real), al cierre de sesión, cola local con flush. ¿Qué pasa con las `occurred_at` de cada acción si se escriben juntas?
R: Al cerrar el turno: un made_complete_ceremony_step y un kmp_write_memory por turno, occurred_at por acción.

**E10. KMP caído a mitad de sesión.** ¿Bloquear el turno, encolar en disco y reintentar, avisar y seguir sin journal? ¿Y si MADE cae con una ceremonia abierta?
R: Encolar en disco, avisar en la cabecera y reintentar.

**E11. Secretos y rutas.** ¿Redactar valores que parezcan claves, rutas fuera del workspace, contenido de ficheros? ¿Quién decide la política: `settings.json`, la persona por sesión?
R: Redactar valores que parezcan claves o tokens.

**E12. Tarjetas de aprobación.** ¿Las escrituras del journal (actor consola) nunca piden tarjeta? ¿Deben contarse en `/approvals` o en el presupuesto de llamadas del turno?
R: Las escrituras de consola nunca piden tarjeta (confirmado por K5a: auto por defecto).

**E13. Idioma y `summary_en`.** Hoy: `axlr_remember` no manda `summary_en`; tu store estricto rechazó una memoria en español de Haiku el 11 de octubre. Para el journal (texto de consola) y para las memorias del modelo: ¿la consola traduce, el modelo aporta `summary_en`, o el modo estricto se relaja para `actor: axlr`?
R: El modelo aporta summary_en (axlr_remember gana summary_en opcional); las entradas de consola en inglés.

**E14. Forged tools y plugins externos.** ¿Las llamadas a `github`, `microsoft-learn` o a una tool forjada se registran igual que `local_exec`?
R: Sí: why obligatorio y registro como acción para plugins externos; las tools forjadas se registran con su nombre y versión. Las lecturas externas no quedan exentas del why.

**E15. Retención y condensación.** ¿El journal crece sin límite? ¿La consola condensa (`kmp_condense`) sesiones viejas, exporta y poda, o nunca borra? ¿Quién lo lanza?
R: Nunca se borra; crece sin límite. Los abouts de sesión cerrados quedan fuera de todo wake salvo petición explícita.

**E16. Olvidar o corregir.** ¿Puede retirarse una entrada del journal (supersede, `valid_until`) cuando fue un error o contiene algo que no debía? ¿Con qué comando?
R: Corregir con supersede/valid_until (/memory forget <ref>).

---

## F. Fin de turno y lo que el trabajo deja asentado

**F1. Quién decide qué es duradero.** Hoy: el modelo con `axlr_remember`, empujado por el recordatorio `[AXLR · memory]`. Alternativas: la consola mecánicamente (ficheros, checks), la persona con `/remember`, Jev juzgando el turno, combinación. ¿Cuál?
R: Jev juzga el turno y propone lo duradero; la consola escribe lo que Jev propone, sin tarjeta. Sin Jev, el modelo con axlr_remember (recordatorio actual).

**F2. El recordatorio.** ¿Mantenerlo tal cual (una petición extra por turno con trabajo), limitarlo a turnos que escribieron ficheros, sustituirlo por una pregunta en la respuesta final, quitarlo?
R: Mantener, solo en turnos que escribieron ficheros o ejecutaron checks; nunca por lecturas.

**F3. Resumen de sesión en el about del proyecto.** Propuesta: al cerrar, una `observation` mecánica (ficheros, checks, memorias registradas) con etiqueta de sesión. ¿Sí? ¿Qué más debe llevar y qué no debe llevar nunca?
R: Nada además de lo que Jev juzgó turno a turno; el cierre completa la instancia MADE y escribe session_closed en ws:<sesión>.

**F4. `needs_review` de enlaces.** Hoy: `axlr_remember` devuelve la revisión y el modelo confirma con la continuación. ¿Debe la consola resolverla sola cuando los enlaces apuntan a refs del wake? ¿Siempre revisión del modelo?
R: La consola resuelve sola el needs_review cuando los links apuntan a refs del wake; con refs desconocidos lo devuelve al modelo.

**F5. Decisiones que cambian.** Cuando una decisión posterior contradice una anterior, ¿quién declara `supersedes` o `corrects`: el modelo, la consola al detectar el mismo about y asunto, nadie?
R: Jev lo detecta (recibe las memorias vivas relacionadas del wake con el turno) y la consola escribe supersedes con why y evidencia.

**F6. Memoria del usuario.** Preferencias ("no uses em-dashes", "commits en inglés") y correcciones: ¿about propio (`user:<nombre>`), etiqueta, fuera de KMP?
R: About propio user:<nombre>, despertado en cada sesión; Jev o el modelo registran preference/feedback ahí.

---

## G. Lo que ve el modelo

**G1. Superficie de KMP en modo normal.** Propuesta: `axlr_recall` y `axlr_remember`; los `kmp_*` crudos siguen por el bridge. Alternativas: ocultar los crudos del todo; mostrarlos solo en `/research`; mostrar solo lecturas. ¿Cuál?
R: axlr_recall + axlr_remember; los kmp_* crudos siguen por el bridge.

**G2. Texto del prompt.** Propuesta: unos 900 bytes estables (párrafo en D8 del diseño) en lugar del skill `axlr-session` (4,4 KB en dos páginas) y las guías actuales. ¿Qué frase es imprescindible para ti? ¿Qué sobra?
R: Imprescindible: "la consola ya recordó y registra; tú nunca registras ni despiertas KMP"; "da un why a cada escritura, comando y llamada externa"; "la consola ejecuta las ceremonias, esta sesión incluida; tú no arrancas ni transicionas"; "UNKNOWN es una respuesta válida de memoria".

**G3. Título de sesión.** Hoy: el modelo debe ponerlo tras el segundo prompt; si no, la consola lo deriva de los dos primeros; F2 para la persona. Propuesta: solo consola y persona. ¿De acuerdo?
R: El modelo puede poner el título (tras el segundo prompt).

**G4. Checkpoint tras compactación.** Hoy: lista de turnos omitidos con ficheros y memorias, `context_id`, y "recupera con kmp_ask, local_read, axlr_history". Propuesta: además "el journal de los turnos omitidos está en KMP bajo ws:<sesión>". ¿Debe la consola despertar el journal por el modelo en lugar de decirle que lo haga?
R: Sin checkpoint (D10); la nota de recall de la sesión sucesora sustituye al checkpoint. Para un proyecto nuevo, la nota dice solo "proyecto nuevo, nada registrado".

**G5. Perfil compacto (modelos pequeños).** ¿Mismo ciclo con topes más bajos, o menos pasos (sin `why`, sin `axlr_recall`)?
R: Mismo ciclo con topes más bajos (nota de recall 1 KiB, why obligatorio, axlr_recall disponible).

**G6. Lo que el modelo no debe poder hacer nunca.** Enumera: registrar agentes, cambiar de store, cambiar de about, transicionar ceremonias, publicar definiciones, borrar memoria…
R: Registrar agentes, cambiar de store o de contexto KMP: kmp_guide con registration_key/context_key fuera del bridge.

---

## H. MADE en el ciclo de vida

**H1. ¿La sesión es una ceremonia?** Alternativas: cada sesión es una instancia MADE y cada turno un paso (todo queda en el journal de MADE); solo los modos (`/debug`, `/delivery`…) y `/ceremony` usan MADE; algo intermedio (una ceremonia "sesión" ligera). ¿Cuál es tu visión?
R: Toda sesión es una instancia MADE: definición axlr_session con un paso turn repetido; la consola reclama al recibir el prompt y completa al cerrar el turno.

**H2. Verbos de MADE visibles en modo normal.** Propuesta: lecturas, diseño y validación; nada que arranque, reclame, complete, transicione, cancele, apruebe, conceda, publique, registre, borre o suba. ¿Alguna excepción?
R: Lecturas y diseño (get/list/read/stream/search, design/validate/explain/diff); nada que arranque, reclame, complete, transicione, apruebe, conceda, publique o borre.

**H3. `/ceremony <definición>`.** Propuesta: driver genérico para cualquier definición publicada (claim, instrucción derivada de la definición, `axlr_step_done`, complete, transition, recordatorio de paso abierto). ¿Cómo debe instruirse al modelo para un paso de una definición arbitraria? ¿Qué expone MADE de un paso que sirva de instrucción?
R: La instrucción de cada paso sale de made_explain_ceremony_draft y la consola la pasa al modelo.

**H4. Definiciones diseñadas por la persona.** Hoy: el modelo puede diseñar y, por el bridge, publicar. ¿Quién publica: `P` en `/mcp`, `/ceremony publish`, el modelo con tarjeta, nunca el modelo?
R: Solo la consola publica (P en /mcp o /ceremony publish).

**H5. Estado del host.** Propuesta: `made_report_ceremony_agent_status` en claim (`running`), hand-back (`finished`), stall (`blocked`) y heartbeat a un cuarto del lease; `made_renew_ceremony_step_lease` antes de expirar. ¿Obligatorio en todo paso dirigido? ¿Frecuencia? ¿Qué identidades (`host_agent_id`, incarnation, `logical_worker_id`)?
R: running al reclamar, finished al cerrar, heartbeat cada N minutos; lease renovado antes de expirar.

**H6. Handoff entre consolas o máquinas.** Propuesta: `made_record_ceremony_host_handoff` al reanudar una instancia cuyo último informe vino de otra encarnación. ¿Cuándo más? ¿Qué evidencia?
R: Handoff con evidencia: run id, host y hora del último informe de la otra consola.

**H7. Guards humanos de MADE y tarjetas de AXLR.** Hoy: dos superficies (aprobador MADE para incidente y merge; tarjetas para tools). ¿Unificarlas: toda aprobación humana es un guard de MADE, o toda es una tarjeta que la consola traduce?
R: Tarjetas y guards siguen separados.

**H8. Espejo MADE ↔ KMP.** ¿Qué del journal de MADE (instancia, pasos, decisiones humanas, intervenciones) debe replicarse en KMP, y qué refs de KMP deben ir en los outputs de MADE? ¿Uno es la fuente de verdad del otro?
R: MADE es la fuente de verdad de la ejecución (cada paso turn con prompt, respuesta y acciones); la entrada turn de KMP lleva solo resultados con el ref del paso MADE como evidencia.

**H9. Councils y sistemas agénticos.** ¿Fuera de alcance del ciclo de vida de la consola, o AXLR debe poder ser un agente de un council?
R: Fuera del ciclo de vida de la consola; el modelo puede leerlos, AXLR no es agente de ningún council.

**H10. Ceremonia cancelada o bloqueada.** ¿Qué debe quedar en KMP y qué debe decirle la consola a la persona?
R: La consola propone el siguiente paso a la persona (repetir con otro check, cambiar de modo, abrir /repair); el estado queda en MADE y ws:<sesión> como hoy.

---

## I. Cierre, reanudación y otras máquinas

**I1. Cierre limpio.** Propuesta: entrada `session_closed` en el journal, observación mecánica en el about del proyecto, export del bundle si `.kmp/` existe. ¿Algo más (resumen pedido al modelo, coste total, abrir el visor)?
R: Completar la instancia al cerrar (/exit, /new, salir) con el resumen.

**I2. Sesiones abandonadas.** Sin `/exit` (cierre de terminal, crash, idle): ¿se cierra al siguiente arranque, por el daemon compartido, nunca?
R: Las abandonadas se completan como "abandonada" cuando la consola vuelve a abrir ese workspace.

**I3. Cambio de máquina.** ¿Viaja solo la memoria (bundle) o también las sesiones (transcripts, labels, ceremonias)? ¿Por git, por `/kmp:save`, por otro medio?
R: Viaja la memoria (bundle .kmp con /memory save y git) y también las sesiones (export de transcript + instancia reanudable en otra máquina).

**I4. Reanudar una sesión en otra máquina.** ¿Debe funcionar? ¿Qué se recupera: journal de KMP, instancia de MADE, el transcript?
R: Debe funcionar mediante el export de sesión.

**I5. Varias personas en un mismo proyecto.** ¿Un store compartido (gRPC), un bundle por persona, abouts por persona? ¿Fuera de alcance?
R: Varias personas: store KMP compartido por gRPC con about común y user:<nombre> por persona; fuera de alcance por ahora.

---

## J. Observabilidad y verificación

**J1. `/mcp`.** Propuesta: por engine, el store o SQLite, la razón de la selección (`operator`, `project`, `user`) y la identidad de trabajo. ¿Qué más?
R: /mcp: store, razón de selección, identidad de trabajo.

**J2. Cabecera y línea de arranque.** ¿Quieres ver el about, el store, el número de memorias, el contexto KMP, la instancia MADE viva?
R: Cabecera: about, memorias, instancia MADE viva. Línea de arranque: store, about, N memorias o "proyecto nuevo". Panel /session con el journal del turno.

**J3. Verificación desde el transcript.** Para cada fila de la tabla del diseño: ¿cómo quieres comprobar que se cumplió (línea en el app log, entrada en el journal, llamada visible en el transcript)?
R: Un test de integración por fila de la tabla con engines reales en un HOME temporal.

**J4. Pruebas.** ¿tmux con Haiku y GLM sobre stores de prueba, como hasta ahora? ¿Un store de fixture con memoria conocida para el smoke?
R: Además, sesiones tmux con Haiku y GLM revisadas a mano, una por fase.

**J5. Objetivos numéricos.** Peticiones al modelo dedicadas a contabilidad por sesión (¿0?), bytes de prompt estables, llamadas MCP de consola por turno, latencia añadida por turno. ¿Cifras?
R: 0 peticiones de contabilidad al modelo; ≤ 1 KB de prompt estable; ≤ 3 llamadas MCP de consola por turno (heartbeat aparte).

**J6. Visor de KMP (ChronoLoom).** ¿La consola debe abrirlo, enlazarlo, mostrar `kmp_view_*` del journal?
R: El panel /session enlaza cada entrada con el visor (ref clicable por acción). Ni /memory view ni URL en la cabecera.

---

## K. Prioridades, límites y compatibilidad

**K1. Anti-objetivos.** ¿Qué no quieres que haga la consola nunca (por ejemplo: commits automáticos, escribir en el about del proyecto sin evidencia, pedir tarjetas por el journal, traducir memorias)?
R: Nunca escribir en el about del proyecto sin evidencia; nunca traducir ni reescribir el texto de la persona o del modelo.

**K2. Convenciones deliberadas.** El diseño trata como deliberados `ws:<sesión>` para ceremonias y el store global fijado al instalar. ¿Cuáles de las convenciones actuales deben quedarse y cuáles liberas?
R: Se queda ws:<sesión> como about de sesión. El resultado terminal de una ceremonia dirigida sigue solo en ws:<sesión>; el about del proyecto lo recibe solo si Jev lo juzga duradero.

**K3. Compatibilidad.** Sesiones guardadas, labels, stores y abouts existentes: ¿migración automática, nota de consola, nada?
R: Nada: empezar de cero. Lo anterior queda donde está; solo lo nuevo sigue el ciclo. kmp-mcp import --from queda documentado para quien quiera migrar a mano.

**K4. Orden de fases.** Propuesta: 1 store y about, 2 protocolo en la consola, 3 journal y `why`, 4 MADE, 5 pulido del setup. ¿Otro orden? ¿Qué puede esperar?
R: Store y about → sesión MADE + journal → protocolo KMP en consola → Jev → setup.

**K5. Lo que falta en este cuestionario.** Lo que tienes en la cabeza y no he preguntado.
R: Ampliado abajo (K5a–K5d).

---

## K5 ampliado (respondido el 13 de octubre, segunda ronda)

**K5a. "El auto es por defecto".** ¿Qué cubre en una instalación limpia?
R: Aprobación auto de kmp y made; autonomía completa activada (autonomous: true) salvo lo destructivo; engines instalados y configurados sin preguntar. Lista destructiva que sigue pidiendo tarjeta: borrados y sobrescrituras fuera del workspace; git push, reset --hard, clean, branch -D, rebase; red con efecto (curl -X POST/PUT/DELETE, gh pr create, deploys, pagos).

**K5b. Qué pregunta todavía el asistente.**
R: Clave de OpenRouter o servidor local y modelo (nada si OPENROUTER_API_KEY está en el entorno); clave de TypeSafe (Jev), opcional. El idioma no se pregunta (del entorno).

**K5c. "Configuración de AXLR por defecto completa".**
R: settings.json escrito completo con todas las claves y sus valores; mcp.json completo para kmp y made escrito por AXLR; stores de KMP y MADE con su configuración por defecto; defaults activos: engines compartidos, forged tools, Jev si hay clave.

**K5d. El modelo y su contexto.**
R: La compactación no debe tirar la caché de prompt; la nota de recall no debe romper la caché en el segundo prompt. El presupuesto de contexto se mide en tokens con tiktoken (o200k) como medida única para todos los modelos: el contexto de cada petición no puede pasar de 75K tokens (por defecto, configurable). Estrategia: checkpoint al final con el prefijo intacto; cuando aun así se supera el tope, sesión sucesora encadenada (nueva sesión con el wake de ws:<sesión anterior> y la instancia sucesora en MADE; el transcript viejo queda entero). Tope de coste en USD y timeouts de comandos: sin respuesta (defaults del diseño).
