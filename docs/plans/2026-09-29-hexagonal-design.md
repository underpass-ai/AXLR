# AXLR: diseño hexagonal de la primera entrega

## Intención

AXLR es el nuevo núcleo de ejecución propio de Underpass, reescrito de forma selectiva a partir de nuestro `underpass-runtime`. Expone una biblioteca Go y un worker JSON de una solicitud para `read`, `write`, `edit` y `exec` en Linux local confiable. El host proporciona el workspace, entorno y límites. El worker no decide permisos ni mantiene estado de negocio.

## Capas y flujo

```text
cmd/axlr -> dto/ + runtime/codec -> runtime/mappers -> casos de uso -> puertos -> adaptadores locales
                                      |                  |
                                      +---- dominio ----+
```

El dominio define identidad de solicitud, ruta relativa, digest, límites y comandos/resultados de cada operación. Sus constructores rechazan valores inválidos. Los casos de uso aplican precondiciones y reglas de edición. Los puertos describen acceso a archivos y procesos. Los adaptadores locales implementan los puertos con `os.Root` y `os/exec`. El mapper traduce entre el contrato JSON y tipos de dominio; ninguna etiqueta JSON debe convertirse en autoridad para elevar límites del host.

En Go, “un archivo = una clase” significa **un tipo principal por archivo**, con sus métodos y funciones constructoras en ese mismo archivo. No se crearán clases ficticias ni un archivo por función. Interfaces de puerto y DTOs tienen cada uno su archivo.

## Reglas del dominio

- `RequestID` es correlación, nunca idempotencia.
- `RelativePath` impide rutas absolutas y traversal; el adaptador usa acceso anclado para impedir escapes por symlinks en herramientas de archivos.
- `Digest` es SHA-256 en minúsculas; reemplazo exige el digest esperado, edición puede usarlo.
- Los límites efectivos son el mínimo del perfil del host y el valor solicitado; una petición que intenta superar el perfil se rechaza.
- Crear usa publicación sin sobrescritura; reemplazar y editar usan temporal más rename y no prometen CAS concurrente.
- `exec` no interpreta argv como shell ni hereda el entorno del worker; timeout/cancelación pueden dejar efectos previos.

## Errores y transporte

El codec acepta un documento JSON de hasta 4 MiB, campos conocidos y una herramienta admitida. El mapper produce comandos validados antes de que un caso de uso toque un puerto. La respuesta tiene estado y código de error estables; un exit code no cero sigue siendo `completed`. El sobre serializado no supera 4 MiB. Una respuesta perdida se interpreta como efecto desconocido por el host.

## Pruebas y CI

Pruebas de dominio para los value objects, pruebas de casos de uso para conflictos y límites, pruebas de adaptadores sobre un directorio temporal y procesos auxiliares Go, y pruebas de CLI para framing y exit codes. CI ejecuta formato, `go vet`, tests con race y cobertura agregada superior a 80 %, y build sin CGO. No necesita servicios externos.

## Ampliación: herramientas de plugins

La [especificación de plugins](../superpowers/specs/2026-09-29-plugin-tools-design.md) añade procesos MCP por stdio como adaptadores de herramientas. `plugins/` implementa el puerto definido en `application/`; `domain/` conserva identidades y argumentos tipados sin depender de MCP. El mismo worker acepta manifiestos explícitos y mantiene las herramientas locales. `mcpclient/` forma parte del módulo Go raíz para que la biblioteca y el worker compartan la conexión MCP.

## Fuera de esta entrega

Sandbox, medidas comparativas y cualquier backend remoto. El contrato mantiene el núcleo independiente de posibles consumidores. La relación con el runtime anterior y la referencia conceptual a Pi se describen en [../provenance.md](../provenance.md).
