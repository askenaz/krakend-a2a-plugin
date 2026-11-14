// Copyright 2026 Los autores del plugin A2A para KrakenD.

package config

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const minimal = `{
  "public_url": "https://gw.example.com",
  "agents": [
    {"id": "billing", "card": {"source": "upstream", "url": "http://billing.internal:8080/.well-known/agent-card.json"}}
  ]
}`

func mustParse(t *testing.T, s string) *Config {
	t.Helper()
	cfg, err := Parse([]byte(s))
	if err != nil {
		t.Fatalf("configuración válida rechazada: %v", err)
	}
	return cfg
}

func TestDefaults(t *testing.T) {
	cfg := mustParse(t, minimal)

	checks := []struct {
		name      string
		got, want interface{}
	}{
		{"base_path", cfg.BasePath, "/a2a"},
		{"internal_prefix", cfg.InternalPrefix, "/__a2a"},
		{"route_granularity", cfg.RouteGranularity, "class"},
		{"max_body_bytes", cfg.Limits.MaxBodyBytes, int64(4 << 20)},
		{"max_inspect_bytes", cfg.Limits.MaxInspectBytes, int64(8 << 20)},
		{"identity.required", cfg.IdentityRequired(), true},
		{"identity.headers.subject", cfg.Identity.Headers["subject"], "X-A2A-Sub"},
		{"identity.owner_scope", strings.Join(cfg.Identity.OwnerScope, ","), "issuer,subject"},
		{"ownership.store", cfg.Ownership.Store, "memory"},
		{"ownership.ttl", cfg.Ownership.TTL.D(), 168 * time.Hour},
		{"ownership.unknown_task", cfg.Ownership.UnknownTask, "deny"},
		{"ownership.list_tasks", cfg.Ownership.ListTasks, "owned_context"},
		{"ownership.on_store_error", cfg.Ownership.OnStoreError, "deny"},
		{"push.require_https", *cfg.PushNotifications.RequireHTTPS, true},
		{"push.deny_private_networks", *cfg.PushNotifications.DenyPrivateNetworks, true},
		{"challenge.scheme", cfg.Challenge.Scheme, "Bearer"},
		{"agent.versions", strings.Join(cfg.Agents[0].Versions, ","), "1.0"},
		{"agent.card.ttl", cfg.Agents[0].Card.TTL.D(), 5 * time.Minute},
		{"agent.card.max_stale", cfg.Agents[0].Card.MaxStale.D(), time.Hour},
		{"agent.card.http_max_age", cfg.Agents[0].Card.HTTPMaxAge.D(), 5 * time.Minute},
		{"agent.streaming", cfg.Agents[0].StreamingEnabled(), true},
		{"agent.push", cfg.Agents[0].PushEnabled(), false},
		{"agent.ownership", cfg.OwnershipEnabled(&cfg.Agents[0]), true},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, se esperaba %v", c.name, c.got, c.want)
		}
	}
	if len(cfg.Warnings) == 0 || !strings.Contains(strings.Join(cfg.Warnings, "\n"), "ownership.store=memory") {
		t.Errorf("falta el aviso del almacén en memoria: %v", cfg.Warnings)
	}
}

