// Copyright 2026 Los autores del plugin A2A para KrakenD.

package conformance

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// versions.env es el único lugar donde se fijan las versiones de KrakenD. Los
// scripts lo leen al ejecutarse, pero Docker y la documentación no pueden: los
// Dockerfiles y los ficheros de Compose llevan una copia como valor por
// defecto, y la documentación nombra imágenes y artefactos en sus comandos.
//
// Estas pruebas fallan si alguna de esas copias nombra otra versión, y dicen
// dónde. Así, cambiar de versión es editar versions.env y seguir la lista.

const (
	pluginRoot   = "../.."
	versionsFile = pluginRoot + "/versions.env"
	matrixFile   = pluginRoot + "/docs/compatibilidad.md"
)

// Rutas, relativas a a2a-plugin, que nombran versiones a propósito y no siguen
// a versions.env.
var versionHistory = []string{
	"prototype",                    // fase 0: se validó con versiones concretas
	"docs/validacion-supuestos.md", // registro de esa validación
	"docs/compatibilidad.md",       // la matriz puede listar más versiones verificadas
}

// Ficheros de fuera de a2a-plugin que también se revisan, si existen.
var versionExtraFiles = []string{
	pluginRoot + "/../README.md",
	pluginRoot + "/../.github/workflows/a2a-plugin.yml",
}

var versionSkipDirs = map[string]bool{
	".git": true, ".secrets": true, ".scannerwork": true, "out": true,
	"node_modules": true, ".venv": true, "__pycache__": true,
}

var editionNames = map[string]string{"ce": "Community", "ee": "Enterprise"}

// versionRef reconoce una referencia a una versión de KrakenD. Si edition está
// vacío, la edición es el primer grupo capturado y la versión el segundo.
type versionRef struct {
	re      *regexp.Regexp
	edition string
}

const semver = `(\d+\.\d+\.\d+)`

var versionRefs = []versionRef{
	// Imágenes oficiales de compilación y de ejecución.
	{regexp.MustCompile(`krakend/builder-ee:` + semver), "ee"},
	{regexp.MustCompile(`krakend/krakend-ee:` + semver), "ee"},
	{regexp.MustCompile(`krakend/builder:` + semver), "ce"},
	{regexp.MustCompile(`(?:^|[^\w/.:-])krakend:` + semver), "ce"},
	// Imagen de KrakenD con el plugin y nombre del artefacto.
	{regexp.MustCompile(`krakend-a2a:(ce|ee)-` + semver), ""},
	{regexp.MustCompile(`krakend-a2a-(ce|ee)-` + semver), ""},
	// Valor por defecto en los ficheros de Compose.
	{regexp.MustCompile(`KRAKEND_(CE|EE)_VERSION:-` + semver), ""},
}

func readVersions(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(versionsFile)
	if err != nil {
		t.Fatalf("falta el fichero de versiones: %v", err)
	}
	vars := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("línea sin formato CLAVE=valor en %s: %q", versionsFile, line)
		}
		vars[key] = value
	}
	exact := regexp.MustCompile(`^` + semver + `$`)
	versions := map[string]string{"ce": vars["KRAKEND_CE_VERSION"], "ee": vars["KRAKEND_EE_VERSION"]}
	for edition, v := range versions {
		if !exact.MatchString(v) {
			t.Fatalf("%s no fija una versión exacta x.y.z para %s: %q", versionsFile, editionNames[edition], v)
		}
	}
	if !regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`).MatchString(vars["CI_GO_VERSION"]) {
		t.Fatalf("%s no fija CI_GO_VERSION: %q", versionsFile, vars["CI_GO_VERSION"])
	}
	return versions
}

// Ninguna referencia a una imagen de KrakenD, a un artefacto del plugin o a un
// valor por defecto de Compose nombra una versión distinta de la de versions.env.
func TestVersionReferences(t *testing.T) {
	versions := readVersions(t)
	found := 0
	check := func(path, shown string) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			for _, ref := range versionRefs {
				for _, m := range ref.re.FindAllStringSubmatch(line, -1) {
					edition, version := ref.edition, m[len(m)-1]
					if edition == "" {
						edition = strings.ToLower(m[1])
					}
					found++
					if version != versions[edition] {
						t.Errorf("%s:%d: %q nombra %s %s, pero versions.env fija %s",
							shown, i+1, strings.TrimSpace(m[0]), editionNames[edition], version, versions[edition])
					}
				}
			}
		}
	}

	err := filepath.WalkDir(pluginRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(pluginRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for _, h := range versionHistory {
			if rel == h {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if d.IsDir() {
			if versionSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !isText(d) {
			return nil
		}
		check(path, "a2a-plugin/"+rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range versionExtraFiles {
		if _, err := os.Stat(extra); err == nil {
			check(extra, strings.TrimPrefix(filepath.ToSlash(filepath.Clean(extra)), "../../../"))
		}
	}

	if found < 20 {
		t.Fatalf("solo se encontraron %d referencias a versiones: la prueba no está mirando donde debe", found)
	}
	if t.Failed() {
		t.Log("Actualiza esas referencias. Si un fichero es un registro histórico, añádelo a versionHistory.")
	}
}

// isText descarta binarios, sumas de módulos y ficheros grandes.
func isText(d fs.DirEntry) bool {
	switch strings.ToLower(filepath.Ext(d.Name())) {
	case ".so", ".sum", ".lock", ".png", ".jpg", ".jpeg", ".gif", ".pdf", ".exe", ".out":
		return false
	}
	info, err := d.Info()
	return err == nil && info.Mode().IsRegular() && info.Size() < 1<<20
}

// El valor por defecto de KRAKEND_VERSION en cada Dockerfile es la versión de
// su edición.
func TestDockerfileDefaults(t *testing.T) {
	versions := readVersions(t)
	arg := regexp.MustCompile(`(?m)^ARG KRAKEND_VERSION=(\S+)\s*$`)
	for edition, file := range map[string]string{
		"ce": "build/community/Dockerfile",
		"ee": "build/enterprise/Dockerfile",
	} {
		raw, err := os.ReadFile(filepath.Join(pluginRoot, file))
		if err != nil {
			t.Fatal(err)
		}
		m := arg.FindAllStringSubmatch(string(raw), -1)
		if len(m) != 1 {
			t.Errorf("%s: se esperaba una única línea \"ARG KRAKEND_VERSION=<versión>\" y hay %d", file, len(m))
			continue
		}
		if m[0][1] != versions[edition] {
			t.Errorf("%s: el valor por defecto de KRAKEND_VERSION es %s, pero versions.env fija %s para %s",
				file, m[0][1], versions[edition], editionNames[edition])
		}
	}
}

// La matriz de compatibilidad tiene una fila para la versión de cada edición:
// no se adopta una versión sin dejar constancia de con qué se verificó.
func TestCompatibilityMatrixListsVersions(t *testing.T) {
	versions := readVersions(t)
	raw, err := os.ReadFile(matrixFile)
	if err != nil {
		t.Fatalf("falta la matriz de compatibilidad: %v", err)
	}
	for edition, version := range versions {
		row := regexp.MustCompile(`(?m)^\|\s*` + editionNames[edition] + `\s*\|\s*` + regexp.QuoteMeta(version) + `\s*\|`)
		if !row.Match(raw) {
			t.Errorf("%s no tiene una fila \"| %s | %s | ...\" en la tabla de KrakenD",
				matrixFile, editionNames[edition], version)
		}
	}
}
