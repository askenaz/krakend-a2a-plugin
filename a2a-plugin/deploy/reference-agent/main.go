// Copyright 2026 Los autores del plugin A2A para KrakenD.

// Agente A2A de referencia, construido con el SDK oficial de Go (a2a-go).
//
// Sirve para el entorno de demostración y las pruebas de conformidad: el
// agente que hay detrás del gateway es el del SDK, no una imitación.
//
//   - Habla A2A 1.0 y 0.3 en la misma URL. Elige el manejador por la cabecera
//     A2A-Version, como hace el gateway.
//   - Guarda tareas y configuraciones de notificaciones push en memoria.
//   - Publica una card compatible (CARD_MODE=compat) o una card 1.0 firmada
//     por el SDK (CARD_MODE=v1 con SIGNING_KEY_FILE).
//   - Registra cada petición recibida en GET /__received, para que las
//     pruebas comprueben si una petición llegó al agente.
//
// El comportamiento de cada tarea se controla con metadatos del mensaje:
// delayMs (pausa entre eventos) y holdMs (tiempo que la tarea sigue abierta).
package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"iter"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
	"github.com/a2aproject/a2a-go/v2/a2acrypto"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/a2aproject/a2a-go/v2/a2asrv/push"
)

// signingKID debe coincidir con el kid que genera deploy/demo-secrets.
const signingKID = "demo-card-signing-1"

func main() {
	port := env("PORT", "9001")
	name := env("AGENT_NAME", "Reference Agent")
	internalURL := env("INTERNAL_URL", "http://reference-agent:"+port+"/")
	mode := env("CARD_MODE", "compat")

	card := &a2a.AgentCard{
		Name:             name,
		Description:      "Agente de referencia construido con el SDK oficial de A2A",
		Version:          "1.0.0",
		DocumentationURL: strings.TrimSuffix(internalURL, "/") + "/docs",
		SupportedInterfaces: []*a2a.AgentInterface{
			{URL: internalURL, ProtocolBinding: a2a.TransportProtocolJSONRPC, ProtocolVersion: "1.0"},
		},
		Capabilities:       a2a.AgentCapabilities{Streaming: true, PushNotifications: true, ExtendedAgentCard: true},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		SecuritySchemes: a2a.NamedSecuritySchemes{
			"internal-key": a2a.APIKeySecurityScheme{Location: "header", Name: "X-Internal-Key"},
		},
		Skills: []a2a.AgentSkill{{ID: "echo", Name: "Eco", Description: "Responde con un artefacto " +
			"de eco", Tags: []string{"demo"}}},
	}
	if mode == "compat" {
		card.SupportedInterfaces = append(card.SupportedInterfaces,
			&a2a.AgentInterface{URL: internalURL, ProtocolBinding: a2a.TransportProtocolJSONRPC, ProtocolVersion: "0.3"})
	}
	extended := *card
	extended.Name = name + " (extendida)"
	extended.Skills = append(append([]a2a.AgentSkill(nil), card.Skills...),
		a2a.AgentSkill{
			ID:          "admin",
			Name:        "Administración",
			Description: "Solo para clientes autenticados",
			Tags:        []string{"privado"},
		})

	handler := a2asrv.NewHandler(executor{},
		a2asrv.WithPushNotifications(push.NewInMemoryStore(),
			push.NewHTTPPushSender(&push.HTTPSenderConfig{Timeout: 3 * time.Second})),
		a2asrv.WithExtendedAgentCard(&extended),
		a2asrv.WithCallInterceptors(gatewayIdentity{}),
	)

	producer := cardProducer(mode, card)

	rec := &recorder{}
	mux := http.NewServeMux()
	mux.Handle("GET "+a2asrv.WellKnownAgentCardPath, a2asrv.NewAgentCardHandler(producer))
	mux.HandleFunc("GET /__received", rec.list)
	mux.HandleFunc("POST /__reset", rec.reset)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.Handle("POST /", rec.wrap(byVersion(a2asrv.NewJSONRPCHandler(handler), a2av0.NewJSONRPCHandler(handler))))

	log.Printf("%s escuchando en :%s (card %s)", name, port, mode)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// cardProducer elige cómo se publica la card: en forma 0.3, firmada si hay
// clave de firma, o tal cual.
func cardProducer(mode string, card *a2a.AgentCard) a2asrv.AgentCardProducer {
	keyFile := os.Getenv("SIGNING_KEY_FILE")
	switch {
	case mode == "compat":
		return a2av0.NewStaticAgentCardProducer(card)
	case keyFile != "":
		return signedProducer(staticProducer{card}, keyFile)
	default:
		return staticProducer{card}
	}
}

func signedProducer(base a2asrv.AgentCardProducer, keyFile string) a2asrv.AgentCardProducer {
	signer, err := loadSigner(keyFile)
	if err != nil {
		log.Fatal(err)
	}
	return a2asrv.NewSignedCardProducer(base, func(context.Context) ([]*a2acrypto.Signer, error) {
		return []*a2acrypto.Signer{signer}, nil
	})
}

// byVersion elige el manejador por A2A-Version; sin valor se asume 0.3.
func byVersion(v1, v03 http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := r.Header.Get("A2A-Version")
		if v == "" {
			v = r.URL.Query().Get("A2A-Version")
		}
		if strings.HasPrefix(strings.TrimSpace(v), "1.") {
			v1.ServeHTTP(w, r)
			return
		}
		v03.ServeHTTP(w, r)
	})
}

