// Copyright 2026 Los autores del plugin A2A para KrakenD.

package card

import (
	"encoding/json"
	"strings"
	"testing"

	"krakend-a2a.local/core/config"
	"krakend-a2a.local/core/protocol"
)

const gw = "https://gw.example.com/a2a/billing"

func tr() *Transformer {
	return &Transformer{
		GatewayURL: gw,
		Versions:   []protocol.Version{protocol.V10, protocol.V03},
		Streaming:  true,
		Push:       true,
		Extended:   true,
	}
}

func transform(t *testing.T, x *Transformer, card string) map[string]interface{} {
	t.Helper()
	out, err := x.Transform([]byte(card))
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("salida no es JSON: %v", err)
	}
	if strings.Contains(string(out), "billing.internal") {
		t.Fatalf("queda una URL interna en la card publicada: %s", out)
	}
	return m
}

const card10 = `{
  "name": "Billing", "description": "x", "version": "1.2.0",
  "supportedInterfaces": [
    {"url": "http://billing.internal:8080/a2a", "protocolBinding": "JSONRPC", "protocolVersion": "1.0"},
    {"url": "billing.internal:9090", "protocolBinding": "GRPC", "protocolVersion": "1.0"},
    {"url": "http://billing.internal:8080/rest", "protocolBinding": "HTTP+JSON", "protocolVersion": "1.0"},
    {"url": "http://billing.internal:8080/a2a", "protocolBinding": "JSONRPC", "protocolVersion": "1.0", "tenant": ` +
	`"acme"}
  ],
  "provider": {"organization": "Example", "url": "https://example.com"},
  "documentationUrl": "http://billing.internal:8080/docs",
  "iconUrl": "https://cdn.example.com/billing.png",
  "capabilities": {"streaming": true, "pushNotifications": true, "extendedAgentCard": true,
                   "extensions": [{"uri": "https://ext.example.com/x", "required": false}]},
  "securitySchemes": {"internal": {"apiKeySecurityScheme": {"location": "header", "name": "X-Internal-Key"}}},
  "securityRequirements": [{"schemes": {"internal": {"list": []}}}],
  "defaultInputModes": ["text/plain"], "defaultOutputModes": ["text/plain"],
  "skills": [{"id": "pay", "name": "Pay", "description": "d", "tags": ["t"], "securityRequirements": [{"schemes": ` +
	`{"internal": {}}}]}],
  "signatures": [{"protected": "eyJhbGciOiJFUzI1NiJ9", "signature": "abc"}],
  "x-vendor-field": {"keep": 42.50}
}`

// Escenario: card 1.0 con varios bindings; solo JSON-RPC sobrevive y apunta al gateway.
func TestInterfaces10(t *testing.T) {
	m := transform(t, tr(), card10)
	ifaces := m["supportedInterfaces"].([]interface{})
	if len(ifaces) != 2 {
		t.Fatalf("se esperaban 2 interfaces JSON-RPC (sin tenant y con tenant), hay %d: %v", len(ifaces), ifaces)
	}
	for _, i := range ifaces {
		iface := i.(map[string]interface{})
		if iface["url"] != gw || iface["protocolBinding"] != "JSONRPC" {
			t.Errorf("interfaz mal reescrita: %v", iface)
		}
	}
	// Escenario: interfaz con tenant.
	if ifaces[1].(map[string]interface{})["tenant"] != "acme" {
		t.Errorf("el tenant debe conservarse: %v", ifaces[1])
	}
}

func TestVersionFilter(t *testing.T) {
	x := tr()
	x.Versions = []protocol.Version{protocol.V03}
	_, err := x.Transform([]byte(card10))
	if err == nil {
		t.Fatal("una card solo 1.0 con 1.0 deshabilitada no tiene interfaces utilizables")
	}
}

// Escenario: card con forma 0.3.
func TestInterfaces03(t *testing.T) {
	m := transform(t, tr(), `{
	  "name": "Legacy", "description": "x", "version": "1", "protocolVersion": "0.3.0",
	  "url": "billing.internal:9090", "preferredTransport": "GRPC",
	  "additionalInterfaces": [
	    {"url": "billing.internal:9090", "transport": "GRPC"},
	    {"url": "http://billing.internal:8080/a2a", "transport": "JSONRPC"}
	  ],
	  "capabilities": {"streaming": true}, "defaultInputModes": [], "defaultOutputModes": [], "skills": []
	}`)
	if m["url"] != gw || m["preferredTransport"] != "JSONRPC" {
		t.Errorf("url/preferredTransport mal reescritos: %v %v", m["url"], m["preferredTransport"])
	}
	add := m["additionalInterfaces"].([]interface{})
	if len(add) != 1 || add[0].(map[string]interface{})["url"] != gw {
		t.Errorf("additionalInterfaces mal reescrito: %v", add)
	}
}

