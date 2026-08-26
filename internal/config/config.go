// Package config carga la configuración del servicio desde variables de entorno.
//
// Estos valores los inyecta el Deployment desde un ConfigMap. Los valores por
// defecto solo aplican al ejecutar la aplicación fuera del cluster.
package config

import "os"

// Config reúne todo lo que parametriza al servicio. Ninguno de estos valores es
// sensible: las credenciales, cuando hacen falta, se referencian por el nombre
// del Secret y nunca por su valor.
type Config struct {
	// Puerto de escucha del servidor HTTP.
	Puerto string
	// Version que anuncia el servicio; cambia al sincronizar Argo CD.
	Version string
	// Entorno lógico (dev, test, local...).
	Entorno string
	// Mensaje descriptivo configurable.
	Mensaje string
	// Servicio es el nombre con el que se identifica esta instancia: así una
	// sola imagen sirve para todos los servicios del taller.
	Servicio string
	// Upstream es el servicio al que llama /api/call. Vacío = no llama a nadie.
	Upstream string
}

// Cargar lee las variables de entorno y devuelve la configuración efectiva.
func Cargar() Config {
	return Config{
		Puerto:   variable("PORT", "8080"),
		Version:  variable("APP_VERSION", "0.0.0-local"),
		Entorno:  variable("APP_ENTORNO", "local"),
		Mensaje:  variable("APP_MENSAJE", "Servicio de demostración del workshop"),
		Servicio: variable("APP_NOMBRE", "demo-service"),
		Upstream: variable("APP_UPSTREAM", ""),
	}
}

// variable devuelve el valor de la variable de entorno o el valor por defecto.
func variable(clave, porDefecto string) string {
	if valor := os.Getenv(clave); valor != "" {
		return valor
	}
	return porDefecto
}
