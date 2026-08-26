package apiservice

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/rh-workshop/workshop-demo-app/shared/config"
)

// Server agrupa la configuración, los datos y los contadores de la API.
type Server struct {
	cfg   config.Config
	store *Store

	// Contadores en memoria, suficientes para enseñar el formato Prometheus.
	requests        atomic.Int64
	errorsCount     atomic.Int64
	paymentsCreated atomic.Int64
}

// New construye el http.Server con todas las rutas del contrato declaradas.
func New(cfg config.Config) *http.Server {
	s := &Server{cfg: cfg, store: NewStore()}

	// Cada ruta registra su método permitido y, además, un handler SIN método
	// que responde 405: sin él, el catch-all "/" absorbería los métodos no
	// permitidos y la API contestaría 404 donde el contrato exige 405.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/accounts", s.listAccounts)
	mux.HandleFunc("/api/v1/accounts", s.methodNotAllowed("GET"))
	mux.HandleFunc("GET /api/v1/accounts/{id}", s.getAccount)
	mux.HandleFunc("/api/v1/accounts/{id}", s.methodNotAllowed("GET"))
	mux.HandleFunc("POST /api/v1/payments", s.createPayment)
	mux.HandleFunc("/api/v1/payments", s.methodNotAllowed("POST"))
	mux.HandleFunc("GET /api/v1/payments/{id}", s.getPayment)
	mux.HandleFunc("/api/v1/payments/{id}", s.methodNotAllowed("GET"))
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /metrics", s.metrics)
	// Cualquier otra ruta cae aquí y responde 404 con cuerpo JSON.
	mux.HandleFunc("/", s.notFound)

	return &http.Server{
		Addr:         "0.0.0.0:" + cfg.Puerto,
		Handler:      s.count(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
}

// count envuelve al enrutador para llevar el total de peticiones atendidas.
func (s *Server) count(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		next.ServeHTTP(w, r)
	})
}

// writeJSON serializa el cuerpo con el código indicado y cuenta los errores.
func (s *Server) writeJSON(w http.ResponseWriter, status int, body any) {
	if status >= 400 {
		s.errorsCount.Add(1)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// El error de codificación se ignora: a estas alturas ya no hay respuesta mejor.
	_ = json.NewEncoder(w).Encode(body)
}

// methodNotAllowed responde 405 anunciando en Allow los métodos del contrato.
func (s *Server) methodNotAllowed(allowed string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allowed)
		s.writeError(w, http.StatusMethodNotAllowed, "el método "+r.Method+" no está permitido en esta ruta; use "+allowed)
	}
}

// writeError responde un error simple con el código y el detalle indicados.
func (s *Server) writeError(w http.ResponseWriter, status int, detail string) {
	s.writeJSON(w, status, map[string]any{"error": http.StatusText(status), "detail": detail})
}

// writeUnprocessable responde 422 con la lista de problemas de validación.
func (s *Server) writeUnprocessable(w http.ResponseWriter, problems []string) {
	s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
		"error":    http.StatusText(http.StatusUnprocessableEntity),
		"problems": problems,
	})
}
