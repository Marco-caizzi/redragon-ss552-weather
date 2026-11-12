"use strict";

let websocket = null;
let actionUUID = "";
let context = "";
let settings = {};
let lastQuery = "";

function $(id) {
  return document.getElementById(id);
}

function setStatus(message) {
  $("status").textContent = message || "";
}

function showCurrent() {
  $("current").textContent = settings.city ? "Seleccionada: " + settings.city : "";
  $("units").value = settings.units === "f" ? "f" : "c";
  $("refresh").value = String(settings.refresh || 15);
}

function send(message) {
  if (websocket && websocket.readyState === WebSocket.OPEN) {
    websocket.send(JSON.stringify(message));
  }
}

function saveSettings() {
  send({ event: "setSettings", context: context, payload: settings });
}

const GEOCODE_URL = "https://geocoding-api.open-meteo.com/v1/search";
const DIRECT_TIMEOUT_MS = 8000;
const PLUGIN_TIMEOUT_MS = 12000;
let pluginTimer = null;

// Same labels as the Go plugin: "Córdoba, Provincia de Córdoba, Argentina".
function toPlace(r) {
  const country = r.country || r.country_code || "";
  const parts = [r.name];
  if (r.admin1 && r.admin1.toLowerCase() !== String(r.name).toLowerCase()) parts.push(r.admin1);
  if (country) parts.push(country);
  return {
    label: parts.join(", "),
    city: country ? r.name + ", " + country : r.name,
    latitude: r.latitude,
    longitude: r.longitude
  };
}

// Searches Open-Meteo directly from the panel. Calls onFail if the web view blocks or the request fails.
function searchDirect(query, onFail) {
  const xhr = new XMLHttpRequest();
  const url = GEOCODE_URL + "?count=10&format=json&language=es&name=" + encodeURIComponent(query);
  xhr.open("GET", url, true);
  xhr.timeout = DIRECT_TIMEOUT_MS;
  xhr.onload = function () {
    if (query !== lastQuery) return;
    if (xhr.status !== 200) {
      onFail();
      return;
    }
    let data;
    try {
      data = JSON.parse(xhr.responseText);
    } catch (e) {
      onFail();
      return;
    }
    showResults((data.results || []).map(toPlace));
  };
  xhr.onerror = onFail;
  xhr.ontimeout = onFail;
  xhr.send();
}

// Fallback: ask the plugin, which searches from Go.
function searchViaPlugin(query) {
  if (query !== lastQuery) return;
  send({
    event: "sendToPlugin",
    action: actionUUID,
    context: context,
    payload: { cmd: "search", query: query, lang: "es" }
  });
  clearTimeout(pluginTimer);
  pluginTimer = setTimeout(function () {
    if (query === lastQuery && $("status").textContent === "Buscando...") {
      setStatus("No hubo respuesta. Revisá la conexión y probá de nuevo.");
    }
  }, PLUGIN_TIMEOUT_MS);
}

function search() {
  const query = $("query").value.trim();
  if (query.length < 2) {
    setStatus("Escribí al menos 2 letras.");
    return;
  }
  lastQuery = query;
  setStatus("Buscando...");
  $("results").innerHTML = "";
  searchDirect(query, function () {
    searchViaPlugin(query);
  });
}

function showResults(places) {
  clearTimeout(pluginTimer);
  const box = $("results");
  box.innerHTML = "";
  if (!places || places.length === 0) {
    setStatus("No se encontraron ciudades.");
    return;
  }
  setStatus("Elegí una:");
  places.forEach(function (place) {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = place.label;
    button.addEventListener("click", function () {
      settings.city = place.city;
      settings.latitude = place.latitude;
      settings.longitude = place.longitude;
      saveSettings();
      box.innerHTML = "";
      setStatus("");
      showCurrent();
    });
    box.appendChild(button);
  });
}

function connectElgatoStreamDeckSocket(inPort, inPropertyInspectorUUID, inRegisterEvent, inInfo, inActionInfo) {
  const actionInfo = JSON.parse(inActionInfo);
  actionUUID = actionInfo.action;
  context = actionInfo.context;
  settings = (actionInfo.payload && actionInfo.payload.settings) || {};
  showCurrent();

  websocket = new WebSocket("ws://127.0.0.1:" + inPort);
  websocket.onopen = function () {
    send({ event: inRegisterEvent, uuid: inPropertyInspectorUUID });
    send({ event: "getSettings", context: context });
  };
  websocket.onmessage = function (evt) {
    let data;
    try {
      data = JSON.parse(evt.data);
    } catch (e) {
      return;
    }
    if (data.event === "didReceiveSettings") {
      settings = (data.payload && data.payload.settings) || {};
      showCurrent();
    } else if (data.event === "sendToPropertyInspector" && data.payload) {
      if (data.payload.query !== lastQuery) return; // Ignore answers to older searches.
      if (data.payload.cmd === "results") {
        showResults(data.payload.places);
      } else if (data.payload.cmd === "error") {
        clearTimeout(pluginTimer);
        setStatus(data.payload.message);
      }
    }
  };
}

document.addEventListener("DOMContentLoaded", function () {
  $("search").addEventListener("click", search);
  $("query").addEventListener("keydown", function (e) {
    if (e.key === "Enter") search();
  });
  $("units").addEventListener("change", function () {
    settings.units = $("units").value;
    saveSettings();
  });
  $("refresh").addEventListener("change", function () {
    settings.refresh = parseInt($("refresh").value, 10);
    saveSettings();
  });
});

window.connectElgatoStreamDeckSocket = connectElgatoStreamDeckSocket;
