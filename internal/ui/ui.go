// Package ui sirve el panel de demostración del reparto de tráfico.
//
// El panel sondea el destino elegido, pinta una celda por respuesta y explica
// QUÉ hay que observar en cada escenario (canary, blue-green, circuit breaker).
// No adivina el escenario: lo elige quien presenta, y según esa elección cambian
// el texto, los controles, la ruta sondeada y la métrica destacada.
//
// Los assets (HTML, CSS y JS) son ficheros reales en assets/ pero viajan
// EMBEBIDOS en el binario a propósito: sin ficheros estáticos ni volúmenes,
// la imagen final sigue siendo una sola capa con un único ejecutable.
package ui

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
)

//go:embed assets
var assets embed.FS

// Handler compone la página una sola vez y devuelve el handler que la sirve.
// Componer en el arranque (y no por petición) hace que un asset roto tumbe el
// proceso al iniciar, no a mitad de una demostración.
func Handler() (http.HandlerFunc, error) {
	pagina, err := componerPagina()
	if err != nil {
		return nil, err
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(pagina)
	}, nil
}

// componerPagina incrusta la hoja de estilos y el script en la plantilla HTML.
// Los tipos template.CSS y template.JS marcan el contenido como confiable (es
// nuestro, embebido): html/template lo inserta sin escaparlo.
func componerPagina() ([]byte, error) {
	plantilla, err := template.ParseFS(assets, "assets/index.html.tmpl")
	if err != nil {
		return nil, err
	}
	estilos, err := assets.ReadFile("assets/panel.css")
	if err != nil {
		return nil, err
	}
	script, err := assets.ReadFile("assets/panel.js")
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	err = plantilla.Execute(&buf, struct {
		Styles template.CSS
		Script template.JS
	}{
		Styles: template.CSS(estilos),
		Script: template.JS(script),
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
