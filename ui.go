package main

import "net/http"

// interfaz sirve el panel de demostración: consulta "/" repetidamente, pinta una
// celda por respuesta y explica QUÉ hay que observar en cada escenario.
//
// El panel no adivina el escenario: lo elige quien presenta, y según esa
// elección cambian el texto, los controles y la lectura de los colores. Sin esa
// indicación las celdas de colores no enseñan nada por sí solas.
//
// Va embebido en el binario a propósito: sin ficheros estáticos ni volúmenes,
// la imagen sigue siendo una sola capa con un único ejecutable.
func interfaz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(pagina))
}

const pagina = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Panel de tráfico — workshop de contenedores y OpenShift</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
<style>
  :root {
    --tinta:      #151515;
    --superficie: #ffffff;
    --borde:      #d2d2d2;
    --texto:      #151515;
    --suave:      #6a6e73;
    --fondo:      #f5f5f5;
    --rojo:       #ee0000;   /* marca y errores, nunca decoración */
    --azul:       #0066cc;
    --verde:      #3e8635;
    --ambar:      #f0ab00;
    --morado:     #8476d1;
  }

  * { box-sizing: border-box; }

  body {
    margin: 0;
    background: var(--fondo);
    color: var(--texto);
    font-family: Inter, system-ui, -apple-system, sans-serif;
    font-size: 15px;
    line-height: 1.5;
  }

  code, .mono { font-family: "JetBrains Mono", ui-monospace, monospace; }

  header {
    background: var(--superficie);
    border-bottom: 1px solid var(--borde);
    padding: 20px 32px 0;
  }

  h1 {
    font-size: 21px;
    font-weight: 700;
    letter-spacing: -0.022em;
    margin: 0 0 3px;
  }

  header p { margin: 0 0 18px; color: var(--suave); font-size: 14px; }

  /* Selector de escenario: es lo primero que se decide al presentar. */
  #pestanas { display: flex; gap: 4px; }

  .pestana {
    appearance: none;
    background: none;
    border: 1px solid transparent;
    border-bottom: none;
    border-radius: 4px 4px 0 0;
    padding: 9px 18px;
    font: inherit;
    font-weight: 500;
    color: var(--suave);
    cursor: pointer;
    position: relative;
    top: 1px;
  }

  .pestana:hover { color: var(--texto); }

  .pestana[aria-selected="true"] {
    background: var(--fondo);
    border-color: var(--borde);
    color: var(--texto);
    font-weight: 600;
  }

  main {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 340px;
    gap: 24px;
    padding: 24px 32px 32px;
    align-items: start;
  }

  .panel {
    background: var(--superficie);
    border: 1px solid var(--borde);
    border-radius: 6px;
    padding: 20px;
  }

  h2 {
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--suave);
    margin: 0 0 14px;
  }

  #rejilla {
    display: grid;
    grid-template-columns: repeat(30, 1fr);
    gap: 3px;
    margin-bottom: 18px;
  }

  .celda {
    aspect-ratio: 1;
    border-radius: 2px;
    background: #ebebeb;
    transition: background 120ms ease-out;
  }

  #leyenda { display: flex; flex-wrap: wrap; gap: 8px 20px; }

  .clave { display: flex; align-items: center; gap: 8px; font-size: 14px; }
  .muestra { width: 11px; height: 11px; border-radius: 2px; flex: none; }
  .clave .cuenta { color: var(--suave); font-variant-numeric: tabular-nums; }
  .clave .pct { font-weight: 600; font-variant-numeric: tabular-nums; }

  /* Explicación del escenario activo. */
  #explicacion p { margin: 0 0 12px; }
  #explicacion p:last-child { margin-bottom: 0; }
  #explicacion strong { font-weight: 600; }

  .observar {
    background: var(--fondo);
    border: 1px solid var(--borde);
    border-radius: 4px;
    padding: 12px 14px;
    margin-top: 14px;
  }

  .observar h3 {
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--suave);
    margin: 0 0 8px;
  }

  .observar ul { margin: 0; padding-left: 18px; }
  .observar li { margin-bottom: 6px; font-size: 14px; }
  .observar li:last-child { margin-bottom: 0; }
  .observar li::marker { color: var(--suave); }

  pre {
    background: var(--tinta);
    color: #f0f0f0;
    border-radius: 4px;
    padding: 12px 14px;
    margin: 12px 0 0;
    font-family: "JetBrains Mono", ui-monospace, monospace;
    font-size: 12.5px;
    line-height: 1.55;
    overflow-x: auto;
  }

  /* Controles para provocar el fallo del circuit breaker desde el propio panel. */
  #controles { display: none; gap: 8px; flex-wrap: wrap; margin-top: 14px; }
  #controles.visible { display: flex; }

  /* Origen del tráfico: a qué servicio se sondea y con qué credencial. */
  #origen { margin-top: 16px; padding-top: 16px; border-top: 1px solid var(--borde); }

  .campo { display: flex; align-items: center; gap: 10px; margin-bottom: 8px; }
  .campo:last-child { margin-bottom: 0; }
  .campo label { font-size: 13px; color: var(--suave); min-width: 62px; }

  .campo input {
    flex: 1;
    font-family: "JetBrains Mono", ui-monospace, monospace;
    font-size: 13px;
    padding: 7px 10px;
    border: 1px solid var(--borde);
    border-radius: 4px;
    background: var(--superficie);
    color: var(--texto);
    min-width: 0;
  }

  .campo input:focus { outline: 2px solid var(--azul); outline-offset: -1px; border-color: var(--azul); }

  /* Aviso del certificado: sin este paso el sondeo remoto falla en silencio. */
  .aviso {
    display: none;
    gap: 9px;
    margin-top: 12px;
    font-size: 13px;
    color: var(--suave);
    line-height: 1.45;
  }

  .aviso.visible { display: flex; }

  .aviso::before {
    content: "";
    width: 3px;
    align-self: stretch;
    background: var(--ambar);
    border-radius: 2px;
    flex: none;
  }

  .aviso a { color: var(--azul); }

  .boton {
    appearance: none;
    background: var(--superficie);
    border: 1px solid var(--borde);
    border-radius: 4px;
    padding: 8px 14px;
    font: inherit;
    font-size: 14px;
    font-weight: 500;
    color: var(--texto);
    cursor: pointer;
  }

  .boton:hover { border-color: var(--suave); }
  .boton:active { background: var(--fondo); }
  .boton.peligro { color: var(--rojo); border-color: #f5c2c2; }
  .boton.peligro:hover { border-color: var(--rojo); }

  #estado {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    color: var(--suave);
    margin-top: 14px;
  }

  #punto { width: 8px; height: 8px; border-radius: 50%; background: var(--verde); flex: none; }
  #punto.parado { background: var(--suave); }

  @media (max-width: 900px) {
    main { grid-template-columns: 1fr; }
    #rejilla { grid-template-columns: repeat(20, 1fr); }
  }