func TestFullExample(t *testing.T) {
	cfg := mustParse(t, `{
	  "public_url": "https://gw.example.com/",
	  "base_path": "/agents/",
	  "internal_prefix": "/__a2a",
	  "route_granularity": "operation",
	  "default_agent": "billing",
	  "limits": {"max_body_bytes": 1048576, "max_inspect_bytes": 2097152},
	  "identity": {
	    "required": true,
	    "headers": {"issuer": "X-A2A-Iss", "subject": "X-A2A-Sub", "scopes": "X-A2A-Scope", "tenant": "X-A2A-Tenant"},
	    "owner_scope": ["issuer", "tenant"]
	  },
	  "authorization": {"operation_scopes": {"GetTask": ["a2a.read"], "CancelTask": ["a2a.write", "a2a.admin"]}},
	  "ownership": {
	    "enabled": true, "store": "redis", "ttl": "720h",
	    "redis": {"address": "redis.internal:6379", "db": 2, "tls": true, "password_env": "A2A_REDIS_PASSWORD", `+
		`"timeout": "500ms"},
	    "unknown_task": "deny", "list_tasks": "deny", "on_store_error": "deny"
	  },
	  "push_notifications": {"require_https": true, "deny_private_networks": true, "allowed_hosts": `+
		`["*.hooks.example.com", "hooks.partner.example"]},
	  "challenge": {"scheme": "Bearer", "realm": "agents", "resource_metadata": true,
	    "authorization_servers": ["https://idp.example.com/realms/agents"], "scopes_supported": ["a2a.read", "a2a.write"]},
	  "agents": [
	    {
	      "id": "billing",
	      "versions": ["1.0", "0.3"],
	      "card": {"source": "upstream", "url": "https://billing.internal/.well-known/agent-card.json",
	        "ttl": "1m", "max_stale": "30m", "max_bytes": 65536, "http_max_age": "0s",
	        "signature": {"verify": true, "jwks_file": "/etc/krakend/a2a/billing.jwks.json"}},
	      "security": {
	"schemes": {"oidc": {"openIdConnectSecurityScheme": {
	  "openIdConnectUrl": "https://idp.example.com/.well-known/openid-configuration"}}},
	        "requirements": [{"schemes": {"oidc": {"list": ["a2a.read"]}}}]
	      },
	      "capabilities": {"streaming": false, "push_notifications": true},
	      "disabled_operations": ["ListTasks"],
	      "ownership": {"unknown_task": "passthrough"}
	    },
	    {
	      "id": "faq-bot",
	      "card": {"source": "static", "inline": {"name": "FAQ", "description": "x", "version": "1"}},
	      "ownership": {"enabled": false}
	    }
	  ]
	}`)

	if cfg.PublicURL != "https://gw.example.com" || cfg.BasePath != "/agents" {
		t.Errorf("barras finales no normalizadas: %q %q", cfg.PublicURL, cfg.BasePath)
	}
	billing, _ := cfg.Agent("billing")
	faq, _ := cfg.Agent("faq-bot")
	if billing.StreamingEnabled() || !billing.PushEnabled() {
		t.Error("capacidades de billing mal leídas")
	}
	if billing.Card.HTTPMaxAge.D() != 0 {
		t.Error("http_max_age=0s debe respetarse, no sustituirse por el valor por defecto")
	}
	if cfg.UnknownTaskPolicy(billing) != "passthrough" || cfg.UnknownTaskPolicy(faq) != "deny" {
		t.Error("la política por agente debe sobrescribir la global y heredarse si falta")
	}
	if cfg.ListTasksPolicy(billing) != "deny" {
		t.Error("list_tasks global no heredado")
	}
	if cfg.OwnershipEnabled(faq) || !cfg.OwnershipEnabled(billing) {
		t.Error("ownership.enabled por agente mal aplicado")
	}
	if strings.Contains(strings.Join(cfg.Warnings, "\n"), "store=memory") {
		t.Error("con redis no debe avisarse del almacén en memoria")
	}
	if !strings.Contains(strings.Join(cfg.Warnings, "\n"), "agents[billing].ownership.unknown_task=passthrough") {
		t.Errorf("falta el aviso de passthrough por agente: %v", cfg.Warnings)
	}
}

// KrakenD entrega la configuración ya decodificada, con números float64.
func TestFromMapWithFloatNumbers(t *testing.T) {
	var m map[string]interface{}
	src := strings.Replace(minimal, `"agents"`, `"limits": {"max_body_bytes": 2097152}, "agents"`, 1)
	if err := json.Unmarshal([]byte(src), &m); err != nil {
		t.Fatal(err)
	}
	cfg, err := FromMap(m)
	if err != nil {
		t.Fatalf("mapa de KrakenD rechazado: %v", err)
	}
	if cfg.Limits.MaxBodyBytes != 2097152 {
		t.Fatalf("max_body_bytes = %d", cfg.Limits.MaxBodyBytes)
	}
}

