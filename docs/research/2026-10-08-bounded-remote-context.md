# Ventana de trabajo acotada para modelos remotos

Análisis de la sesión `c33e8e868d98add5c9208ddb441205f5` (Haiku 5.5 vía OpenRouter,
proveedor «Claude Platform on AWS», 8 oct 2026, 16:50–17:43 UTC). Todas las
cifras están medidas sobre la traza `trace-132142-230609794.jsonl`, sus 198
payloads y el transcript guardado; el contrafactual reejecuta ese transcript por
el `ModelContextProjector` real. Sin cambios de código: el único fichero nuevo es
el harness `tui/application/zz_counterfactual_test.go` (build tag
`counterfactual`, sin commitear).

## 0. Resumen y respuesta a «pensaba que estaba solucionado»

Estaba solucionado y se deshizo:

| Fecha | PR | Techo de historia | Efecto |
|---|---|---:|---|
| 30 sep | #8 | 96 KiB (64 KiB marca baja, 16 KiB/resultado, 8 KiB checkpoint) | Política comparada con Codex, Hermes y Pi (`docs/research/2026-09-30-context-policy.md`): proyección acotada, checkpoint extractivo, `axlr_history`. Medido entonces: −86,8 % de bytes por request. |
| 2 oct | #48 | 96 KiB | Un turno que no cabe se compacta en vez de abortar; `axlr_history` rehúsa releer el turno actual. |
| 4 oct | #58 | **1 MiB** (768 KiB, 64 KiB, 16 KiB) | «Requested by Tirso (1M ceiling)», para glm-5.3-flash (ventana 1 M). El propio PR avisaba: «long sessions send many more input tokens per request». |
| 7 oct | #68 | 1 MiB, o 3/4 de la ventana × 3 B/token si la ventana es conocida | Solo los modelos locales declaran ventana. Un modelo remoto devuelve ventana 0 → presupuesto de 1 MiB. |

Resultado en esta sesión: en 198 requests el proyector **no soltó ni un turno**
(`dropped_messages = 0` en todas); la request 198 llevó 484 mensajes, 766.064
bytes y 315.677 tokens. Haiku 5.5 cobra ×5 por encima de 100K tokens de prompt
(medido: 94.063 tokens → $0,10/M; 103.844 → $0,50/M; la salida pasa de $0,50 a
$2,50/M). 133 de las 198 requests pagaron el tramo caro: $13,77 de los $14,13.
Y no hubo caché: `cached_tokens = cache_write_tokens = 0` en todas, porque el
adaptador no manda `cache_control`.

Con el presupuesto de #8 esta misma sesión habría costado $0,76 sin caché; con
un presupuesto de 64K tokens y caché automática, **$0,28** (÷50).

## 1. Qué se reenvía y qué hace falta

Composición de la request 198 (760 KB de mensajes + 6 KB de esquemas), y qué
queda con un presupuesto de 64K tokens (139 KB de historia):

| Parte | 1 MiB (real) | 64K tokens | ¿Necesaria para trabajar? | Dónde vive si se suelta |
|---|---:|---:|---|---|
| Resultados `local_read` | 106.0 KB | 14.9 KB | Solo los del turno actual | En disco: `local_read` otra vez |
| Resultados `local_exec` | 92.4 KB | 25.5 KB | Solo los del turno actual | `axlr_history` (efímeros) |
| Argumentos `local_write` (nunca se recortan) | 74.3 KB | 28.9 KB | No: es el contenido que el modelo ya escribió | En disco: el fichero |
| Texto del asistente | 71.3 KB | 10.1 KB | Los 1–2 turnos recientes | Checkpoint (excerpts), `axlr_history` |
| Resultados `kmp_write_memory` | 65.3 KB (15 llamadas) | 5.8 KB | Solo el recibo (`accepted`, `status`) | KMP: el propio registro |
| Resultados `axlr_tools` | 46.7 KB | — | Solo mientras se usa el esquema | Catálogo congelado: `axlr_tools` otra vez |
| Argumentos `kmp_write_memory` | 37.6 KB | 1.8 KB | No | KMP (`kmp_ask`, `kmp_wake`) |
| `made_list_ceremony_instances` / `_definitions` | 34.7 + 27.6 KB | — | No | MADE |
| `kmp_relate` / `kmp_wake` / `kmp_ask` | 20.3 + ~20 + ~13 KB | — | Solo en el turno que los pidió | KMP: se vuelve a preguntar |
| Mensajes del usuario | ~5 KB | 1.2 KB + 5.5 KB en el checkpoint | **Sí, exactos** | Checkpoint (49/49 exactos a 64K) |
| System prompt | 4.8–7.7 KB | 4.9 KB | Sí | — |
| Esquemas de herramientas | 6.0–9.7 KB | 6.0 KB | Sí | — |

