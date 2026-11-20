"""Cliente A2A con el que un agente llama a otro a través del gateway KrakenD.

Obtiene un token del IdP con sus propias credenciales de cliente y habla A2A
con el cliente de FastA2A, que usa los nombres de método de A2A 0.3. El gateway
no recibe cabecera A2A-Version, así que aplica 0.3.

La respuesta llega por streaming (message/stream): el cliente no sondea el
estado de la tarea, sino que lee los eventos SSE hasta que la tarea termina.
"""

from __future__ import annotations

import asyncio
import json
import time
import uuid
from pathlib import Path

import httpx
from fasta2a.client import A2AClient
from fasta2a.schema import Message

_FINALES = {"completed", "failed", "canceled", "rejected"}


class ClienteA2A:
    def __init__(self, gateway: str, agente: str, idp: str, credenciales: Path, espera: float = 60.0):
        self.base = f"{gateway.rstrip('/')}/a2a/{agente}"
        self.idp = idp.rstrip("/")
        self.credenciales = credenciales
        self.espera = espera
        self._token: str | None = None
        self._caduca = 0.0

    async def preguntar(self, texto: str) -> str:
        """Envía texto al agente y devuelve el texto de su respuesta cuando la tarea termina."""
        token = await self._obtener_token()
        mensaje = Message(role="user", parts=[{"text": texto}], message_id=uuid.uuid4().hex)
        respuesta = _Respuesta()
        async with httpx.AsyncClient(headers={"Authorization": f"Bearer {token}"}, timeout=self.espera) as http:
            cliente = A2AClient(self.base, http_client=http)
            async with asyncio.timeout(self.espera):
                async for evento in cliente.stream_message(mensaje):
                    respuesta.recibir(evento)
                    if respuesta.terminada:
                        break
        return respuesta.texto()

    async def _obtener_token(self) -> str:
        if self._token and time.monotonic() < self._caduca:
            return self._token
        datos = json.loads(self.credenciales.read_text(encoding="utf-8"))
        async with httpx.AsyncClient(timeout=10) as http:
            r = await http.post(
                f"{self.idp}/token",
                data={"grant_type": "client_credentials"},
                auth=(datos["client_id"], datos["secret"]),
            )
            r.raise_for_status()
            cuerpo = r.json()
        self._token = cuerpo["access_token"]
        self._caduca = time.monotonic() + float(cuerpo.get("expires_in", 300)) - 30
        return self._token


class _Respuesta:
    """Reúne lo que llega por el flujo SSE: el texto de los artefactos y el estado de la tarea."""

    def __init__(self) -> None:
        self.artefactos: dict[str, str] = {}
        self.estado = "submitted"

    @property
    def terminada(self) -> bool:
        return self.estado in _FINALES

    def recibir(self, evento: dict) -> None:
        if "error" in evento:
            error = evento["error"]
            raise RuntimeError(f"message/stream rechazado: {error.get('code')} {error.get('message')}")
        resultado = evento["result"]
        if "artifact_update" in resultado:
            self._artefacto(resultado["artifact_update"])
        for clave in ("task", "status_update"):
            if clave in resultado:
                self.estado = resultado[clave]["status"]["state"]

    def _artefacto(self, actualizacion: dict) -> None:
        # Los trozos llegan con append; el artefacto completo, al final, sin él.
        artefacto = actualizacion["artifact"]
        texto = "".join(parte.get("text", "") for parte in artefacto.get("parts", []))
        previo = self.artefactos.get(artefacto["artifact_id"], "") if actualizacion.get("append") else ""
        self.artefactos[artefacto["artifact_id"]] = previo + texto

    def texto(self) -> str:
        if self.estado != "completed":
            raise RuntimeError(f"la tarea terminó en estado {self.estado}")
        return "".join(self.artefactos.values()).strip()
