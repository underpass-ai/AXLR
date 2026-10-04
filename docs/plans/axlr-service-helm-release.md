# Plan de implementación: API de AXLR, Helm y releases

> **Historical record.** This page preserves its original design or investigation context. For current behavior and operating instructions, use the [current documentation](../index.md) and its audited revision.

Estado: **implementado en código, pendiente de release candidata y pruebas con KMP/MADE remotos**. Fecha del plan: 30-09-2026; revisión: 01-10-2026. Especificación: [API de servicio](../specs/axlr-service-api.md). El repositorio contiene `axlr` (worker JSON de una petición), `axlr-tui` (consola local) y `axlr-serve` (API HTTP mTLS). Este documento conserva el contrato y la lista de aceptación; la [guía de releases](../releasing.md) describe la publicación.

## 0. Contrato de producto y decisiones cerradas

AXLR procede de simplificar la capa de ejecución de `underpass-runtime`, con Pi como referencia conceptual para una superficie pequeña de herramientas. AXLR ejecuta turnos agénticos, herramientas, sesiones y aprobaciones. KMP gobierna memoria duradera y MADE gobierna orquestación y decisiones ceremoniales. El servicio debe ejecutar herramientas tanto dentro de un turno como por una llamada directa explícita.

La API y Helm conservarán la compatibilidad ya adoptada con los manifiestos de plugins de Codex para skills y MCP. La instalación seguirá siendo propiedad de AXLR y sus servidores conservarán política de aprobación propia. Ningún componente del paquete que hoy solo se retiene pasará a activarse implícitamente por desplegar el servicio.

La primera API será HTTP `/v1`, con JSON para comandos y SSE para eventos. Exigirá mTLS incluso en localhost. Un despliegue remoto se hará con Helm; el chart instala **solo AXLR** y se conecta a KMP y MADE existentes. En Kubernetes, `kmp-mcp` y `made-mcp` son adaptadores MCP stdio dentro del pod; hablan gRPC con los motores remotos usando mTLS. No alojan el store de KMP ni el de MADE. Se mantiene una réplica de AXLR hasta disponer de almacenamiento y leases distribuidos.

### Estado comprobado que condiciona el plan

| Hecho actual | Código que lo demuestra | Consecuencia |
|:--|:--|:--|
| El worker consume una petición y termina | `cmd/axlr/worker.go`, `runtime/executor.go` | No presentarlo como API de servicio ni reutilizarlo como servidor de sesiones |
| El ciclo agéntico ya está fuera del renderer | `tui/application/start_turn_use_case.go`, `agent_turn_use_case.go`, `continue_turn_use_case.go` | Reutilizar esos casos de uso desde el adaptador HTTP |
| Las sesiones guardan snapshots y usan un lock por escritor | `tui/adapters/storage/session_store.go` | Añadir idempotencia y journal de eventos antes de anunciar reanudación HTTP |
| Las aprobaciones y llamadas MCP ya tienen política por identidad | `tui/application/resolve_tool_use_case.go`, `tui/adapters/axlr/plugin_manager.go` | Compartir el resolvedor y mantener denegación por defecto |
| El cliente MCP acepta stdio y HTTP | `mcpclient/server.go` | Usar stdio con los adaptadores remotos de KMP y MADE sin inventar protocolo |
| `mcp.json` solo expone entorno a procesos stdio | `tui/adapters/storage/mcp_config.go` | Generar registros internos con entorno desde Secrets montados; no meter secretos en el chart |
| MADE remoto es gRPC con mTLS, no un endpoint MCP HTTP | `made/docs/operations/deploy-kubernetes.md`, `made-mcp/src/backend.rs` | `made-mcp` en modo `grpc` dentro del pod |
| KMP remoto puede servirse con adaptador gRPC o gateway HTTP | `kmp/docs/enterprise/README.md` | Elegir adaptador gRPC para simetría y probar el gateway en otra fase |
| La CI previa solo probaba Linux | `.github/workflows/ci.yml` | La matriz de release valida construcción y ejecución por plataforma antes de publicar |
| El ejecutor y los locks usaban primitivas Unix sin separación por sistema | `adapters/local/process_adapter.go`, `file_adapter.go`, `tui/adapters/storage/session_lock.go`, `mcp_config_lock.go`, `mcp_config.go` | Los adaptadores de plataforma y las pruebas nativas son necesarios antes de ofrecer Windows |

