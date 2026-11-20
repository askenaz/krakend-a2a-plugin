"""Sirve un agente por A2A con FastA2A. AGENTE elige cuál: soporte o coordinador."""

from __future__ import annotations

import os

import uvicorn
from fasta2a.pydantic_ai import agent_to_a2a
from fasta2a.schema import Skill

from . import coordinador, soporte

_AGENTES = {
    "soporte": (
        soporte.crear_agente,
        "Resuelve dudas de clientes con la base de conocimiento de soporte.",
        Skill(
            id="faq",
            name="Preguntas frecuentes",
            description="Contraseñas, facturas, envíos y devoluciones.",
            tags=["soporte"],
            input_modes=["text/plain"],
            output_modes=["text/plain"],
        ),
    ),
    "coordinador": (
        coordinador.crear_agente,
        "Atiende al cliente y delega en el agente de soporte por A2A.",
        Skill(
            id="atencion",
            name="Atención al cliente",
            description="Recibe la consulta y la resuelve con el agente de soporte.",
            tags=["coordinacion"],
            input_modes=["text/plain"],
            output_modes=["text/plain"],
        ),
    ),
}


def crear_app(nombre: str, url_interna: str):
    crear, descripcion, habilidad = _AGENTES[nombre]
    return agent_to_a2a(crear(), name=nombre, url=url_interna, description=descripcion, skills=[habilidad])


def main() -> None:
    # URL_INTERNA es la URL por la que el gateway llega al agente; la Agent Card
    # la anuncia y el gateway la sustituye por la suya al publicarla.
    nombre = os.environ["AGENTE"]
    url_interna = os.environ["URL_INTERNA"]
    puerto = int(os.environ.get("PUERTO", "8000"))
    uvicorn.run(crear_app(nombre, url_interna), host=os.environ.get("HOST", "0.0.0.0"), port=puerto)


if __name__ == "__main__":
    main()
