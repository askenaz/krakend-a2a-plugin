// Copyright 2026 Los autores del plugin A2A para KrakenD.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const block = `{"public_url":"https://gw.example.com","agents":[{"id":"billing",
  "card":{"source":"static","inline":{"name":"B","supportedInterfaces":[{"url":"http://b.internal/",` +
	`"protocolBinding":"JSONRPC","protocolVersion":"1.0"}],"capabilities":{}}}}]}`

func decode(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// Las dos formas reales en que llega la configuración (fase 0).
func TestPluginConfigForms(t *testing.T) {
	cfg := decode(t, block)
	ce := map[string]interface{}{"name": []interface{}{serverName}, serverName: cfg}
	got, form := pluginConfig(serverName, ce)
	if form != "Community 2.x" || got["public_url"] != "https://gw.example.com" {
		t.Fatalf("Community: %s %v", form, got)
	}
	got, form = pluginConfig(serverName, cfg)
	if form != "Enterprise 3.x" || got["public_url"] != "https://gw.example.com" {
		t.Fatalf("Enterprise: %s %v", form, got)
	}
	got, form = pluginConfig(modifierName, map[string]interface{}{"name": []interface{}{modifierName}})
	if len(got) != 0 || !strings.Contains(form, "sin configuración") {
		t.Fatalf("Community sin configuración del modificador: %s %v", form, got)
	}
	if got, _ = pluginConfig(serverName, nil); len(got) != 0 {
		t.Fatal("sin mapa la configuración debe estar vacía")
	}
}

func TestModifierAgentOption(t *testing.T) {
	if a, err := modifierAgent(map[string]interface{}{"agent": "billing"}); err != nil || a != "billing" {
		t.Fatalf("%q %v", a, err)
	}
	if _, err := modifierAgent(map[string]interface{}{"agnet": "billing"}); err == nil {
		t.Fatal("un campo desconocido debe rechazarse")
	}
	if _, err := modifierAgent(map[string]interface{}{"agent": 3}); err == nil {
		t.Fatal("agent debe ser una cadena")
	}
}

// Una configuración inválida detiene el arranque.
func TestInvalidConfigExits(t *testing.T) {
	code := -1
	exit = func(c int) { code = c }
	defer func() { exit = func(c int) { panic(c) } }()
	extra := map[string]interface{}{"agents": []interface{}{}}
	_, err := serverRegisterer(serverName).factory(context.Background(), extra, http.NotFoundHandler())
	if err == nil || code != 1 {
		t.Fatalf("se esperaba salida con código 1: code=%d err=%v", code, err)
	}
}

type fakeRequest struct{ h map[string][]string }

type (
	handlerFactory  = func(context.Context, map[string]interface{}, http.Handler) (http.Handler, error)
	modifierFactory = func(map[string]interface{}) func(interface{}) (interface{}, error)
)

func (f fakeRequest) Headers() map[string][]string { return f.h }

// Registro de los dos símbolos y recorrido completo a través del borde.
func TestEndToEndThroughSymbols(t *testing.T) {
	var newHandler handlerFactory
	HandlerRegisterer.RegisterHandlers(func(name string, f handlerFactory) {
		if name != "a2a-gateway" {
			t.Fatalf("nombre del plugin de servidor %q", name)
		}
		newHandler = f
	})
	var newModifier modifierFactory
	ModifierRegisterer.RegisterModifiers(func(name string, f modifierFactory, req, resp bool) {
		if name != "a2a-authz" || !req || resp {
			t.Fatalf("modificador mal registrado: %q %v %v", name, req, resp)
		}
		newModifier = f
	})

	// Como en Community, el modificador se crea antes que el borde.
	modify := newModifier(map[string]interface{}{
		"name":      []interface{}{"a2a-authz"},
		"a2a-authz": map[string]interface{}{"agent": "billing"},
	})
	if _, err := modify(fakeRequest{h: map[string][]string{}}); err == nil {
		t.Fatal("sin borde el modificador debe denegar")
	}

	krakend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-A2A-Sub", "alice")
		r.Header.Set("X-A2A-Iss", "https://idp")
		if _, err := modify(fakeRequest{h: r.Header}); err != nil {
			w.WriteHeader(err.(interface{ StatusCode() int }).StatusCode())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"task":{"id":"t1","contextId":"c1","status":{}}}}`))
	})
	h, err := newHandler(context.Background(), decode(t, block), krakend)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	send := `{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a/billing", strings.NewReader(send))
	req.Header.Set("A2A-Version", "1.0")
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"t1"`) {
		t.Fatalf("recorrido completo: %d %s", rec.Code, rec.Body)
	}
}