</style>
</head>
<body>

<header>
  <h1>Panel de tráfico</h1>
  <p>Una petición cada 200 ms al destino indicado abajo. Cada celda es una respuesta,
     y su color, la versión que la atendió.</p>
  <div id="pestanas" role="tablist"></div>
</header>

<main>
  <section class="panel">
    <h2>Respuestas en vivo</h2>
    <div id="rejilla"></div>
    <div id="leyenda"></div>
    <div id="controles"></div>

    <div id="origen">
      <div class="campo">
        <label for="url">Destino</label>
        <input id="url" type="text" placeholder="/" value="/"
               title="Vacío o «/» sondea este mismo pod. Para ver el reparto real, pon la URL del servicio expuesto por el gateway.">
      </div>
      <div class="campo">
        <label for="token">Token</label>
        <input id="token" type="password" placeholder="JWT (solo si la AuthPolicy lo exige)"
               title="Se guarda en este navegador, nunca se envía a otro sitio que el destino indicado.">
      </div>
      <div class="aviso" id="aviso-cert">
        <span>El gateway usa un certificado propio, y el navegador rechaza las peticiones
        hasta que se acepte. Abre <a id="enlace-destino" href="#" target="_blank" rel="noopener">el
        destino en otra pestaña</a>, acepta el aviso de seguridad y vuelve aquí.</span>
      </div>
    </div>

    <div id="estado"><span id="punto"></span><span id="estado-texto">Sondeando…</span></div>
  </section>

  <aside class="panel" id="explicacion"></aside>