Lo que el modelo necesita de verdad para seguir: el prefijo fijo (system +
esquemas, ~12 KB), el turno en curso entero (llamadas y resultados en orden),
uno o dos turnos cerrados recientes para continuidad, y un checkpoint que diga
qué se pidió (entradas exactas del usuario), con qué identidad de KMP se trabaja
y dónde está cada cosa que se soltó. Todo lo demás es recuperable sin el modelo
adivinando: disco, KMP, MADE o `axlr_history`.

Dos datos que corrigen la lectura inicial:

- La reducción 3,8 MB → 755 KB de la proyección es casi un solo mensaje:
  `made_list_ceremony_instances` devolvió **2.760.505 bytes** (110 instancias;
  1,53 MB de texto más 1,17 MB de `structured_content` duplicado) y se recortó a
  34.657. Quitando ese mensaje, el proyector apenas ahorró 68 KB en 482 mensajes.
- El prefijo de la request ya no es fijo: el system prompt cambió 5 veces en la
  sesión (al fijarse título y `about` en la metadata de sesión, requests 22→23;
  y durante la ceremonia `axlr_delivery`, requests 165–176, con el paso y el
  intento dentro del system prompt más una recuperación parcial de KMP de 2,7
  KB), y la lista de herramientas cambió 2 veces (`axlr_step_done` entra y sale).

## 2. Diseño: ventana de trabajo acotada apoyada en KMP

### 2.1 Presupuesto y unidad de corte

- **Unidad de corte: el turno de usuario**, como hoy. Mantiene íntegra cada
  pareja llamada/resultado y hace el corte determinista (reproducible al
  restaurar). No cambia.
- **Presupuesto en tokens del modelo, no en bytes fijos.** El dominio ya tiene
  `ContextBudgetForWindow`; falta un `ContextBudgetForTokens(objetivo,
  bytesPorToken)` donde el objetivo es el **tamaño total de prompt que
  queremos pagar**, no la ventana. Haiku tokeniza a 2,44 B/token (medido sobre
  198 requests; el código asume 3), así que un presupuesto «por ventana» de un
  modelo de 200K tokens daría 450 KB → ~185K tokens: dentro de la ventana y muy
  por encima del escalón de precio.
- **Objetivo por defecto para modelos remotos: 64K tokens totales**, con marca
  baja a la mitad del máximo (no a 3/4). En bytes a 2,44 B/token, reservando
  17 KiB para system + esquemas: máximo 138.752, marca baja 69.376, 17.344 por
  resultado, 11.562 de checkpoint (un sexto de la marca baja: el mapa por turno del PR3 no cabía en un octavo). Medido en esta sesión: $0,90 sin caché /
  $0,28 con caché, 11 cortes, 87 % de acierto de caché, 56 mensajes por request
  de media. La marca baja a 1/2 reduce los cortes de 16 a 11 y el coste un 15 %
  frente a 3/4, y es exactamente lo que #8 ya hacía (96 KiB / 64 KiB).
- Configurable con un ajuste nuevo (`remote_context_tokens` o similar), **no**
  con `context_tokens`: ese cap va por `Windows.Cap`, y `compactProfile` convierte
  en perfil compact cualquier modelo con ventana conocida ≤ 65.536. Un
  presupuesto de coste no debe cambiar el perfil de las ceremonias.
- El tope de página de `axlr_history` (32 KiB) y de resultado de host (64 KiB)
  deben seguir al presupuesto: con 17 KiB por resultado, una página de 32 KiB
  del turno actual se recorta otra vez. `hostHistory` debe acotar `limit_bytes`
  al presupuesto vigente y decirlo en la descripción de la herramienta.

### 2.2 Qué debe garantizar el checkpoint (y qué pedir a KMP) para soltar con seguridad

El checkpoint actual a 64K (12.060 B en la request 198) lleva: las 49 entradas
del usuario exactas (5.511 B), excerpts de prosa del asistente (4.844 B), las
identidades de KMP —`agent_id`, `context_id`, `guide_revision`— y el
`message_index` de la guía y del último protocolo (1.247 B), el rango omitido
y la instrucción de `axlr_history`. Falta lo que haría innecesario releer
historia:

1. **Mapa de lo soltado, por turno** (determinista, desde el transcript):
   entrada del usuario exacta (ya está), **ficheros tocados** (rutas de
   `local_write`/`local_edit` del turno), **claves de memoria escritas**
   (`about` + `idempotency_key` de cada `kmp_write_memory`; en esta sesión 15,
   todas bajo `project:AXLR`), y un indicador `unrecorded: true` cuando el
   turno hizo trabajo (escribió o ≥ 5 llamadas locales) sin escribir en KMP.
   Esto reutiliza el criterio de `memory_reminder.go`.
2. **Sustituir los excerpts de prosa del asistente por ese mapa.** Los excerpts
   son el 40 % del checkpoint y son citas, no hechos; el mapa dice dónde está
   cada cosa.
3. **Orden de recuperación en la guía**, una frase: «para lo que esta sesión ya
   zanjó, pregunta a KMP (`kmp_ask` bajo el `about` de la sesión, con el
   `context_id` del checkpoint); para un fichero, léelo; para un resultado
   concreto, `axlr_history` con su índice». Hoy la guía solo menciona
   `axlr_history`.
4. **Registro en KMP**: no hace falta nada nuevo en el lado de la consola. El
   recordatorio de memoria (`[AXLR · memory]`, una vez por petición) ya empuja
   al modelo a registrar lo que zanja; el indicador `unrecorded` del punto 1
   cierra el hueco cuando no lo hizo. Lo que sí hay que pedir a KMP es que sus
   respuestas pesen menos (§4).

Con 1–4, soltar un turno cerrado deja al modelo con: lo que el usuario pidió,
qué ficheros cambió, qué dejó anotado en KMP y con qué identidad, y dónde
están los originales. Es la postura de «si tienes KMP, no reenvíes la
conversación».

### 2.3 Ceremonias y perfil compact

- **Compact**: `ledgerProjection` ya sustituye la historia por brief + ledger y
  `CompactContextBudget` (80 KiB) sigue siendo el menor de los dos. No cambia.
- **Estándar**: el paso, el intento y la recuperación de KMP van hoy en el
  system prompt y cambian en cada transición (3 roturas de prefijo medidas en
  12 requests de ceremonia). Mover la instrucción del paso al último mensaje de
  consola `[AXLR]` (que ya existe para «the step is still open») deja el system
  prompt fijo durante la ceremonia. Lo mismo para la metadata de sesión
  (título/`about`): sacarla del system prompt a un mensaje de consola, o
  aceptar una rotura por sesión al fijar el título.
- El presupuesto de 64K se aplica igual en ceremonia: el planificador (`focused`)
  ya recibe solo su instrucción.

## 3. Caché de prompt

Hechos medidos y modelados sobre la sesión (prefijo = esquemas + system +
mensajes serializados; acierto = prefijo común en bytes con la request
anterior; 5 min de TTL bastan: la sesión duró 53 min sin huecos > 5 min):

