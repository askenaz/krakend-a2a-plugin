"""Pruebas de extremo a extremo del plugin A2A con los agentes Pydantic AI.

    docker compose run --rm pruebas

Todas las peticiones van al gateway KrakenD, nunca directamente a los agentes.
Termina con código 1 si alguna comprobación falla.
"""

from __future__ import annotations

import json
import os
import sys
import time
import uuid

import httpx

GATEWAY = os.environ.get("GATEWAY_URL", "http://localhost:19080").rstrip("/")
PUBLICA = os.environ.get("PUBLIC_URL", "http://localhost:19080").rstrip("/")
IDP = os.environ.get("IDP_URL", "http://localhost:19090").rstrip("/")
CLIENTES = os.environ.get("CLIENTES", ".secrets/idp/clients.json")
# Con el modelo local las respuestas son deterministas y se comprueba su texto.
LOCAL = os.environ.get("DEMO_MODELO", "auto") == "local" or (
    os.environ.get("DEMO_MODELO", "auto") == "auto" and not os.environ.get("CLAUDE")
)

TASK_NOT_FOUND = -32001
_FINALES = {"completed", "failed", "canceled", "rejected"}

http = httpx.Client(timeout=60)
resultados: list[tuple[str, bool, str]] = []


def comprobar(nombre: str, ok: bool, detalle: str = "") -> bool:
    resultados.append((nombre, ok, detalle))
    print(f"  {'OK   ' if ok else 'FALLO'} {nombre}" + (f": {detalle}" if detalle and not ok else ""))
    return ok


def token(cliente: str) -> str:
    with open(CLIENTES, encoding="utf-8") as f:
        secreto = json.load(f)[cliente]["secret"]
    r = http.post(f"{IDP}/token", data={"grant_type": "client_credentials"}, auth=(cliente, secreto))
    r.raise_for_status()
    return r.json()["access_token"]


def cabeceras(tok: str | None, version: str | None, extra: dict[str, str] | None = None) -> dict[str, str]:
    h = {"Content-Type": "application/json"}
    if tok:
        h["Authorization"] = f"Bearer {tok}"
    if version:
        h["A2A-Version"] = version
    h.update(extra or {})
    return h


def rpc(tok: str | None, agente: str, metodo: str, params: dict, version: str | None = "1.0",
        extra: dict[str, str] | None = None) -> httpx.Response:
    cuerpo = {"jsonrpc": "2.0", "id": uuid.uuid4().hex[:8], "method": metodo, "params": params}
    return http.post(f"{GATEWAY}/a2a/{agente}", json=cuerpo, headers=cabeceras(tok, version, extra))


def mensaje(texto: str, version: str = "1.0", contexto: str | None = None) -> dict:
    rol = "ROLE_USER" if version == "1.0" else "user"
    m = {"messageId": uuid.uuid4().hex, "role": rol, "parts": [{"text": texto}]}
    if contexto:
        m["contextId"] = contexto
    return {"message": m}


def tarea_de(resultado: dict) -> dict:
    """La tarea de un resultado: SendMessage la envuelve en "task"; GetTask, no."""
    return resultado.get("task", resultado)


def estado(tarea: dict) -> str:
    return tarea["status"]["state"].removeprefix("TASK_STATE_").lower().replace("_", "-")


def esperar(tok: str, agente: str, tarea: dict, version: str = "1.0", segundos: float = 60) -> dict:
    metodo = "GetTask" if version == "1.0" else "tasks/get"
    limite = time.monotonic() + segundos
    while estado(tarea) not in _FINALES and time.monotonic() < limite:
        time.sleep(0.3)
        tarea = tarea_de(rpc(tok, agente, metodo, {"id": tarea["id"]}, version).json()["result"])
    return tarea


def texto(tarea: dict) -> str:
    return "".join(p.get("text", "") for a in tarea.get("artifacts", []) for p in a.get("parts", [])).strip()


