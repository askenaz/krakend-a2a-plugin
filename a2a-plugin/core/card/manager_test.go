// Copyright 2026 Los autores del plugin A2A para KrakenD.

package card

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"krakend-a2a.local/core/config"
)

const upstreamCard = `{"name":"Billing","description":"x","version":"1",
  "supportedInterfaces":[{"url":"http://billing.internal:8080/","protocolBinding":"JSONRPC","protocolVersion":"1.0"}],
  "capabilities":{"streaming":true},"defaultInputModes":[],"defaultOutputModes":[],"skills":[]}`

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time      { return c.t }
func (c *fakeClock) add(d time.Duration) { c.t = c.t.Add(d) }
func newClock() *fakeClock               { return &fakeClock{t: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)} }
func cfgFor(t *testing.T, s string) *config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// agente simulado que cuenta las peticiones y puede caerse.
type upstream struct {
	srv  *httptest.Server
	hits atomic.Int32
	down atomic.Bool
	body atomic.Value
	etag string
}

func newUpstream(t *testing.T) *upstream {
	u := &upstream{etag: `"up-1"`}
	u.body.Store(upstreamCard)
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.hits.Add(1)
		if u.down.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if r.Header.Get("If-None-Match") == u.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", u.etag)
		_, _ = w.Write([]byte(u.body.Load().(string)))
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func upstreamManager(t *testing.T, u *upstream, clock *fakeClock, extra string) *Manager {
	t.Helper()
	cfg := cfgFor(t, fmt.Sprintf(`{"public_url":"https://gw.example.com","agents":[{"id":"billing",
	  "card":{"source":"upstream","url":%q,"ttl":"1m","max_stale":"10m"%s}}]}`, u.srv.URL, extra))
	m, err := NewManager(cfg, &cfg.Agents[0], Options{StreamingAvailable: true, Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	m.failureBackoff = 0
	return m
}

// Escenario: card en caché vigente.
func TestUpstreamCaching(t *testing.T) {
	u, clock := newUpstream(t), newClock()
	m := upstreamManager(t, u, clock, "")
	p1, err := m.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	clock.add(30 * time.Second)
	if _, err := m.Get(context.Background()); err != nil || u.hits.Load() != 1 {
		t.Fatalf("dentro del TTL no debe contactarse al agente (peticiones=%d, err=%v)", u.hits.Load(), err)
	}
	clock.add(time.Minute)
	p2, err := m.Get(context.Background())
	if err != nil || u.hits.Load() != 2 {
		t.Fatalf("vencido el TTL debe revalidarse (peticiones=%d, err=%v)", u.hits.Load(), err)
	}
	if p1.ETag != p2.ETag {
		t.Error("una revalidación 304 no debe cambiar la card publicada")
	}
	if !strings.Contains(string(p1.Body), "https://gw.example.com/a2a/billing") {
		t.Errorf("la card debe apuntar al gateway: %s", p1.Body)
	}
}

// Escenarios: agente caído con copia obsoleta admisible y sin copia.
func TestUpstreamStaleAndUnavailable(t *testing.T) {
	u, clock := newUpstream(t), newClock()
	m := upstreamManager(t, u, clock, "")
	if _, err := m.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	u.down.Store(true)
	clock.add(5 * time.Minute)
	if _, err := m.Get(context.Background()); err != nil {
		t.Fatalf("con copia de menos de max_stale debe servirse la copia: %v", err)
	}
	clock.add(10 * time.Minute)
	if _, err := m.Get(context.Background()); err != ErrUnavailable {
		t.Fatalf("superado max_stale debe fallar: %v", err)
	}

	u2 := newUpstream(t)
	u2.down.Store(true)
	m2 := upstreamManager(t, u2, newClock(), "")
	rec := httptest.NewRecorder()
	m2.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin copia y con el agente caído se espera 503, obtenido %d", rec.Code)
	}
}

func TestFailureBackoff(t *testing.T) {
	u, clock := newUpstream(t), newClock()
	u.down.Store(true)
	m := upstreamManager(t, u, clock, "")
	m.failureBackoff = 5 * time.Second
	_, _ = m.Get(context.Background())
	_, _ = m.Get(context.Background())
	if u.hits.Load() != 1 {
		t.Fatalf("tras un fallo no debe reintentarse antes de la pausa (peticiones=%d)", u.hits.Load())
	}
	clock.add(6 * time.Second)
	_, _ = m.Get(context.Background())
	if u.hits.Load() != 2 {
		t.Fatalf("pasada la pausa debe reintentarse (peticiones=%d)", u.hits.Load())
	}
}

func TestUpstreamMaxBytes(t *testing.T) {
	u, clock := newUpstream(t), newClock()
	u.body.Store(strings.Replace(upstreamCard, `"x"`, `"`+strings.Repeat("a", 4096)+`"`, 1))
	m := upstreamManager(t, u, clock, `,"max_bytes":1024`)
	if _, err := m.Get(context.Background()); err == nil {
		t.Fatal("una card mayor que max_bytes debe rechazarse")
	}
}

// Escenario: agente configurado (estático) y caché HTTP.
func TestStaticAndHTTP(t *testing.T) {
	cfg := cfgFor(t, `{"public_url":"https://gw.example.com","agents":[{"id":"faq",
	  "card":{"source":"static","inline":{"name":"FAQ","url":"http://faq.internal/","capabilities":{}},`+
		`"http_max_age":"2m"}}]}`)
	m, err := NewManager(cfg, &cfg.Agents[0], Options{})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/a2a/faq/.well-known/agent-card.json", nil))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("estado %d, tipo %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	etag := rec.Header().Get("ETag")
	if etag == "" || rec.Header().Get("Cache-Control") != "public, max-age=120" {
		t.Fatalf("faltan ETag o Cache-Control: %v", rec.Header())
	}
	if !strings.Contains(rec.Body.String(), `"url":"https://gw.example.com/a2a/faq"`) {
		t.Fatalf("la card estática debe transformarse: %s", rec.Body)
	}

	// Escenario: petición condicional.
	for _, inm := range []string{etag, "W/" + etag, `"otro", ` + etag, "*"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("If-None-Match", inm)
		rec = httptest.NewRecorder()
		m.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
			t.Fatalf("If-None-Match %q: estado %d, cuerpo %q", inm, rec.Code, rec.Body)
		}
	}

	rec = httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/", nil))
	if rec.Code != 200 || rec.Body.Len() != 0 {
		t.Fatalf("HEAD: estado %d, cuerpo %d bytes", rec.Code, rec.Body.Len())
	}
	rec = httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: estado %d", rec.Code)
	}
}