</main>

<script>
const TOTAL = 150;   // celdas visibles a la vez
const RITMO = 200;   // ms entre peticiones

// Cada escenario define qué se explica y qué controles hacen falta. El texto es
// la mitad del valor didáctico: sin él las celdas de colores no dicen nada.
const ESCENARIOS = {
  canary: {
    etiqueta: "Canary",
    titulo: "Despliegue canario por peso",
    descripcion:
      "<p>El <strong>HTTPRoute</strong> de Connectivity Link reparte el tráfico entre dos " +
      "versiones del servicio según el peso declarado en Git. Ambas atienden a la vez.</p>" +
      "<p>Al cambiar el peso en el repositorio de configuración y sincronizar Argo CD, " +
      "la proporción de colores cambia sin reiniciar nada.</p>",
    observar: [
      "Dos colores mezclados: cada uno es una versión.",
      "La proporción de la leyenda tiende al peso configurado.",
      "Ninguna celda roja: el canario reparte, no falla.",
    ],
    codigo:
      "backendRefs:\n" +
      "  - name: canary-service-v1\n" +
      "    weight: 90\n" +
      "  - name: canary-service-v2\n" +
      "    weight: 10",
    controles: [],
  },

  bluegreen: {
    etiqueta: "Blue-green",
    titulo: "Conmutación blue-green",
    descripcion:
      "<p>Dos versiones desplegadas a la vez, pero <strong>solo una recibe tráfico</strong>. " +
      "La conmutación cambia el destino de golpe, no gradualmente.</p>" +
      "<p>La versión en reposo queda lista para volver: revertir es conmutar de nuevo, " +
      "sin esperar a que se construya ni se despliegue nada.</p>",
    observar: [
      "Un único color en pantalla mientras no se conmuta.",
      "Al conmutar, el color cambia por completo en una o dos celdas.",
      "No hay mezcla: es un corte limpio, a diferencia del canario.",
    ],
    codigo:
      "# El Service apunta a una versión a la vez.\n" +
      "selector:\n" +
      "  app: demo-service\n" +
      "  version: blue      # -> green para conmutar",
    controles: [],
  },

  circuito: {
    etiqueta: "Circuit breaker",
    titulo: "Corte de circuito",
    descripcion:
      "<p>La <strong>DestinationRule</strong> de Istio vigila las respuestas. Tras " +
      "<strong>3 errores 5xx consecutivos</strong> expulsa la instancia del balanceo " +
      "durante 30 s (<code>outlierDetection</code>).</p>" +
      "<p>El sidecar también corta cuando se superan las conexiones del " +
      "<code>connectionPool</code>: entonces responde 503 sin llegar a molestar al backend.</p>",
    observar: [
      "Pulsa «Provocar 3 errores»: aparecen celdas rojas.",
      "Tras el tercero, la instancia sale del balanceo y el rojo cesa.",
      "Con varias réplicas, el tráfico sigue por las sanas.",
      "«Saturar el pool» abre el circuito por concurrencia, no por error.",
    ],
    codigo:
      "outlierDetection:\n" +
      "  consecutive5xxErrors: 3\n" +
      "  interval: 10s\n" +
      "  baseEjectionTime: 30s",
    controles: [
      { texto: "Provocar 3 errores", accion: "errores", peligro: true },
      { texto: "Saturar el pool", accion: "saturar", peligro: false },
    ],
  },

  rollout: {
    etiqueta: "Argo Rollouts",
    titulo: "Entrega progresiva con Argo Rollouts",
    descripcion:
      "<p>El controlador <strong>Argo Rollouts</strong> avanza por pasos declarados: " +
      "sube el peso, espera, y continúa solo si la versión nueva se comporta.</p>" +
      "<p>A diferencia del canario manual, aquí el progreso lo conduce el controlador, " +
      "y se puede <strong>promover</strong> o <strong>abortar</strong> a mitad de camino.</p>",
    observar: [
      "El color nuevo aparece poco a poco, no de golpe.",
      "Se estabiliza en cada pausa: son los pasos del Rollout.",
      "Al abortar, el color vuelve al anterior sin pasar por errores.",
    ],
    codigo:
      "strategy:\n" +
      "  canary:\n" +
      "    steps:\n" +
      "      - setWeight: 20\n" +
      "      - pause: {duration: 30s}\n" +
      "      - setWeight: 60",
    controles: [],
  },
};

