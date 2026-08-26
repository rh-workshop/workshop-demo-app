# workshop-demo-app — monorepo de las aplicaciones del workshop

Este repositorio es un **monorepo**: contiene **el código de todas las
aplicaciones** que se despliegan en el workshop. Hoy son dos, cada una con su
punto de entrada en `cmd/`:

| Aplicación | Propósito |
|---|---|
| `cmd/demo-service` | Servicio de demostración del flujo GitOps y de los patrones de despliegue (canary, blue-green, circuit breaker) |
| `cmd/api-service` | API REST de negocio (cuentas y pagos): propagación de identidad desde el gateway y códigos de estado reales |

Todas comparten un único `go.mod`, un único `Containerfile` (el argumento `APP`
elige qué binario se construye) y los paquetes comunes de `internal/`. El valor
pedagógico no está en la lógica de negocio, sino en hacer visible el ciclo
completo de entrega.

```
código (este repositorio)
  -> Pipeline de Tekton construye la imagen
  -> registro de imágenes
  -> Argo CD la despliega con Kustomize
  -> Connectivity Link la expone con políticas de autenticación y de consumo
```

La **configuración declarativa** (Deployment, Service, políticas, Pipeline) NO
vive aquí, sino en el repositorio `workshop-demo-app-config`. Esa separación
entre código y configuración es intencional y es uno de los principios que
enseña el workshop: Argo CD observa el repositorio de configuración, nunca el de
código. La configuración de los productos de plataforma (Keycloak, Quay, el
gateway compartido) vive a su vez en `workshop-demo-platform-config`.

## Estructura del monorepo

Sigue el layout estándar de Go: `cmd/<app>/main.go` es el punto de entrada de
cada aplicación (solo cableado) e `internal/` los paquetes, separados por
responsabilidad. Todo usa la biblioteca estándar, sin dependencias.

```
cmd/
  demo-service/main.go      punto de entrada del servicio de demostración
  api-service/main.go       punto de entrada de la API de negocio
internal/
  config/                   COMÚN — configuración desde variables de entorno
  identity/                 COMÚN — usuario efectivo bajo el UID arbitrario de OpenShift
  server/                   demo-service — rutas, handlers y métricas del demo
  ui/                       demo-service — panel de reparto de tráfico (go:embed)
  apiservice/               api-service — store en memoria, handlers y contadores
Containerfile               ÚNICO para todas las apps: --build-arg APP=<app>
go.mod                      un solo módulo para todo el monorepo
```

**Criterio de separación entre lo común y lo específico:**

- `internal/config` e `internal/identity` son **comunes**: no saben nada de
  ningún endpoint, solo leen el entorno de ejecución (variables y UID). Ambas
  apps los importan tal cual.
- `internal/server` e `internal/ui` son **propios de demo-service**: sus
  handlers *son* la demo (eco, error a demanda, latencia, panel). Compartirlos
  obligaría a la API de negocio a arrastrar endpoints que no forman parte de su
  contrato.
- `internal/apiservice` es **propio de api-service**: modelo de datos, reglas
  de validación (422), enrutado con métodos (405) y sus propios contadores.
  Cada app expone su `/health` y su `/metrics` porque son parte de su contrato,
  no infraestructura común.

Los assets del panel son ficheros reales (editables como HTML/CSS/JS normales),
pero viajan **embebidos** en el binario: la imagen final sigue siendo un único
ejecutable, sin ficheros estáticos ni volúmenes.

## Cómo se añade una aplicación nueva

1. **Crear el punto de entrada**: `cmd/<nueva-app>/main.go`, solo cableado
   (cargar `internal/config`, construir el servidor, arranque y parada
   ordenada). Copiar `cmd/api-service/main.go` como plantilla.
2. **Crear su paquete**: `internal/<nuevaapp>/` con los handlers y el estado
   propios. Reutilizar `internal/config` e `internal/identity`; no tocar los
   paquetes de otras apps.
3. **Comprobar en local**: `go build ./... && go vet ./...` y
   `go run ./cmd/<nueva-app>`.
4. **La imagen ya sabe construirla** — el `Containerfile` es único:
   `podman build -t <nueva-app>:1.0.0 --build-arg APP=<nueva-app> .`
5. **Declarar su despliegue** en `workshop-demo-app-config`: carpeta
   `apps/<nueva-app>/` (base + overlays + gateway-route + image-puller),
   sus Applications en `bootstrap/applications/` y, si usa un namespace nuevo,
   añadirlo a los `destinations` del AppProject `workshop-platform`.
6. **Construir en el cluster**: un `PipelineRun` de `ci-build-image` con el
   parámetro `build-args: ["APP=<nueva-app>"]` y el `overlay-path` de la app.

Una **sola imagen** sirve a todos los servicios del workshop: `demo-service`,
`canary-service` v1/v2, `bluegreen-service` blue/green y la pareja
`service1-frontend` / `service2-backend` del circuit breaker. Lo que cambia entre
ellos son las variables de entorno, no el código: así el material demuestra el
enrutado y las políticas sin distraer con lógica de negocio distinta.

## Endpoints de api-service

| Método y ruta | Respuesta |
|---|---|
| `GET /api/v1/accounts` | **200** — cuentas del cliente autenticado (o todas, avisando, si no llega identidad) |
| `GET /api/v1/accounts/{id}` | **200** — la cuenta pedida; **404** si no existe |
| `POST /api/v1/payments` | **201** con cabecera `Location`; **422** si el cuerpo es inválido |
| `GET /api/v1/payments/{id}` | **200** — el pago pedido; **404** si no existe |
| `GET /health` | Sonda de vida y disponibilidad |
| `GET /metrics` | Contadores en formato Prometheus |