func TestStaticFileAndErrors(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "card.json")
	_ = os.WriteFile(good, []byte(upstreamCard), 0o600)
	bad := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(bad, []byte(`{"name":"sin interfaces"}`), 0o600)

	mk := func(file string) error {
		cfg := cfgFor(t, fmt.Sprintf(`{"public_url":"https://gw.example.com","agents":[{"id":"a","card":{"source":"static",`+
			`"file":%q,"http_max_age":"0s"}}]}`, file))
		m, err := NewManager(cfg, &cfg.Agents[0], Options{})
		if err == nil {
			rec := httptest.NewRecorder()
			m.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			if rec.Header().Get("Cache-Control") != "no-cache" {
				t.Errorf("http_max_age=0s debe dar no-cache: %q", rec.Header().Get("Cache-Control"))
			}
		}
		return err
	}
	if err := mk(good); err != nil {
		t.Fatalf("fichero válido rechazado: %v", err)
	}
	if err := mk(bad); err == nil {
		t.Fatal("una card estática sin interfaces debe impedir arrancar")
	}
	if err := mk(filepath.Join(dir, "no-existe.json")); err == nil {
		t.Fatal("un fichero inexistente debe impedir arrancar")
	}
}

// ---------------------------------------------------------------------------
// Firmas

// sign firma una card como lo hace el SDK oficial (a2acrypto.Signer).
func sign(t *testing.T, card []byte, key crypto.Signer, alg, kid string) []byte {
	t.Helper()
	m, err := decodeCard(card)
	if err != nil {
		t.Fatal(err)
	}
	delete(m, "signatures")
	payload, err := Canonicalize(m)
	if err != nil {
		t.Fatal(err)
	}
	header, _ := json.Marshal(map[string]string{"alg": alg, "kid": kid, "typ": "JOSE"})
	protected := base64.RawURLEncoding.EncodeToString(header)
	input := protected + "." + base64.RawURLEncoding.EncodeToString(payload)

	var sig []byte
	switch k := key.(type) {
	case ed25519.PrivateKey:
		sig = ed25519.Sign(k, []byte(input))
	case *ecdsa.PrivateKey:
		h := sha256.Sum256([]byte(input))
		r, s, err := ecdsa.Sign(rand.Reader, k, h[:])
		if err != nil {
			t.Fatal(err)
		}
		sig = append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	case *rsa.PrivateKey:
		h := sha256.Sum256([]byte(input))
		sig, err = rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, h[:])
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("tipo de clave no admitido %T", key)
	}
	m["signatures"] = []interface{}{map[string]interface{}{
		"protected": protected, "signature": base64.RawURLEncoding.EncodeToString(sig),
	}}
	out, _ := json.Marshal(m)
	return out
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func TestVerifyCard(t *testing.T) {
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	_, ed, _ := ed25519.GenerateKey(rand.Reader)
	rs, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwks := fmt.Sprintf(`{"keys":[
	  {"kty":"EC","kid":"ec-1","crv":"P-256","x":%q,"y":%q},
	  {"kty":"OKP","kid":"ed-1","crv":"Ed25519","x":%q},
	  {"kty":"RSA","kid":"rsa-1","n":%q,"e":%q}]}`,
		b64(ec.X.FillBytes(make([]byte, 32))), b64(ec.Y.FillBytes(make([]byte, 32))),
		b64(ed.Public().(ed25519.PublicKey)),
		b64(rs.N.Bytes()), b64([]byte{1, 0, 1}))
	ks, err := ParseKeySet([]byte(jwks))
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		key      crypto.Signer
		alg, kid string
	}{{ec, "ES256", "ec-1"}, {ed, "EdDSA", "ed-1"}, {rs, "RS256", "rsa-1"}} {
		signed := sign(t, []byte(upstreamCard), c.key, c.alg, c.kid)
		if err := ks.VerifyCard(signed); err != nil {
			t.Errorf("%s: firma válida rechazada: %v", c.alg, err)
		}
		// El orden de los campos y los espacios no afectan a la forma canónica.
		var v interface{}
		_ = json.Unmarshal(signed, &v)
		pretty, _ := json.MarshalIndent(v, "", "    ")
		if err := ks.VerifyCard(pretty); err != nil {
			t.Errorf("%s: la misma card con otro formato debe verificar: %v", c.alg, err)
		}
		tampered := strings.Replace(string(signed), `"Billing"`, `"Billing2"`, 1)
		if err := ks.VerifyCard([]byte(tampered)); err == nil {
			t.Errorf("%s: una card alterada debe rechazarse", c.alg)
		}
	}

	if err := ks.VerifyCard([]byte(upstreamCard)); err == nil {
		t.Error("una card sin firmas debe rechazarse")
	}
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err := ks.VerifyCard(sign(t, []byte(upstreamCard), other, "ES256", "desconocida")); err == nil {
		t.Error("una firma con un kid desconocido debe rechazarse")
	}
	if err := ks.VerifyCard(sign(t, []byte(upstreamCard), ec, "ES384", "ec-1")); err == nil {
		t.Error("un alg que no corresponde a la clave debe rechazarse")
	}
}

