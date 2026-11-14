// Copyright 2026 Los autores del plugin A2A para KrakenD.

package e2e

import (
	"bufio"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

const billingCardPath = "/a2a/billing/.well-known/agent-card.json"

// Descubrimiento a través de KrakenD (spec a2a-agent-discovery).
func TestDiscovery(t *testing.T) {
	e := env(t)
	etag := checkBillingCard(t, e.gw)

	req, _ := http.NewRequest(http.MethodGet, e.gw+billingCardPath, nil)
	req.Header.Set("If-None-Match", etag)
	if r, _ := http.DefaultClient.Do(req); r == nil || r.StatusCode != 304 {
		t.Errorf("petición condicional sin 304")
	}

	root, _ := http.Get(e.gw + "/.well-known/agent-card.json")
	if root == nil || root.StatusCode != 200 || !strings.Contains(readAll(root), `"name":"Billing"`) {
		t.Errorf("la raíz debe servir la card del agente por defecto")
	}

	checkSignedCard(t, e.gw)

	if r, _ := http.Get(e.gw + "/.well-known/oauth-protected-resource/a2a/billing"); r == nil || r.StatusCode != 200 {
		t.Errorf("faltan los metadatos de recurso protegido")
	}
}

// checkBillingCard comprueba la card publicada de billing y devuelve su ETag.
func checkBillingCard(t *testing.T, gw string) string {
	t.Helper()
	res, err := http.Get(gw + billingCardPath)
	if err != nil {
		t.Fatal(err)
	}
	b := readAll(res)
	if res.StatusCode != 200 || res.Header.Get("ETag") == "" ||
		!strings.HasPrefix(res.Header.Get("Cache-Control"), "public, max-age=") {
		t.Fatalf("card de billing: %d %v", res.StatusCode, res.Header)
	}
	for _, internal := range []string{"billing:9001", "X-Internal-Key", "signatures"} {
		if strings.Contains(b, internal) {
			t.Errorf("la card publicada contiene %q", internal)
		}
	}
	if !strings.Contains(b, gw+"/a2a/billing") || !strings.Contains(b, `"httpAuthSecurityScheme"`) {
		t.Errorf("la card debe anunciar el gateway y sus esquemas de seguridad: %s", b)
	}
	return res.Header.Get("ETag")
}

// checkSignedCard: research publica una card firmada por el SDK y el gateway
// verifica la firma antes de publicarla.
func checkSignedCard(t *testing.T, gw string) {
	t.Helper()
	r, _ := http.Get(gw + "/a2a/research/.well-known/agent-card.json")
	if r == nil || r.StatusCode != 200 {
		t.Fatalf("la card firmada de research debe verificarse y publicarse")
	}
	if body := readAll(r); strings.Contains(body, "signatures") || strings.Contains(body, "research:9002") {
		t.Errorf("la card de research no debe llevar firma original ni URLs internas: %s", body)
	}
}

// Pasarela de protocolo (spec a2a-protocol-gateway).
func TestProtocolThroughKrakenD(t *testing.T) {
	e := env(t)
	alice := token(t, "alice-app")
	resetAgent(t, "billing")

	r := rawRPC(t, e.gw, "billing", alice, "1.0", "SendMessage", newMsgParams())
	if r.Status != 200 || r.code() != 0 {
		t.Fatalf("SendMessage: %d %s", r.Status, r.Body)
	}
	got := agentReceived(t, "billing")
	if len(got) != 1 || got[0].Method != "SendMessage" {
		t.Fatalf("el agente debe recibir exactamente la petición: %+v", got)
	}
	if got[0].Headers["X-A2a-Sub"] != "alice" || got[0].Headers["X-A2a-Operation"] != "SendMessage" || got[0].Authz {
		t.Errorf("cabeceras recibidas por el agente: %+v", got[0])
	}

	cases := []struct {
		name, agent, version, method, params string
		code                                 int
	}{
		{"método desconocido", "billing", "1.0", "DoSomething", `{}`, -32601},
		{"nombre de otra versión", "billing", "1.0", "message/send", `{}`, -32601},
		{"versión no habilitada", "research", "", "GetTask", `{"id":"x"}`, -32009},
		{"operación deshabilitada", "research", "1.0", "ListTasks", `{"contextId":"x"}`, -32004},
		{"push deshabilitado", "research", "1.0", "CreateTaskPushNotificationConfig",
			`{"taskId":"x","url":"https://hooks.example.com/a"}`, -32003},
	}
	resetAgent(t, "billing")
	resetAgent(t, "research")
	for _, c := range cases {
		if r := rawRPC(t, e.gw, c.agent, alice, c.version, c.method, c.params); r.code() != c.code {
			t.Errorf("%s: %d %s", c.name, r.code(), r.Body)
		}
	}
	if n := len(agentReceived(t, "billing")) + len(agentReceived(t, "research")); n != 0 {
		t.Errorf("%d peticiones rechazadas llegaron a un agente", n)
	}

	internalRes, _ := http.Post(e.gw+"/__a2a/billing/unary", "application/json", strings.NewReader(`{}`))
	if internalRes == nil || internalRes.StatusCode != 404 {
		t.Errorf("las rutas internas deben dar 404 desde fuera")
	}
	if r, _ := http.Get(e.gw + "/__health"); r == nil || r.StatusCode != 200 {
		t.Errorf("las rutas propias de KrakenD deben seguir funcionando")
	}
}

// Streaming incremental a través de KrakenD. En Community depende del flush del
// plugin (supuesto S2).
func TestStreamingThroughKrakenD(t *testing.T) {
	e := env(t)
	body := `{"jsonrpc":"2.0","id":"s","method":"SendStreamingMessage","params":{"message":{"messageId":"m-s",` +
		`"role":"ROLE_USER","parts":[{"text":"x"}],"metadata":{"delayMs":500}}}}`
	req, _ := http.NewRequest(http.MethodPost, e.gw+"/a2a/billing", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token(t, "alice-app"))
	req.Header.Set("A2A-Version", "1.0")
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("tipo %q", res.Header.Get("Content-Type"))
	}
	var arrivals []time.Duration
	rd := bufio.NewReader(res.Body)
	for {
		line, err := rd.ReadString('\n')
		if strings.HasPrefix(line, "data:") {
			arrivals = append(arrivals, time.Since(start))
		}
		if err != nil {
			break
		}
	}
	if len(arrivals) < 4 {
		t.Fatalf("eventos recibidos: %v", arrivals)
	}
	if arrivals[1]-arrivals[0] < 300*time.Millisecond || arrivals[len(arrivals)-1]-arrivals[0] < 1200*time.Millisecond {
		t.Errorf("los eventos llegaron juntos, no según se emitían: %v", arrivals)
	}
}

