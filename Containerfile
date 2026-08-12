# ─────────────────────────────────────────────────────────────────────────────
# servicio-demo — imagen del servicio de demostración del flujo GitOps
#
# Buenas prácticas que ilustra:
#   · base certificada de Red Hat (UBI mínima), no una imagen community
#   · imagen mínima: solo lo indispensable y se limpia la caché en la misma capa
#   · usuario NO-root (uid 1001), como exige OpenShift por defecto (SCC)
#   · orden de capas de lo que menos cambia (base) a lo que más cambia (código)
#   · sin secretos horneados en la imagen
#
# La construye el Pipeline de Tekton definido en el repositorio workshop-config
# (platform/pipelines). Para construirla en local:
#   podman build -t servicio-demo:1.0.0 .
#   podman run --rm -p 8080:8080 servicio-demo:1.0.0
#   curl -s localhost:8080 | python3 -m json.tool
# ─────────────────────────────────────────────────────────────────────────────

FROM registry.access.redhat.com/ubi9/ubi-minimal:9.6

LABEL org.opencontainers.image.title="servicio-demo" \
      org.opencontainers.image.description="Servicio de demostración del flujo GitOps del workshop" \
      org.opencontainers.image.vendor="Red Hat Consulting"

RUN microdnf install -y python3 \
    && microdnf clean all \
    && rm -rf /var/cache/dnf

WORKDIR /app

# El código se copia al final: es lo que más cambia (mejor uso de la caché).
COPY app.py .

# No-root: OpenShift asigna un UID arbitrario del rango del proyecto; se declara
# el 1001 y se deja el directorio accesible al grupo root (gid 0), patrón OCP.
RUN chgrp -R 0 /app && chmod -R g=u /app
USER 1001

EXPOSE 8080

CMD ["python3", "app.py"]
