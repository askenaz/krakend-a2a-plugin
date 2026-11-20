"""Agente coordinador: atiende al cliente y consulta al agente de soporte por A2A.

La consulta no va directa al agente de soporte: pasa por el gateway KrakenD,
con un token propio del coordinador, igual que la de cualquier otro cliente.
"""

from __future__ import annotations

import os
from pathlib import Path

from pydantic_ai import Agent, ModelMessage

from . import modelo
from .cliente_a2a import ClienteA2A

INSTRUCCIONES = (
    "Eres el coordinador de atención al cliente. Para cualquier duda sobre la cuenta, facturas, envíos o "
    "devoluciones, consulta al agente de soporte con la herramienta consultar_soporte y resume su respuesta "
    "en español. Indica que la información viene del agente de soporte."
)

_soporte: ClienteA2A | None = None


def _cliente() -> ClienteA2A:
    global _soporte
    if _soporte is None:
        _soporte = ClienteA2A(
            gateway=os.environ.get("GATEWAY_URL", "http://gateway:8080"),
            agente=os.environ.get("AGENTE_SOPORTE", "soporte"),
            idp=os.environ.get("IDP_URL", "http://mock-idp:8080"),
            credenciales=Path(os.environ.get("CREDENCIALES", "/run/demo/cliente.json")),
        )
    return _soporte


async def consultar_soporte(pregunta: str) -> str:
    """Pregunta al agente de soporte, a través del gateway A2A, y devuelve su respuesta."""
    return await _cliente().preguntar(pregunta)


def decidir(messages: list[ModelMessage]) -> modelo.Decision:
    """Reglas del modelo local: primero consulta a soporte y después resume su respuesta."""
    respuesta = modelo.resultado_de_herramienta(messages)
    if respuesta is None:
        return modelo.Decision(tool="consultar_soporte", args={"pregunta": modelo.ultimo_texto_del_usuario(messages)})
    return modelo.Decision(text=f"He consultado al agente de soporte a través del gateway A2A. Su respuesta: {respuesta}")


def crear_agente() -> Agent:
    model, ajustes = modelo.elegir("coordinador", decidir)
    return Agent(model, name="Coordinador", instructions=INSTRUCCIONES, tools=[consultar_soporte], model_settings=ajustes)
