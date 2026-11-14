// Copyright 2026 Los autores del plugin A2A para KrakenD.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"krakend-a2a.local/core/config"
	"krakend-a2a.local/core/ownership"
	"krakend-a2a.local/core/pushguard"
)

// harness simula el pipeline de KrakenD alrededor del gateway: enrutador,
// auth/validator con propagate_claims, el modificador a2a-authz y un agente.
type harness struct {
	t     *testing.T
	g     *Gateway
	srv   *httptest.Server
	log   *memLog
	store ownership.Store

	withAuth  bool // el endpoint interno valida token
	withAuthz bool // el endpoint interno tiene el modificador
	pipeline  func(w http.ResponseWriter, r *http.Request) bool

	mu       sync.Mutex
	hits     int
	lastReq  *http.Request
	lastBody []byte
	nextTask int
	agentFn  func(w http.ResponseWriter, r *http.Request, rpc rpcReq)
}

type rpcReq struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params struct {
		ID      string `json:"id"`
		TaskID  string `json:"taskId"`
		Message struct {
			TaskID    string `json:"taskId"`
			ContextID string `json:"contextId"`
		} `json:"message"`
	} `json:"params"`
}

// withConfig añade campos de configuración al bloque base.
func withConfig(fields string) string {
	return strings.Replace(baseConfig, `"default_agent"`, fields+`, "default_agent"`, 1)
}

const baseConfig = `{
  "public_url": "https://gw.example.com",
  "default_agent": "billing",
  "push_notifications": {"allowed_hosts": ["hooks.example.com"]},
  "authorization": {"operation_scopes": {"CancelTask": ["a2a.write"]}},
  "challenge": {"realm": "agents", "resource_metadata": true,
    "authorization_servers": ["https://idp.example.com"], "scopes_supported": ["a2a.read", "a2a.write"]},
  "agents": [
    {"id": "billing", "versions": ["1.0", "0.3"],
     "card": {"source": "static", "inline": {"name": "Billing", "description": "x", "version": "1",
       "supportedInterfaces": [{"url": "http://billing.internal:8080/", "protocolBinding": "JSONRPC", ` +
	`"protocolVersion": "1.0"},
                               {"url": "http://billing.internal:8080/", "protocolBinding": "JSONRPC", ` +
	`"protocolVersion": "0.3"}],
       "capabilities": {"streaming": true, "pushNotifications": true, "extendedAgentCard": true}}},
     "capabilities": {"streaming": true, "push_notifications": true}},
    {"id": "modern", "card": {"source": "static", "inline": {"name": "M", "supportedInterfaces": [{"url": ` +
	`"http://m.internal/", "protocolBinding": "JSONRPC", "protocolVersion": "1.0"}], "capabilities": {}}},
     "capabilities": {"streaming": false}, "disabled_operations": ["ListTasks"]}
  ]
}`

