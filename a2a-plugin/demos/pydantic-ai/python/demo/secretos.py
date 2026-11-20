"""Genera los secretos locales de la demostración.

    python -m demo.secretos <directorio>

Escribe, sin sobrescribir lo que ya existe:

    <directorio>/idp/clients.json          clientes del IdP simulado con secretos aleatorios
    <directorio>/coordinador/cliente.json  credenciales con las que el coordinador llama al gateway

Para rotarlos, borra el directorio.
"""

from __future__ import annotations

import json
import secrets
import sys
from pathlib import Path

ESCRITURA = "a2a.read a2a.write"

# Cliente -> (sujeto, inquilino, scopes).
CLIENTES = {
    "ana-app": ("ana", "acme", ESCRITURA),
    "beto-app": ("beto", "acme", ESCRITURA),
    "coordinador-app": ("coordinador", "acme", ESCRITURA),
}


def main(destino: Path) -> None:
    clientes = destino / "idp" / "clients.json"
    credenciales = destino / "coordinador" / "cliente.json"
    if clientes.exists() and credenciales.exists():
        print(f"los secretos ya existen en {destino}; no se modifican")
        return
    datos = {
        cliente: {"secret": secrets.token_urlsafe(24), "sub": sub, "tenant": tenant, "scope": scope}
        for cliente, (sub, tenant, scope) in CLIENTES.items()
    }
    _escribir(clientes, datos)
    _escribir(credenciales, {"client_id": "coordinador-app", "secret": datos["coordinador-app"]["secret"]})
    print(f"secretos generados en {destino}")


def _escribir(ruta: Path, datos: dict) -> None:
    ruta.parent.mkdir(parents=True, exist_ok=True)
    ruta.write_text(json.dumps(datos, indent=2) + "\n", encoding="utf-8")
    # Lectura para todos: los contenedores que los montan no se ejecutan como root.
    ruta.chmod(0o644)


if __name__ == "__main__":
    main(Path(sys.argv[1] if len(sys.argv) > 1 else ".secrets"))
