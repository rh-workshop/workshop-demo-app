// api-service — API REST de negocio del workshop (cuentas y pagos).
//
// Una API pequeña pero realista, sin dependencias externas (solo biblioteca
// estándar de Go), pensada para enseñar dos cosas desde el gateway:
//
//   - propagación de identidad: la AuthPolicy de Connectivity Link valida el
//     JWT e inyecta la cabecera x-identidad; la API la refleja en cada
//     respuesta (y avisa cuando NO llega).
//   - códigos de estado reales: 200, 201 con Location, 404, 422 y 405, que
//     son los que las RateLimitPolicies y las pruebas del taller ejercitan.
//
// Este main solo hace el cableado: carga la configuración compartida,
// construye el servidor (internal/apiservice) y gobierna arranque y parada.
//
// Endpoints:
//
//	GET  /api/v1/accounts        cuentas del cliente autenticado
//	GET  /api/v1/accounts/{id}   una cuenta; 404 si no existe
//	POST /api/v1/payments        crea un pago; 201 con Location; 422 si es inválido
//	GET  /api/v1/payments/{id}   un pago; 404 si no existe
//	GET  /health                 sonda de vida y disponibilidad
//	GET  /metrics                contadores en formato Prometheus
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rh-workshop/workshop-demo-app/api-service/internal/apiservice"
	"github.com/rh-workshop/workshop-demo-app/shared/config"
	"github.com/rh-workshop/workshop-demo-app/shared/identity"
)

func main() {
	cfg := config.Cargar()

	server := apiservice.New(cfg)

	log.Printf("%s %s (%s) escuchando en 0.0.0.0:%s como usuario %s",
		cfg.Servicio, cfg.Version, cfg.Entorno, cfg.Puerto, identity.Usuario())

	// El binario es el PID 1 del contenedor: SIGTERM (parada del pod) debe
	// drenar las peticiones en curso, no matarlas a media respuesta.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	failures := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			failures <- err
		}
	}()

	select {
	case err := <-failures:
		log.Fatalf("el servidor no pudo arrancar: %v", err)
	case <-ctx.Done():
		log.Printf("señal de parada recibida; drenando peticiones en curso")
	}

	// Margen acorde al terminationGracePeriod por defecto del pod (30 s).
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("parada forzada: %v", err)
	}
}
