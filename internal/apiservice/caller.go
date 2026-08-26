package apiservice

import (
	"encoding/json"
	"net/http"
)

// CallerIdentity es la identidad que la AuthPolicy de Connectivity Link inyecta
// en la cabecera x-identidad tras validar el JWT en el gateway.
type CallerIdentity struct {
	// Client es el cliente OIDC que pidió el token (claim azp).
	Client string `json:"cliente"`
	// Subject es el sujeto del token (claim sub).
	Subject string `json:"sujeto"`
}

// IdentityHeader es el nombre de la cabecera que inyecta la AuthPolicy.
const IdentityHeader = "x-identidad"

// callerFromRequest extrae la identidad propagada; ok=false si no llegó.
func callerFromRequest(r *http.Request) (CallerIdentity, bool) {
	raw := r.Header.Get(IdentityHeader)
	if raw == "" {
		return CallerIdentity{}, false
	}
	var id CallerIdentity
	// Una cabecera malformada se trata como identidad ausente, no como error 500.
	if err := json.Unmarshal([]byte(raw), &id); err != nil || id.Client == "" {
		return CallerIdentity{}, false
	}
	return id, true
}

// identityEnvelope es el bloque que toda respuesta incluye para hacer VISIBLE
// la propagación de identidad gateway -> servicio (o su ausencia).
type identityEnvelope struct {
	Received bool   `json:"received"`
	Client   string `json:"client,omitempty"`
	Subject  string `json:"subject,omitempty"`
	Note     string `json:"note"`
}

// envelopeFor traduce la identidad (o su ausencia) a un bloque explicativo.
func envelopeFor(id CallerIdentity, ok bool) identityEnvelope {
	if !ok {
		return identityEnvelope{
			Received: false,
			Note:     "la cabecera x-identidad NO llegó: la petición no pasó por la AuthPolicy del gateway",
		}
	}
	return identityEnvelope{
		Received: true,
		Client:   id.Client,
		Subject:  id.Subject,
		Note:     "identidad propagada por la AuthPolicy de Connectivity Link (claims azp y sub del JWT)",
	}
}
