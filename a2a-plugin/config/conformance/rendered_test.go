// Copyright 2026 Los autores del plugin A2A para KrakenD.

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"krakend-a2a.local/core/config"
)

// Comprueba la configuración que generan las plantillas de Flexible Config con
// el KrakenD real de cada edición (config/templates/render.sh). Se omite si no
// se ha renderizado.
func TestRenderedTemplates(t *testing.T) {
	for _, edition := range []string{"community", "enterprise"} {
		t.Run(edition, func(t *testing.T) { checkRendered(t, edition) })
	}
}

type renderedEndpoint struct {
	Endpoint     string                            `json:"endpoint"`
	InputHeaders []string                          `json:"input_headers"`
	ExtraConfig  map[string]map[string]interface{} `json:"extra_config"`
	Backend      []struct {
		ExtraConfig map[string]interface{} `json:"extra_config"`
	} `json:"backend"`
}

// templateSettings es la parte de la configuración que solo usan las
// plantillas: la autenticación común de settings/a2a.json y el gateway de cada
// agente.
type templateSettings struct {
	Auth   map[string]interface{} `json:"auth"`
	Agents []templateAgent        `json:"agents"`
}

type templateAgent struct {
	ID      string          `json:"id"`
	Gateway templateGateway `json:"gateway"`
}

type templateGateway struct {
	Auth               map[string]interface{} `json:"auth"`
	BackendExtraConfig map[string]interface{} `json:"backend_extra_config"`
}

const templateSettingsDir = "../templates/settings"

type renderedDoc struct {
	Version     int                    `json:"version"`
	ExtraConfig map[string]interface{} `json:"extra_config"`
	Endpoints   []renderedEndpoint     `json:"endpoints"`
}

func checkRendered(t *testing.T, edition string) {
	path := filepath.Join("..", "..", "out", "rendered-"+edition+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("sin %s: ejecuta config/templates/render.sh %s", path, edition)
	}
	var doc renderedDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.FromMap(a2aBlock(t, edition, &doc))
	if err != nil {
		t.Fatalf("el bloque A2A generado no es válido para el plugin: %v", err)
	}

	// Escenario: alta de un agente. Cada agente tiene sus endpoints internos.
	want := map[string]bool{}
	for _, a := range cfg.Agents {
		want[cfg.InternalPrefix+"/"+a.ID+"/unary"] = true
		want[cfg.InternalPrefix+"/"+a.ID+"/stream"] = true
	}
	if len(doc.Endpoints) != len(want) {
		t.Fatalf("se esperaban %d endpoints internos, hay %d", len(want), len(doc.Endpoints))
	}

	// Escenario: piezas de seguridad presentes en cada endpoint interno.
	settings := readTemplateSettings(t)
	for _, e := range doc.Endpoints {
		if !want[e.Endpoint] {
			t.Errorf("endpoint inesperado %s", e.Endpoint)
		}
		checkInternalEndpoint(t, cfg, e)
		checkAgentTemplateFields(t, cfg, settings, e)
	}
}

// readTemplateSettings lee los agentes en las formas que admite el ensamblador
// del catálogo (config/catalog): la lista agents de a2a.json, un fichero
// agents/<id>.json o una carpeta agents/<id>/ con agent.json y, si existe,
// gateway.json.
func readTemplateSettings(t *testing.T) *templateSettings {
	t.Helper()
	var s templateSettings
	readJSONFile(t, filepath.Join(templateSettingsDir, "a2a.json"), &s)
	catalog := filepath.Join(templateSettingsDir, "agents")
	entries, err := os.ReadDir(catalog)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		path := filepath.Join(catalog, entry.Name())
		var a templateAgent
		switch {
		case entry.IsDir():
			readJSONFile(t, filepath.Join(path, "agent.json"), &a)
			gateway := filepath.Join(path, "gateway.json")
			if _, err := os.Stat(gateway); err == nil {
				readJSONFile(t, gateway, &a.Gateway)
			}
		case filepath.Ext(entry.Name()) == ".json":
			readJSONFile(t, path, &a)
		default:
			continue
		}
		s.Agents = append(s.Agents, a)
	}
	return &s
}

func readJSONFile(t *testing.T, path string, v interface{}) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("%s no es válido: %v", path, err)
	}
}

// checkAgentTemplateFields comprueba gateway.auth y gateway.backend_extra_config:
// el validador del endpoint es auth con las claves del agente sustituidas, y el
// backend lleva exactamente el extra_config del agente.
func checkAgentTemplateFields(t *testing.T, cfg *config.Config, s *templateSettings, e renderedEndpoint) {
	t.Helper()
	id := strings.SplitN(strings.TrimPrefix(e.Endpoint, cfg.InternalPrefix+"/"), "/", 2)[0]
	for _, a := range s.Agents {
		if a.ID != id {
			continue
		}
		want := map[string]interface{}{}
		for k, v := range s.Auth {
			want[k] = v
		}
		for k, v := range a.Gateway.Auth {
			want[k] = v
		}
		got := e.ExtraConfig["auth/validator"]
		for k, v := range want {
			if k != "propagate_claims" && !reflect.DeepEqual(got[k], v) {
				t.Errorf("%s: auth/validator.%s es %v; se esperaba %v", e.Endpoint, k, got[k], v)
			}
		}
		if len(e.Backend) != 1 {
			t.Fatalf("%s: se esperaba un backend, hay %d", e.Endpoint, len(e.Backend))
		}
		if !reflect.DeepEqual(e.Backend[0].ExtraConfig, a.Gateway.BackendExtraConfig) {
			t.Errorf("%s: extra_config del backend es %v; se esperaba %v",
				e.Endpoint, e.Backend[0].ExtraConfig, a.Gateway.BackendExtraConfig)
		}
		return
	}
	t.Errorf("%s: el agente %q no está en %s", e.Endpoint, id, templateSettingsDir)
}

// a2aBlock extrae el bloque A2A en la forma de cada edición.
func a2aBlock(t *testing.T, edition string, doc *renderedDoc) map[string]interface{} {
	t.Helper()
	server := doc.ExtraConfig["plugin/http-server"].(map[string]interface{})
	switch edition {
	case "community":
		if doc.Version != 3 {
			t.Fatalf("Community usa version 3, no %d", doc.Version)
		}
		return server["a2a-gateway"].(map[string]interface{})
	default:
		if doc.Version != 4 {
			t.Fatalf("Enterprise usa version 4, no %d", doc.Version)
		}
		plugins := server["plugins"].([]interface{})
		return plugins[0].(map[string]interface{})["config"].(map[string]interface{})
	}
}

func checkInternalEndpoint(t *testing.T, cfg *config.Config, e renderedEndpoint) {
	t.Helper()
	v, ok := e.ExtraConfig["auth/validator"]
	if !ok || v["propagate_claims"] == nil {
		t.Errorf("%s: falta auth/validator con propagate_claims", e.Endpoint)
	}
	if _, ok := e.ExtraConfig["plugin/req-resp-modifier"]; !ok {
		t.Errorf("%s: falta el modificador a2a-authz", e.Endpoint)
	}
	for _, h := range cfg.Identity.Headers {
		if !contains(e.InputHeaders, h) {
			t.Errorf("%s: input_headers no incluye la cabecera de identidad %s", e.Endpoint, h)
		}
	}
	for _, h := range []string{"X-A2A-Request-Id", "A2A-Version", "A2A-Extensions"} {
		if !contains(e.InputHeaders, h) {
			t.Errorf("%s: input_headers no incluye %s", e.Endpoint, h)
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
