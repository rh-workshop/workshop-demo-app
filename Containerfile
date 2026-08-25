# ─────────────────────────────────────────────────────────────────────────────
# demo-service — imagen del servicio de demostración del flujo GitOps
#
# Buenas prácticas que ilustra:
#   · construcción MULTI-ETAPA: el compilador no viaja en la imagen final
#   · imágenes certificadas de Red Hat (go-toolset y UBI mínima), no community
#   · binario estático (CGO_ENABLED=0): la imagen final no necesita toolchain
#   · usuario NO-root (uid 1001), como exige OpenShift por defecto (SCC)
#   · sin secretos horneados en la imagen
#
# La construye el Pipeline de Tekton definido en el repositorio
# workshop-demo-app-config (platform/pipelines). Para construirla en local:
#   podman build -t demo-service:1.0.0 .
#   podman run --rm -p 8080:8080 demo-service:1.0.0
#   curl -s localhost:8080 | python3 -m json.tool
# ─────────────────────────────────────────────────────────────────────────────

# ── Etapa 1: compilación ─────────────────────────────────────────────────────
FROM registry.access.redhat.com/ubi9/go-toolset:1.26.5 AS builder

WORKDIR /opt/app-root/src

# go.mod primero: mientras no cambien las dependencias, esta capa se reutiliza.
COPY go.mod ./
RUN go mod download

COPY *.go ./

# CGO_ENABLED=0 produce un binario estático, sin dependencias de bibliotecas
# del sistema: por eso la imagen final puede ser una UBI mínima.
# -trimpath quita rutas de compilación; -ldflags "-s -w" descarta la tabla de
# símbolos y la información de depuración, reduciendo el tamaño del binario.
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /tmp/demo-service .

# ── Etapa 2: imagen final ────────────────────────────────────────────────────
FROM registry.access.redhat.com/ubi9/ubi-minimal:9.8

LABEL org.opencontainers.image.title="demo-service" \
      org.opencontainers.image.description="Servicio de demostración del flujo GitOps del workshop" \
      org.opencontainers.image.source="https://github.com/rh-workshop/workshop-demo-app" \
      org.opencontainers.image.vendor="Red Hat Consulting"

WORKDIR /app

# Solo viaja el binario: ni compilador, ni código fuente, ni módulos.
COPY --from=builder /tmp/demo-service /app/demo-service

# OpenShift asigna un UID ARBITRARIO del grupo root (gid 0). Dar al grupo los
# mismos permisos que al propietario es lo que permite que ese usuario lea la
# aplicación; sin esto el contenedor no arranca bajo la SCC por defecto.
RUN chgrp -R 0 /app && chmod -R g=u /app

# UID numérico (no un nombre): OpenShift lo sustituye por el suyo al desplegar.
USER 1001

EXPOSE 8080

# Forma exec: el binario es el PID 1 y recibe las señales de parada.
CMD ["/app/demo-service"]
