// Package server construye el servidor HTTP del servicio de demostración:
// rutas, handlers, contadores y timeouts. El arranque y la parada quedan fuera,
// en cmd/demo-service.
package server

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/rh-workshop/workshop-demo-app/internal/config"
)

// Servidor agrupa la configuración y el estado en memoria de los handlers.
type Servidor struct {
	cfg config.Config

	// Cliente con timeout corto a propósito: si el destino tarda, se quiere ver
	// el fallo enseguida, no dejar la petición colgada. Sin timeout, el hilo se
	// queda esperando y el fallo en cascada tardaría minutos en apreciarse.
	clienteUpstream *http.Client

	// Contadores en memoria; suficiente para enseñar el formato de exposición.
	peticiones atomic.Int64
	errores    atomic.Int64

	// El limitador reproduce un backend con capacidad finita: cuando /api/slow
	// agota los permisos, las peticiones esperan. Es lo que satura el
	// connectionPool de la DestinationRule y hace que Envoy responda 503.
	limitador chan struct{}
}

// New construye el http.Server listo para arrancar. El handler del panel llega
// como dependencia para que TODAS las rutas queden declaradas en un solo sitio.
func New(cfg config.Config, panel http.HandlerFunc) *http.Server {
	s := &Servidor{
		cfg:             cfg,
		clienteUpstream: &http.Client{Timeout: 5 * time.Second},
		limitador:       make(chan struct{}, 5),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.raiz)
	mux.HandleFunc("/health", s.salud)
	mux.HandleFunc("/api/echo", s.eco)
	mux.HandleFunc("/api/error", s.fallo)
	mux.HandleFunc("/api/slow", s.lento)
	mux.HandleFunc("/api/call", s.llamar)
	mux.HandleFunc("/metrics", s.metricas)
	mux.HandleFunc("/ui", panel)

	return &http.Server{
		Addr:    "0.0.0.0:" + cfg.Puerto,
		Handler: s.contar(mux),
		// Sin este margen, /api/slow con demoras largas moriría por timeout.
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
}

// contar envuelve al enrutador para llevar el total de peticiones atendidas.
func (s *Servidor) contar(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.peticiones.Add(1)
		siguiente.ServeHTTP(w, r)
	})
}
