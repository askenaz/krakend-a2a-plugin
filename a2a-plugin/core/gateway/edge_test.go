// Copyright 2026 Los autores del plugin A2A para KrakenD.

package gateway

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Escenario: agente configurado. La petición llega por la ruta interna con el
// cuerpo intacto y las cabeceras de contexto.
func TestRPCReachesInternalRoute(t *testing.T) {
	h := newHarness(t, baseConfig)
	body := `{"jsonrpc":"2.0","id":"x-1","method":"SendMessage","params":` + newTask + `}`
	res := h.do(http.MethodPost, "/a2a/billing", alice, body,
		map[string]string{"A2A-Version": "1.0", "A2A-Extensions": "https://ext.example.com/a"})
	if res.status != 200 || res.code() != 0 || string(res.rpc.ID) != `"x-1"` {
		t.Fatalf("respuesta inesperada: %d %s", res.status, res.body)
	}
	r := h.lastReq
	if r.URL.Path != "/__a2a/billing/unary" || string(h.lastBody) != body {
		t.Fatalf("ruta %q o cuerpo alterado", r.URL.Path)
	}
	wantHeaders := map[string]string{
		HeaderOperation: "SendMessage", HeaderAgent: "billing", HeaderVersion: "1.0",
		"A2A-Version": "1.0", "A2A-Extensions": "https://ext.example.com/a",
	}
	for hdr, want := range wantHeaders {
		if got := r.Header.Get(hdr); got != want {
			t.Errorf("%s = %q, se esperaba %q", hdr, got, want)
		}
	}
	if r.Header.Get(HeaderRequestID) == "" {
		t.Error("falta X-A2A-Request-Id")
	}
	// Escenario: extensión devuelta por el agente.
	if res.header.Get("A2A-Extensions") != "https://ext.example.com/a" {
		t.Error("la cabecera A2A-Extensions del agente debe llegar al cliente")
	}
}

// Escenarios: agente no configurado y método HTTP no admitido.
func TestUnknownAgentAndMethod(t *testing.T) {
	h := newHarness(t, baseConfig)
	if res := h.do(http.MethodPost, "/a2a/desconocido", alice, `{}`, nil); res.status != 404 {
		t.Errorf("agente desconocido: %d", res.status)
	}
	res := h.do(http.MethodPut, "/a2a/billing", alice, `{}`, nil)
	if res.status != 405 || res.header.Get("Allow") != "POST" {
		t.Errorf("PUT: %d Allow=%q", res.status, res.header.Get("Allow"))
	}
	if h.agentHits() != 0 {
		t.Error("ninguna de estas peticiones debe llegar al agente")
	}
}

func TestEnvelopeAndSize(t *testing.T) {
	h := newHarness(t, withConfig(`"limits": {"max_body_bytes": 2048}`))
	cases := []struct {
		name, body string
		status     int
		code       int
		id         string
	}{
		{"JSON mal formado", `{"jsonrpc":`, 200, -32700, "null"},
		{"sobre inválido", `{"jsonrpc":"1.0","id":7,"method":"GetTask"}`, 200, -32600, "7"},
		{"lote", `[{"jsonrpc":"2.0","id":1,"method":"GetTask"}]`, 200, -32600, "null"},
		{"cuerpo demasiado grande", `{"x":"` + strings.Repeat("a", 4096) + `"}`, 413, -32600, "null"},
	}
	for _, c := range cases {
		res := h.do(http.MethodPost, "/a2a/billing", alice, c.body, map[string]string{"A2A-Version": "1.0"})
		if res.status != c.status || res.code() != c.code || string(res.rpc.ID) != c.id {
			t.Errorf("%s: estado %d código %d id %s (%s)", c.name, res.status, res.code(), res.rpc.ID, res.body)
		}
	}
	if h.agentHits() != 0 {
		t.Error("ninguna petición inválida debe llegar al agente")
	}
}