// Escenario: firma remota inválida impide publicar la card.
func TestUpstreamSignatureRequired(t *testing.T) {
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	dir := t.TempDir()
	jwksFile := filepath.Join(dir, "trusted.jwks.json")
	_ = os.WriteFile(jwksFile, []byte(fmt.Sprintf(`{"keys":[{"kty":"EC","kid":"k","crv":"P-256","x":%q,"y":%q}]}`,
		b64(ec.X.FillBytes(make([]byte, 32))), b64(ec.Y.FillBytes(make([]byte, 32))))), 0o600)

	u, clock := newUpstream(t), newClock()
	extra := fmt.Sprintf(`,"signature":{"verify":true,"jwks_file":%q}`, jwksFile)
	m := upstreamManager(t, u, clock, extra)
	if _, err := m.Get(context.Background()); err == nil {
		t.Fatal("una card sin firma válida no debe publicarse")
	}

	u.body.Store(string(sign(t, []byte(upstreamCard), ec, "ES256", "k")))
	u.etag = `"up-2"`
	clock.add(time.Hour)
	p, err := m.Get(context.Background())
	if err != nil {
		t.Fatalf("una card con firma válida debe publicarse: %v", err)
	}
	if strings.Contains(string(p.Body), "signatures") {
		t.Error("la card publicada no lleva la firma original")
	}
}

func TestParseKeySetErrors(t *testing.T) {
	for _, s := range []string{`{}`, `{"keys":[{"kty":"EC","crv":"P-256","x":"AA","y":"AA"}]}`,
		`{"keys":[{"kty":"oct","kid":"a","k":"AA"}]}`, `{"keys":[{"kty":"RSA","kid":"a","n":"AQAB","e":"AQAB"}]}`, `no`} {
		if _, err := ParseKeySet([]byte(s)); err == nil {
			t.Errorf("JWKS %s debería rechazarse", s)
		}
	}
}

// Vector de prueba de RFC 8785, sección 3.2.3.
func TestCanonicalizeRFC8785(t *testing.T) {
	want := `{"literals":[null,true,false],"numbers":[333333333.3333333,1e+30,4.5,0.002,1e-27],` +
		`"string":"€$\u000f\nA'B\"\\\\\"/"}`
	v, err := decodeCard([]byte(esc(rfc8785Input)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Canonicalize(v)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("\nobtenido %s\nesperado %s", got, want)
	}
	// Orden por unidades UTF-16: U+1F600 (par subrogado D83D) va antes que U+FB01.
	v, _ = decodeCard([]byte(`{"ﬁ":1,"😀":2,"a":3}`))
	got, _ = Canonicalize(v)
	if string(got) != "{\"a\":3,\"\U0001F600\":2,\"ﬁ\":1}" {
		t.Fatalf("orden UTF-16 incorrecto: %s", got)
	}
}

// El vector se escribe con %u en lugar de la barra invertida para que ninguna
// herramienta convierta las secuencias de escape al guardar el fichero.
const rfc8785Input = `{"numbers":[333333333.33333329,1E30,4.50,2e-3,0.000000000000000000000000001],` +
	`"string":"%u20ac$%u000F%u000aA'%u0042%u0022%u005c\\\"\/","literals":[null,true,false]}`

func esc(s string) string { return strings.ReplaceAll(s, "%u", "\\"+"u") }
