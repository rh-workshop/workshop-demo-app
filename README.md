# workshop-app — código del servicio de demostración

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
vive aquí, sino en el repositorio `workshop-config`. Esa separación entre código
y configuración es intencional y es uno de los principios que enseña el
workshop: Argo CD observa el repositorio de configuración, nunca el de código.

## Contenido

| Archivo | Propósito |
|---|---|
| `app.py` | Servidor HTTP sin dependencias externas (biblioteca estándar de Python) |
| `Containerfile` | Construcción de la imagen sobre UBI 9 mínima, con usuario no-root |

## Endpoints

| Método y ruta | Respuesta |
|---|---|
| `GET /` | Identificación del servicio: versión, entorno, pod, usuario y UID |
| `GET /health` | Sonda de vida y disponibilidad, usada por las probes de Kubernetes |
| `GET /api/eco` | Datos de la petición recibida, para comprobar el paso por el gateway |

La respuesta incluye siempre la **versión**, que se inyecta desde un ConfigMap
mediante la variable de entorno `APP_VERSION`. Al cambiar la versión en el
repositorio de configuración y sincronizar Argo CD, el cambio se aprecia
directamente en la respuesta del servicio: así se comprueba que el despliegue
está realmente gobernado por Git.

## Variables de entorno

| Variable | Valor por defecto | Descripción |
|---|---|---|
| `PORT` | `8080` | Puerto de escucha |
| `APP_VERSION` | `0.0.0-local` | Versión que anuncia el servicio |
| `APP_ENTORNO` | `local` | Entorno lógico (`dev`, `test`) |
| `APP_MENSAJE` | Texto genérico | Mensaje descriptivo configurable |

Ninguna de ellas contiene información sensible. Las credenciales, cuando hacen
falta, se referencian por el **nombre** del Secret y nunca por su valor.

## Ejecución en local

```bash
podman build -t servicio-demo:1.0.0 .
podman run --rm -p 8080:8080 servicio-demo:1.0.0
curl -s localhost:8080 | python3 -m json.tool
```

Para comprobar que el contenedor no corre como root, el campo `usuario` de la
respuesta debe mostrar un usuario distinto de `root` y `uid` distinto de `0`.

## Construcción en el cluster

La construye el Pipeline `construir-y-publicar` definido en `workshop-config`
(`platform/pipelines/`), que clona este repositorio, construye la imagen con
Buildah y la publica en el registro configurado.