func TestVersionsAndMethods(t *testing.T) {
	h := newHarness(t, baseConfig)
	// 0.3 sin cabecera: se asume 0.3 y tasks/get se reconoce.
	res := h.do(http.MethodPost, "/a2a/billing", alice,
		`{"jsonrpc":"2.0","id":1,"method":"message/send","params":{"message":{"kind":"message",`+
			`"messageId":"m","role":"user","parts":[]}}}`, nil)
	if res.status != 200 || res.code() != 0 ||
		h.lastReq.Header.Get(HeaderOperation) != "SendMessage" || h.lastReq.Header.Get(HeaderVersion) != "0.3" {
		t.Fatalf("0.3 sin cabecera: %d %s", res.status, res.body)
	}
	// Nombre de otra versión.
	res = h.do(http.MethodPost, "/a2a/billing", alice, `{"jsonrpc":"2.0","id":1,"method":"message/send",`+
		`"params":{}}`, map[string]string{"A2A-Version": "1.0"})
	if res.code() != -32601 {
		t.Errorf("message/send con 1.0: %s", res.body)
	}
	// Método desconocido.
	if res = h.rpc(alice, "DoSomething", `{}`); res.code() != -32601 || string(res.rpc.ID) != `"r-1"` {
		t.Errorf("método desconocido: %s", res.body)
	}
	// Versión no habilitada: modern solo admite 1.0 y sin cabecera se asume 0.3.
	res = h.do(http.MethodPost, "/a2a/modern", alice, `{"jsonrpc":"2.0","id":1,"method":"GetTask",`+
		`"params":{"id":"t"}}`, nil)
	if res.code() != -32009 {
		t.Errorf("versión no habilitada: %s", res.body)
	}
	// La versión también puede venir en la consulta.
	res = h.do(http.MethodPost, "/a2a/modern?A2A-Version=1.0", alice,
		`{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":`+newTask+`}`, nil)
	if res.code() != 0 {
		t.Errorf("versión en la consulta: %s", res.body)
	}
}

func TestOperationsEnabled(t *testing.T) {
	h := newHarness(t, baseConfig)
	modern := func(method, params string) *response {
		return h.do(http.MethodPost, "/a2a/modern", alice, `{"jsonrpc":"2.0","id":1,`+
			`"method":"`+method+`","params":`+params+`}`, map[string]string{"A2A-Version": "1.0"})
	}
	if res := modern("SendStreamingMessage", newTask); res.code() != -32004 {
		t.Errorf("streaming deshabilitado: %s", res.body)
	}
	pushParams := `{"taskId":"t","url":"https://hooks.example.com/x"}`
	if res := modern("CreateTaskPushNotificationConfig", pushParams); res.code() != -32003 {
		t.Errorf("push deshabilitado: %s", res.body)
	}
	if res := modern("ListTasks", `{"contextId":"c"}`); res.code() != -32004 {
		t.Errorf("operación deshabilitada: %s", res.body)
	}
	// Un SendMessage con webhook a un agente sin push también se rechaza.
	if res := modern("SendMessage", `{"message":{},`+
		`"configuration":{"taskPushNotificationConfig":{"url":"https://hooks.example.com/x"}}}`); res.code() != -32003 {
		t.Errorf("webhook con push deshabilitado: %s", res.body)
	}
	if h.agentHits() != 0 {
		t.Error("ninguna operación deshabilitada debe llegar al agente")
	}
}

// Escenario: granularidad por operación.
func TestOperationGranularity(t *testing.T) {
	h := newHarness(t, withConfig(`"route_granularity": "operation"`))
	h.rpc(alice, "SendMessage", newTask)
	if h.lastReq.URL.Path != "/__a2a/billing/SendMessage" {
		t.Errorf("ruta %q", h.lastReq.URL.Path)
	}
}

// Escenario: acceso directo a una ruta interna, también con variantes.
func TestInternalRoutesBlocked(t *testing.T) {
	h := newHarness(t, baseConfig)
	internal := []string{
		"/__a2a/billing/unary", "/__A2A/billing/unary", "//__a2a/billing/unary",
		"/x/../__a2a/billing/stream", "/__a2a",
	}
	for _, p := range internal {
		if res := h.do(http.MethodPost, p, alice, `{}`, nil); res.status != 404 {
			t.Errorf("%s: estado %d", p, res.status)
		}
	}
	res := h.do(http.MethodGet, "/__a2a/_stats", "", "", nil)
	if res.status != 200 || !strings.Contains(res.body, "liveEntries") {
		t.Errorf("estadísticas desde loopback: %d %s", res.status, res.body)
	}
	if h.agentHits() != 0 {
		t.Error("las rutas internas no deben alcanzarse desde fuera")
	}
	if res := h.do(http.MethodGet, "/plain", "", "", nil); res.body != "plain" {
		t.Error("las rutas no A2A deben seguir funcionando")
	}
}

