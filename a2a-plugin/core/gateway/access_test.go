// Copyright 2026 Los autores del plugin A2A para KrakenD.

package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Escenarios: token válido y cabecera de identidad enviada por el cliente.
func TestIdentityFromNativeAuth(t *testing.T) {
	h := newHarness(t, baseConfig)
	h.rpc(alice, "SendMessage", newTask)
	if h.lastReq.Header.Get("X-A2A-Sub") != "alice" {
		t.Fatal("la identidad debe salir de la autenticación nativa")
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":` + newTask + `}`
	res := h.do(http.MethodPost, "/a2a/billing", "", body,
		map[string]string{"A2A-Version": "1.0", "X-A2A-Sub": "alice", "x-a2a-request-id": "forjado"})
	if res.status != 401 {
		t.Fatalf("sin token, las cabeceras de identidad del cliente no deben servir: %d", res.status)
	}
}

// Escenarios: petición sin credenciales (401 con desafío) y rol insuficiente.
func TestAuthErrorsAtHTTPLevel(t *testing.T) {
	h := newHarness(t, baseConfig)
	res := h.rpc("", "SendMessage", newTask)
	if res.status != 401 || res.code() != -32603 {
		t.Fatalf("sin token: %d %s", res.status, res.body)
	}
	want := `Bearer realm="agents", ` +
		`resource_metadata="https://gw.example.com/.well-known/oauth-protected-resource/a2a/billing"`
	if res.header.Get("WWW-Authenticate") != want {
		t.Errorf("WWW-Authenticate = %q", res.header.Get("WWW-Authenticate"))
	}
	if res = h.rpc("sub=x,iss=i,role=none", "SendMessage", newTask); res.status != 403 {
		t.Errorf("rol insuficiente: %d", res.status)
	}
	if h.agentHits() != 0 {
		t.Error("nada de esto debe llegar al agente")
	}
}

// Escenario: endpoint interno sin autenticación nativa: fallo cerrado.
func TestFailClosedWithoutIdentity(t *testing.T) {
	h := newHarness(t, baseConfig)
	h.withAuth = false
	res := h.rpc(alice, "SendMessage", newTask)
	if res.status != 401 || h.agentHits() != 0 {
		t.Fatalf("sin identidad propagada se espera 401 sin llegar al agente: %d", res.status)
	}
}

// Escenarios: scope insuficiente y suficiente.
func TestOperationScopes(t *testing.T) {
	h := newHarness(t, baseConfig)
	created := h.rpc(reader, "SendMessage", newTask)
	taskID := taskOf(t, created)
	hits := h.agentHits()
	res := h.rpc(reader, "CancelTask", `{"id":"`+taskID+`"}`)
	if res.status != 403 || h.agentHits() != hits {
		t.Fatalf("CancelTask sin a2a.write: %d %s", res.status, res.body)
	}
	if res = h.rpc(reader, "GetTask", `{"id":"`+taskID+`"}`); res.code() != 0 {
		t.Fatalf("GetTask sin scopes exigidos: %s", res.body)
	}
}

// Escenarios: registro de propiedad y acceso solo para el propietario.
func TestTaskOwnership(t *testing.T) {
	h := newHarness(t, baseConfig)
	taskID := taskOf(t, h.rpc(alice, "SendMessage", newTask))

	if res := h.rpc(alice, "GetTask", `{"id":"`+taskID+`"}`); res.code() != 0 {
		t.Fatalf("el propietario debe acceder: %s", res.body)
	}
	hits := h.agentHits()
	for _, c := range []struct{ method, params string }{
		{"CancelTask", `{"id":"` + taskID + `"}`},
		{"GetTask", `{"id":"` + taskID + `"}`},
		{"GetTaskPushNotificationConfig", `{"taskId":"` + taskID + `","id":"cfg"}`},
		{"SubscribeToTask", `{"id":"` + taskID + `"}`},
		{"SendMessage", `{"message":{"messageId":"m","taskId":"` + taskID + `"}}`},
	} {
		res := h.rpc(bob, c.method, c.params)
		if res.status != 200 || res.code() != -32001 || res.rpc.Error.Message != "Task not found" {
			t.Errorf("%s de otro principal: %d %s", c.method, res.status, res.body)
		}
	}
	if h.agentHits() != hits {
		t.Error("las peticiones de otro principal no deben llegar al agente")
	}
	// Respuesta indistinguible de una tarea inexistente.
	ajena := h.rpc(bob, "GetTask", `{"id":"`+taskID+`"}`)
	inexistente := h.rpc(bob, "GetTask", `{"id":"no-existe"}`)
	if ajena.body != inexistente.body {
		t.Errorf("tarea ajena e inexistente deben responder igual:\n%s\n%s", ajena.body, inexistente.body)
	}
}

// Escenario: tarea creada en un flujo; la propiedad existe antes del primer
// evento.
func TestOwnershipFromStreamFirstEvent(t *testing.T) {
	h := newHarness(t, baseConfig)
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/a2a/billing",
		strings.NewReader(`{"jsonrpc":"2.0","id":"s","method":"SendStreamingMessage","params":`+newTask+`}`))
	req.Header.Set("Authorization", "Bearer "+alice)
	req.Header.Set("A2A-Version", "1.0")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	line, _ := bufio.NewReader(res.Body).ReadString('\n')
	var ev struct {
		Result struct {
			Task struct{ ID string } `json:"task"`
		} `json:"result"`
	}
	data := strings.TrimPrefix(strings.TrimSpace(line), "data: ")
	if err := json.Unmarshal([]byte(data), &ev); err != nil || ev.Result.Task.ID == "" {
		t.Fatalf("primer evento inesperado: %q", line)
	}
	// Mientras el flujo sigue abierto, el propietario ya puede consultar la tarea.
	if r := h.rpc(alice, "GetTask", `{"id":"`+ev.Result.Task.ID+`"}`); r.code() != 0 {
		t.Fatalf("la propiedad debe existir antes del primer evento: %s", r.body)
	}
	if r := h.rpc(bob, "GetTask", `{"id":"`+ev.Result.Task.ID+`"}`); r.code() != -32001 {
		t.Fatalf("otro principal no debe acceder: %s", r.body)
	}
}