func newHarness(t *testing.T, cfgJSON string) *harness {
	t.Helper()
	cfg, err := config.Parse([]byte(cfgJSON))
	if err != nil {
		t.Fatalf("configuración de prueba inválida: %v", err)
	}
	h := &harness{t: t, log: &memLog{}, withAuth: true, withAuthz: true}
	h.agentFn = h.defaultAgent
	g, err := New(cfg, Options{
		Logger: h.log,
		PushGuard: pushguard.Config{Lookup: func(_ context.Context, host string) ([]netip.Addr, error) {
			if host == "hooks.example.com" {
				return []netip.Addr{netip.MustParseAddr("93.184.215.14")}, nil
			}
			return nil, fmt.Errorf("no such host")
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.g = g
	h.store = g.store
	h.srv = httptest.NewServer(g.Handler(http.HandlerFunc(h.krakend)))
	t.Cleanup(func() {
		h.srv.Close()
		if n := g.LiveEntries(); n != 0 {
			t.Errorf("quedan %d entradas de correlación vivas", n)
		}
	})
	return h
}

// krakend simula el enrutador y el pipeline de un endpoint interno.
func (h *harness) krakend(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/plain" {
		_, _ = io.WriteString(w, "plain")
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/__a2a/") {
		http.NotFound(w, r)
		return
	}
	if h.withAuth && !authenticate(w, r) {
		return
	}
	if h.pipeline != nil && !h.pipeline(w, r) {
		return
	}
	if h.withAuthz {
		parts := strings.Split(r.URL.Path, "/")
		if err := h.g.Authorize(r.Context(), r.Header, parts[2]); err != nil {
			w.WriteHeader(err.(interface{ StatusCode() int }).StatusCode())
			return
		}
	}
	body, _ := io.ReadAll(r.Body)
	var rpc rpcReq
	_ = json.Unmarshal(body, &rpc)
	h.mu.Lock()
	h.hits++
	h.lastReq = r.Clone(context.Background())
	h.lastBody = body
	fn := h.agentFn
	h.mu.Unlock()
	fn(w, r, rpc)
}

// claimHeaders imita propagate_claims de KrakenD.
var claimHeaders = map[string]string{
	"sub":    "X-A2A-Sub",
	"iss":    "X-A2A-Iss",
	"scope":  "X-A2A-Scope",
	"tenant": "X-A2A-Tenant",
}

// authenticate imita auth/validator: el token de prueba es una lista de
// claims clave=valor separadas por comas.
func authenticate(w http.ResponseWriter, r *http.Request) bool {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" || tok == "invalid" {
		w.WriteHeader(http.StatusUnauthorized) // auth/validator responde sin cuerpo
		return false
	}
	claims := map[string]string{}
	for _, kv := range strings.Split(tok, ",") {
		k, v, _ := strings.Cut(kv, "=")
		claims[k] = v
	}
	if claims["role"] == "none" {
		w.WriteHeader(http.StatusForbidden)
		return false
	}
	for claim, hdr := range claimHeaders {
		if v := claims[claim]; v != "" {
			r.Header.Set(hdr, v)
		}
	}
	return true
}

func (h *harness) defaultAgent(w http.ResponseWriter, r *http.Request, rpc rpcReq) {
	h.mu.Lock()
	h.nextTask++
	n := h.nextTask
	h.mu.Unlock()
	ctxID := rpc.Params.Message.ContextID
	if ctxID == "" {
		ctxID = fmt.Sprintf("ctx-%d", n)
	}
	taskID := rpc.Params.Message.TaskID
	if taskID == "" {
		taskID = fmt.Sprintf("task-%d", n)
	}
	w.Header().Set("Content-Type", "application/json")
	if ext := r.Header.Get("A2A-Extensions"); ext != "" {
		w.Header().Set("A2A-Extensions", ext)
	}
	switch rpc.Method {
	case "SendMessage":
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"task":{"id":%q,"contextId":%q,`+
			`"status":{"state":"TASK_STATE_COMPLETED"}}}}`, rpc.ID, taskID, ctxID)
	case "message/send":
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"kind":"task","id":%q,"contextId":%q,`+
			`"status":{"state":"completed"}}}`, rpc.ID, taskID, ctxID)
	case "GetExtendedAgentCard":
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"name":"Billing (extendida)",`+
			`"supportedInterfaces":[{"url":"http://billing.internal:8080/","protocolBinding":"JSONRPC",`+
			`"protocolVersion":"1.0"}],"documentationUrl":"http://billing.internal:8080/private-docs",`+
			`"capabilities":{}}}`, rpc.ID)
	case "SendStreamingMessage", "SubscribeToTask":
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f := w.(http.Flusher)
		events := []string{
			fmt.Sprintf(`{"task":{"id":%q,"contextId":%q,"status":{"state":"TASK_STATE_SUBMITTED"}}}`, taskID, ctxID),
			fmt.Sprintf(`{"statusUpdate":{"taskId":%q,"contextId":%q,"status":{"state":"TASK_STATE_COMPLETED`+
				`"}}}`, taskID, ctxID),
		}
		for i, ev := range events {
			if i > 0 {
				select {
				case <-time.After(300 * time.Millisecond):
				case <-r.Context().Done():
					h.mu.Lock()
					h.hits += 1000 // marca: el cierre del cliente llegó al agente
					h.mu.Unlock()
					return
				}
			}
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":%s}\n\n", rpc.ID, ev)
			f.Flush()
		}
	default:
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"id":%q,"contextId":"ctx","status":{"state":"TASK_STATE_WORKING`+
			`"}}}`, rpc.ID, rpc.Params.ID)
	}
}

type response struct {
	status int
	header http.Header
	body   string
	rpc    struct {
		ID     json.RawMessage `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
}

func (r *response) code() int {
	if r.rpc.Error == nil {
		return 0
	}
	return r.rpc.Error.Code
}

func (h *harness) do(method, path, token, body string, hdr map[string]string) *response {
	h.t.Helper()
	req, _ := http.NewRequest(method, h.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	out := &response{status: res.StatusCode, header: res.Header, body: string(b)}
	_ = json.Unmarshal(b, &out.rpc)
	return out
}

// rpc envía una petición JSON-RPC 1.0 al agente billing.
func (h *harness) rpc(token, method, params string) *response {
	h.t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":"r-1","method":%q,"params":%s}`, method, params)
	return h.do(http.MethodPost, "/a2a/billing", token, body, map[string]string{"A2A-Version": "1.0"})
}

func (h *harness) agentHits() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.hits
}

const (
	alice   = "sub=alice,iss=https://idp.example.com,scope=a2a.read a2a.write,tenant=acme"
	bob     = "sub=bob,iss=https://idp.example.com,scope=a2a.read a2a.write,tenant=acme"
	reader  = "sub=carol,iss=https://idp.example.com,scope=a2a.read,tenant=acme"
	newTask = `{"message":{"messageId":"m","role":"ROLE_USER","parts":[{"text":"hola"}]}}`
)

// memLog guarda los registros para comprobar qué se escribe.
type memLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *memLog) add(level string, v ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, level+" "+fmt.Sprint(v...))
}
func (l *memLog) Debug(v ...interface{})    { l.add("DEBUG", v...) }
func (l *memLog) Info(v ...interface{})     { l.add("INFO", v...) }
func (l *memLog) Warning(v ...interface{})  { l.add("WARNING", v...) }
func (l *memLog) Error(v ...interface{})    { l.add("ERROR", v...) }
func (l *memLog) Critical(v ...interface{}) { l.add("CRITICAL", v...) }
func (l *memLog) text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

// failingStore simula un almacén caído.
type failingStore struct{}

func (failingStore) Get(context.Context, string) (string, bool, error) {
	return "", false, fmt.Errorf("conexión rechazada")
}
func (failingStore) Claim(context.Context, string, string, time.Duration) (string, error) {
	return "", fmt.Errorf("conexión rechazada")
}
func (failingStore) Touch(context.Context, string, time.Duration) error { return nil }
func (failingStore) Close() error                                       { return nil }

var _ = atomic.Int64{}