| Presupuesto | Tokens de prompt | Máx. por request | > 100K | Cortes | Roturas de prefijo | $ sin caché | $ con caché | Acierto |
|---|---:|---:|---:|---:|---|---:|---:|---:|
| 1 MiB (real, #58) | 30,3 M | 314.761 | 133 | 0 | system 3, tools 2 | **14,08** (real 14,13) | 2,57 | 94 % |
| 96 KiB (#8) | 6,8 M | 48.513 | 0 | 18 | + checkpoint 18, P8 1 | 0,76 | 0,27 | 83 % |
| 32K tokens | 4,7 M | 33.208 | 0 | 22 | checkpoint 22, P8 12 | 0,54 | 0,23 | 80 % |
| 32K, marca baja ½ | 4,4 M | 32.419 | 0 | 18 | checkpoint 18, P8 10 | 0,52 | 0,21 | 81 % |
| 64K tokens | 9,8 M | 65.307 | 0 | 16 | checkpoint 16, P8 1 | 1,05 | 0,34 | 85 % |
| **64K, marca baja ½** | 8,2 M | 62.484 | 0 | 11 | checkpoint 11, P8 1 | 0,90 | **0,28** | 87 % |
| 96K tokens | 13,7 M | 96.604 | 0 | 11 | checkpoint 10, P8 1 | 1,44 | 0,39 | 89 % |
| 96K, marca baja ½ | 12,0 M | 94.415 | 0 | 6 | checkpoint 6, P8 1 | 1,28 | 0,32 | 91 % |
| 100K tokens | 14,1 M | 100.306 | 1 | 10 | checkpoint 10, P8 1 | 1,52 | 0,41 | 89 % |

Lecturas:

- **Estabilidad del prefijo.** Con caché automática cada request reutiliza
  todo lo anterior salvo donde el prefijo cambia. Las roturas vienen de: (a) el
  system prompt (título/`about`, pasos de ceremonia), (b) la lista de
  herramientas en ceremonia, (c) **cada corte**: el checkpoint es el mensaje 1,
  así que un corte reescribe todo lo que va detrás del system prompt (una
  escritura entera a 1,25×), y (d) **P8**: un resultado recortado en el turno
  actual cambia su texto `retrieval` al cerrarse el turno. P8 pesa poco a 64K
  (1 rotura) y bastante a 32K (12), porque un límite de 7,6 KB por resultado
  recorta mucho más. El arreglo de P8: un único texto de `retrieval`, idéntico
  en los dos lugares, que cubra ambos casos («si este turno sigue abierto,
  repite la llamada con una consulta más estrecha; si no, `axlr_history({message_index: N})`»).
  Así no hay rotura y tampoco una llamada perdida a `axlr_history` que
  `hostHistory` rechazaría (#48 se mantiene).
- **Los cortes son el coste de la histéresis**: menos cortes (marca baja ½) =
  más acierto y menos tokens. No hay forma de cortar sin invalidar lo que sigue
  al checkpoint; Pi y Hermes tienen el mismo límite.
- **Dónde poner `cache_control`.** En `requestDTO` un campo de nivel superior
  `"cache_control": {"type": "ephemeral"}` cuando el modelo es `anthropic/*`
  (OpenRouter lo aplica a Anthropic, Claude Platform on AWS, Bedrock y Vertex;
  mínimo 512 tokens para Haiku/Sonnet/Opus 5.5; escrituras 1,25×, lecturas
  0,1×), **más un breakpoint explícito en el system prompt**: `content` como
  bloques `[{type: "text", text, cache_control: {type: "ephemeral"}}]`, que la
  documentación de OpenRouter muestra en mensajes `system` y `user`. Es la
  combinación recomendada para bucles de agente: el prefijo fijo (esquemas +
  system, 2–3K tokens > el mínimo de 512) tiene un punto de lectura garantizado
  que sobrevive a los cortes y a P8, y la caché automática cubre la cola que
  crece. Hay que leer
  `prompt_tokens_details.cached_tokens` y `cache_write_tokens` en `usage_dto.go`
  y volcarlos en `provider_done`, para que el acierto sea verificable.
- **El escalón de precio y la caché.** El modelo asume (conservador, sin
  fuente que lo confirme) que las lecturas de caché cuentan como tokens de
  prompt para el tramo: por eso 1 MiB con caché queda en $2,57 (133 requests a
  0,1 × $0,50). Si no contaran, esa cifra bajaría, pero seguiría ≥ 7× la de 64K
  con caché, y la recomendación no cambia. El presupuesto debe mantener el
  prompt total (system + esquemas + historia) por debajo de 100K para Haiku: el
  objetivo de 100K roza el escalón (una request a 100.306); 96K lo evita.
- **Nota sobre la tabla.** El acierto cuenta como leídos los esquemas y el
  system prompt también tras un corte o una rotura P8; eso solo es cierto con
  el breakpoint explícito en el system prompt (abajo). Solo con caché
  automática el breakpoint está en el último bloque y una reescritura del
  checkpoint lo pierde todo: unos 5K tokens menos de acierto por corte. Y a
  1 MiB la rotura P8 del resultado de 2,76 MB de MADE coincidió con un cambio
  del system prompt de la ceremonia, así que la tabla la cuenta como «system».

## 4. Otros recortes

- **Argumentos históricos de `local_write`/`local_edit`**: el mayor componente
  a 64K (28,9 KB de 139). Recortar los argumentos de llamadas de turnos
  cerrados a `{path, bytes, note: "content is on disk; local_read"}` (~200 B).
  Rotura de caché: una por turno cerrado en la posición de la llamada, como P8;
  barata porque solo invalida el turno recién cerrado.
- **`content` + `structured_content` en el bridge**: 26 resultados llevan ambos
  y solo 2 son idénticos, así que la deduplicación actual no actúa. Política de
  Pi: cuando `content` tiene texto, omitir `structured_content` en la proyección
  dejando `structured_content_omitted: <bytes>` (el original queda en
  `axlr_history`). Un `kmp_write_memory` pasa de 5.240 B a ~900 B.
- **Verbosidad de KMP** (issue en KMP, no en AXLR). Un `kmp_write_memory` de 912 B
  de argumentos devuelve 5.240 B: `structured_content` 4.292 B
  (`proposed_relations` 1.732, `clocks` 575, `viewer` 389, `receipt` 365,
  `labels` 338, `coverage` 203, `local_refs` 145, `generated_refs` 112) más 885 B
  de texto (recibo de 68 B y `kmp_guidance` de 681 B). `kmp_wake` 9.760 B,
  `kmp_ask` 5.466–8.580 B, `kmp_relate` 10.517 B. Propuesta: un modo de respuesta
  mínimo (recibo + `status` + continuación) y `kmp_guidance` solo cuando cambia.
- **Verbosidad de MADE** (issue en MADE): `made_list_ceremony_instances` devolvió
  2.760.505 B por 110 instancias sin paginar; `made_list_ceremony_definitions`
  60.098 B. Necesitan página y vista resumida.
- **Metadata de sesión en el system prompt** (`sessionContextGuidance`): es el
  único dato variable del prefijo fijo fuera de las ceremonias.

## 5. Contrafactual: método y validez

Harness: `tui/application/zz_counterfactual_test.go` carga el transcript
(483 mensajes), y para cada request N toma los primeros `len(messages)−1` del
payload real como prefijo (la proyección nunca cortó, así que coincide con el
transcript de entonces), aplica `markSteered`, lo pasa por
`ModelContextProjector` con cada presupuesto, antepone el system prompt real de
esa request y suma los bytes de esquemas reales. Tokens por el ajuste medido
`prompt_tokens = 0,40977 × bytes + 1.523` (R² 0,9998, error medio 1,7 %; razón
2,31–2,69 B/token, mediana 2,41); precio por tramo medido ($0,10/$0,50 ≤ 100K; $0,50/$2,50 encima);
caché: lectura 0,1×, escritura 1,25× del tramo, acierto = prefijo común en bytes
con la request anterior, 0 si < 512 tokens.

Validación: reejecutar con el presupuesto real de 1 MiB da $14,08 frente a
$14,13 medidos (−0,4 %) y los bytes de mensajes difieren < 0,3 % (la nota de
steer y la ceremonia). Límites: los tokens son estimados por el ajuste (error medio 1,7 %, mayor solo
en las requests pequeñas, donde domina la constante);
el modelo no se simula —con menos contexto podría hacer alguna llamada más a
`axlr_history` o KMP—, pero #8 funcionó en sesiones reales con 96 KiB y #48
eliminó el modo de fallo por desbordamiento.

## 6. Recomendación y plan de PRs

Recomendación: volver a una ventana acotada como la de #8, expresada en tokens
y con caché. Objetivo por defecto 64K tokens con marca baja a la mitad; 96K
sería la alternativa si se prefiere menos cortes (÷36 en vez de ÷50 en esta
sesión).

| PR | Contenido | Tamaño | Efecto medido en esta sesión |
|---|---|---|---|
| 1. Presupuesto remoto | `ContextBudgetForTokens` (2,44 B/token para `anthropic/*`, 3 para el resto), por defecto 64K totales y marca baja ½ cuando la ventana es desconocida; ajuste `remote_context_tokens`; `hostHistory` acota la página al presupuesto; arreglo de P8 (mismo texto de excerpt); docs de `console.md` | pequeño (dominio + cableado + docs) | $14,13 → $0,90 |
| 2. Caché | `cache_control` de nivel superior para `anthropic/*` y breakpoint explícito en el system prompt (bloques de contenido); `cached_tokens`/`cache_write_tokens` en `usage_dto.go`, traza `provider_done` y estado de la TUI | pequeño (adaptador) | $0,90 → $0,28 |
| 3. Checkpoint v2 | Mapa por turno soltado (ficheros tocados, claves KMP, `unrecorded`), sin excerpts de prosa; guía con el orden de recuperación KMP → disco → `axlr_history` | medio (proyector + tests) | Seguridad al soltar; −4,8 KB de checkpoint |
| 4. Recortes de proyección | Argumentos históricos de `local_write`/`local_edit`; `structured_content` omitido cuando hay texto | medio | −35 KB por request a 64K |
| 5. Prefijo estable en ceremonias | Paso/intento y metadata de sesión fuera del system prompt | pequeño–medio | 5 roturas menos por sesión |
| Issues | KMP: respuesta mínima y `kmp_guidance` solo si cambia. MADE: paginar `made_list_ceremony_*` | — | — |

Orden: 1 y 2 juntos (paran la sangría), luego 3, 4, 5. Espero tu aprobación
antes de tocar código.