// Escenarios: entrega incremental y cliente desconectado.
func TestStreaming(t *testing.T) {
	h := newHarness(t, baseConfig)
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/a2a/billing",
		strings.NewReader(`{"jsonrpc":"2.0","id":"s","method":"SendStreamingMessage","params":`+newTask+`}`))
	req.Header.Set("Authorization", "Bearer "+alice)
	req.Header.Set("A2A-Version", "1.0")
	start := time.Now()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("tipo %q", res.Header.Get("Content-Type"))
	}
	rd := bufio.NewReader(res.Body)
	var arrivals []time.Duration
	for {
		line, err := rd.ReadString('\n')
		if strings.HasPrefix(line, "data:") {
			arrivals = append(arrivals, time.Since(start))
		}
		if err != nil {
			break
		}
	}
	res.Body.Close()
	if len(arrivals) != 2 || arrivals[1]-arrivals[0] < 200*time.Millisecond {
		t.Fatalf("los eventos deben llegar según se emiten: %v", arrivals)
	}
	if h.lastReq.URL.Path != "/__a2a/billing/stream" {
		t.Errorf("ruta %q", h.lastReq.URL.Path)
	}

	// El cliente se va tras el primer evento: el agente debe enterarse.
	req, _ = http.NewRequest(http.MethodPost, h.srv.URL+"/a2a/billing",
		strings.NewReader(`{"jsonrpc":"2.0","id":"s","method":"SendStreamingMessage","params":`+newTask+`}`))
	req.Header.Set("Authorization", "Bearer "+alice)
	req.Header.Set("A2A-Version", "1.0")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = bufio.NewReader(res.Body).ReadString('\n')
	res.Body.Close()
	deadline := time.Now().Add(2 * time.Second)
	for h.agentHits() < 1000 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if h.agentHits() < 1000 {
		t.Error("el cierre del cliente debe propagarse al agente")
	}
}

// Escenario: error del agente sin alterar.
func TestAgentErrorUnchanged(t *testing.T) {
	h := newHarness(t, baseConfig)
	agentBody := `{"jsonrpc":"2.0","id":"r-1","error":{"code":-32002,"message":"Task not cancelable","data":[{"x":1}]}}`
	h.agentFn = func(w http.ResponseWriter, r *http.Request, rpc rpcReq) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(agentBody))
	}
	res := h.rpc(alice, "SendMessage", newTask)
	if res.body != agentBody {
		t.Fatalf("la respuesta del agente debe llegar sin cambios:\n%s", res.body)
	}
}

// Escenarios: tiempo de espera agotado y límite de peticiones.
func TestPipelineErrors(t *testing.T) {
	h := newHarness(t, baseConfig)
	h.pipeline = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.Contains(r.Header.Get("Authorization"), "sub=limited") {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
			return false
		}
		if strings.Contains(r.Header.Get("Authorization"), "sub=slow") {
			w.WriteHeader(http.StatusGatewayTimeout)
			return false
		}
		return true
	}
	res := h.rpc("sub=limited,iss=i", "SendMessage", newTask)
	if res.status != 429 || res.header.Get("Retry-After") != "30" || res.code() != -32603 ||
		string(res.rpc.ID) != `"r-1"` {
		t.Errorf("límite: %d %v %s", res.status, res.header, res.body)
	}
	res = h.rpc("sub=slow,iss=i", "SendMessage", newTask)
	if res.status != 504 || res.code() != -32603 || string(res.rpc.ID) != `"r-1"` {
		t.Errorf("timeout: %d %s", res.status, res.body)
	}
}

