"""
Servicio de demostración del flujo GitOps del workshop.

Un servidor HTTP mínimo, sin dependencias externas, cuyo propósito es hacer
visible el ciclo completo: código -> pipeline -> registro de imágenes ->
Argo CD -> Connectivity Link.

La respuesta incluye la VERSIÓN del servicio para que, al publicar una imagen
nueva y sincronizar Argo CD, el cambio se aprecie en la propia respuesta.

Endpoints:
  GET /         identificación del servicio en JSON
  GET /health   sonda de vida y disponibilidad (liveness / readiness)
  GET /api/eco  devuelve datos de la petición (útil para ver el paso por el gateway)
"""
import json
import os
import pwd
import socket
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = int(os.environ.get("PORT", "8080"))

# Estos valores los inyecta el Deployment desde un ConfigMap. Los valores por
# defecto solo aplican al ejecutar la aplicación fuera del cluster.
VERSION = os.environ.get("APP_VERSION", "0.0.0-local")
ENTORNO = os.environ.get("APP_ENTORNO", "local")
MENSAJE = os.environ.get("APP_MENSAJE", "Servicio de demostración del workshop")


def _usuario_actual() -> str:
    """Devuelve el nombre del usuario efectivo; el UID numérico si no tiene nombre."""
    uid = os.getuid()
    try:
        return pwd.getpwuid(uid).pw_name
    except KeyError:
        return f"uid={uid}"


class Handler(BaseHTTPRequestHandler):
    def _responder(self, status: int, cuerpo: dict) -> None:
        payload = json.dumps(cuerpo, ensure_ascii=False, indent=2).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def do_GET(self) -> None:
        if self.path == "/health":
            self._responder(200, {"estado": "ok", "version": VERSION})
            return

        if self.path.startswith("/api/eco"):
            # Las cabeceras permiten comprobar que la petición atravesó el
            # gateway y que la identidad viajó en el token.
            self._responder(200, {
                "servicio": "servicio-demo",
                "version": VERSION,
                "ruta": self.path,
                "cabeceras_reenvio": {
                    "x-forwarded-for": self.headers.get("x-forwarded-for", ""),
                    "x-forwarded-proto": self.headers.get("x-forwarded-proto", ""),
                    "autorizacion_presente": bool(self.headers.get("authorization")),
                },
            })
            return

        self._responder(200, {
            "servicio": "servicio-demo",
            "version": VERSION,
            "entorno": ENTORNO,
            "mensaje": MENSAJE,
            "pod": socket.gethostname(),
            "usuario": _usuario_actual(),   # debe ser NO-root
            "uid": os.getuid(),
        })

    def log_message(self, fmt, *args):  # trazas a stdout (patrón de contenedor)
        print("%s - %s" % (self.address_string(), fmt % args), flush=True)


if __name__ == "__main__":
    print(
        f"servicio-demo {VERSION} ({ENTORNO}) escuchando en 0.0.0.0:{PORT} "
        f"como usuario {_usuario_actual()}",
        flush=True,
    )
    ThreadingHTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
