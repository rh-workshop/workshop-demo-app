// Servicio de demostración del flujo GitOps del workshop.
//
// Un servidor HTTP mínimo, sin dependencias externas (solo biblioteca estándar
// de Go), cuyo propósito es hacer visible el ciclo completo: código -> pipeline
// -> registro de imágenes -> Argo CD -> Connectivity Link.
//
// La respuesta incluye la VERSIÓN del servicio para que, al publicar una imagen
// nueva y sincronizar Argo CD, el cambio se aprecie en la propia respuesta.
//
// Este main solo hace el cableado: carga la configuración, construye el
// servidor (internal/server) y gobierna el arranque y la parada ordenada.
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
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rh-workshop/workshop-demo-app/demo-service/internal/server"
	"github.com/rh-workshop/workshop-demo-app/demo-service/internal/ui"
	"github.com/rh-workshop/workshop-demo-app/internal/config"
	"github.com/rh-workshop/workshop-demo-app/internal/identity"
)

func main() {
	cfg := config.Cargar()

	panel, err := ui.Handler()
	if err != nil {
		log.Fatalf("no se pudo componer el panel: %v", err)
	}

	servidor := server.New(cfg, panel)

	log.Printf("%s %s (%s) escuchando en 0.0.0.0:%s como usuario %s",
		cfg.Servicio, cfg.Version, cfg.Entorno, cfg.Puerto, identity.Usuario())

	// El binario es el PID 1 del contenedor: SIGTERM (parada del pod) debe
	// drenar las peticiones en curso, no matarlas a media respuesta.
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	fallos := make(chan error, 1)
	go func() {
		if err := servidor.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fallos <- err
		}
	}()

	select {
	case err := <-fallos:
		log.Fatalf("el servidor no pudo arrancar: %v", err)
	case <-ctx.Done():
		log.Printf("señal de parada recibida; drenando peticiones en curso")
	}

	// Margen acorde al terminationGracePeriod por defecto del pod (30 s).
	ctxParada, cancelar := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancelar()
	if err := servidor.Shutdown(ctxParada); err != nil {
		log.Printf("parada forzada: %v", err)
	}
}