// Escenario de la spec a2a-edition-packaging: el error nombra el campo y el
// agente afectado.
func TestAgentPolicyErrorNamesFieldAndAgent(t *testing.T) {
	_, err := Parse([]byte(`{
	  "public_url": "https://gw.example.com",
	  "agents": [{"id": "billing", "card": {"source": "upstream", "url": "https://b.internal/c.json"},
	              "ownership": {"unknown_task": "allow"}}]
	}`))
	assertFieldError(t, err, "agents[billing].ownership.unknown_task", `valor "allow" no admitido`)
}

// Escenario de la spec a2a-edition-packaging: identificadores duplicados.
func TestDuplicateAgentIDs(t *testing.T) {
	_, err := Parse([]byte(`{
	  "public_url": "https://gw.example.com",
	  "agents": [
	    {"id": "billing", "card": {"source": "upstream", "url": "https://a.internal/c.json"}},
	    {"id": "billing", "card": {"source": "upstream", "url": "https://b.internal/c.json"}}
	  ]
	}`))
	assertFieldError(t, err, "agents[billing].id", `identificador duplicado "billing"`)
}

func TestStrictReading(t *testing.T) {
	cases := []struct {
		name    string
		patch   func(string) string
		path    string
		message string
	}{
		{"campo desconocido en la raíz",
			func(s string) string { return strings.Replace(s, `"agents"`, `"publicURL": "x", "agents"`, 1) },
			"publicURL", "campo desconocido"},
		{"campo desconocido dentro de un agente",
			func(s string) string {
				return strings.Replace(s, `"source": "upstream"`, `"source": "upstream", "refresh": "1m"`, 1)
			},
			"agents[billing].card.refresh", "campo desconocido"},
		{"número escrito como texto",
			func(s string) string {
				return strings.Replace(s, `"agents"`, `"limits": {"max_body_bytes": "4MB"}, "agents"`, 1)
			},
			"limits.max_body_bytes", "debe ser un número entero, no una cadena"},
		{"número decimal",
			func(s string) string {
				return strings.Replace(s, `"agents"`, `"limits": {"max_body_bytes": 1.5}, "agents"`, 1)
			},
			"limits.max_body_bytes", "debe ser un número entero"},
		{"booleano escrito como texto",
			func(s string) string {
				return strings.Replace(s, `"agents"`, `"identity": {"required": "yes"}, "agents"`, 1)
			},
			"identity.required", "debe ser true o false"},
		{"null",
			func(s string) string { return strings.Replace(s, `"agents"`, `"base_path": null, "agents"`, 1) },
			"base_path", "no puede ser null"},
		{"duración mal escrita",
			func(s string) string {
				return strings.Replace(s, `"source": "upstream"`, `"source": "upstream", "ttl": "5 minutos"`, 1)
			},
			"agents[billing].card.ttl", "duración inválida"},
		{"duración numérica",
			func(s string) string {
				return strings.Replace(s, `"source": "upstream"`, `"source": "upstream", "ttl": 300`, 1)
			},
			"agents[billing].card.ttl", "debe ser una duración"},
		{"lista donde va un objeto",
			func(s string) string { return strings.Replace(s, `"agents"`, `"identity": [], "agents"`, 1) },
			"identity", "debe ser un objeto, no una lista"},
		{"agentes no es una lista",
			func(s string) string { return `{"public_url": "https://gw.example.com", "agents": {}}` },
			"agents", "debe ser una lista"},
		{"agente sin id se nombra por posición",
			func(s string) string { return strings.Replace(s, `"id": "billing", `, `"name": "billing", `, 1) },
			"agents[0].name", "campo desconocido"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.patch(minimal)))
			assertFieldError(t, err, c.path, c.message)
		})
	}
}