func TestInterfaces03WithoutJSONRPCIsUnusable(t *testing.T) {
	_, err := tr().Transform([]byte(`{"name":"x","url":"h:1","preferredTransport":"GRPC","capabilities":{}}`))
	if err == nil {
		t.Fatal("una card 0.3 solo gRPC no tiene interfaz utilizable")
	}
}

// Una card compatible (como la del SDK oficial) con 0.3 deshabilitada pierde
// los campos de 0.3.
func TestCompatCardWithout03(t *testing.T) {
	x := tr()
	x.Versions = []protocol.Version{protocol.V10}
	m := transform(t, x, `{
	  "name": "c", "url": "http://billing.internal:8080/", "preferredTransport": "JSONRPC", "protocolVersion": "0.3",
	  "supportedInterfaces": [
	    {"url": "http://billing.internal:8080/", "protocolBinding": "JSONRPC", "protocolVersion": "1.0"},
	    {"url": "http://billing.internal:8080/", "protocolBinding": "JSONRPC", "protocolVersion": "0.3"}
	  ],
	  "capabilities": {}
	}`)
	for _, k := range []string{"url", "preferredTransport", "protocolVersion"} {
		if _, ok := m[k]; ok {
			t.Errorf("con 0.3 deshabilitada sobra el campo %s", k)
		}
	}
	if n := len(m["supportedInterfaces"].([]interface{})); n != 1 {
		t.Errorf("solo debe quedar la interfaz 1.0, hay %d", n)
	}
}

// Escenarios: esquemas sustituidos y sin esquemas configurados.
func TestSecurity(t *testing.T) {
	m := transform(t, tr(), card10)
	if _, ok := m["securitySchemes"].(map[string]interface{})["internal"]; !ok {
		t.Error("sin configuración deben conservarse los esquemas del agente")
	}

	x := tr()
	x.Security = &config.Security{
		Schemes: map[string]json.RawMessage{
			"oidc": json.RawMessage(`{"openIdConnectSecurityScheme": ` +
				`{"openIdConnectUrl": "https://idp.example.com/.well-known/openid-configuration"}}`),
		},
		Requirements: []json.RawMessage{json.RawMessage(`{"schemes": {"oidc": {"list": ["a2a.read"]}}}`)},
	}
	m = transform(t, x, card10)
	schemes := m["securitySchemes"].(map[string]interface{})
	if len(schemes) != 1 || schemes["oidc"] == nil {
		t.Fatalf("deben quedar solo los esquemas del gateway: %v", schemes)
	}
	if _, ok := schemes["oidc"].(map[string]interface{})["type"]; ok {
		t.Error("una card solo 1.0 no lleva campos de 0.3 en los esquemas")
	}
	reqs := m["securityRequirements"].([]interface{})
	if len(reqs) != 1 {
		t.Fatalf("requisitos mal sustituidos: %v", reqs)
	}
	if _, ok := m["security"]; ok {
		t.Error("una card solo 1.0 no lleva security de 0.3")
	}
	skill := m["skills"].([]interface{})[0].(map[string]interface{})
	if _, ok := skill["securityRequirements"]; ok {
		t.Error("los requisitos por skill que nombran esquemas del agente deben quitarse")
	}
}

// Card compatible: los esquemas se publican en las dos formas, como el SDK.
func TestSecurityDualForCompatCard(t *testing.T) {
	x := tr()
	x.Security = &config.Security{
		Schemes: map[string]json.RawMessage{
			"key":    json.RawMessage(`{"apiKeySecurityScheme": {"location": "header", "name": "X-Key", "description": "d"}}`),
			"bearer": json.RawMessage(`{"httpAuthSecurityScheme": {"scheme": "Bearer", "bearerFormat": "JWT"}}`),
			"mtls":   json.RawMessage(`{"mtlsSecurityScheme": {}}`),
		},
		Requirements: []json.RawMessage{json.RawMessage(`{"schemes": {"bearer": {"list": ["a2a.read"]}}}`)},
	}
	m := transform(t, x, `{
	  "name": "c", "url": "http://billing.internal:8080/", "protocolVersion": "0.3",
	  "supportedInterfaces": [{"url": "http://billing.internal:8080/", "protocolBinding": "JSONRPC", "protocolVersion": `+
		`"1.0"}],
	  "capabilities": {}
	}`)
	key := m["securitySchemes"].(map[string]interface{})["key"].(map[string]interface{})
	if key["type"] != "apiKey" || key["in"] != "header" || key["name"] != "X-Key" || key["apiKeySecurityScheme"] == nil {
		t.Errorf("esquema dual mal construido: %v", key)
	}
	schemes := m["securitySchemes"].(map[string]interface{})
	bearer := schemes["bearer"].(map[string]interface{})
	if bearer["type"] != "http" || bearer["scheme"] != "Bearer" || bearer["bearerFormat"] != "JWT" {
		t.Errorf("esquema http mal traducido: %v", bearer)
	}
	if mtls := schemes["mtls"].(map[string]interface{}); mtls["type"] != "mutualTLS" {
		t.Errorf("esquema mTLS mal traducido: %v", mtls)
	}
	sec := m["security"].([]interface{})[0].(map[string]interface{})
	if scopes := sec["bearer"].([]interface{}); len(scopes) != 1 || scopes[0] != "a2a.read" {
		t.Errorf("requisito 0.3 mal traducido: %v", sec)
	}
	if _, ok := m["securityRequirements"]; !ok {
		t.Error("la card compatible conserva también los requisitos de 1.0")
	}
}

