# Configuración del prototipo

La clave simétrica de prueba no está en el repositorio. Se genera en
`.secrets/jwk.json`, excluido de git, con:

```bash
go run ./tools/probe -init
```

Solo firma tokens del entorno local de la fase 0 y no protege nada real.