// Escenario: respuesta con error del agente: no se registra propiedad.
func TestNoOwnershipOnAgentError(t *testing.T) {
	h := newHarness(t, baseConfig)
	h.agentFn = func(w http.ResponseWriter, r *http.Request, rpc rpcReq) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"r-1","error":{"code":-32603,"message":"fallo"},` +
			`"result":{"task":{"id":"t-err","contextId":"c","status":{}}}}`))
	}
	h.rpc(alice, "SendMessage", newTask)
	if _, found, _ := h.store.Get(context.Background(), "a2a:own:billing:t:t-err"); found {
		t.Fatal("una respuesta con error no debe registrar propiedad")
	}
}

// Escenarios: contexto ajeno y contexto nuevo aportado por el cliente.
func TestContextOwnership(t *testing.T) {
	h := newHarness(t, baseConfig)
	h.rpc(alice, "SendMessage", `{"message":{"messageId":"m","contextId":"ctx-alice"}}`)
	res := h.rpc(bob, "SendMessage", `{"message":{"messageId":"m","contextId":"ctx-alice"}}`)
	if res.code() != -32001 {
		t.Fatalf("contexto ajeno: %s", res.body)
	}
	if res := h.rpc(bob, "SendMessage", `{"message":{"messageId":"m","contextId":"ctx-nuevo"}}`); res.code() != 0 {
		t.Fatalf("contexto nuevo: %s", res.body)
	}
	if owner, found, _ := h.store.Get(context.Background(), "a2a:own:billing:c:ctx-nuevo"); !found || owner == "" {
		t.Fatal("el contexto nuevo debe quedar registrado")
	}
}

// Escenarios: tareas no registradas con política por defecto y permisiva.
func TestUnknownTaskPolicy(t *testing.T) {
	h := newHarness(t, baseConfig)
	if res := h.rpc(alice, "GetTask", `{"id":"desconocida"}`); res.code() != -32001 || h.agentHits() != 0 {
		t.Fatalf("política por defecto: %s", res.body)
	}
	h2 := newHarness(t, strings.Replace(baseConfig, `"capabilities": {"streaming": true, "push_notifications": true}}`,
		`"capabilities": {"streaming": true, "push_notifications": true}, "ownership": {"unknown_task": "passthrough"}}`, 1))
	if res := h2.rpc(alice, "GetTask", `{"id":"desconocida"}`); res.code() != 0 || h2.agentHits() != 1 {
		t.Fatalf("política permisiva: %s", res.body)
	}
}

// Escenarios del listado de tareas.
func TestListTasks(t *testing.T) {
	h := newHarness(t, baseConfig)
	h.rpc(alice, "SendMessage", `{"message":{"messageId":"m","contextId":"ctx-a"}}`)
	if res := h.rpc(alice, "ListTasks", `{"contextId":"ctx-a"}`); res.code() != 0 {
		t.Errorf("contexto propio: %s", res.body)
	}
	if res := h.rpc(alice, "ListTasks", `{}`); res.code() != -32602 {
		t.Errorf("sin contexto: %s", res.body)
	}
	if res := h.rpc(bob, "ListTasks", `{"contextId":"ctx-a"}`); res.code() != -32001 {
		t.Errorf("contexto ajeno: %s", res.body)
	}
	hd := newHarness(t, withConfig(`"ownership": {"list_tasks": "deny"}`))
	if res := hd.rpc(alice, "ListTasks", `{"contextId":"x"}`); res.code() != -32004 {
		t.Errorf("list_tasks=deny: %s", res.body)
	}
}

// Escenarios: propiedad por sujeto y por tenant.
func TestOwnerScope(t *testing.T) {
	tenantCfg := strings.Replace(baseConfig, `"default_agent"`,
		`"identity": {"headers": {"issuer": "X-A2A-Iss", "subject": "X-A2A-Sub", "scopes": "X-A2A-Scope", "tenant": `+
			`"X-A2A-Tenant"}, "owner_scope": ["tenant"]}, "default_agent"`, 1)
	h := newHarness(t, tenantCfg)
	taskID := taskOf(t, h.rpc(alice, "SendMessage", newTask))
	if res := h.rpc(bob, "GetTask", `{"id":"`+taskID+`"}`); res.code() != 0 {
		t.Fatalf("mismo tenant debe compartir tareas: %s", res.body)
	}
	other := "sub=dave,iss=https://idp.example.com,scope=a2a.read,tenant=globex"
	if res := h.rpc(other, "GetTask", `{"id":"`+taskID+`"}`); res.code() != -32001 {
		t.Fatalf("otro tenant no debe acceder: %s", res.body)
	}
}

// Escenario: almacén caído con la política por defecto, y permisiva.
func TestStoreFailure(t *testing.T) {
	h := newHarness(t, baseConfig)
	h.g.store = failingStore{}
	if res := h.rpc(alice, "GetTask", `{"id":"t"}`); res.code() != -32603 || h.agentHits() != 0 {
		t.Fatalf("almacén caído: %s", res.body)
	}
	h2 := newHarness(t, withConfig(`"ownership": {"on_store_error": "passthrough"}`))
	h2.g.store = failingStore{}
	if res := h2.rpc(alice, "GetTask", `{"id":"t"}`); res.code() != 0 {
		t.Fatalf("on_store_error=passthrough: %s", res.body)
	}
}

// Escenarios de la guardia de webhooks.
func TestPushGuard(t *testing.T) {
	h := newHarness(t, baseConfig)
	taskID := taskOf(t, h.rpc(alice, "SendMessage", newTask))
	hits := h.agentHits()
	denied := []string{
		"https://10.0.0.1/x", "http://hooks.example.com/x", "https://evil.example.org/x", "https://169.254.169.254/",
	}
	for _, u := range denied {
		res := h.rpc(alice, "CreateTaskPushNotificationConfig", `{"taskId":"`+taskID+`","url":"`+u+`"}`)
		if res.code() != -32602 {
			t.Errorf("%s: %s", u, res.body)
		}
	}
	res := h.rpc(alice, "SendMessage", `{"message":{"messageId":"m"},"configuration":{"taskPushNotificationConfig":{"url`+
		`":"https://127.0.0.1/"}}}`)
	if res.code() != -32602 {
		t.Errorf("webhook en SendMessage: %s", res.body)
	}
	if h.agentHits() != hits {
		t.Error("los webhooks no permitidos no deben llegar al agente")
	}
	allowed := `{"taskId":"` + taskID + `","url":"https://hooks.example.com/a2a"}`
	if res := h.rpc(alice, "CreateTaskPushNotificationConfig", allowed); res.code() != 0 {
		t.Errorf("destino permitido: %s", res.body)
	}
}

// Escenario: card extendida solo con identidad, y reescrita.
func TestExtendedCard(t *testing.T) {
	open := withConfig(`"identity": {"required": false}, "ownership": {"enabled": false}, "authorization": {}`)
	open = strings.Replace(open, `"authorization": {"operation_scopes": {"CancelTask": ["a2a.write"]}},`, "", 1)
	h := newHarness(t, open)
	h.withAuth = false
	res := h.rpc("", "GetExtendedAgentCard", `{}`)
	if res.status != 401 || h.agentHits() != 0 {
		t.Fatalf("sin identidad: %d %s", res.status, res.body)
	}
	h.withAuth = true
	res = h.rpc(alice, "GetExtendedAgentCard", `{}`)
	if res.code() != 0 || strings.Contains(res.body, "billing.internal") ||
		!strings.Contains(res.body, "https://gw.example.com/a2a/billing") {
		t.Fatalf("card extendida: %s", res.body)
	}
}

// Fallo cerrado: el endpoint interno no tiene el modificador de autorización.
func TestAuthzNotConfigured(t *testing.T) {
	h := newHarness(t, baseConfig)
	h.withAuthz = false
	res := h.rpc(alice, "SendMessage", newTask)
	if res.code() != -32603 {
		t.Fatalf("sin autorización la respuesta del agente debe retenerse: %s", res.body)
	}
	if !strings.Contains(h.log.text(), "CRITICAL") {
		t.Error("debe registrarse un error crítico de configuración")
	}
}

func TestAuthorizeDirectCalls(t *testing.T) {
	h := newHarness(t, baseConfig)
	err := h.g.Authorize(context.Background(), map[string][]string{"X-A2a-Sub": {"alice"}}, "")
	if de, ok := err.(*DenyError); !ok || de.StatusCode() != 403 {
		t.Fatalf("sin entrada de correlación debe denegarse: %v", err)
	}
	// Endpoint de otro agente.
	h.pipeline = func(w http.ResponseWriter, r *http.Request) bool {
		if err := h.g.Authorize(r.Context(), r.Header, "modern"); err != nil {
			w.WriteHeader(err.(*DenyError).StatusCode())
			return false
		}
		return true
	}
	if res := h.rpc(alice, "SendMessage", newTask); res.code() != -32603 {
		t.Fatalf("agente cruzado: %s", res.body)
	}
	if !strings.Contains(h.log.text(), "recibió una petición del agente billing") {
		t.Error("debe registrarse el cruce de agentes")
	}
}

// Escenario: decisión de acceso registrada sin secretos.
func TestDenyLogging(t *testing.T) {
	h := newHarness(t, baseConfig)
	taskID := taskOf(t, h.rpc(alice, "SendMessage", newTask))
	h.rpc(bob, "GetTask", `{"id":"`+taskID+`"}`)
	log := h.log.text()
	if !strings.Contains(log, "resultado=denegado:tarea de otro principal") || !strings.Contains(log, "principal=") {
		t.Fatalf("la denegación debe registrarse con el motivo y el resumen del principal:\n%s", log)
	}
	for _, s := range []string{"bob", "alice", "sub="} {
		if strings.Contains(log, s) {
			t.Errorf("el registro contiene %q", s)
		}
	}
}

func taskOf(t *testing.T, res *response) string {
	t.Helper()
	var r struct {
		Task struct{ ID string } `json:"task"`
	}
	if err := json.Unmarshal(res.rpc.Result, &r); err != nil || r.Task.ID == "" {
		t.Fatalf("no se creó la tarea: %s", res.body)
	}
	return r.Task.ID
}