// Control de acceso (spec a2a-access-control).
func TestAccessControlThroughKrakenD(t *testing.T) {
	e := env(t)
	alice, bob, reader := token(t, "alice-app"), token(t, "bob-app"), token(t, "reader-app")
	created := rawRPC(t, e.gw, "billing", alice, "1.0", "SendMessage", newMsgParams())
	taskID, ctxID := taskIDOf(t, created)

	// Sin credenciales: 401 con desafío.
	r := rawRPC(t, e.gw, "billing", "", "1.0", "GetTask", `{"id":"`+taskID+`"}`)
	if r.Status != 401 || !strings.Contains(r.Header.Get("WWW-Authenticate"), "resource_metadata=") {
		t.Errorf("sin token: %d %q", r.Status, r.Header.Get("WWW-Authenticate"))
	}
	// Token manipulado: lo rechaza la validación nativa.
	if r := rawRPC(t, e.gw, "billing", alice+"x", "1.0", "GetTask", `{"id":"`+taskID+`"}`); r.Status != 401 {
		t.Errorf("token con firma inválida: %d", r.Status)
	}

	resetAgent(t, "billing")
	// Otro principal: -32001, sin llegar al agente.
	for _, c := range []struct{ method, params string }{
		{"GetTask", `{"id":"` + taskID + `"}`},
		{"CancelTask", `{"id":"` + taskID + `"}`},
		{"SendMessage", `{"message":{"messageId":"m","role":"ROLE_USER","parts":[],"taskId":"` + taskID + `"}}`},
		{"SendMessage", `{"message":{"messageId":"m","role":"ROLE_USER","parts":[],"contextId":"` + ctxID + `"}}`},
		{"ListTasks", `{"contextId":"` + ctxID + `"}`},
	} {
		if r := rawRPC(t, e.gw, "billing", bob, "1.0", c.method, c.params); r.code() != -32001 {
			t.Errorf("%s de otro principal: %d %s", c.method, r.code(), r.Body)
		}
	}
	// Scope insuficiente: 403.
	if r := rawRPC(t, e.gw, "billing", reader, "1.0", "CancelTask", `{"id":"x"}`); r.Status != 403 {
		t.Errorf("CancelTask sin a2a.write: %d", r.Status)
	}
	// Webhook hacia una red privada.
	privateHook := `{"taskId":"` + taskID + `","url":"https://169.254.169.254/latest"}`
	if r := rawRPC(t, e.gw, "billing", alice, "1.0", "CreateTaskPushNotificationConfig", privateHook); r.code() != -32602 {
		t.Errorf("webhook privado: %s", r.Body)
	}
	// Cabecera de identidad falsificada por el cliente.
	getTask := `{"jsonrpc":"2.0","id":1,"method":"GetTask","params":{"id":"` + taskID + `"}}`
	req, _ := http.NewRequest(http.MethodPost, e.gw+"/a2a/billing", strings.NewReader(getTask))
	req.Header.Set("Authorization", "Bearer "+bob)
	req.Header.Set("A2A-Version", "1.0")
	req.Header.Set("X-A2A-Sub", "alice")
	res, _ := http.DefaultClient.Do(req)
	if b := readAll(res); !strings.Contains(b, "-32001") {
		t.Errorf("una cabecera X-A2A-Sub del cliente no debe suplantar la identidad: %s", b)
	}
	if n := len(agentReceived(t, "billing")); n != 0 {
		t.Errorf("%d peticiones denegadas llegaron al agente", n)
	}

	// El propietario sí accede.
	if r := rawRPC(t, e.gw, "billing", alice, "1.0", "GetTask", `{"id":"`+taskID+`"}`); r.code() != 0 {
		t.Errorf("el propietario debe acceder: %s", r.Body)
	}
}

