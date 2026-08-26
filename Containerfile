# ─────────────────────────────────────────────────────────────────────────────
# Containerfile ÚNICO del monorepo: construye cualquier binario de cmd/<APP>.
#
# El argumento APP elige la aplicación (por defecto, demo-service):
#   podman build -t demo-service:1.0.0 .
#   podman build -t api-service:1.0.0 --build-arg APP=api-service .
#   podman run --rm -p 8080:8080 demo-service:1.0.0
#   curl -s localhost:8080 | python3 -m json.tool
#
# Buenas prácticas que ilustra:
#   · construcción MULTI-ETAPA: el compilador no viaja en la imagen final
#   · imágenes certificadas de Red Hat (go-toolset y UBI mínima), no community
#   · binario estático (CGO_ENABLED=0): la imagen final no necesita toolchain
#   · usuario NO-root (uid 1001), como exige OpenShift por defecto (SCC)
#   · sin secretos horneados en la imagen
#
# La construye el Pipeline de Tekton definido en el repositorio
# workshop-demo-app-config (platform/pipelines), que pasa APP en BUILD_ARGS.
# ─────────────────────────────────────────────────────────────────────────────

# APP se declara antes del primer FROM para poder reutilizarlo en ambas etapas.
ARG APP=demo-service

# ── Etapa 1: compilación ─────────────────────────────────────────────────────
FROM registry.access.redhat.com/ubi9/go-toolset:1.26.5 AS builder

# Cada etapa debe re-declarar el ARG para que el valor llegue a sus RUN.
ARG APP

WORKDIR /opt/app-root/src

# go.mod primero: mientras no cambien las dependencias, esta capa se reutiliza.
COPY go.mod ./
RUN go mod download

# El código: cmd/ (cableado del binario) e internal/ (paquetes del servicio,
# incluidos los assets del panel, que van embebidos con go:embed).
COPY cmd/ cmd/
COPY internal/ internal/

# CGO_ENABLED=0 produce un binario estático, sin dependencias de bibliotecas
# del sistema: por eso la imagen final puede ser una UBI mínima.
# -trimpath quita rutas de compilación; -ldflags "-s -w" descarta la tabla de
# símbolos y la información de depuración, reduciendo el tamaño del binario.
# El binario sale siempre como /tmp/service: así la etapa final no depende del
# nombre de la app y el CMD (forma exec, sin expansión de variables) es fijo.
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /tmp/service "./cmd/${APP}"

# ── Etapa 2: imagen final ────────────────────────────────────────────────────
FROM registry.access.redhat.com/ubi9/ubi-minimal:9.8

# Se re-declara para que las LABEL reflejen la app realmente construida.
ARG APP=demo-service

LABEL org.opencontainers.image.title="${APP}" \
      org.opencontainers.image.description="Aplicación ${APP} del monorepo del workshop GitOps" \
      org.opencontainers.image.source="https://github.com/rh-workshop/workshop-demo-app" \
      org.opencontainers.image.vendor="Red Hat Consulting"

WORKDIR /app

# Solo viaja el binario: ni compilador, ni código fuente, ni módulos.
COPY --from=builder /tmp/service /app/service

# OpenShift asigna un UID ARBITRARIO del grupo root (gid 0). Dar al grupo los
# mismos permisos que al propietario es lo que permite que ese usuario lea la
# aplicación; sin esto el contenedor no arranca bajo la SCC por defecto.
RUN chgrp -R 0 /app && chmod -R g=u /app

# UID numérico (no un nombre): OpenShift lo sustituye por el suyo al desplegar.
USER 1001

EXPOSE 8080

# Forma exec: el binario es el PID 1 y recibe las señales de parada.
CMD ["/app/service"]