## 1. Estructura exacta de la entrega

Las rutas son propuestas concretas para implementar. Mantener las responsabilidades separadas; no duplicar el bucle del agente en handlers.

| Ruta | Responsabilidad |
|:--|:--|
| `tui/cmd/axlr-serve/main.go`, `run.go` | CLI del servicio: configuración, señales, arranque, cierre y modo `--probe`. Vive en el módulo `tui` porque el módulo raíz no puede importar `tui/application` sin un ciclo. El binario se llama `axlr-serve`. |
| `tui/service/config.go` | Configuración estricta: listen, workspace fijo, modelo, límites, rutas TLS, policy, state, KMP y MADE. Rechaza campos desconocidos, rutas relativas y secretos literales. |
| `tui/service/server.go` | Composición de casos de uso, HTTP server, timeouts y shutdown. Ninguna lógica de herramientas en handlers. |
| `tui/service/http_*.go` | Handlers separados para sesiones/turnos, SSE, aprobaciones, herramientas, probes y errores. |
| `tui/service/auth.go` | Verificación mTLS y mapa de huellas SHA-256 de certificado a principal y roles. |
| `tui/service/operations.go` | Coordinador de operaciones por sesión, serialización, cancelación e idempotencia. |
| `tui/service/event_store.go` o `tui/adapters/storage/session_events.go` | Journal persistente de eventos con cursor creciente y replay. |
| `tui/service/tool_calls.go` | Intención, revisión, ejecución y resultado durable de llamadas directas; utiliza `runtime.Executor` y el mismo catálogo/política de herramientas. |
| `tui/service/remote_engines.go` | Registros MCP con `kmp-mcp`/`made-mcp` y su entorno de gRPC/mTLS. |
| `api/openapi/axlr-v1.yaml` | Contrato OpenAPI 3.1 de todas las rutas JSON, estados y ejemplos. SSE descrito con esquema de eventos. |
| `distribution/engines.lock.json` | Versiones, URLs de origen y SHA-256 por arquitectura de los dos adaptadores MCP incluidos en imagen. |
| `distribution/Dockerfile` | Imagen Linux de `axlr-serve` con los dos adaptadores verificados; usuario no root y sin gestor de paquetes en runtime. |
| `charts/axlr/Chart.yaml`, `values.yaml`, `values.schema.json` | Versión, valores y validación del chart. |
| `charts/axlr/templates/` | Deployment, Service, PVC, ServiceAccount, ConfigMap sin secretos, probes, NetworkPolicy opcional y ayudas de validación. |
| `scripts/release/` | Preflight de tag, compilación/paquete, inventario, comprobación de checksums y pruebas de artefactos. |
| `.github/workflows/release.yml` | Matriz de builds y pruebas con permisos de lectura; publicación en job separado y solo para tags. |
| `docs/api.md`, `docs/helm.md`, `docs/releasing.md` | Referencia operativa creada al implementar, sin sustituir la especificación actual. |

No mover los paquetes de `tui/application` al módulo raíz en esta entrega. La extracción sería una migración independiente si se quiere una dependencia más limpia. El adaptador HTTP sí debe importar y ejecutar esos mismos casos de uso, no copiar sus transiciones.

## 2. Contrato HTTP al byte

### 2.1 Convenciones