// gatewayIdentity toma como usuario de la llamada la identidad que el gateway
// ya verificó y propaga en X-A2A-Sub. El agente confía en ella porque solo es
// accesible a través del gateway, que borra esa cabecera de las peticiones de
// los clientes. Así el SDK puede acotar ListTasks al llamante.
type gatewayIdentity struct {
	a2asrv.PassthroughCallInterceptor
}

func (gatewayIdentity) Before(ctx context.Context, cc *a2asrv.CallContext,
	_ *a2asrv.Request) (context.Context, any, error) {
	if sp := cc.ServiceParams(); sp != nil {
		if v, ok := sp.Get("x-a2a-sub"); ok && len(v) > 0 && v[0] != "" {
			cc.User = &a2asrv.User{Name: v[0], Authenticated: true}
		}
	}
	return ctx, nil, nil
}

type staticProducer struct{ card *a2a.AgentCard }

func (p staticProducer) Card(context.Context) (*a2a.AgentCard, error) { return p.card, nil }

// ---------------------------------------------------------------------------

type executor struct{}

func (executor) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	delay, hold := duration(ec, "delayMs"), duration(ec, "holdMs")
	return func(yield func(a2a.Event, error) bool) {
		if ec.StoredTask == nil {
			if !yield(a2a.NewSubmittedTask(ec, ec.Message), nil) {
				return
			}
		}
		if !sleep(ctx, delay) || !yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil), nil) {
			return
		}
		if !sleep(ctx, delay+hold) {
			return
		}
		id := ""
		if ec.Message != nil {
			id = ec.Message.ID
		}
		if !yield(a2a.NewArtifactEvent(ec, a2a.NewTextPart("eco de "+id)), nil) || !sleep(ctx, delay) {
			return
		}
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil), nil)
	}
}

func (executor) Cancel(_ context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCanceled, nil), nil)
	}
}

func duration(ec *a2asrv.ExecutorContext, key string) time.Duration {
	for _, m := range []map[string]any{ec.Metadata, messageMetadata(ec)} {
		switch v := m[key].(type) {
		case float64:
			return time.Duration(v) * time.Millisecond
		case string:
			if n, err := strconv.Atoi(v); err == nil {
				return time.Duration(n) * time.Millisecond
			}
		default:
			// Sin valor o de otro tipo: se prueba el siguiente mapa.
		}
	}
	return 0
}

func messageMetadata(ec *a2asrv.ExecutorContext) map[string]any {
	if ec.Message == nil {
		return nil
	}
	return ec.Message.Metadata
}

func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

// ---------------------------------------------------------------------------
// Registro de peticiones recibidas

type received struct {
	At       time.Time         `json:"at"`
	Path     string            `json:"path"`
	Method   string            `json:"rpcMethod"`
	Headers  map[string]string `json:"headers"`
	HasAuthz bool              `json:"hasAuthorizationHeader"`
}

type recorder struct {
	mu    sync.Mutex
	items []received
}

func (rc *recorder) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 16<<20))
		r.Body = io.NopCloser(bytes.NewReader(body))
		var env struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &env)
		rec := received{At: time.Now().UTC(), Path: r.URL.Path, Method: env.Method, Headers: map[string]string{},
			HasAuthz: r.Header.Get("Authorization") != ""}
		for k, vs := range r.Header {
			lk := strings.ToLower(k)
			if strings.HasPrefix(lk, "x-a2a-") || strings.HasPrefix(lk, "a2a-") {
				rec.Headers[k] = strings.Join(vs, ",")
			}
		}
		rc.mu.Lock()
		rc.items = append(rc.items, rec)
		rc.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}

func (rc *recorder) list(w http.ResponseWriter, _ *http.Request) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rc.items)
}

func (rc *recorder) reset(w http.ResponseWriter, _ *http.Request) {
	rc.mu.Lock()
	rc.items = nil
	rc.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func loadSigner(path string) (*a2acrypto.Signer, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("%s no es PEM", path)
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	signer, ok := k.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("%s no contiene una clave de firma", path)
	}
	return a2acrypto.NewSigner(a2acrypto.SignerConfig{PrivateKey: signer, KeyID: signingKID})
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
