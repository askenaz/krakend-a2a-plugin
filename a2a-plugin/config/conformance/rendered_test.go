// Copyright 2026 Los autores del plugin A2A para KrakenD.

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
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
}

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
	for _, e := range doc.Endpoints {
		if !want[e.Endpoint] {
			t.Errorf("endpoint inesperado %s", e.Endpoint)
		}
		checkInternalEndpoint(t, cfg, e)
	}
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
