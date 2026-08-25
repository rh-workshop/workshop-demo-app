package main

import "net/http"

// interfaz sirve una página que consulta "/" repetidamente y pinta un cuadrito
// por respuesta. Es la forma de VER el reparto de tráfico en vivo:
//
//   - canary por peso de HTTPRoute: aparecen dos colores mezclados en la
//     proporción configurada, y al cambiar el peso cambia la mezcla
//   - circuit breaker: los cuadritos se vuelven rojos cuando el sidecar corta
//   - Argo Rollouts: el color se desplaza a medida que avanzan los pasos
//
// Va embebida en el binario a propósito: sin ficheros estáticos ni volúmenes,
// la imagen sigue siendo una sola capa con un único ejecutable.
func interfaz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(pagina))
}

const pagina = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="utf-8">
<title>Reparto de tráfico — workshop</title>
<style>
  :root { color-scheme: dark; }
  body { margin: 0; padding: 24px; background: #151515; color: #e0e0e0;
         font-family: "Red Hat Text", "Inter", system-ui, sans-serif; }
  h1 { font-size: 18px; font-weight: 600; margin: 0 0 4px; }
  p  { font-size: 13px; color: #9a9a9a; margin: 0 0 20px; }
  #rejilla { display: grid; grid-template-columns: repeat(40, 1fr);
             gap: 3px; margin-bottom: 20px; }
  .celda { aspect-ratio: 1; border-radius: 2px; background: #2a2a2a; }
  #leyenda { display: flex; flex-wrap: wrap; gap: 16px; font-size: 13px; }
  .clave { display: flex; align-items: center; gap: 7px; }
  .muestra { width: 12px; height: 12px; border-radius: 2px; }
  .cuenta { color: #9a9a9a; font-variant-numeric: tabular-nums; }
  code { background: #2a2a2a; padding: 1px 5px; border-radius: 3px; font-size: 12px; }
</style>
</head>
<body>
  <h1>Reparto de tráfico en vivo</h1>
  <p>Una petición a <code>/</code> cada 200 ms. Cada cuadrito es una respuesta;
     el color identifica la versión que la atendió. El rojo indica error.</p>
  <div id="rejilla"></div>
  <div id="leyenda"></div>

<script>
const TOTAL = 200;            // cuadritos visibles a la vez
const rejilla = document.getElementById("rejilla");
const leyenda = document.getElementById("leyenda");
const celdas = [];

for (let i = 0; i < TOTAL; i++) {
  const celda = document.createElement("div");
  celda.className = "celda";
  rejilla.appendChild(celda);
  celdas.push(celda);
}

// Paleta estable: la misma versión recibe siempre el mismo color, sea cual sea
// el orden en que aparezcan. El rojo queda reservado a los errores.
const PALETA = ["#0066cc", "#3e8635", "#f0ab00", "#8476d1", "#009596", "#ec7a08"];
const colores = new Map();
const cuentas = new Map();
let siguiente = 0;
let posicion = 0;

function colorDe(clave) {
  if (!colores.has(clave)) {
    colores.set(clave, PALETA[siguiente % PALETA.length]);
    siguiente++;
  }
  return colores.get(clave);
}

function pintarLeyenda() {
  leyenda.innerHTML = "";
  for (const [clave, total] of [...cuentas].sort((a, b) => b[1] - a[1])) {
    const fila = document.createElement("div");
    fila.className = "clave";
    const muestra = document.createElement("span");
    muestra.className = "muestra";
    muestra.style.background = clave === "error" ? "#c9190b" : colorDe(clave);
    const texto = document.createElement("span");
    texto.textContent = clave;
    const cuenta = document.createElement("span");
    cuenta.className = "cuenta";
    cuenta.textContent = total;
    fila.append(muestra, texto, cuenta);
    leyenda.appendChild(fila);
  }
}

async function sondear() {
  let clave, color;
  try {
    const resp = await fetch("/", { cache: "no-store" });
    if (!resp.ok) {
      // Incluye el 503 que devuelve el sidecar con el circuito abierto.
      clave = "error";
      color = "#c9190b";
    } else {
      const datos = await resp.json();
      clave = datos.version;
      color = colorDe(clave);
    }
  } catch (e) {
    clave = "error";
    color = "#c9190b";
  }

  celdas[posicion].style.background = color;
  posicion = (posicion + 1) % TOTAL;
  cuentas.set(clave, (cuentas.get(clave) || 0) + 1);
  pintarLeyenda();
}

setInterval(sondear, 200);
</script>
</body>
</html>
`