- Prefijo `/v1`; `Content-Type: application/json` para comandos; UTF-8; JSON estricto sin campos desconocidos; cuerpo máximo 4 MiB como el worker. Responder `415` a otro tipo de contenido y `413` si excede el límite.
- `X-Request-Id` opcional del cliente; si falta, generar uno. Devolverlo en cabecera y errores. No usarlo como clave de idempotencia.
- `Idempotency-Key` obligatoria en POST que inicia turno, crea llamada directa o toma una decisión. Longitud 16–128 caracteres ASCII seguros; guardar hash de método+ruta+cuerpo y principal. Igual clave e igual petición devuelve recurso anterior; igual clave con distinto contenido devuelve `409 idempotency_conflict`.
- IDs de sesión, operación y llamada generados por servidor: 16 bytes aleatorios codificados en 32 caracteres hexadecimales minúsculos. Eventos usan una secuencia decimal creciente por sesión. Todas las rutas validan IDs antes de tocar almacenamiento. `If-Match: "<revision>"` o `expected_revision` obliga a compare-and-set al iniciar turno, decidir o cancelar. Falta de revisión devuelve `428 revision_required`; valores contradictorios en cabecera y cuerpo devuelven `400 revision_conflict`; perder la carrera devuelve `409 stale_revision`.
- Errores uniformes: `{"error":{"code":"...","message":"...","request_id":"..."}}`; sin rutas de secretos, argumentos confidenciales ni detalles internos. `400/401/403/404/409/410/413/415/422/428/429/500/503` tienen códigos estables en OpenAPI.
- El servidor limita concurrencia, tamaño de respuesta y tiempo de handler; streaming SSE tiene heartbeat y límite de clientes. El modelo puede seguir trabajando aunque el cliente SSE se desconecte; desconectar no equivale a cancelar.

### 2.2 Rutas y estados

| Ruta | Entrada | Salida y precondición |
|:--|:--|:--|
| `POST /v1/sessions` | `{model}` | `201 {id,model,status,revision}`; modelo validado, workspace fijo del servidor |
| `GET /v1/sessions/{id}` | Sin cuerpo | Snapshot proyectado, revisión y última secuencia; solo propietario o rol administrador |
| `POST /v1/sessions/{id}/turns` | `{prompt}` + `Idempotency-Key` y revisión | `202 {operation_id,session_id,status,events_url}`; solo sesión idle/complete/interrupted sin llamada pendiente |
| `GET /v1/sessions/{id}/events?after=N` | `Accept: text/event-stream` | Replay desde secuencia N y eventos nuevos; `Last-Event-ID` como alternativa; `410` si se podaron eventos anteriores |
| `POST /v1/sessions/{id}/approvals/{call_id}` | `{decision:"approve"|"deny",expected_revision}` | Persiste decisión, ejecuta solo la llamada pendiente exacta y avanza la sesión; `409` en revisión/call no vigente |
| `POST /v1/sessions/{id}/cancel` | `{expected_revision}` | `202`; marca cancelación e interrupción, sin prometer revertir efectos externos |
| `GET /v1/tools` | Filtros opcionales de origen | Nombres exactos, esquemas y disponibilidad local/MCP; nunca confundir catálogo built-in con servidor conectado |
| `POST /v1/tool-calls` | `{tool,arguments}` + `Idempotency-Key` | `202 {call_id,status:"pending_approval"|"running",revision}`; intención persistida antes de ejecutar |
| `POST /v1/tool-calls/{id}/decisions` | `{decision,expected_revision}` + clave | `202` estado de la llamada; autoriza la llamada exacta, no una capacidad genérica |
| `GET /v1/tool-calls/{id}` | Sin cuerpo | Estado de ciclo de vida (`pending_approval`, `running`), estados finales compartidos con el worker (`completed`, `failed`, `rejected`, `cancelled`, `timed_out`) y estado adicional de servicio `uncertain` |
| `GET /livez`, `GET /readyz` | Sin cuerpo, listener de probes separado | Liveness local; readiness comprueba proveedor, KMP y MADE requeridos; datos mínimos |

Los eventos SSE tienen `id: <secuencia>`, `event: <tipo>` y `data: <JSON>`. El cursor `after` y `Last-Event-ID` son enteros decimales no negativos, exclusivos: se devuelve la secuencia siguiente. Si ambos están presentes deben coincidir; en caso contrario responder `400 cursor_conflict`. Un cursor futuro devuelve `409 cursor_ahead`; uno anterior al mínimo retenido, `410 events_expired` con ese mínimo. Tipos mínimos: `turn.started`, `text.delta`, `tool.requested`, `approval.required`, `tool.completed`, `turn.completed`, `turn.interrupted`, `operation.failed`. Cada evento lleva `session_id`, `operation_id`, `sequence`, `time` y payload tipado. Nunca enviar un delta antes de que su registro durable esté disponible para replay. Coalescer deltas de texto en fragmentos limitados para evitar fsync por carácter, pero fijar el ID solo después de persistir el fragmento. El servidor responde al replay en orden estricto y evita emitir dos veces el mismo ID dentro de una conexión.