// Escenario: regla nativa sobre la operación.
func TestNativeRuleUsesOperationHeader(t *testing.T) {
	h := newHarness(t, baseConfig)
	h.pipeline = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get(HeaderOperation) == "CancelTask" {
			w.WriteHeader(http.StatusForbidden)
			return false
		}
		return true
	}
	if res := h.rpc(alice, "CancelTask", `{"id":"t"}`); res.status != 403 {
		t.Errorf("la regla nativa debe poder rechazar por operación: %d", res.status)
	}
	if h.agentHits() != 0 {
		t.Error("la operación rechazada no debe llegar al agente")
	}
}

// Escenario: registro sin contenido sensible.
func TestLogging(t *testing.T) {
	h := newHarness(t, baseConfig)
	h.rpc(alice, "SendMessage", `{"message":{"messageId":"m","role":"ROLE_USER","parts":[{"text":"secreto-del-cliente"`+
		`}]}}`)
	h.do(http.MethodPost, "/a2a/modern", alice, `{"jsonrpc":"2.0","id":1,"method":"GetTask","params":{"id":"t"}}`, nil)
	log := h.log.text()
	for _, want := range []string{"agente=billing op=SendMessage version=1.0 " +
		"resultado=ok", "agente=modern op=GetTask version= resultado=rechazado codigo=-32009"} {
		if !strings.Contains(log, want) {
			t.Errorf("falta en el registro %q:\n%s", want, log)
		}
	}
	for _, forbidden := range []string{"secreto-del-cliente", "sub=alice", "alice", "Bearer"} {
		if strings.Contains(log, forbidden) {
			t.Errorf("el registro contiene %q", forbidden)
		}
	}
	sum := sha256.Sum256([]byte("alice"))
	_ = hex.EncodeToString(sum[:])
}

// Grupo 4.4: rutas de descubrimiento por agente y del agente por defecto.
func TestDiscoveryRoutes(t *testing.T) {
	h := newHarness(t, baseConfig)
	res := h.do(http.MethodGet, "/a2a/billing/.well-known/agent-card.json", "", "", nil)
	if res.status != 200 || !strings.Contains(res.body, `"url":"https://gw.example.com/a2a/billing"`) ||
		strings.Contains(res.body, "billing.internal") {
		t.Fatalf("card del agente: %d %s", res.status, res.body)
	}
	etag := res.header.Get("ETag")
	if etag == "" || !strings.HasPrefix(res.header.Get("Cache-Control"), "public, max-age=") {
		t.Fatalf("faltan ETag o Cache-Control: %v", res.header)
	}
	conditional := map[string]string{"If-None-Match": etag}
	if r := h.do(http.MethodGet, "/a2a/billing/.well-known/agent-card.json", "", "", conditional); r.status != 304 {
		t.Errorf("petición condicional: %d", r.status)
	}
	root := h.do(http.MethodGet, "/.well-known/agent-card.json", "", "", nil)
	if root.status != 200 || root.body != res.body {
		t.Errorf("la raíz debe servir la card del agente por defecto: %d", root.status)
	}
	if r := h.do(http.MethodGet, "/a2a/desconocido/.well-known/agent-card.json", "", "", nil); r.status != 404 {
		t.Errorf("card de agente desconocido: %d", r.status)
	}

	// Sin agente por defecto, la raíz la resuelve el enrutador.
	h2 := newHarness(t, strings.Replace(baseConfig, `"default_agent": "billing",`, "", 1))
	r := h2.do(http.MethodGet, "/.well-known/agent-card.json", "", "", nil)
	if r.status != 404 || strings.Contains(r.body, "Billing") {
		t.Errorf("sin agente por defecto la raíz no es del plugin: %d %s", r.status, r.body)
	}
}

// Metadatos de recurso protegido (RFC 9728).
func TestProtectedResourceMetadata(t *testing.T) {
	h := newHarness(t, baseConfig)
	res := h.do(http.MethodGet, "/.well-known/oauth-protected-resource/a2a/billing", "", "", nil)
	if res.status != 200 || !strings.Contains(res.body, `"resource":"https://gw.example.com/a2a/billing"`) ||
		!strings.Contains(res.body, `"authorization_servers":["https://idp.example.com"]`) {
		t.Fatalf("metadatos: %d %s", res.status, res.body)
	}
}
