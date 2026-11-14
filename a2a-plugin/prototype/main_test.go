// Copyright 2026 Los autores del plugin A2A para KrakenD.

package main

import "testing"

// Las dos formas reales observadas en la fase 0 (ver docs/validacion-supuestos.md).
func TestPluginConfigForms(t *testing.T) {
	cases := []struct {
		name     string
		extra    map[string]interface{}
		wantForm string
		wantBase string
	}{
		{
			name: "Community 2.13: mapa del espacio de nombres con la config bajo el nombre",
			extra: map[string]interface{}{
				"name":              []interface{}{edgeName},
				"a2a-gateway-proto": map[string]interface{}{"base_path": "/agents"},
			},
			wantForm: "anidada-por-nombre (2.x)",
			wantBase: "/agents",
		},
		{
			name:     "Enterprise 3.0: contenido de config directamente",
			extra:    map[string]interface{}{"base_path": "/agents"},
			wantForm: "directa (3.x)",
			wantBase: "/agents",
		},
		{
			name:     "Sin configuración",
			extra:    nil,
			wantForm: "vacia",
			wantBase: "/a2a",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, form := pluginConfig(edgeName, c.extra)
			if form != c.wantForm {
				t.Fatalf("forma = %q, se esperaba %q", form, c.wantForm)
			}
			if got := cfgString(cfg, "base_path", "/a2a"); got != c.wantBase {
				t.Fatalf("base_path = %q, se esperaba %q", got, c.wantBase)
			}
		})
	}
}