### 2.3 Principal y autorización

- `tls.Config.ClientAuth = RequireAndVerifyClientCert` en el listener API, TLS 1.3 o piso justificado por compatibilidad. CA de cliente explícita; el certificado servidor debe cubrir el DNS anunciado. Probes en otro listener solo pod-local, sin endpoints de datos.
- Archivo privado de política cargado al iniciar: `version: 1`, entradas `{certificate_sha256,principal_id,roles}`. Huella SHA-256 del DER en minúsculas; roles permitidos `session_client`, `approver`, `tool_operator`, `admin`. Rechazar duplicados y roles desconocidos. Un certificado validado pero ausente del mapa recibe `403`.
- Una sesión pertenece a un `principal_id`; leer, continuar o cancelar requiere ser propietario o `admin`. `approver` ve el detalle necesario de llamadas pendientes y decide, sin acceso implícito a transcriptos completos. `tool_operator` crea llamadas directas, sujetas a política de herramientas. `admin` no cambia la política por la API v1.
- Herramientas locales y MCP mantienen revisión manual por defecto. Solo un registro persistido con aprobación `auto` puede saltar la decisión. La llamada directa nunca usa mTLS como aprobación. Rechazar nombres desconocidos incluso si el modelo los propuso.
- Auditar principal, acción, herramienta exacta, decisión, estado y correlación; omitir argumentos, resultado y secretos por defecto. La API no escribe credenciales en el JSONL diagnóstico.

## 3. Persistencia, concurrencia y fallos

1. Mantener el store de sesiones actual como fuente de snapshots; añadir `owner`, `revision` y `operation_id` sin romper sesiones viejas. Migración de schema con lectura de versión anterior; probar carga de fixture previo.
2. Crear journal por sesión con secuencia monótona, checksum o framing que detecte cola truncada, escritura atómica y archivo owner-only. Al arrancar, recuperar hasta el último registro válido y marcar operación inconclusa como `interrupted`, nunca como completada. Compactar solo tras conservar un snapshot y un cursor mínimo documentado; devolver `410` a clientes que pidan eventos podados.
3. Persistir la clave de idempotencia y el hash de petición **antes** de aceptar un turno/call. Serializar una operación por sesión. Llamadas directas usan su propio lock por call y un límite global de concurrencia.
4. Antes de ejecutar un efecto externo, registrar `execution_started`. Si el proceso cae o la conexión se pierde sin resultado, registrar/reconstruir estado `uncertain` y exigir inspección humana. No reintentar automáticamente `write`, `edit`, `exec` ni MCP con efectos. `request_id` y la clave de idempotencia evitan crear otra intención, pero no prometen exactamente una ejecución frente a un fallo del destino.
5. Guardar decisión y resultado de una herramienta antes de avanzar el modelo. Mantener el orden de llamadas que ya exige `tui/domain/session.go`. Al reiniciar, una aprobación pendiente sigue pendiente y requiere nueva decisión explícita; un efecto `uncertain` bloquea avance automático.
6. Una réplica/volumen persistente en Helm. PVC con permisos compatibles con UID no root. Rechazar `replicaCount > 1` en schema o plantilla. Documentar backup del volumen de AXLR y restauración separada de KMP/MADE, cuyos stores no están en ese PVC.

## 4. Integración remota de KMP y MADE

### 4.1 KMP

