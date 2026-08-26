# workshop-demo-app — código del servicio de demostración

Este repositorio contiene **el código** del servicio que se despliega en el
workshop. Es deliberadamente mínimo: su valor pedagógico no está en la lógica de
negocio, sino en hacer visible el ciclo completo de entrega.

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

## Estructura del repositorio

Sigue el layout estándar de Go: `cmd/` contiene el punto de entrada (solo
cableado) e `internal/` los paquetes del servicio, separados por
responsabilidad. Todo usa la biblioteca estándar, sin dependencias.

| Ruta | Propósito |
|---|---|
| `cmd/demo-service/main.go` | Cableado: configuración, construcción del servidor, arranque y parada ordenada |
| `internal/config/` | Carga de la configuración desde variables de entorno |
| `internal/server/` | Construcción del servidor HTTP: rutas, handlers y métricas |
| `internal/identity/` | Resolución del usuario efectivo bajo el UID arbitrario de OpenShift |
| `internal/ui/` | Panel de visualización del reparto de tráfico |
| `internal/ui/assets/` | HTML, CSS y JS del panel, embebidos en el binario con `go:embed` |
| `Containerfile` | Construcción multi-etapa: `go-toolset` compila, UBI mínima ejecuta |

Los assets del panel son ficheros reales (editables como HTML/CSS/JS normales),
pero viajan **embebidos** en el binario: la imagen final sigue siendo un único
ejecutable, sin ficheros estáticos ni volúmenes.

Una **sola imagen** sirve a todos los servicios del workshop: `demo-service`,
`canary-service` v1/v2, `bluegreen-service` blue/green y la pareja
`service1-frontend` / `service2-backend` del circuit breaker. Lo que cambia entre
ellos son las variables de entorno, no el código: así el material demuestra el
enrutado y las políticas sin distraer con lógica de negocio distinta.

## Endpoints

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
podman build -t demo-service:1.0.0 .
podman run --rm -p 8080:8080 demo-service:1.0.0
curl -s localhost:8080 | python3 -m json.tool
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