// Escenario: capacidades efectivas.
func TestCapabilities(t *testing.T) {
	x := tr()
	x.Streaming, x.Push, x.Extended = false, false, false
	m := transform(t, x, card10)
	caps := m["capabilities"].(map[string]interface{})
	for _, k := range []string{"streaming", "pushNotifications", "extendedAgentCard"} {
		if caps[k] != false {
			t.Errorf("capabilities.%s debe ser false si el gateway no la admite: %v", k, caps[k])
		}
	}
	m = transform(t, tr(), `{"name":"x","url":"http://billing.internal/","capabilities":{"streaming":false},`+
		`"supportsAuthenticatedExtendedCard":true}`)
	if m["capabilities"].(map[string]interface{})["streaming"] != false {
		t.Error("una capacidad que el agente no declara no se activa")
	}
	if m["supportsAuthenticatedExtendedCard"] != true {
		t.Error("supportsAuthenticatedExtendedCard admitida debe conservarse")
	}
}

// Escenarios: campos desconocidos, firmas y URLs internas.
func TestPreservationSignaturesAndScrub(t *testing.T) {
	m := transform(t, tr(), card10)
	if v, ok := m["x-vendor-field"].(map[string]interface{}); !ok || v["keep"] != 42.5 {
		t.Errorf("los campos desconocidos deben conservarse: %v", m["x-vendor-field"])
	}
	if _, ok := m["signatures"]; ok {
		t.Error("una card transformada no puede llevar signatures")
	}
	if _, ok := m["documentationUrl"]; ok {
		t.Error("documentationUrl interna debe eliminarse")
	}
	if m["iconUrl"] != "https://cdn.example.com/billing.png" {
		t.Error("las URLs externas deben conservarse")
	}
	if m["provider"].(map[string]interface{})["url"] != "https://example.com" {
		t.Error("la URL del proveedor debe conservarse")
	}
	ext := m["capabilities"].(map[string]interface{})["extensions"].([]interface{})
	if len(ext) != 1 {
		t.Error("las extensiones deben conservarse")
	}
}

func TestInvalidCards(t *testing.T) {
	for _, c := range []string{`[]`, `"x"`, `{`, `{"name":"sin interfaces"}`, `{} {}`} {
		if _, err := tr().Transform([]byte(c)); err == nil {
			t.Errorf("%q debería rechazarse", c)
		}
	}
}

func TestNewTransformerFromConfig(t *testing.T) {
	cfg, err := config.Parse([]byte(`{
	  "public_url": "https://gw.example.com/", "base_path": "/agents",
	  "agents": [{"id": "billing", "versions": ["1.0"],
	    "card": {"source": "upstream", "url": "http://billing.internal:8080/.well-known/agent-card.json"},
	    "capabilities": {"streaming": true, "push_notifications": true},
	    "disabled_operations": ["GetExtendedAgentCard", "CreateTaskPushNotificationConfig"]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	x := NewTransformer(cfg, &cfg.Agents[0], true)
	if x.GatewayURL != "https://gw.example.com/agents/billing" {
		t.Errorf("GatewayURL = %q", x.GatewayURL)
	}
	if !x.Streaming || x.Push || x.Extended {
		t.Errorf("capacidades efectivas mal calculadas: streaming=%v push=%v extended=%v", x.Streaming, x.Push, x.Extended)
	}
	if len(x.InternalOrigins) != 1 || x.InternalOrigins[0] != "http://billing.internal:8080" {
		t.Errorf("origen interno no registrado: %v", x.InternalOrigins)
	}
	if NewTransformer(cfg, &cfg.Agents[0], false).Streaming {
		t.Error("sin streaming en la edición la capacidad debe desactivarse")
	}
}