- Imagen de AXLR incluye `kmp-mcp` release-matched y checksummed para `linux/amd64` y `linux/arm64`; no ejecuta un kernel embebido en el pod remoto.
- Registro MCP stdio con `command` absoluto, ID `kmp`, `allow_tools:["*"]`, propósito `memory` y política manual de inicio. Entorno completo: `KMP_MCP_BACKEND=grpc`, `KMP_KERNEL_GRPC_ENDPOINT`, `KMP_KERNEL_GRPC_TLS_MODE=mutual`, `KMP_KERNEL_GRPC_TLS_CA_PATH`, `KMP_KERNEL_GRPC_TLS_CERT_PATH`, `KMP_KERNEL_GRPC_TLS_KEY_PATH`, `KMP_KERNEL_GRPC_TLS_DOMAIN_NAME`. Rutas TLS apuntan a Secret montado.
- La URL apunta al KMP enterprise existente. Seleccionar el store remoto y las autorizaciones en KMP, no en un directorio `.kernel/` del pod. Verificar `kmp_wake` de lectura y respuesta `UNKNOWN` como respuesta válida, además de la salud de transporte.

### 4.2 MADE

- Imagen incluye `made-mcp` release-matched y checksummed; backend `grpc`, no `embedded`.
- Registro MCP stdio con ID `made`, `allow_tools:["*"]`, propósito `ceremony` y política manual de inicio. Entorno completo: `MADE_MCP_BACKEND=grpc`, `MADE_MCP_GRPC_ENDPOINT`, `MADE_MCP_GRPC_TLS_MODE=mutual`, `MADE_MCP_GRPC_TLS_CA_PATH`, `MADE_MCP_GRPC_TLS_CERT_PATH`, `MADE_MCP_GRPC_TLS_KEY_PATH`, `MADE_MCP_GRPC_TLS_DOMAIN_NAME`.
- MADE remoto ya debe tener política de autorización y principal de host configurados en su despliegue. Verificar `made_discover_capabilities` y `made_get_help`, luego un procedimiento inocuo autorizado. Una herramienta descubierta no garantiza que el host soporte su paso concreto.

### 4.3 Fallo y ciclo de vida

- Iniciar AXLR con ambos registros obligatorios en perfil Helm. Si falla la autenticación, DNS, catálogo o versión compatible, `readyz` falla y las llamadas devuelven `engine_unavailable` con ID preciso; el proceso puede seguir vivo para diagnóstico. Readiness consulta el estado cacheado de inicialización y conexión, actualizado mediante reconexión con backoff; una probe no escribe memoria, inicia ceremonias ni genera una respuesta de modelo facturable.
- No hacer fallback silencioso a KMP/MADE embebidos, ni crear stores vacíos en el pod. No copiar sus credenciales a `mcp.json`/ConfigMap; generar registros desde rutas de Secret y configuración no secreta.
- Rotación de certificados: montar Secrets actualizables, vigilar cambio o reiniciar pod de modo controlado; volver a conectar MCP sin reejecutar una llamada con efecto. Probar revocación/CA incorrecta y DNS incorrecto.

## 5. Helm: valores y manifiestos

### 5.1 Valores propuestos

`replicaCount: 1`; `image.repository`, `image.digest`; `workspace.existingClaim` y `workspace.mountPath`; `state.existingClaim` o `state.volumeClaimTemplate` (persistencia obligatoria para sesiones); `service.port`, `service.type: ClusterIP`; `api.serverTLS.existingSecret`; `api.clientCA.existingSecret`; `api.principals.existingSecret`; `model.apiKey.existingSecret`; `kmp.endpoint`, `kmp.tls.existingSecret`, `kmp.tls.serverName`; `made.endpoint`, `made.tls.existingSecret`, `made.tls.serverName`; `resources`; `networkPolicy.enabled`; `ingress.enabled:false`.

`values.schema.json` exige los Secret refs, DNS, endpoints HTTPS/gRPC válidos, digest no vacío, volumen de estado, workspace fijo y réplica 1. El chart no genera CA, claves, credenciales, stores ni policy de MADE. El ejemplo `values.example.yaml` usa solo nombres de Secrets y placeholders no ejecutables. Un Secret de cada motor contiene `ca.crt`, `tls.crt`, `tls.key`; el Secret API contiene certificado/clave de servidor, CA de cliente y mapa de principales (pueden ser Secret refs separados).

### 5.2 Recursos y defaults

