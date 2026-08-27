package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rh-workshop/workshop-demo-app/internal/identity"
)

// responder serializa el cuerpo como JSON con el código indicado.
func responder(w http.ResponseWriter, status int, cuerpo map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	// SetEscapeHTML(false) conserva los acentos legibles en la respuesta.
	enc.SetEscapeHTML(false)
	_ = enc.Encode(cuerpo)
}

// identidad devuelve los datos que identifican a esta instancia. El pod es
// clave en las demostraciones: revela QUÉ réplica atendió cada petición.
func (s *Servidor) identidad() map[string]any {
	host, _ := os.Hostname()
	return map[string]any{
		"servicio": s.cfg.Servicio,
		"version":  s.cfg.Version,
		"entorno":  s.cfg.Entorno,
		"pod":      host,
	}
}

// raiz identifica al servicio: versión, entorno, pod, usuario y UID.
func (s *Servidor) raiz(w http.ResponseWriter, r *http.Request) {
	// Sin esta comprobación cualquier ruta inexistente respondería 200.
	if r.URL.Path != "/" {
		responder(w, http.StatusNotFound, map[string]any{
			"error": "ruta no encontrada",
			"ruta":  r.URL.Path,
		})
		return
	}
	cuerpo := s.identidad()
	cuerpo["mensaje"] = s.cfg.Mensaje
	cuerpo["usuario"] = identity.Usuario() // debe ser NO-root
	cuerpo["uid"] = os.Getuid()
	responder(w, http.StatusOK, cuerpo)
}

// salud es la sonda de vida y disponibilidad (liveness / readiness).
func (s *Servidor) salud(w http.ResponseWriter, r *http.Request) {
	responder(w, http.StatusOK, map[string]any{"estado": "ok", "version": s.cfg.Version})
}

// eco devuelve datos de la petición recibida. Las cabeceras permiten comprobar
// que la petición atravesó el gateway y que la identidad viajó en el token.
func (s *Servidor) eco(w http.ResponseWriter, r *http.Request) {
	cuerpo := s.identidad()
	cuerpo["ruta"] = r.URL.Path
	cuerpo["cabeceras_reenvio"] = map[string]any{
		"x-forwarded-for":   r.Header.Get("x-forwarded-for"),
		"x-forwarded-proto": r.Header.Get("x-forwarded-proto"),
		// La AuthPolicy de Connectivity Link propaga aquí la identidad validada.
		"x-identidad":           r.Header.Get("x-identidad"),
		"autorizacion_presente": r.Header.Get("authorization") != "",
	}
	responder(w, http.StatusOK, cuerpo)
}

// fallo responde 500 a demanda. La outlierDetection de la DestinationRule
// expulsa del balanceo la instancia que acumula 3 errores consecutivos.
func (s *Servidor) fallo(w http.ResponseWriter, r *http.Request) {
	s.errores.Add(1)
	cuerpo := s.identidad()
	cuerpo["error"] = "fallo provocado para la demostración del circuit breaker"
	responder(w, http.StatusInternalServerError, cuerpo)
}

// lento ocupa uno de los permisos del limitador durante unos segundos. Con
// suficientes peticiones simultáneas el pool se agota y el sidecar corta.
func (s *Servidor) lento(w http.ResponseWriter, r *http.Request) {
	segundos := 3
	if valor := r.URL.Query().Get("s"); valor != "" {
		if n, err := strconv.Atoi(valor); err == nil && n > 0 {
			segundos = n
		}
	}
	// Tope defensivo: nadie deja un pod colgado media hora desde la barra del navegador.
	if segundos > 30 {
		segundos = 30
	}

	s.limitador <- struct{}{}
	defer func() { <-s.limitador }()
	time.Sleep(time.Duration(segundos) * time.Second)

	cuerpo := s.identidad()
	cuerpo["demora_segundos"] = segundos
	responder(w, http.StatusOK, cuerpo)
}

// llamar invoca a OTRO servicio y devuelve lo que responde, midiendo cuánto
// tardó. Es lo que hace VISIBLE el circuit breaker: mientras el destino falla,
// cada llamada agota el timeout; en cuanto Envoy abre el circuito, el 503 llega
// de inmediato. La diferencia entre esperar y fallar rápido es justo lo que el
// patrón aporta, y sin dos servicios no hay forma de enseñarla.
//
// El destino sale de APP_UPSTREAM (el Service del otro servicio); el parámetro
// "path" elige a qué endpoint suyo se llama, para poder pedirle que falle.
func (s *Servidor) llamar(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Upstream == "" {
		responder(w, http.StatusNotImplemented, map[string]any{
			"error": "este servicio no tiene APP_UPSTREAM configurado",
		})
		return
	}

	ruta := r.URL.Query().Get("path")
	if ruta == "" {
		ruta = "/"
	}
	// Anti-SSRF: solo rutas relativas al upstream; sin esto, "path=@evil/x" o
	// "path=//evil/x" reescriben la autoridad de la URL y la llamada server-side
	// sale hacia un host arbitrario alcanzable desde el pod.
	if destino, err := url.Parse(ruta); err != nil || destino.IsAbs() || destino.Host != "" || !strings.HasPrefix(destino.Path, "/") {
		responder(w, http.StatusBadRequest, map[string]any{
			"error": "path debe ser una ruta relativa que empiece por \"/\"",
			"path":  ruta,
		})
		return
	}

	inicio := time.Now()
	resp, err := s.clienteUpstream.Get(s.cfg.Upstream + ruta)
	transcurrido := time.Since(inicio)

	cuerpo := s.identidad()
	cuerpo["upstream"] = s.cfg.Upstream + ruta
	cuerpo["duracion_ms"] = transcurrido.Milliseconds()

	if err != nil {
		// Sin respuesta: o el destino no contesta, o el sidecar cortó la llamada.
		s.errores.Add(1)
		cuerpo["error"] = err.Error()
		responder(w, http.StatusBadGateway, cuerpo)
		return
	}
	defer resp.Body.Close()

	cuerpo["upstream_status"] = resp.StatusCode
	// El corte de Envoy llega como 503 SIN haber tocado el destino: por eso se
	// distingue del 500 que devuelve el propio servicio cuando falla de verdad.
	if resp.StatusCode >= 500 {
		s.errores.Add(1)
		responder(w, http.StatusBadGateway, cuerpo)
		return
	}
	responder(w, http.StatusOK, cuerpo)
}

// metricas expone los contadores en formato de texto de Prometheus.
func (s *Servidor) metricas(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP demo_peticiones_total Peticiones atendidas por el pod.\n")
	fmt.Fprintf(w, "# TYPE demo_peticiones_total counter\n")
	fmt.Fprintf(w, "demo_peticiones_total{servicio=%q,version=%q} %d\n",
		s.cfg.Servicio, s.cfg.Version, s.peticiones.Load())
	fmt.Fprintf(w, "# HELP demo_errores_total Respuestas 5xx provocadas.\n")
	fmt.Fprintf(w, "# TYPE demo_errores_total counter\n")
	fmt.Fprintf(w, "demo_errores_total{servicio=%q,version=%q} %d\n",
		s.cfg.Servicio, s.cfg.Version, s.errores.Load())
}
