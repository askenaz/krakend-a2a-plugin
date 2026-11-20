"""Agente de soporte: responde con los artículos de una pequeña base de conocimiento."""

from __future__ import annotations

import unicodedata

from pydantic_ai import Agent, ModelMessage

from . import modelo

ARTICULOS = {
    "contraseña": "Para restablecer la contraseña, abre Ajustes > Seguridad y pulsa «Restablecer». "
    "Recibirás un enlace válido durante 15 minutos.",
    "factura": "Las facturas se emiten el día 1 de cada mes y se descargan en PDF desde Cuenta > Facturación.",
    "envío": "Los envíos nacionales tardan de 24 a 48 horas laborables y el número de seguimiento llega por correo.",
    "devolución": "Tienes 30 días para devolver un producto sin usar desde Pedidos > Devolver.",
}

# Palabras clave, sin tildes, que el modelo local asocia a cada tema.
_CLAVES = {
    "contraseña": ("contrasena", "clave", "password", "acceso"),
    "factura": ("factura", "cobro", "pago"),
    "envío": ("envio", "pedido", "entrega", "seguimiento"),
    "devolución": ("devolucion", "devolver", "reembolso"),
}

INSTRUCCIONES = (
    "Eres el agente de soporte de una tienda en línea. Responde en español, en dos o tres frases, "
    "usando solo lo que devuelva la herramienta buscar_articulo. Si no hay artículo, dilo."
)


def buscar_articulo(tema: str) -> str:
    """Busca en la base de conocimiento el artículo de un tema: contraseña, factura, envío o devolución."""
    articulo = ARTICULOS.get(tema.strip().lower())
    if articulo is None:
        return f"No hay ningún artículo sobre «{tema}». Temas disponibles: {', '.join(ARTICULOS)}."
    return articulo


def tema_de(texto: str) -> str:
    """El tema de la base de conocimiento al que se refiere el texto, o "general"."""
    normal = _sin_tildes(texto.lower())
    for tema, claves in _CLAVES.items():
        if any(clave in normal for clave in claves):
            return tema
    return "general"


def decidir(messages: list[ModelMessage]) -> modelo.Decision:
    """Reglas del modelo local: primero busca el artículo y después responde con él."""
    articulo = modelo.resultado_de_herramienta(messages)
    if articulo is None:
        return modelo.Decision(tool="buscar_articulo", args={"tema": tema_de(modelo.ultimo_texto_del_usuario(messages))})
    return modelo.Decision(text=f"Según la base de conocimiento de soporte: {articulo}")


def crear_agente() -> Agent:
    model, ajustes = modelo.elegir("soporte", decidir)
    return Agent(model, name="Soporte", instructions=INSTRUCCIONES, tools=[buscar_articulo], model_settings=ajustes)


def _sin_tildes(texto: str) -> str:
    return "".join(c for c in unicodedata.normalize("NFD", texto) if unicodedata.category(c) != "Mn")