- Deployment sin token de service account montado, UID/GID no root, `readOnlyRootFilesystem`, `allowPrivilegeEscalation:false`, capacidades Linux descartadas y `seccompProfile:RuntimeDefault`. Volúmenes de solo lectura para credenciales; PVC de estado y workspace; `emptyDir` acotado para temporal.
- Service `ClusterIP` en puerto API. Listener de probes HTTP en `127.0.0.1:9090`, sin rutas de datos ni Service. Kubernetes usa probes `exec`: `["/usr/local/bin/axlr-serve","probe","--kind","live","--address","127.0.0.1:9090"]`, y `--kind ready` para readiness. El subcomando solo consulta el endpoint local correspondiente, con timeout de un segundo, y devuelve exit code `0` para HTTP `200` y `1` en otro caso; no carga claves ni inicia el agente. Una probe `httpGet` del kubelet no alcanza ese loopback y no debe generarse. Probes no revelan nombres de herramienta ni secretos. `PodDisruptionBudget` opcional coherente con una réplica.
- NetworkPolicy opcional con ingreso solo de clientes/ingress previstos y salida a OpenRouter, KMP, MADE y DNS. No inventar selectors universales; documentar los que el operador debe completar.
- `helm template` falla si falta un motor, Secret, digest o PVC requerido. Las plantillas nunca incluyen bytes de secretos. Etiquetas llevan chart/app version. `helm upgrade` conserva PVC y no borra estados al desinstalar chart.

## 6. CI y versiones, tomando MADE como referencia

### 6.1 Preflight único

Crear `scripts/release/preflight.py` y pruebas de tabla. En PR/main: producir versión de desarrollo `0.0.0-dev+<sha>` sin publicar. En tag `vMAJOR.MINOR.PATCH` o prerelease semver: verificar que tag, `Chart.yaml` `version`/`appVersion` y la versión inyectada en binarios coinciden; rechazar tag malformado, checkout sucio en empaque local, asset repetido, versión sin lock de motores o commit fuera de la rama de release acordada. Generar inventario esperado **antes** de compilar: seis archivos binarios, sus seis `.sha256`, chart `.tgz` y checksum, imagen multiarch/digest metadata y SBOM/attestation si se activa esa política. Mantener el inventario como artefacto inmutable del job.

### 6.2 Portabilidad antes de la matriz

El código actual usa `Setpgid`, señales de grupo, `Flock`, `O_NOFOLLOW`, `O_NONBLOCK` y fixtures `/bin/sh` sin una implementación Windows equivalente. No basta con cambiar `GOOS`. Separar esas operaciones tras helpers: los archivos `_unix.go` requieren `//go:build unix` explícito; los `_windows.go` se seleccionan por el sufijo de sistema. Revisar también los build tags de sus tests:

- **Procesos:** conservar cancelación de todo el grupo en Unix. En Windows, asignar el proceso a un Job Object con cierre que termine sus descendientes; probar timeout y cancelación con un hijo que cree otro proceso. No sustituirlo por matar solo al padre.
- **Archivos privados y locks:** conservar comprobación sobre el descriptor abierto, rechazo de reparse points/symlinks en archivos de estado y adquisición exclusiva no bloqueante. Windows necesita `LockFileEx` y ACL limitada al propietario; `chmod 0600` por sí solo no demuestra privacidad allí. Conservar validación de archivos regulares y límites antes de leer.
- **Entorno y rutas:** probar volúmenes y separadores Windows, rutas absolutas, variables y herramientas `.exe`. Adaptar los defaults XDG de estado/configuración y documentar el fallback nativo. El perfil remoto Helm continúa siendo Linux.
- **Fixtures:** sustituir pruebas dependientes de `/bin/sh` y `Mkfifo` por helpers de prueba nativos o limitar solo esas pruebas con tags, aportando una prueba equivalente de la propiedad que protegían. Un skip no demuestra portabilidad.

Antes del empaquetado, `go build`, `go vet`, tests y un smoke de lectura/edición/exec/cancelación deben pasar en cada OS nativo para ambos módulos y para el servicio cuando exista. Registrar el soporte verificado en documentación; no extender la promesa actual de Linux hasta completar este paso.

### 6.3 Matriz de binarios

