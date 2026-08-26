const TOTAL = 120;   // celdas visibles a la vez
const RITMO = 200;   // ms de pausa entre una respuesta y la petición siguiente
const BARRAS = 40;   // llamadas visibles en el gráfico de latencia
const TIMEOUT_MS = 5000;  // timeout del cliente upstream; tope de la escala

// Cada escenario declara qué producto lo gobierna, qué ruta se sondea, qué
// métrica se destaca y qué controles hacen falta. El texto es la mitad del
// valor didáctico: sin él las celdas de colores no dicen nada.
const ESCENARIOS = {
  canary: {
    etiqueta: "Canary",
    titulo: "Despliegue canario por peso",
    producto: "Connectivity Link · HTTPRoute",
    productoClase: "cl",
    ruta: "/",
    latencia: false,
    descripcion:
      "<p>El <strong>HTTPRoute</strong> reparte el tráfico entre dos versiones " +
      "del servicio según el peso declarado en Git (90/10). <strong>Las dos " +
      "versiones atienden a la vez</strong>.</p>" +
      "<p>Al cambiar el peso en el repositorio de configuración y sincronizar " +
      "Argo CD, la proporción de colores cambia sin reiniciar nada.</p>",
    observar: [
      "Dos colores mezclados: cada color es una versión.",
      "El porcentaje de la leyenda tiende al peso configurado (90/10).",
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
    producto: "Connectivity Link · HTTPRoute",
    productoClase: "cl",
    ruta: "/",
    latencia: false,
    descripcion:
      "<p>El mismo <strong>HTTPRoute</strong>, con peso <strong>100/0</strong>: " +
      "una versión recibe todo el tráfico y la otra queda en reserva, desplegada " +
      "y lista. Conmutar es intercambiar los pesos; revertir, volver a intercambiarlos.</p>" +
      "<p>Una regla adicional enruta por la cabecera <strong>x-version: green</strong> " +
      "hacia la reserva: permite probarla sin exponerla al resto de usuarios.</p>",
    observar: [
      "Un único color mientras no se conmuta: no hay mezcla.",
      "Pulsa «Probar la reserva»: el sondeo añade la cabecera y el color cambia.",
      "Al conmutar los pesos en Git, el corte es limpio, no gradual.",
    ],
    codigo:
      "backendRefs:\n" +
      "  - name: bluegreen-service-blue\n" +
      "    weight: 100        # activa\n" +
      "  - name: bluegreen-service-green\n" +
      "    weight: 0          # reserva\n" +
      "# Regla extra: x-version: green -> reserva",
    controles: [
      { texto: "Probar la reserva (x-version: green)", accion: "reserva", peligro: false, alternador: true },
    ],
  },

  circuito: {
    etiqueta: "Circuit breaker",
    titulo: "Corte de circuito entre servicios",
    producto: "OpenShift Service Mesh · DestinationRule",
    productoClase: "mesh",
    ruta: "/api/call",
    latencia: true,
    descripcion:
      "<p><strong>service1-frontend</strong> llama a <strong>service2-backend</strong> " +
      "mediante <strong>/api/call</strong>; el panel sondea esa llamada entre servicios, " +
      "no al servicio directamente. La <strong>DestinationRule</strong> la vigila: tras " +
      "<strong>3 errores 5xx seguidos</strong> expulsa la réplica durante 30 s.</p>" +
      "<p>La lectura clave es el <strong>tiempo</strong>: con el destino fallando, cada " +
      "llamada agota el timeout de 5 s; con el circuito abierto, el error llega en " +
      "milisegundos. Fallar rápido es lo que evita el fallo en cascada.</p>",
    observar: [
      "Pulsa «Provocar 3 errores»: aparecen celdas rojas.",
      "Las barras saltan de ~5000 ms a pocos ms al abrirse el circuito.",
      "Pasados 30 s, Envoy reintenta y el verde vuelve.",
      "Esto NO es Connectivity Link: lo gobierna Service Mesh.",
    ],
    codigo:
      "outlierDetection:\n" +
      "  consecutive5xxErrors: 3\n" +
      "  interval: 10s\n" +
      "  baseEjectionTime: 30s\n" +
      "  maxEjectionPercent: 50",
    controles: [
      { texto: "Provocar 3 errores", accion: "errores", peligro: true },
      { texto: "Saturar el pool", accion: "saturar", peligro: false },
    ],
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
const latencia   = document.getElementById("latencia");
const latUltima  = document.getElementById("lat-ultima");
const latMedia   = document.getElementById("lat-media");
const latBarras  = document.getElementById("lat-barras");

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
const PALETA = ["#0066cc", "#3e8635", "#f0ab00", "#6753ac", "#009596", "#ec7a08"];
const colores = new Map();
const cuentas = new Map();
let siguienteColor = 0;
let posicion = 0;
let activo = "canary";
// La llamada entre servicios correcta se pinta siempre en verde: el contraste
// con el rojo del error es la lectura del escenario, no el reparto de versiones.
colores.set("llamada correcta", "#3e8635");

function colorDe(clave) {
  // Todo error comparte el rojo, en celdas Y en leyenda: nunca color de versión.
  if (clave.startsWith("error") || clave === "sin respuesta") return "#ee0000";
  if (!colores.has(clave)) {
    colores.set(clave, PALETA[siguienteColor % PALETA.length]);
    siguienteColor++;
  }
  return colores.get(clave);
}

// Historial de latencias del escenario de circuito: {ms, mal} por llamada.
const ultimas = [];

function registrarLatencia(ms, mal) {
  ultimas.push({ ms: ms, mal: mal });
  if (ultimas.length > BARRAS) ultimas.shift();
  pintarLatencia();
}

function pintarLatencia() {
  const ultima = ultimas[ultimas.length - 1];
  latUltima.textContent = ultima ? ultima.ms.toLocaleString("es") : "—";
  latUltima.className = ultima && ultima.mal ? "mal" : "";
  const cola = ultimas.slice(-20);
  const media = cola.length ? Math.round(cola.reduce((a, b) => a + b.ms, 0) / cola.length) : null;
  latMedia.innerHTML = "media de las últimas 20: <strong>" +
    (media === null ? "—" : media.toLocaleString("es")) + "</strong> ms";
  latBarras.innerHTML = "";
  for (const punto of ultimas) {
    const barra = document.createElement("div");
    barra.className = "lat-barra" + (punto.mal ? " mal" : "");
    // Escala lineal sobre el timeout: 5000 ms llena el gráfico y 3 ms queda plano.
    barra.style.height = Math.min(100, (punto.ms / TIMEOUT_MS) * 100) + "%";
    barra.title = punto.ms + " ms" + (punto.mal ? " (error)" : "");
    latBarras.appendChild(barra);
  }
}

function pintarPestanas() {
  pestanas.innerHTML = "";
  for (const [id, esc] of Object.entries(ESCENARIOS)) {
    const boton = document.createElement("button");
    boton.className = "pestana";
    boton.id = "pestana-" + id;
    boton.setAttribute("role", "tab");
    boton.textContent = esc.etiqueta;
    const seleccionado = id === activo;
    boton.setAttribute("aria-selected", seleccionado ? "true" : "false");
    boton.setAttribute("tabindex", seleccionado ? "0" : "-1");
    boton.onclick = () => cambiarEscenario(id);
    pestanas.appendChild(boton);
  }
  document.getElementById("panel-vivo").setAttribute("aria-labelledby", "pestana-" + activo);
}

// Las flechas mueven el foco y la selección entre pestañas (patrón tablist ARIA).
pestanas.addEventListener("keydown", (ev) => {
  const ids = Object.keys(ESCENARIOS);
  const idx = ids.indexOf(activo);
  let destino = null;
  if (ev.key === "ArrowRight") destino = ids[(idx + 1) % ids.length];
  if (ev.key === "ArrowLeft")  destino = ids[(idx - 1 + ids.length) % ids.length];
  if (destino) {
    cambiarEscenario(destino);
    document.getElementById("pestana-" + destino).focus();
    ev.preventDefault();
  }
});

function cambiarEscenario(id) {
  activo = id;
  cabeceraGreen = false;
  erroresPendientes = 0;
  pintarPestanas();
  pintarExplicacion();
  reiniciar();
}

function pintarExplicacion() {
  const esc = ESCENARIOS[activo];
  const puntos = esc.observar.map(o => "<li>" + o + "</li>").join("");
  explicacion.innerHTML =
    "<h2>" + esc.etiqueta + "</h2>" +
    "<span class='producto " + esc.productoClase + "'>" + esc.producto + "</span>" +
    "<p class='titulo-escenario'>" + esc.titulo + "</p>" +
    esc.descripcion +
    "<div class='observar'><h3>Qué observar</h3><ul>" + puntos + "</ul></div>" +
    "<pre>" + esc.codigo + "</pre>";

  latencia.className = esc.latencia ? "visible" : "";
  document.getElementById("titulo-vivo").textContent =
    esc.latencia ? "Llamadas service1-frontend → service2-backend" : "Respuestas en vivo";

  controles.innerHTML = "";
  controles.className = esc.controles.length ? "visible" : "";
  for (const ctrl of esc.controles) {
    const boton = document.createElement("button");
    boton.className = "boton" + (ctrl.peligro ? " peligro" : "");
    boton.textContent = ctrl.texto;
    if (ctrl.alternador) boton.setAttribute("aria-pressed", "false");
    boton.onclick = () => ACCIONES[ctrl.accion](boton);
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
  ultimas.length = 0;
  for (const celda of celdas) celda.style.background = "";
  pintarLeyenda();
  pintarLatencia();
}

// Mientras vale más de cero, el sondeo pide al backend que falle vía /api/call.
// Así los errores viajan por el MISMO canal que la rejilla observa y el sidecar
// los cuenta para la outlierDetection; por otro canal no habría nada que ver.
let erroresPendientes = 0;

// Con el alternador activo, el sondeo añade la cabecera que enruta a la reserva.
let cabeceraGreen = false;

// Acciones que provocan el escenario desde el propio panel: sin ellas habría
// que salir a la terminal en mitad de la demostración.
const ACCIONES = {
  errores() {
    erroresPendientes = 3;
    estadoTexto.textContent = "Provocando 3 errores consecutivos en service2-backend…";
  },
  saturar() {
    estadoTexto.textContent = "Saturando el pool con 20 llamadas lentas…";
    for (let i = 0; i < 20; i++) {
      fetch(base() + "/api/call?path=" + encodeURIComponent("/api/slow?s=5"),
        { cache: "no-store", headers: cabeceras() }).catch(() => {});
    }
    setTimeout(() => { estadoTexto.textContent = "Sondeando…"; }, 6000);
  },
  reserva(boton) {
    cabeceraGreen = !cabeceraGreen;
    boton.setAttribute("aria-pressed", cabeceraGreen ? "true" : "false");
    estadoTexto.textContent = cabeceraGreen
      ? "Sondeando con la cabecera x-version: green (ruta a la reserva)…"
      : "Sondeando…";
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
  const extra = {};
  const token = campoToken.value.trim();
  if (token) extra["Authorization"] = "Bearer " + token;
  if (activo === "bluegreen" && cabeceraGreen) extra["x-version"] = "green";
  return extra;
}

const avisoCert = document.getElementById("aviso-cert");
const enlaceDestino = document.getElementById("enlace-destino");

function mostrarAvisoCert() {
  enlaceDestino.href = base() + "/";
  avisoCert.classList.add("visible");
}

async function sondear() {
  const esc = ESCENARIOS[activo];
  let clave;
  let esCircuito = activo === "circuito";
  // El destino decide qué se pinta: la rejilla siempre refleja lo que se pide.
  let ruta = esc.ruta;
  if (esCircuito && erroresPendientes > 0) {
    ruta = "/api/call?path=" + encodeURIComponent("/api/error");
    erroresPendientes--;
    if (erroresPendientes === 0) {
      setTimeout(() => { estadoTexto.textContent = "Sondeando…"; }, RITMO);
    }
  }
  // La latencia se mide también en el cliente por si la respuesta no trae JSON.
  const inicio = performance.now();
  try {
    const resp = await fetch(base() + ruta, { cache: "no-store", headers: cabeceras() });
    let datos = null;
    try { datos = await resp.json(); } catch (e) {}
    if (esCircuito) {
      // duracion_ms la mide service1-frontend: es el tiempo REAL de la llamada
      // entre servicios, sin el trayecto navegador -> frontend.
      const ms = datos && typeof datos.duracion_ms === "number"
        ? datos.duracion_ms
        : Math.round(performance.now() - inicio);
      registrarLatencia(ms, !resp.ok);
      clave = resp.ok ? "llamada correcta" : "error " + resp.status;
    } else if (!resp.ok) {
      // Incluye el 503 que devuelve el sidecar con el circuito abierto.
      clave = "error " + resp.status;
    } else {
      clave = datos ? datos.version : "sin versión";
    }
    punto.className = "";
    avisoCert.classList.remove("visible");
  } catch (e) {
    // Un destino remoto que no responde suele ser el certificado propio del
    // gateway, no un fallo del servicio: se avisa en lugar de dejar la rejilla
    // en gris sin explicación.
    clave = "sin respuesta";
    punto.className = "parado";
    if (esCircuito) registrarLatencia(Math.round(performance.now() - inicio), true);
    if (base() !== "") mostrarAvisoCert();
  }

  const esError = clave.startsWith("error") || clave === "sin respuesta";
  celdas[posicion].style.background = esError ? "#ee0000" : colorDe(clave);
  posicion = (posicion + 1) % TOTAL;
  cuentas.set(clave, (cuentas.get(clave) || 0) + 1);
  pintarLeyenda();
  // Sondeo secuencial, no solapado: cada barra mide UNA llamada completa, y si
  // el destino tarda 5 s se VE que el flujo entero se frena — esa es la lección.
  setTimeout(sondear, RITMO);
}

pintarPestanas();
pintarExplicacion();
pintarLatencia();
sondear();