def enviar_y_esperar(tok: str, agente: str, pregunta: str, version: str = "1.0",
                     contexto: str | None = None) -> dict | None:
    metodo = "SendMessage" if version == "1.0" else "message/send"
    r = rpc(tok, agente, metodo, mensaje(pregunta, version, contexto), version if version == "1.0" else None)
    cuerpo = r.json()
    if r.status_code != 200 or "result" not in cuerpo:
        comprobar(f"{metodo} a {agente}", False, f"{r.status_code} {r.text[:200]}")
        return None
    return esperar(tok, agente, tarea_de(cuerpo["result"]), version)


def descubrimiento() -> None:
    print("Descubrimiento de agentes")
    for agente in ("soporte", "coordinador"):
        r = http.get(f"{GATEWAY}/a2a/{agente}/.well-known/agent-card.json")
        card = r.json() if r.status_code == 200 else {}
        urls = [i.get("url") for i in card.get("supportedInterfaces", [])]
        comprobar(f"card de {agente} publicada con ETag", r.status_code == 200 and bool(r.headers.get("ETag")),
                  f"estado {r.status_code}")
        comprobar(f"card de {agente} anuncia solo el gateway", urls == [f"{PUBLICA}/a2a/{agente}"], str(urls))
        comprobar(f"card de {agente} sin URLs internas", f"{agente}:8000" not in r.text, r.text[:200])
        comprobar(f"card de {agente} declara el esquema bearer", "bearer" in card.get("securitySchemes", {}))
    r = http.get(f"{GATEWAY}/.well-known/agent-card.json")
    comprobar("la raíz publica la card del agente por defecto", r.status_code == 200 and r.json().get("name") == "soporte")


def autenticacion(ana: str) -> None:
    print("Autenticación y rutas")
    r = rpc(None, "soporte", "SendMessage", mensaje("hola"))
    reto = r.headers.get("WWW-Authenticate", "")
    comprobar("sin token: 401 con desafío Bearer", r.status_code == 401 and reto.startswith("Bearer"), f"{r.status_code} {reto}")
    r = http.post(f"{GATEWAY}/__a2a/soporte/unary", json={}, headers=cabeceras(ana, "1.0"))
    comprobar("las rutas internas no son accesibles desde fuera", r.status_code == 404, str(r.status_code))
    r = rpc(ana, "inexistente", "SendMessage", mensaje("hola"))
    comprobar("un agente no configurado da 404", r.status_code == 404, str(r.status_code))
    r = rpc(ana, "soporte", "DoSomething", {})
    comprobar("un método desconocido es -32601", r.json().get("error", {}).get("code") == -32601, r.text[:200])
    # Un byte 0xF1 suelto, como una «ñ» enviada en Latin-1 desde una consola: JSON bien
    # formado, pero no UTF-8. El gateway lo rechaza sin contactar con el agente.
    cuerpo = (b'{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"messageId":"m",'
              b'"role":"ROLE_USER","parts":[{"text":"espa' + bytes([0xF1]) + b'ol"}]}}}')
    r = http.post(f"{GATEWAY}/a2a/soporte", content=cuerpo, headers=cabeceras(ana, "1.0"))
    comprobar("un cuerpo que no es UTF-8 es -32700", r.json().get("error", {}).get("code") == -32700, r.text[:200])


