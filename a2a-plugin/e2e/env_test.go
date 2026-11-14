// Copyright 2026 Los autores del plugin A2A para KrakenD.

// Package e2e prueba el plugin A2A a través de KrakenD real, con el agente de
// referencia del SDK oficial detrás y el cliente del SDK oficial delante.
//
// Requiere el entorno de deploy/compose levantado. Las pruebas se omiten si no
// se indica la edición:
//
//	A2A_EDITION=ce go test ./...   # Community, puertos 18080..18092
//	A2A_EDITION=ee go test ./...   # Enterprise, puertos 28080..28092
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"

	"krakend-a2a.local/e2e/internal/secrets"
)

type environment struct {
	edition, gw, gw2, idp string
	admin                 map[string]string
}

var (
	envOnce sync.Once
	envVal  *environment
)

func env(t *testing.T) *environment {
	t.Helper()
	envOnce.Do(func() {
		base := map[string]int{"ce": 18000, "ee": 28000}[os.Getenv("A2A_EDITION")]
		if base == 0 {
			return
		}
		u := func(port int) string { return fmt.Sprintf("http://localhost:%d", base+port) }
		envVal = &environment{
			edition: os.Getenv("A2A_EDITION"),
			gw:      u(80), gw2: u(81), idp: u(90),
			admin: map[string]string{"billing": u(91), "research": u(92)},
		}
	})
	if envVal == nil {
		t.Skip("define A2A_EDITION=ce o A2A_EDITION=ee con el entorno de deploy/compose levantado")
	}
	return envVal
}

// ---------------------------------------------------------------------------
// Tokens del IdP simulado

var (
	tokMu  sync.Mutex
	tokens = map[string]string{}
)

func token(t *testing.T, client string) string {
	t.Helper()
	tokMu.Lock()
	defer tokMu.Unlock()
	if tok, ok := tokens[client]; ok {
		return tok
	}
	e := env(t)
	secret, err := secrets.ClientSecret(secrets.ClientsFile(), client)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, e.idp+"/token", strings.NewReader("grant_type=client_credentials"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(client, secret)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil || body.AccessToken == "" {
		t.Fatalf("no se obtuvo token para %s: %v", client, err)
	}
	tokens[client] = body.AccessToken
	return body.AccessToken
}

// ---------------------------------------------------------------------------
// Peticiones JSON-RPC directas, para comprobar códigos exactos

type rpcResponse struct {
	Status int
	Header http.Header
	Body   string
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (r *rpcResponse) code() int {
	if r.Error == nil {
		return 0
	}
	return r.Error.Code
}

func rawRPC(t *testing.T, gw, agent, tok, version, method, params string) *rpcResponse {
	t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":"e2e","method":%q,"params":%s}`, method, params)
	req, _ := http.NewRequest(http.MethodPost, gw+"/a2a/"+agent, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if version != "" {
		req.Header.Set("A2A-Version", version)
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	out := &rpcResponse{Status: res.StatusCode, Header: res.Header, Body: string(b)}
	_ = json.Unmarshal(b, out)
	return out
}

func taskIDOf(t *testing.T, r *rpcResponse) (string, string) {
	t.Helper()
	var res struct {
		Task struct {
			ID        string `json:"id"`
			ContextID string `json:"contextId"`
		} `json:"task"`
	}
	if err := json.Unmarshal(r.Result, &res); err != nil || res.Task.ID == "" {
		t.Fatalf("no se creó la tarea: %d %s", r.Status, r.Body)
	}
	return res.Task.ID, res.Task.ContextID
}

const sendParams = `{"message":{"messageId":"%s","role":"ROLE_USER","parts":[{"text":"hola"}]}}`

func newMsgParams() string {
	return fmt.Sprintf(sendParams, fmt.Sprintf("m-%d", time.Now().UnixNano()))
}

// ---------------------------------------------------------------------------
// Registro de peticiones recibidas por el agente

type received struct {
	Method  string            `json:"rpcMethod"`
	Headers map[string]string `json:"headers"`
	Authz   bool              `json:"hasAuthorizationHeader"`
}

func agentReceived(t *testing.T, agent string) []received {
	t.Helper()
	res, err := http.Get(env(t).admin[agent] + "/__received")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out []received
	_ = json.NewDecoder(res.Body).Decode(&out)
	return out
}

func resetAgent(t *testing.T, agent string) {
	t.Helper()
	res, err := http.Post(env(t).admin[agent]+"/__reset", "application/json", nil)
	if err == nil {
		res.Body.Close()
	}
}

// ---------------------------------------------------------------------------
// Cliente del SDK oficial

type bearer string

func (b bearer) Before(ctx context.Context, req *a2aclient.Request) (context.Context, any, error) {
	if req.ServiceParams == nil {
		req.ServiceParams = a2aclient.ServiceParams{}
	}
	req.ServiceParams["authorization"] = []string{"Bearer " + string(b)}
	return ctx, nil, nil
}

func (bearer) After(context.Context, *a2aclient.Response) error { return nil }

// sdkClient crea un cliente del SDK oficial a partir de la card que publica el
// gateway, usando solo las interfaces de la versión pedida.
func sdkClient(t *testing.T, gw, agent, tok, version string) *a2aclient.Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// El resolutor del SDK usa como URL de la card cualquier URL con ruta, así que
	// con varios agentes por gateway hay que darle la URL completa de la card.
	card, err := agentcard.DefaultResolver.Resolve(ctx, gw+"/a2a/"+agent+"/.well-known/agent-card.json")
	if err != nil {
		t.Fatalf("el SDK no pudo resolver la card a través del gateway: %v", err)
	}
	var ifaces []*a2a.AgentInterface
	for _, i := range card.SupportedInterfaces {
		if string(i.ProtocolVersion) == version {
			ifaces = append(ifaces, i)
		}
	}
	if len(ifaces) == 0 {
		t.Fatalf("la card no anuncia ninguna interfaz %s", version)
	}
	for _, i := range ifaces {
		u, _ := url.Parse(i.URL)
		if !strings.HasPrefix(i.URL, gw) && u.Host != "localhost:"+strings.Split(gw, ":")[2] {
			t.Fatalf("la card anuncia una URL que no es del gateway: %s", i.URL)
		}
	}
	opts := []a2aclient.FactoryOption{a2aclient.WithCallInterceptors(bearer(tok))}
	if version == "0.3" {
		opts = append(opts, a2av0.WithJSONRPCTransport(a2av0.JSONRPCTransportConfig{}))
	}
	c, err := a2aclient.NewFromEndpoints(context.Background(), ifaces, opts...)
	if err != nil {
		t.Fatalf("no se pudo crear el cliente %s: %v", version, err)
	}
	t.Cleanup(func() { _ = c.Destroy() })
	return c
}

func userMessage(meta map[string]any) *a2a.Message {
	m := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hola desde el SDK"))
	m.Metadata = meta
	return m
}