func TestSemanticErrors(t *testing.T) {
	agent := func(extra string) string {
		return `{"public_url": "https://gw.example.com", "agents": [{"id": "billing", "card": {"source": "upstream", ` +
			`"url": "https://b.internal/c.json"}` + extra + `}]}`
	}
	root := func(extra string) string {
		return `{"public_url": "https://gw.example.com", ` + extra + `, "agents": [` +
			`{"id": "billing", "card": {"source": "upstream", "url": "https://b.internal/c.json"}}]}`
	}
	cases := []struct {
		name, cfg, path, message string
	}{
		{"sin public_url",
			`{"agents": [{"id": "a", "card": {"source": "static", "file": "/c.json"}}]}`, "public_url", "es obligatorio"},
		{"public_url con ruta",
			`{"public_url": "https://gw.example.com/x", "agents": [{"id": "a", "card": {"source": "static",` +
				` "file": "/c.json"}}]}`, "public_url", "la ruta va en base_path"},
		{"public_url ftp",
			`{"public_url": "ftp://gw", "agents": [{"id": "a", "card": {"source": "static", "file": "/c.json"}}]}`, "public_url",
			"debe usar http o https"},
		{"public_url con credenciales",
			`{"public_url": "https://u:p@gw", "agents": [{"id": "a", "card": {"source": "static", "file": "/c.json"}}]}`,
			"public_url", "credenciales"},
		{"base_path raíz", root(`"base_path": "/"`), "base_path", "no puede ser la raíz"},
		{"base_path con ..", root(`"base_path": "/a/../b"`), "base_path", "segmentos"},
		{"base_path con comodín", root(`"base_path": "/a2a/*"`), "base_path", "inválida"},
		{"prefijos solapados", root(`"base_path": "/__a2a/public"`), "internal_prefix", "solaparse"},
		{"base_path bajo well-known", root(`"base_path": "/.well-known/a2a"`), "base_path", "/.well-known"},
		{"granularidad desconocida", root(`"route_granularity": "agent"`), "route_granularity", "no admitido"},
		{"agente por defecto inexistente", root(`"default_agent": "otro"`), "default_agent", `"otro"`},
		{"cuerpo máximo excesivo",
			root(`"limits": {"max_body_bytes": 1073741824}`), "limits.max_body_bytes", "debe estar entre"},
		{"sin cabecera de subject",
			root(`"identity": {"headers": {"issuer": "X-Iss"}, "owner_scope": ["issuer"]}`), "identity.headers", "subject"},
		{"cabecera de identidad prohibida",
			root(`"identity": {"headers": {"subject": "Authorization"}}`), "identity.headers.subject", "borra esta cabecera"},
		{"cabecera reservada",
			root(`"identity": {"headers": {"subject": "X-A2A-Operation"}}`), "identity.headers.subject", "interna"},
		{"cabecera repetida",
			root(`"identity": {"headers": {"subject": "X-User", "issuer": "x-user"}}`), "identity.headers.subject", "ya se usa"},
		{"nombre de cabecera inválido",
			root(`"identity": {"headers": {"subject": "X User"}}`), "identity.headers.subject", "no es un nombre de cabecera"},
		{"owner_scope con scopes",
			root(`"identity": {"owner_scope": ["scopes"]}`), "identity.owner_scope[0]", "no identifica"},
		{"owner_scope sin cabecera",
			root(`"identity": {"owner_scope": ["subject", "tenant"]}`), "identity.owner_scope[1]", "no está definido"},
		{"operación desconocida en scopes",
			root(`"authorization": {"operation_scopes": {"tasks/get": ["a2a.read"]}}`),
			"authorization.operation_scopes.tasks/get", "operación desconocida"},
		{"scope vacío",
			root(`"authorization": {"operation_scopes": {"GetTask": []}}`), "authorization.operation_scopes.GetTask",
			"al menos un scope"},
		{"scope con espacio",
			root(`"authorization": {"operation_scopes": {"GetTask": ["a b"]}}`), "authorization.operation_scopes.GetTask[0]",
			"no es un scope"},
		{"scopes sin cabecera de scopes",
			root(`"identity": {"headers": {"subject": "X-Sub"}}, "authorization": {"operation_scopes": {"GetTask": ["r"]}}`),
			"authorization.operation_scopes", "identity.headers.scopes"},
		{"propiedad sin identidad", root(`"identity": {"required": false}`), "ownership.enabled", "identity.required=true"},
		{"almacén desconocido", root(`"ownership": {"store": "etcd"}`), "ownership.store", "no admitido"},
		{"redis sin dirección", root(`"ownership": {"store": "redis"}`), "ownership.redis.address", "obligatorio"},
		{"redis sin puerto",
			root(`"ownership": {"store": "redis", "redis": {"address": "redis.internal"}}`), "ownership.redis.address",
			"host:puerto"},
		{"variable de entorno inválida",
			root(`"ownership": {"store": "redis", "redis": {"address": "r:6379", "username_env": "A2A-USER"}}`),
			"ownership.redis.username_env", "variable de entorno"},
		{"ttl de propiedad demasiado corto", root(`"ownership": {"ttl": "10s"}`), "ownership.ttl", "debe estar entre"},
		{"política list_tasks desconocida",
			root(`"ownership": {"list_tasks": "all"}`), "ownership.list_tasks", "no admitido"},
		{"host de webhook inválido",
			root(`"push_notifications": {"allowed_hosts": ["https://hooks.example.com"]}`),
			"push_notifications.allowed_hosts[0]", "no es un host válido"},
		{"comodín en medio",
			root(`"push_notifications": {"allowed_hosts": ["hooks.*.example.com"]}`), "push_notifications.allowed_hosts[0]",
			"no es un host válido"},
		{"metadatos sin servidores",
			root(`"challenge": {"resource_metadata": true}`), "challenge.authorization_servers", "obligatorio"},
		{"realm con comillas", root(`"challenge": {"realm": "a\"b"}`), "challenge.realm", "comillas"},
		{"sin agentes", `{"public_url": "https://gw.example.com", "agents": []}`, "agents", "al menos un agente"},
		{"id de agente inválido",
			`{"public_url": "https://gw.example.com", "agents": [{"id": "Billing_1", "card": {"source": "static",` +
				` "file": "/c"}}]}`, "agents[Billing_1].id", "inválido"},
		{"versión no admitida", agent(`, "versions": ["1.0.0"]`), "agents[billing].versions[0]", "no admitida"},
		{"versión repetida", agent(`, "versions": ["1.0", "1.0"]`), "agents[billing].versions[1]", "repetida"},
		{"operación deshabilitada desconocida",
			agent(`, "disabled_operations": ["message/send"]`), "agents[billing].disabled_operations[0]",
			"operación desconocida"},
		{"card sin source",
			`{"public_url": "https://gw.example.com", "agents": [{"id": "a", "card": {"url": "https://x/c"}}]}`,
			"agents[a].card.source", "obligatorio"},
		{"card estática sin contenido",
			`{"public_url": "https://gw.example.com", "agents": [{"id": "a", "card": {"source": "static"}}]}`, "agents[a].card",
			"exactamente uno"},
		{"card estática con dos orígenes",
			`{"public_url": "https://gw.example.com", "agents": [{"id": "a", "card": {"source": "static", "file": "/c",` +
				` "inline": {}}}]}`, "agents[a].card", "exactamente uno"},
		{"card inline que no es objeto",
			`{"public_url": "https://gw.example.com", "agents": [{"id": "a", "card": {"source": "static", "inline": ["x"]}}]}`,
			"agents[a].card.inline", "objeto JSON"},
		{"firma con card estática",
			`{"public_url": "https://gw.example.com", "agents": [{"id": "a", "card": {"source": "static", "file": "/c",` +
				` "signature": {"verify": true, "jwks_file": "/k"}}}]}`, "agents[a].card.signature.verify",
			"solo aplica a source upstream"},
		{"card remota sin url",
			`{"public_url": "https://gw.example.com", "agents": [{"id": "a", "card": {"source": "upstream"}}]}`,
			"agents[a].card.url", "obligatorio"},
		{"max_stale menor que ttl",
			`{"public_url": "https://gw.example.com", "agents": [{"id": "a", "card": {"source": "upstream",` +
				` "url": "https://b/c", "ttl": "10m", "max_stale": "1m"}}]}`, "agents[a].card.max_stale", "menor que ttl"},
		{"firma sin claves",
			`{"public_url": "https://gw.example.com", "agents": [{"id": "a", "card": {"source": "upstream",` +
				` "url": "https://b/c", "signature": {"verify": true}}}]}`, "agents[a].card.signature.jwks_file", "obligatorio"},
		{"esquema de seguridad desconocido",
			agent(`, "security": {"schemes": {"k": {"basicAuth": {}}}}`), "agents[billing].security.schemes.k",
			"exactamente un tipo"},
		{"requisito con esquema no definido",
			agent(`, "security": {"schemes": {"k": {"apiKeySecurityScheme": {"location": "header", "name": "X-Key"}}},` +
				` "requirements": [{"schemes": {"otro": {}}}]}`), "agents[billing].security.requirements[0].schemes.otro",
			"no está definido"},
		{"propiedad por agente sin identidad",
			`{"public_url": "https://gw.example.com", "identity": {"required": false}, "ownership": {"enabled": false},` +
				` "agents": [{"id": "a", "card": {"source": "static", "file": "/c"}, "ownership": {"enabled": true}}]}`,
			"agents[a].ownership.enabled", "identity.required=true"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.cfg))
			assertFieldError(t, err, c.path, c.message)
		})
	}
}

