"""Elección del modelo de los agentes.

Sin credenciales, los agentes usan un modelo local y determinista: un
FunctionModel de Pydantic AI que llama a las herramientas y redacta la
respuesta con reglas fijas. Así la demostración prueba el gateway sin coste ni
claves. Con ANTHROPIC_API_KEY definida, los agentes usan Claude Opus 5.5.

DEMO_MODELO fuerza la elección: "local", "claude" o "auto" (por defecto).
"""

from __future__ import annotations

import asyncio
import json
import os
import uuid
from collections.abc import AsyncIterator, Callable
from dataclasses import dataclass

from pydantic_ai import ModelMessage, ModelRequest, ModelResponse, TextPart, ToolCallPart, ToolReturnPart, UserPromptPart
from pydantic_ai.models.anthropic import AnthropicModelSettings
from pydantic_ai.models.function import AgentInfo, DeltaToolCall, FunctionModel

CLAUDE = "anthropic:claude-opus-5-5"


@dataclass
class Decision:
    """Lo que hace el modelo local en un turno: llamar a una herramienta o responder."""

    tool: str | None = None
    args: dict[str, str] | None = None
    text: str = ""


Decide = Callable[[list[ModelMessage]], Decision]


def ultimo_texto_del_usuario(messages: list[ModelMessage]) -> str:
    """El texto del último mensaje del usuario en el historial."""
    for message in reversed(messages):
        if not isinstance(message, ModelRequest):
            continue
        for part in reversed(message.parts):
            if not isinstance(part, UserPromptPart):
                continue
            if isinstance(part.content, str):
                return part.content
            return " ".join(item for item in part.content if isinstance(item, str))
    return ""


def resultado_de_herramienta(messages: list[ModelMessage]) -> str | None:
    """El resultado de la herramienta si el último mensaje lo trae; None si no."""
    last = messages[-1] if messages else None
    if not isinstance(last, ModelRequest):
        return None
    for part in last.parts:
        if isinstance(part, ToolReturnPart):
            return part.model_response_str()
    return None


def modelo_local(nombre: str, decide: Decide, pausa: float) -> FunctionModel:
    """Un FunctionModel que sigue las decisiones de decide, también en streaming."""

    def responder(messages: list[ModelMessage], info: AgentInfo) -> ModelResponse:
        d = decide(messages)
        if d.tool:
            return ModelResponse(parts=[ToolCallPart(d.tool, d.args or {}, tool_call_id=_id())])
        return ModelResponse(parts=[TextPart(d.text)])

    async def responder_en_streaming(messages: list[ModelMessage], info: AgentInfo) -> AsyncIterator[str | dict[int, DeltaToolCall]]:
        d = decide(messages)
        if d.tool:
            yield {0: DeltaToolCall(name=d.tool, json_args=json.dumps(d.args or {}), tool_call_id=_id())}
            return
        # La respuesta sale a trozos, con una pausa entre ellos, para que se vea el
        # streaming SSE incremental a través de KrakenD.
        for trozo in _trozos(d.text):
            await asyncio.sleep(pausa)
            yield trozo

    return FunctionModel(responder, stream_function=responder_en_streaming, model_name=f"local-{nombre}")


def elegir(nombre: str, decide: Decide) -> tuple[FunctionModel | str, AnthropicModelSettings | None]:
    """El modelo y sus ajustes según DEMO_MODELO y las credenciales disponibles."""
    eleccion = os.environ.get("DEMO_MODELO", "auto")
    if eleccion == "auto":
        eleccion = "claude" if os.environ.get("ANTHROPIC_API_KEY") else "local"
    if eleccion == "claude":
        # Si Claude rechaza una petición por sus clasificadores de seguridad, la
        # API la reintenta en otro modelo (fallbacks del lado del servidor).
        ajustes = AnthropicModelSettings(
            anthropic_betas=["server-side-fallback-2026-07-01"],
            extra_body={"fallbacks": "default"},
        )
        return CLAUDE, ajustes
    if eleccion != "local":
        raise ValueError(f"DEMO_MODELO={eleccion!r} no es válido: usa local, claude o auto")
    pausa = float(os.environ.get("DEMO_PAUSA_STREAMING", "0.3"))
    return modelo_local(nombre, decide, pausa), None


def _trozos(texto: str, palabras: int = 4) -> list[str]:
    tokens = texto.split(" ")
    return [" ".join(tokens[i : i + palabras]) + (" " if i + palabras < len(tokens) else "") for i in range(0, len(tokens), palabras)]


def _id() -> str:
    return "llamada-" + uuid.uuid4().hex[:12]
