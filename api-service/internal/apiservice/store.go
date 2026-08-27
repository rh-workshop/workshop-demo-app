// Package apiservice implementa la API REST de negocio del workshop: cuentas y
// pagos en memoria, con códigos de estado reales y propagación de identidad.
package apiservice

import (
	"fmt"
	"sync"
	"time"
)

// Account representa una cuenta del catálogo precargado.
type Account struct {
	// ID es el identificador público de la cuenta.
	ID string `json:"id"`
	// Owner es el cliente OIDC dueño de la cuenta (claim azp del token).
	Owner string `json:"owner"`
	// Type distingue cuentas corrientes y de ahorro.
	Type string `json:"type"`
	// Currency es el código ISO de la moneda.
	Currency string `json:"currency"`
	// BalanceCents guarda el saldo en centavos para no usar coma flotante.
	BalanceCents int64 `json:"balance_cents"`
}

// Payment representa una orden de pago entre dos cuentas.
type Payment struct {
	ID          string    `json:"id"`
	FromAccount string    `json:"from_account"`
	ToAccount   string    `json:"to_account"`
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	Concept     string    `json:"concept,omitempty"`
	Status      string    `json:"status"`
	RequestedBy string    `json:"requested_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// Store guarda los datos en memoria; un mutex basta porque no hay persistencia.
type Store struct {
	mu       sync.RWMutex
	accounts []Account
	payments map[string]Payment
	// nextPaymentSeq numera los pagos creados en caliente de forma determinista.
	nextPaymentSeq int
}

// NewStore precarga un juego de datos determinista, pensado para el taller.
func NewStore() *Store {
	// seedTime fija las fechas para que dos pods devuelvan lo mismo.
	seedTime := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	s := &Store{
		accounts: []Account{
			{ID: "ACC-001", Owner: "user1", Type: "checking", Currency: "USD", BalanceCents: 1_250_00},
			{ID: "ACC-002", Owner: "user1", Type: "savings", Currency: "USD", BalanceCents: 8_400_50},
			{ID: "ACC-003", Owner: "user2", Type: "checking", Currency: "USD", BalanceCents: 310_75},
			{ID: "ACC-004", Owner: "user3", Type: "savings", Currency: "USD", BalanceCents: 15_000_00},
			{ID: "ACC-005", Owner: "user2", Type: "savings", Currency: "USD", BalanceCents: 2_780_00},
		},
		payments: map[string]Payment{
			"PAY-001": {ID: "PAY-001", FromAccount: "ACC-001", ToAccount: "ACC-003", AmountCents: 45_00, Currency: "USD", Concept: "seed payment", Status: "completed", RequestedBy: "user1", CreatedAt: seedTime},
			"PAY-002": {ID: "PAY-002", FromAccount: "ACC-004", ToAccount: "ACC-002", AmountCents: 120_00, Currency: "USD", Concept: "seed payment", Status: "completed", RequestedBy: "user3", CreatedAt: seedTime.Add(30 * time.Minute)},
		},
		nextPaymentSeq: 3,
	}
	return s
}

// Accounts devuelve las cuentas del dueño indicado, o todas si owner está vacío.
func (s *Store) Accounts(owner string) []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if owner == "" {
		return append([]Account(nil), s.accounts...)
	}
	filtered := make([]Account, 0, len(s.accounts))
	for _, a := range s.accounts {
		if a.Owner == owner {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

// Account busca una cuenta por su ID; el segundo valor indica si existe.
func (s *Store) Account(id string) (Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.accounts {
		if a.ID == id {
			return a, true
		}
	}
	return Account{}, false
}

// Payment busca un pago por su ID; el segundo valor indica si existe.
func (s *Store) Payment(id string) (Payment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.payments[id]
	return p, ok
}

// maxPayments acota el mapa en memoria: las demos de rate-limit bombardean el
// POST y sin tope la memoria crecería hasta el límite del pod (OOMKilled).
const maxPayments = 1000

// CreatePayment registra un pago nuevo y devuelve la entidad ya numerada.
// El segundo valor es false si el almacén alcanzó su capacidad máxima.
func (s *Store) CreatePayment(from, to string, amountCents int64, currency, concept, requestedBy string) (Payment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.payments) >= maxPayments {
		return Payment{}, false
	}
	p := Payment{
		ID:          fmt.Sprintf("PAY-%03d", s.nextPaymentSeq),
		FromAccount: from,
		ToAccount:   to,
		AmountCents: amountCents,
		Currency:    currency,
		Concept:     concept,
		Status:      "accepted",
		RequestedBy: requestedBy,
		CreatedAt:   time.Now().UTC(),
	}
	s.nextPaymentSeq++
	s.payments[p.ID] = p
	return p, true
}
