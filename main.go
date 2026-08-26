// Servicio de demostración del flujo GitOps del workshop.
//
// Un servidor HTTP mínimo, sin dependencias externas (solo biblioteca estándar
// de Go), cuyo propósito es hacer visible el ciclo completo: código -> pipeline
// -> registro de imágenes -> Argo CD -> Connectivity Link.
//
// La respuesta incluye la VERSIÓN del servicio para que, al publicar una imagen
// nueva y sincronizar Argo CD, el cambio se aprecie en la propia respuesta.
//
// Endpoints:
//
//	GET /            identificación del servicio en JSON
//	GET /health      sonda de vida y disponibilidad (liveness / readiness)
//	GET /api/echo    datos de la petición (para ver el paso por el gateway)
//	GET /api/error   responde 500 a demanda (abre el circuit breaker)
//	GET /api/slow    respuesta lenta (satura el pool de conexiones)
//	GET /api/call    llama a otro servicio (muestra el corte en cascada)
//	GET /metrics     contadores en formato Prometheus
//	GET /ui          página que visualiza el reparto de tráfico en vivo
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Estos valores los inyecta el Deployment desde un ConfigMap. Los valores por
// defecto solo aplican al ejecutar la aplicación fuera del cluster.
var (
	puerto  = variable("PORT", "8080")
	version = variable("APP_VERSION", "0.0.0-local")
	entorno = variable("APP_ENTORNO", "local")
	mensaje = variable("APP_MENSAJE", "Servicio de demostración del workshop")
	// El nombre lo inyecta cada Deployment: así una sola imagen sirve para todos
	// los servicios del taller sin que se anuncien todos igual.
	servicio = variable("APP_NOMBRE", "demo-service")
	// Servicio al que llama /api/call. Vacío = este servicio no llama a nadie.
	upstream = variable("APP_UPSTREAM", "")
)

// Cliente con timeout corto a propósito: si el destino tarda, se quiere ver el
// fallo enseguida, no dejar la petición colgada. Sin timeout, el hilo se queda
// esperando y el fallo en cascada tardaría minutos en apreciarse.
var clienteUpstream = &http.Client{Timeout: 5 * time.Second}

// Contadores en memoria; suficiente para enseñar el formato de exposición.
var (
	peticiones atomic.Int64
	errores    atomic.Int64
)

// El limitador reproduce un backend con capacidad finita: cuando /api/lento
// agota los permisos, las peticiones esperan. Es lo que satura el
// connectionPool de la DestinationRule y hace que Envoy responda 503.
var limitador = make(chan struct{}, 5)

func variable(clave, porDefecto string) string {
	if valor := os.Getenv(clave); valor != "" {
		return valor
	}
	return porDefecto
}

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
func identidad() map[string]any {
	host, _ := os.Hostname()
	return map[string]any{
		"servicio": servicio,
		"version":  version,
		"entorno":  entorno,
		"pod":      host,
	}
}

func raiz(w http.ResponseWriter, r *http.Request) {
	// Sin esta comprobación cualquier ruta inexistente respondería 200.
	if r.URL.Path != "/" {
		responder(w, http.StatusNotFound, map[string]any{
			"error": "ruta no encontrada",
			"ruta":  r.URL.Path,
		})
		return
	}
	cuerpo := identidad()
	cuerpo["mensaje"] = mensaje
	cuerpo["usuario"] = usuario() // debe ser NO-root
	cuerpo["uid"] = os.Getuid()
	responder(w, http.StatusOK, cuerpo)
}

func salud(w http.ResponseWriter, r *http.Request) {
	responder(w, http.StatusOK, map[string]any{"estado": "ok", "version": version})
}

func eco(w http.ResponseWriter, r *http.Request) {
	// Las cabeceras permiten comprobar que la petición atravesó el gateway y
	// que la identidad viajó en el token.
	cuerpo := identidad()
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
func fallo(w http.ResponseWriter, r *http.Request) {
	errores.Add(1)
	cuerpo := identidad()
	cuerpo["error"] = "fallo provocado para la demostración del circuit breaker"
	responder(w, http.StatusInternalServerError, cuerpo)
}

// lento ocupa uno de los permisos del limitador durante unos segundos. Con
// suficientes peticiones simultáneas el pool se agota y el sidecar corta.
func lento(w http.ResponseWriter, r *http.Request) {
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

	limitador <- struct{}{}
	defer func() { <-limitador }()
	time.Sleep(time.Duration(segundos) * time.Second)

	cuerpo := identidad()
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
func llamar(w http.ResponseWriter, r *http.Request) {
	if upstream == "" {
		responder(w, http.StatusNotImplemented, map[string]any{
			"error": "este servicio no tiene APP_UPSTREAM configurado",
		})
		return
	}

	ruta := r.URL.Query().Get("path")
	if ruta == "" {
		ruta = "/"
	}

	inicio := time.Now()
	resp, err := clienteUpstream.Get(upstream + ruta)
	transcurrido := time.Since(inicio)

	cuerpo := identidad()
	cuerpo["upstream"] = upstream + ruta
	cuerpo["duracion_ms"] = transcurrido.Milliseconds()

	if err != nil {
		// Sin respuesta: o el destino no contesta, o el sidecar cortó la llamada.
		errores.Add(1)
		cuerpo["error"] = err.Error()
		responder(w, http.StatusBadGateway, cuerpo)
		return
	}
	defer resp.Body.Close()

	cuerpo["upstream_status"] = resp.StatusCode
	// El corte de Envoy llega como 503 SIN haber tocado el destino: por eso se
	// distingue del 500 que devuelve el propio servicio cuando falla de verdad.
	if resp.StatusCode >= 500 {
		errores.Add(1)
		responder(w, http.StatusBadGateway, cuerpo)
		return
	}
	responder(w, http.StatusOK, cuerpo)
}

func metricas(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP demo_peticiones_total Peticiones atendidas por el pod.\n")
	fmt.Fprintf(w, "# TYPE demo_peticiones_total counter\n")
	fmt.Fprintf(w, "demo_peticiones_total{servicio=%q,version=%q} %d\n",
		servicio, version, peticiones.Load())
	fmt.Fprintf(w, "# HELP demo_errores_total Respuestas 5xx provocadas.\n")
	fmt.Fprintf(w, "# TYPE demo_errores_total counter\n")
	fmt.Fprintf(w, "demo_errores_total{servicio=%q,version=%q} %d\n",
		servicio, version, errores.Load())
}

// contar envuelve al enrutador para llevar el total de peticiones atendidas.
func contar(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peticiones.Add(1)
		siguiente.ServeHTTP(w, r)
	})
}

var unaVez sync.Once

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", raiz)
	mux.HandleFunc("/health", salud)
	mux.HandleFunc("/api/echo", eco)
	mux.HandleFunc("/api/error", fallo)
	mux.HandleFunc("/api/slow", lento)
	mux.HandleFunc("/api/call", llamar)
	mux.HandleFunc("/metrics", metricas)
	mux.HandleFunc("/ui", interfaz)

	unaVez.Do(func() {
		log.Printf("%s %s (%s) escuchando en 0.0.0.0:%s como usuario %s",
			servicio, version, entorno, puerto, usuario())
	})

	servidor := &http.Server{
		Addr:    "0.0.0.0:" + puerto,
		Handler: contar(mux),
		// Sin este margen, /api/lento con demoras largas moriría por timeout.
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	log.Fatal(servidor.ListenAndServe())
}