func TestAllErrorsReportedTogether(t *testing.T) {
	_, err := Parse([]byte(`{"public_url": "ftp://x", "route_granularity": "x", "agents": [{"id": "a", "card": ` +
		`{"source": "nope"}}]}`))
	var cfgErr *Error
	if !errors.As(err, &cfgErr) {
		t.Fatalf("se esperaba *config.Error, obtenido %v", err)
	}
	if len(cfgErr.Fields) < 3 {
		t.Fatalf("se esperaban al menos 3 errores juntos, hay %d: %v", len(cfgErr.Fields), err)
	}
}

func TestErrorOrderIsStable(t *testing.T) {
	src := `{"zeta": 1, "alfa": 2, "beta": 3, "public_url": "https://gw.example.com", "agents": []}`
	_, err := Parse([]byte(src))
	first := err.Error()
	for i := 0; i < 20; i++ {
		if _, err = Parse([]byte(src)); err.Error() != first {
			t.Fatalf("el orden de los errores cambia entre ejecuciones:\n%s\n%s", first, err)
		}
	}
}

func TestParseRejectsNonObjects(t *testing.T) {
	for _, s := range []string{`[]`, `"x"`, `{`, `{} {}`} {
		if _, err := Parse([]byte(s)); err == nil {
			t.Errorf("%q debería rechazarse", s)
		}
	}
	if _, err := FromMap(nil); err == nil {
		t.Error("un bloque ausente debería rechazarse")
	}
}

func assertFieldError(t *testing.T, err error, path, message string) {
	t.Helper()
	var cfgErr *Error
	if !errors.As(err, &cfgErr) {
		t.Fatalf("se esperaba un error de configuración en %s, obtenido %v", path, err)
	}
	for _, f := range cfgErr.Fields {
		if f.Path == path && strings.Contains(f.Message, message) {
			return
		}
	}
	t.Fatalf("no hay error en %s con %q; errores: %v", path, message, err)
}