def conversacion_y_propiedad(ana: str, beto: str) -> None:
    print("Conversación con soporte (A2A 1.0) y propiedad de tareas")
    tarea = enviar_y_esperar(ana, "soporte", "¿Cómo restablezco mi contraseña?")
    if tarea is None:
        return
    respuesta = texto(tarea)
    comprobar("SendMessage y GetTask completan la tarea", estado(tarea) == "completed", estado(tarea))
    comprobar("soporte responde", bool(respuesta) and (not LOCAL or "Ajustes > Seguridad" in respuesta), respuesta)
    print(f"      soporte: {respuesta}")

    error = rpc(beto, "soporte", "GetTask", {"id": tarea["id"]}).json().get("error", {})
    comprobar("otro usuario no ve la tarea (TaskNotFound)", error.get("code") == TASK_NOT_FOUND, str(error))
    falsa = rpc(beto, "soporte", "GetTask", {"id": tarea["id"]}, extra={"X-A2A-Sub": "ana"}).json().get("error", {})
    comprobar("una cabecera X-A2A-Sub falsificada no suplanta al propietario", falsa.get("code") == TASK_NOT_FOUND, str(falsa))

    contexto = tarea["contextId"]
    intruso = rpc(beto, "soporte", "SendMessage", mensaje("Soy yo, sigue", contexto=contexto)).json().get("error", {})
    comprobar("otro usuario no puede escribir en una conversación ajena", intruso.get("code") == TASK_NOT_FOUND, str(intruso))
    seguida = enviar_y_esperar(ana, "soporte", "¿Y cuánto tarda un envío?", contexto=contexto)
    comprobar("el propietario continúa su conversación",
              seguida is not None and estado(seguida) == "completed" and seguida["contextId"] == contexto,
              str(seguida)[:200])
    listado = rpc(ana, "soporte", "ListTasks", {"contextId": contexto}).json().get("error", {})
    comprobar("ListTasks, que FastA2A no implementa, se rechaza en el gateway", listado.get("code") == -32004, str(listado))


def protocolo_03(ana: str) -> None:
    print("Compatibilidad con A2A 0.3")
    tarea = enviar_y_esperar(ana, "soporte", "¿Cuánto tarda un envío?", version="0.3")
    if tarea is not None:
        respuesta = texto(tarea)
        comprobar("message/send y tasks/get sin cabecera A2A-Version", estado(tarea) == "completed" and bool(respuesta),
                  f"{estado(tarea)} {respuesta}")


def streaming(ana: str) -> None:
    print("Streaming SSE a través de KrakenD")
    cuerpo = {"jsonrpc": "2.0", "id": "st-1", "method": "SendStreamingMessage",
              "params": mensaje("Quiero devolver un producto")}
    llegadas: list[float] = []
    inicio = time.monotonic()
    with http.stream("POST", f"{GATEWAY}/a2a/soporte", json=cuerpo, headers=cabeceras(ana, "1.0")) as r:
        tipo = r.headers.get("Content-Type", "")
        for linea in r.iter_lines():
            if linea.startswith("data:"):
                llegadas.append(time.monotonic() - inicio)
    comprobar("SendStreamingMessage responde con text/event-stream", r.status_code == 200 and tipo.startswith("text/event-stream"),
              f"{r.status_code} {tipo}")
    comprobar("llegan varios eventos", len(llegadas) >= 3, str(len(llegadas)))
    if LOCAL and llegadas:
        comprobar("los eventos llegan según se emiten, no al final", llegadas[-1] - llegadas[0] >= 0.6,
                  ", ".join(f"{t:.2f}s" for t in llegadas))


def agente_a_agente(ana: str) -> None:
    print("Agente a agente: el coordinador consulta a soporte a través del gateway")
    tarea = enviar_y_esperar(ana, "coordinador", "Mi factura no aparece, ¿dónde la encuentro?")
    if tarea is None:
        return
    respuesta = texto(tarea)
    comprobar("el coordinador completa la tarea", estado(tarea) == "completed", estado(tarea))
    esperado = not LOCAL or ("agente de soporte" in respuesta and "Cuenta > Facturación" in respuesta)
    comprobar("la respuesta incluye lo que contestó soporte", bool(respuesta) and esperado, respuesta)
    print(f"      coordinador: {respuesta}")


def main() -> int:
    print(f"Gateway {GATEWAY} · modelo {'local' if LOCAL else 'Claude'}\n")
    ana, beto = token("ana-app"), token("beto-app")
    for prueba in (descubrimiento, lambda: autenticacion(ana), lambda: conversacion_y_propiedad(ana, beto),
                   lambda: protocolo_03(ana), lambda: streaming(ana), lambda: agente_a_agente(ana)):
        prueba()
        print()
    fallos = [n for n, ok, _ in resultados if not ok]
    print(f"{len(resultados) - len(fallos)} de {len(resultados)} comprobaciones correctas")
    return 1 if fallos else 0


if __name__ == "__main__":
    sys.exit(main())