const rejilla    = document.getElementById("rejilla");
const leyenda    = document.getElementById("leyenda");
const pestanas   = document.getElementById("pestanas");
const explicacion= document.getElementById("explicacion");
const controles  = document.getElementById("controles");
const estadoTexto= document.getElementById("estado-texto");
const punto      = document.getElementById("punto");
const campoUrl   = document.getElementById("url");
const campoToken = document.getElementById("token");

// Destino y token sobreviven al refresco: en una sesión no se quiere volver a
// pegar el JWT cada vez. Quedan en este navegador, no viajan a ningún sitio.
campoUrl.value   = localStorage.getItem("panel-url")   || "/";
campoToken.value = localStorage.getItem("panel-token") || "";
campoUrl.oninput   = () => { localStorage.setItem("panel-url", campoUrl.value); reiniciar(); };
campoToken.oninput = () => localStorage.setItem("panel-token", campoToken.value);

const celdas = [];
for (let i = 0; i < TOTAL; i++) {
  const celda = document.createElement("div");
  celda.className = "celda";
  rejilla.appendChild(celda);
  celdas.push(celda);
}

// Paleta estable: una versión conserva su color aunque cambie el orden de
// aparición. El rojo queda reservado a los errores, nunca a una versión.
const PALETA = ["#0066cc", "#3e8635", "#f0ab00", "#8476d1", "#009596", "#ec7a08"];
const colores = new Map();
const cuentas = new Map();
let siguienteColor = 0;
let posicion = 0;
let activo = "canary";

function colorDe(clave) {
  if (clave === "error") return "#ee0000";
  if (!colores.has(clave)) {
    colores.set(clave, PALETA[siguienteColor % PALETA.length]);
    siguienteColor++;
  }
  return colores.get(clave);
}

function pintarPestanas() {
  pestanas.innerHTML = "";
  for (const [id, esc] of Object.entries(ESCENARIOS)) {
    const boton = document.createElement("button");
    boton.className = "pestana";
    boton.role = "tab";
    boton.textContent = esc.etiqueta;
    boton.setAttribute("aria-selected", id === activo ? "true" : "false");
    boton.onclick = () => { activo = id; pintarPestanas(); pintarExplicacion(); reiniciar(); };
    pestanas.appendChild(boton);
  }
}

function pintarExplicacion() {
  const esc = ESCENARIOS[activo];
  const puntos = esc.observar.map(o => "<li>" + o + "</li>").join("");
  explicacion.innerHTML =
    "<h2>" + esc.etiqueta + "</h2>" +
    "<p style='font-weight:600;margin-bottom:10px'>" + esc.titulo + "</p>" +
    esc.descripcion +
    "<div class='observar'><h3>Qué observar</h3><ul>" + puntos + "</ul></div>" +
    "<pre>" + esc.codigo + "</pre>";

  controles.innerHTML = "";
  controles.className = esc.controles.length ? "visible" : "";
  for (const ctrl of esc.controles) {
    const boton = document.createElement("button");
    boton.className = "boton" + (ctrl.peligro ? " peligro" : "");
    boton.textContent = ctrl.texto;
    boton.onclick = () => ACCIONES[ctrl.accion]();
    controles.appendChild(boton);
  }
}