Un método no permitido sobre una ruta del contrato responde **405** con la
cabecera `Allow`; cualquier ruta fuera del contrato responde **404**.

La API lee la cabecera **`x-identidad`** que inyecta la AuthPolicy de
Connectivity Link tras validar el JWT, y la refleja en todas sus respuestas
(bloque `identity`). Si la cabecera no llega — por ejemplo accediendo al
Service por dentro del cluster, sin pasar por el gateway — la respuesta lo dice
explícitamente: así se enseña la propagación de identidad.

## Endpoints de demo-service

| Método y ruta | Respuesta |
|---|---|
| `GET /` | Identificación del servicio: versión, entorno, pod, usuario y UID |
| `GET /health` | Sonda de vida y disponibilidad, usada por las probes de Kubernetes |
| `GET /api/echo` | Datos de la petición recibida, para comprobar el paso por el gateway |
| `GET /api/error` | Responde **500** a demanda: abre el circuit breaker |
| `GET /api/slow?s=N` | Tarda N segundos (máx. 30): satura el pool de conexiones |
| `GET /api/call?path=/x` | Llama a `APP_UPSTREAM` y mide cuánto tarda la respuesta |
| `GET /metrics` | Contadores de peticiones y errores en formato Prometheus |
| `GET /ui` | Página que visualiza el reparto de tráfico en vivo |

Cualquier otra ruta responde **404**.

La respuesta incluye siempre la **versión**, que se inyecta desde un ConfigMap
mediante la variable de entorno `APP_VERSION`. Al cambiar la versión en el
repositorio de configuración y sincronizar Argo CD, el cambio se aprecia
directamente en la respuesta del servicio: así se comprueba que el despliegue
está realmente gobernado por Git.

### La página `/ui`

Consulta `/` cada 200 ms y pinta un cuadrito por respuesta, con un color por
versión y rojo para los errores. Es la forma de **ver** lo que las políticas
hacen:

- **Canary por peso de `HTTPRoute`**: aparecen dos colores mezclados en la
  proporción configurada; al cambiar el peso, cambia la mezcla.
- **Blue-green**: un único color, y al conmutar los pesos cambia de golpe.
- **Circuit breaker**: los cuadritos se vuelven rojos cuando el sidecar corta el
  tráfico, y vuelven al color normal cuando el circuito se cierra.

### Cómo se demuestra el circuit breaker

El corte se demuestra con DOS servicios: `service1-frontend` llama a
`service2-backend` a través de `/api/call`, y la `DestinationRule` vigila esa
llamada. Con un solo servicio no se aprecia nada, porque `maxEjectionPercent: 50`
impide expulsar la única réplica y el usuario nunca ve el corte.

```bash
# 1) Llamada normal: service1 responde con lo que le dio service2.
curl -s https://<host>/api/call | python3 -m json.tool

# 2) Se pide a service2 que falle. Tras 3 errores seguidos, Envoy lo expulsa.
for i in 1 2 3; do curl -s -o /dev/null -w "%{http_code}\n" "https://<host>/api/call?path=/api/error"; done

# 3) Lo que se OBSERVA es el tiempo: antes del corte cada llamada agota el
#    timeout de 5 s; con el circuito abierto, el 503 llega de inmediato.
curl -s "https://<host>/api/call?path=/api/slow%3Fs=8" | python3 -m json.tool
```

## Variables de entorno

| Variable | Valor por defecto | Descripción |
|---|---|---|
| `PORT` | `8080` | Puerto de escucha |
| `APP_VERSION` | `0.0.0-local` | Versión que anuncia el servicio |
| `APP_NOMBRE` | `demo-service` | Nombre con el que se identifica el servicio |
| `APP_ENTORNO` | `local` | Entorno lógico (`dev`, `test`) |
| `APP_MENSAJE` | Texto genérico | Mensaje descriptivo configurable |
| `APP_UPSTREAM` | *(vacío)* | Servicio al que llama `/api/call`; vacío = no llama a nadie |

Ninguna de ellas contiene información sensible. Las credenciales, cuando hacen
falta, se referencian por el **nombre** del Secret y nunca por su valor.

## Ejecución en local

```bash
# demo-service (APP por defecto)
podman build -t demo-service:1.0.0 .
podman run --rm -p 8080:8080 demo-service:1.0.0
curl -s localhost:8080 | python3 -m json.tool

# api-service — mismo Containerfile, cambia el argumento APP
podman build -t api-service:1.0.0 --build-arg APP=api-service .
podman run --rm -p 8080:8080 api-service:1.0.0
curl -s localhost:8080/api/v1/accounts | python3 -m json.tool
```

Y para ver la página de reparto de tráfico: `http://localhost:8080/ui`.

Para comprobar que el contenedor no corre como root, el campo `usuario` de la
respuesta debe mostrar un usuario distinto de `root` y `uid` distinto de `0`.

Sin contenedor, con Go instalado:

```bash
APP_VERSION=1.0.0-local go run ./cmd/demo-service
```

## Construcción en el cluster

La construye el Pipeline **`ci-build-image`**, definido en
`workshop-demo-app-config` (`platform/pipelines/`): clona este repositorio,
construye la imagen con Buildah y la publica en el registro; después actualiza el
overlay de configuración con el digest recién construido, que es lo que Argo CD
sincroniza. El pipeline **`cd-deploy-application`** fuerza esa sincronización
cuando se quiere desplegar sin esperar al sondeo de Argo CD.

La imagen final pesa unos **5,7 MB de binario** sobre UBI mínima: el compilador
se queda en la etapa de construcción y no viaja al registro ni al cluster.