| Target | Artefacto | Prueba requerida |
|:--|:--|:--|
| `linux/amd64` | `axlr-vX.Y.Z-linux-amd64.tar.gz` | Native Ubuntu x64 |
| `linux/arm64` | `axlr-vX.Y.Z-linux-arm64.tar.gz` | Native Ubuntu ARM64 |
| `darwin/amd64` | `axlr-vX.Y.Z-darwin-amd64.tar.gz` | Native macOS Intel |
| `darwin/arm64` | `axlr-vX.Y.Z-darwin-arm64.tar.gz` | Native macOS ARM |
| `windows/amd64` | `axlr-vX.Y.Z-windows-amd64.zip` | Native Windows x64 |
| `windows/arm64` | `axlr-vX.Y.Z-windows-arm64.zip` | Native Windows ARM64 |

Cada archivo incluye `axlr`, `axlr-tui`, `axlr-serve` (con `.exe` en Windows), `VERSION`, licencia y README de instalación; el empaquetador verifica nombre, arquitectura y versión de los tres binarios. Construir con `CGO_ENABLED=0`, `-trimpath`, versión por `-ldflags`, `GOWORK=off` para raíz y `go -C tui` para los otros dos. No cambiar el worker a proceso persistente. Ejecutar `go vet` y `go test -race` en runners nativos pertinentes; los cross-builds no cuentan como smoke nativo. Antes de publicar, confirmar etiquetas de runners disponibles en GitHub o aportar runners propios, especialmente para Windows ARM64; si falta uno, fallar la release o declarar ese target no soportado con aprobación explícita. No etiquetar seis targets como verificados cuando solo cuatro arrancaron.

### 6.4 Paquete, imagen y chart

- `scripts/release/package.sh` recibe versión, GOOS y GOARCH, crea staging nuevo, compila ambos módulos, prueba `--version`, empaqueta orden estable y genera SHA-256. Test local de reproducibilidad: dos empaques del mismo commit producen igual checksum salvo metadatos deliberados documentados.
- Construir imagen Linux `amd64/arm64` desde el mismo commit y versión. Descargar adaptadores KMP/MADE según `distribution/engines.lock.json`, verificar checksum antes de copiarlos. Probar arranque, `livez`, rechazo de cliente sin certificado y fallo de `readyz` sin motores. Publicar digest multiarch; jamás `latest` como identidad de release.
- `helm lint`, `helm template` con overlay de prueba y validación de schema. Publicar chart OCI en `ghcr.io/underpass-ai/charts/axlr` con versión correspondiente **después** de publicar y verificar el digest de imagen. Adjuntar el chart `.tgz` y checksum a GitHub Release. El chart renderizado usa imagen por digest.

### 6.5 Permisos y publicación

Seguir la separación de MADE: jobs de preflight, tests, matriz y empaquetado con `contents: read`; solo un job final, condicionado a tag, usa `contents: write` y `packages: write`. Ese job no hace checkout ni ejecuta scripts de la PR: descarga artefactos, compara inventario exacto y verifica cada checksum antes de crear/adjuntar la release. En reejecución, si existe un asset con el mismo nombre compara bytes; no sobreescribe un asset distinto. Rechaza assets inesperados y releases draft o prerelease con identidad distinta. Pin de acciones por SHA revisado, `timeout-minutes`, `concurrency` por ref y sin publicar desde pull requests.

## 7. Orden de implementación y pruebas por cambio

Cada paso deja el repositorio compilando; no fusionar un paso si falta su prueba. La implementación puede repartirse en PRs en este orden:

1. **Contrato y versión.** Añadir OpenAPI, fixtures de respuestas/eventos y `--version` para los tres binarios cuando exista `axlr-serve`. Test de parsing estricto, ids, status y versión inyectada. No anunciar API todavía.
2. **Persistencia de servicio.** Extender snapshots con propietario/revisión; journal e idempotencia. Tests de reinicio, cola truncada, dos solicitudes concurrentes, misma clave/mismo cuerpo, misma clave/cuerpo distinto y snapshot antiguo. Probar que nunca se ejecuta un efecto al cargar una sesión.
3. **Coordinador agéntico.** Adaptar `StartTurnUseCase`, `AgentTurnUseCase`, `ResolveToolUseCase` al coordinador HTTP. Fake model/MCP deterministas. Comparar la secuencia de estados con la TUI para prompt, tool call, aprobación, denegación y final.
4. **API mTLS y roles.** Crear `axlr-serve`, config y middleware. Tests con CA de prueba: cliente válido mapeado, válido sin rol, sin certificado, CA equivocada, certificado expirado, DNS inválido, rol insuficiente. Verificar que ningún handler de datos está en el listener de probes.
5. **Sesiones, SSE y cancelación.** Implementar rutas de sesiones/turnos/eventos. Test de replay con `Last-Event-ID`, orden, reconexión, heartbeat, cliente lento, cierre de cliente, cancelación antes/durante modelo y restart.
6. **Herramientas directas.** Crear recurso de llamada, decisión y resultado usando registro/política ya existente. Tests `read`, `write`, `edit`, `exec`, MCP, nombre desconocido, JSON inválido, denegación, auto policy explícita, timeout y estado `uncertain` tras fallo simulado. Nunca reintentar efectos automáticamente.
7. **Motores remotos.** Implementar registros internos y packaging de adaptadores. Tests de entorno completo, KMP/MADE exact IDs, certificados, endpoint caído, versión incompatible, `readyz` y ausencia de fallback local. Smoke contra despliegues de prueba de ambos motores.
8. **Helm.** Chart y Dockerfile. `helm lint/template` de perfiles válidos y pruebas negativas de cada Secret/ref/digest/motor/PVC ausente; verificar recursos de seguridad y borrado del release sin borrar stores externos.
9. **Portabilidad nativa.** Separar primitivas de archivo, locks y procesos; implementar Windows y verificar macOS. Pruebas de permisos, bloqueo concurrente, rutas, descendientes y cancelación por OS; los dos módulos y el servicio deben compilar y funcionar antes de habilitar un target.
10. **Release CI.** Preflight, empaquetador, matriz de seis y job final sin checkout, más imagen/chart. Test de inventario faltante/sobrante, checksum alterado, tag y versiones incoherentes, release existente diferente y PR sin permisos de escritura. Hacer release candidata antes de `v1.0.0` y descargar/probar cada archivo.
11. **Documentación de operación.** Actualizar README, `docs/index.md`, arquitectura, consola, API, Helm, runbooks local/remoto y troubleshooting contra binarios/manifest reales. Eliminar frases de “API en diseño” solo cuando los tests y release estén publicados.

## 8. Criterios de aceptación y límites

- Cliente con certificado y rol adecuados crea sesión, recibe eventos reanudables, aprueba una herramienta y obtiene resultado; cliente sin certificado o rol no puede hacerlo.
- Turnos de TUI y API pasan por los mismos casos de uso y respetan el mismo catálogo exacto, límite de 32 llamadas y política de aprobación. El servicio ejecuta herramientas locales y MCP; un modelo no concede capacidades por mencionarlas.
- Reinicio, cancelación y timeout preservan el estado; una ejecución incierta nunca se repite silenciosamente. Las claves de idempotencia evitan intenciones duplicadas sin prometer ejecución exactamente una vez fuera de AXLR.
- Helm despliega una réplica de AXLR con mTLS y PVC, conecta KMP/MADE externos por mTLS, no genera stores locales de los motores y falla readiness al perder uno. El chart no instala ni borra KMP o MADE.
- Cada tag publica seis archivos con checksums, imagen Linux multiarch y chart de la misma versión; se prueban los binarios en su arquitectura nativa. Si un runner no existe o falla un target, la release completa se detiene.
- `go test`, `go vet`, pruebas de carrera, OpenAPI/fixtures, render de Helm y smoke de artefactos pasan antes del job con permisos de publicación. Documentación de API y Helm coincide con la release ejecutable.

Quedan fuera de v1: múltiples tenants por servidor, escalado horizontal, almacén distribuido de sesiones, instalación de KMP/MADE por el chart, gateway KMP HTTP/OIDC como segunda ruta, SDKs de terceros y compatibilidad automática con clientes OpenAI. Esas extensiones requieren contratos y pruebas propios.
