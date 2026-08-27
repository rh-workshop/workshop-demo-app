package apiservice

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// listAccounts responde 200 con las cuentas del cliente autenticado; si la
// identidad no llegó, devuelve el catálogo completo y lo dice en la respuesta.
func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	id, ok := callerFromRequest(r)
	accounts := s.store.Accounts(id.Client)
	scope := "all"
	if ok {
		scope = "owned-by-" + id.Client
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"service":  s.cfg.Servicio,
		"identity": envelopeFor(id, ok),
		"scope":    scope,
		"count":    len(accounts),
		"accounts": accounts,
	})
}

// getAccount responde 200 con la cuenta pedida o 404 si no existe.
func (s *Server) getAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := callerFromRequest(r)
	account, found := s.store.Account(r.PathValue("id"))
	if !found {
		s.writeError(w, http.StatusNotFound, fmt.Sprintf("la cuenta %q no existe", r.PathValue("id")))
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"identity": envelopeFor(id, ok),
		"account":  account,
	})
}

// paymentRequest es el cuerpo esperado por POST /api/v1/payments.
type paymentRequest struct {
	FromAccount string `json:"from_account"`
	ToAccount   string `json:"to_account"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	Concept     string `json:"concept"`
}

// validate concentra las reglas de negocio; devuelve los errores acumulados.
func (p paymentRequest) validate() []string {
	var problems []string
	if strings.TrimSpace(p.FromAccount) == "" {
		problems = append(problems, "from_account es obligatorio")
	}
	if strings.TrimSpace(p.ToAccount) == "" {
		problems = append(problems, "to_account es obligatorio")
	}
	if p.FromAccount != "" && p.FromAccount == p.ToAccount {
		problems = append(problems, "from_account y to_account no pueden ser la misma cuenta")
	}
	if p.AmountCents <= 0 {
		problems = append(problems, "amount_cents debe ser un entero positivo (centavos)")
	}
	if strings.TrimSpace(p.Currency) == "" {
		problems = append(problems, "currency es obligatorio (código ISO, p. ej. USD)")
	}
	return problems
}

// createPayment responde 201 con Location, o 422 si el cuerpo no es procesable.
func (s *Server) createPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := callerFromRequest(r)

	var req paymentRequest
	// Un límite de tamaño evita cuerpos absurdos; DisallowUnknownFields enseña
	// que un contrato de API también rechaza campos que no existen.
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		s.writeUnprocessable(w, []string{"el cuerpo no es JSON válido para un pago: " + err.Error()})
		return
	}
	// Un segundo documento JSON en el cuerpo también invalida la petición.
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		s.writeUnprocessable(w, []string{"el cuerpo debe contener un único objeto JSON"})
		return
	}
	if problems := req.validate(); len(problems) > 0 {
		s.writeUnprocessable(w, problems)
		return
	}

	// La identidad del JWT queda registrada en el pago; si no llegó, se anota.
	requestedBy := id.Client
	if !ok {
		requestedBy = "unknown (sin cabecera x-identidad)"
	}
	payment, created := s.store.CreatePayment(req.FromAccount, req.ToAccount, req.AmountCents, strings.ToUpper(req.Currency), req.Concept, requestedBy)
	// El almacén está acotado: al llegar al tope se responde 507 en vez de crecer hasta el OOM del pod.
	if !created {
		s.writeJSON(w, http.StatusInsufficientStorage, map[string]any{
			"identity": envelopeFor(id, ok),
			"error":    "el almacén en memoria de pagos alcanzó su capacidad máxima",
		})
		return
	}
	s.paymentsCreated.Add(1)

	w.Header().Set("Location", "/api/v1/payments/"+payment.ID)
	s.writeJSON(w, http.StatusCreated, map[string]any{
		"identity": envelopeFor(id, ok),
		"payment":  payment,
	})
}

// getPayment responde 200 con el pago pedido o 404 si no existe.
func (s *Server) getPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := callerFromRequest(r)
	payment, found := s.store.Payment(r.PathValue("id"))
	if !found {
		s.writeError(w, http.StatusNotFound, fmt.Sprintf("el pago %q no existe", r.PathValue("id")))
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"identity": envelopeFor(id, ok),
		"payment":  payment,
	})
}

// health responde la sonda de vida y disponibilidad del pod.
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": s.cfg.Servicio})
}

// metrics expone los contadores en el formato de texto de Prometheus.
func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP api_http_requests_total Peticiones HTTP atendidas.\n")
	fmt.Fprintf(w, "# TYPE api_http_requests_total counter\napi_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP api_http_errors_total Respuestas con código 4xx o 5xx.\n")
	fmt.Fprintf(w, "# TYPE api_http_errors_total counter\napi_http_errors_total %d\n", s.errorsCount.Load())
	fmt.Fprintf(w, "# HELP api_payments_created_total Pagos creados desde el arranque.\n")
	fmt.Fprintf(w, "# TYPE api_payments_created_total counter\napi_payments_created_total %d\n", s.paymentsCreated.Load())
}

// notFound es el fallback para rutas fuera del contrato de la API.
func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, http.StatusNotFound, fmt.Sprintf("la ruta %q no forma parte de la API", r.URL.Path))
}
