# Documentation site · Sitio de documentación

## Español

Las guías en [español](../docs/README.md) y [inglés](../docs/en/README.md) se pueden leer directamente en el repositorio. Desde esta carpeta, ejecuta `npm ci`, `npm run build` y `npm run preview` para abrir una vista web local.

La generación se detiene si una guía contiene datos que parecen privados: claves, correos, rutas de usuario o URLs que no son de ejemplo. Los nombres propios que tampoco deben publicarse, como los de clientes, van en `private-markers.txt`, un patrón por línea. Git ignora ese fichero, así que esos nombres nunca llegan al repositorio.

## English

The [English](../docs/en/README.md) and [Spanish](../docs/README.md) guides are readable directly in the repository. From this directory, run `npm ci`, `npm run build`, and `npm run preview` to open a local web preview.

The build stops if a guide contains data that looks private: keys, email addresses, user paths, or URLs that are not examples. Names that must not be published either, such as customers, go in `private-markers.txt`, one pattern per line. Git ignores that file, so those names never reach the repository.
