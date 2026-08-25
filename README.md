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

## Contenido

| Archivo | Propósito |
|---|---|
| `main.go` | Servidor HTTP y endpoints (biblioteca estándar de Go, sin dependencias) |
| `ui.go` | Página de visualización del reparto de tráfico, embebida en el binario |
| `usuario.go` | Resolución del usuario efectivo bajo el UID arbitrario de OpenShift |
| `Containerfile` | Construcción multi-etapa: `go-toolset` compila, UBI mínima ejecuta |

Una **sola imagen** sirve a los tres servicios del workshop (`demo-service`,
`canary-service` v1/v2 y `circuit-breaker-service`). Lo que cambia entre ellos
son las variables de entorno, no el código: así el material demuestra el
enrutado y las políticas sin distraer con lógica de negocio distinta.

## Endpoints

| Método y ruta | Respuesta |
|---|---|
| `GET /` | Identificación del servicio: versión, entorno, pod, usuario y UID |
| `GET /health` | Sonda de vida y disponibilidad, usada por las probes de Kubernetes |
| `GET /api/eco` | Datos de la petición recibida, para comprobar el paso por el gateway |
| `GET /api/error` | Responde **500** a demanda: abre el circuit breaker |
| `GET /api/lento?s=N` | Tarda N segundos (máx. 30): satura el pool de conexiones |
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
- **Circuit breaker**: los cuadritos se vuelven rojos cuando el sidecar corta el
  tráfico, y vuelven al color normal cuando el circuito se cierra.
- **Argo Rollouts**: el color se desplaza a medida que avanzan los pasos.

### Cómo se demuestra el circuit breaker

La `DestinationRule` del workshop expulsa del balanceo la instancia que acumula
**3 respuestas 5xx consecutivas** (`outlierDetection`) y corta cuando se superan
las conexiones del `connectionPool`. Los dos endpoints de fallo provocan cada
caso a demanda:

```bash
# Expulsión por errores: tres llamadas seguidas bastan.
for i in 1 2 3; do curl -s -o /dev/null -w "%{http_code}\n" https://<host>/api/error; done

# Saturación del pool: peticiones lentas en paralelo.
for i in $(seq 20); do curl -s -o /dev/null "https://<host>/api/lento?s=5" & done; wait
```

## Variables de entorno

| Variable | Valor por defecto | Descripción |
|---|---|---|
| `PORT` | `8080` | Puerto de escucha |
| `APP_VERSION` | `0.0.0-local` | Versión que anuncia el servicio |
| `APP_NOMBRE` | `demo-service` | Nombre con el que se identifica el servicio |
| `APP_ENTORNO` | `local` | Entorno lógico (`dev`, `test`) |
| `APP_MENSAJE` | Texto genérico | Mensaje descriptivo configurable |

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
APP_VERSION=1.0.0-local go run .
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