function pintarLeyenda() {
  const total = [...cuentas.values()].reduce((a, b) => a + b, 0) || 1;
  leyenda.innerHTML = "";
  for (const [clave, n] of [...cuentas].sort((a, b) => b[1] - a[1])) {
    const fila = document.createElement("div");
    fila.className = "clave";
    const muestra = document.createElement("span");
    muestra.className = "muestra";
    muestra.style.background = colorDe(clave);
    const texto = document.createElement("span");
    texto.className = "mono";
    texto.textContent = clave;
    const pct = document.createElement("span");
    pct.className = "pct";
    pct.textContent = Math.round((n / total) * 100) + "%";
    const cuenta = document.createElement("span");
    cuenta.className = "cuenta";
    cuenta.textContent = "(" + n + ")";
    fila.append(muestra, texto, pct, cuenta);
    leyenda.appendChild(fila);
  }
}

function reiniciar() {
  cuentas.clear();
  posicion = 0;
  for (const celda of celdas) celda.style.background = "";
  pintarLeyenda();
}

// Mientras vale más de cero, el sondeo pide /api/error en lugar de /. Sin esto
// el botón dispararía los errores por un canal que la rejilla no observa: en el
// cluster el corte lo provoca el sidecar, pero en local no habría nada que ver.
let erroresPendientes = 0;

// Acciones que provocan el escenario desde el propio panel: sin ellas habría
// que salir a la terminal en mitad de la demostración.
const ACCIONES = {
  errores() {
    erroresPendientes = 3;
    estadoTexto.textContent = "Provocando 3 errores consecutivos…";
  },
  saturar() {
    estadoTexto.textContent = "Saturando el pool con 20 peticiones lentas…";
    for (let i = 0; i < 20; i++) {
      fetch(base() + "/api/lento?s=5", { cache: "no-store", headers: cabeceras() }).catch(() => {});
    }
    setTimeout(() => { estadoTexto.textContent = "Sondeando…"; }, 6000);
  },
};

// base devuelve la raíz del servicio sondeado, sin barra final. Vacío = este
// mismo pod, que es lo útil en local; una URL completa apunta al servicio
// expuesto por el gateway, que es donde el reparto de tráfico es REAL.
function base() {
  const valor = campoUrl.value.trim().replace(/\/+$/, "");
  return valor === "" ? "" : valor;
}

function cabeceras() {
  const token = campoToken.value.trim();
  return token ? { Authorization: "Bearer " + token } : {};
}

const avisoCert = document.getElementById("aviso-cert");
const enlaceDestino = document.getElementById("enlace-destino");

function mostrarAvisoCert() {
  enlaceDestino.href = base() + "/";
  avisoCert.classList.add("visible");
}

async function sondear() {
  let clave;
  // El destino decide qué se pinta: la rejilla siempre refleja lo que se pide.
  let ruta = "/";
  if (erroresPendientes > 0) {
    ruta = "/api/error";
    erroresPendientes--;
    if (erroresPendientes === 0) {
      setTimeout(() => { estadoTexto.textContent = "Sondeando…"; }, RITMO);
    }
  }
  try {
    const resp = await fetch(base() + ruta, { cache: "no-store", headers: cabeceras() });
    if (!resp.ok) {
      // Incluye el 503 que devuelve el sidecar con el circuito abierto.
      clave = "error " + resp.status;
    } else {
      const datos = await resp.json();
      clave = datos.version;
    }
    punto.className = "";
    avisoCert.classList.remove("visible");
  } catch (e) {
    // Un destino remoto que no responde suele ser el certificado propio del
    // gateway, no un fallo del servicio: se avisa en lugar de dejar la rejilla
    // en gris sin explicación.
    clave = "sin respuesta";
    punto.className = "parado";
    if (base() !== "") mostrarAvisoCert();
  }

  const esError = clave.startsWith("error") || clave === "sin respuesta";
  celdas[posicion].style.background = esError ? "#ee0000" : colorDe(clave);
  posicion = (posicion + 1) % TOTAL;
  cuentas.set(clave, (cuentas.get(clave) || 0) + 1);
  pintarLeyenda();
}

pintarPestanas();
pintarExplicacion();
setInterval(sondear, RITMO);
</script>
</body>
</html>
`