// Tareas 8.8 y 8.9: la propiedad registrada en un nodo se respeta en el otro,
// a través de Redis.
func TestOwnershipAcrossNodes(t *testing.T) {
	e := env(t)
	alice, bob := token(t, "alice-app"), token(t, "bob-app")
	taskID, _ := taskIDOf(t, rawRPC(t, e.gw, "billing", alice, "1.0", "SendMessage", newMsgParams()))
	if r := rawRPC(t, e.gw2, "billing", alice, "1.0", "GetTask", `{"id":"`+taskID+`"}`); r.code() != 0 {
		t.Errorf("el propietario debe acceder desde el otro nodo: %s", r.Body)
	}
	if r := rawRPC(t, e.gw2, "billing", bob, "1.0", "GetTask", `{"id":"`+taskID+`"}`); r.code() != -32001 {
		t.Errorf("otro principal tampoco accede desde el otro nodo: %s", r.Body)
	}
}

// Los componentes nativos del endpoint interno se aplican: research tiene un
// límite de peticiones en su configuración.
func TestNativeRateLimit(t *testing.T) {
	e := env(t)
	alice := token(t, "alice-app")
	time.Sleep(1100 * time.Millisecond) // vacía el cubo de peticiones
	resetAgent(t, "research")
	var mu sync.Mutex
	statuses := map[int]int{}
	var limited *rpcResponse
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := rawRPC(t, e.gw, "research", alice, "1.0", "SendMessage", newMsgParams())
			mu.Lock()
			statuses[r.Status]++
			if r.Status == 429 || r.Status == 503 {
				limited = r
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	rejected := statuses[429] + statuses[503]
	if rejected == 0 {
		t.Fatalf("el límite nativo debe rechazar parte de la ráfaga: %v", statuses)
	}
	if limited.code() != -32603 || string(limited.ID) != `"e2e"` {
		t.Errorf("el rechazo por límite (%d) debe llevar un error JSON-RPC con el id de la "+
			"petición: %s", limited.Status, limited.Body)
	}
	if n := len(agentReceived(t, "research")); n != statuses[200] {
		t.Errorf("el agente recibió %d peticiones y se aceptaron %d", n, statuses[200])
	}
}

func readAll(r *http.Response) string {
	if r == nil {
		return ""
	}
	defer r.Body.Close()
	var b strings.Builder
	_, _ = bufio.NewReader(r.Body).WriteTo(&b)
	return b.String()
}
