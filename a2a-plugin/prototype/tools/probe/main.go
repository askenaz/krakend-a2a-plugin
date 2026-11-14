// Copyright 2026 Los autores del plugin A2A para KrakenD.

// probe ejecuta los escenarios de la fase 0 contra un gateway con el prototipo
// cargado y contra el agente simulado, y emite un informe por supuesto.
//
// Uso:
//
//	go run ./tools/probe -init
//	go run ./tools/probe -edition ce -gw http://localhost:8081 -agent http://localhost:8090
//	go run ./tools/probe -print-token alice
//
// Los tokens se firman con la clave de prueba de .secrets/jwk.json, que genera
// -init y no se versiona. Esa clave solo existe para este prototipo.
package main

import (
	"bufio"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type check struct {
	ID        string `json:"id"`
	Supuestos string `json:"supuestos"`
	Escenario string `json:"escenario"`
	Resultado string `json:"resultado"` // PASA, FALLA, INFO
	Detalle   string `json:"detalle"`
}

type rec struct {
	Path       string            `json:"path"`
	RPCMethod  string            `json:"rpcMethod"`
	BodySHA256 string            `json:"bodySha256"`
	Headers    map[string]string `json:"headers"`
	HasAuthz   bool              `json:"hasAuthorizationHeader"`
}

var (
	gw, admin string
	key       []byte
	kid       string
	client    = &http.Client{Timeout: 60 * time.Second}
	checks    []check
)

const (
	issuer   = "https://idp.test.local"
	audience = "a2a-gateway"

	// Resultados de una comprobación.
	pass       = "PASA"
	fail       = "FALLA"
	infoResult = "INFO"

	edgePath        = "/a2a/x"
	hdrSub          = "X-A2A-Sub"
	hdrOperation    = "X-A2A-Operation"
	hdrRequestID    = "X-A2A-Request-Id"
	hdrContentType  = "Content-Type"
	contentTypeJSON = "application/json"

	emptySend     = `{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{}}`
	agentRequests = "estado=%d peticiones_en_agente=%d"
)

func main() {
	edition := flag.String("edition", "ce", "ce o ee, solo para el informe")
	flag.StringVar(&gw, "gw", "http://localhost:8081", "URL del gateway")
	flag.StringVar(&admin, "agent", "http://localhost:8090", "URL de administración del agente simulado")
	jwkPath := flag.String("jwk", ".secrets/jwk.json", "JWK de prueba, excluida de git")
	initKey := flag.Bool("init", false, "genera la JWK de prueba si no existe y termina")
	delay := flag.Duration("delay", 1500*time.Millisecond, "pausa entre eventos SSE del agente simulado")
	out := flag.String("out", "", "fichero JSON de resultados (opcional)")
	printToken := flag.String("print-token", "", "imprime un token para ese sujeto y termina")
	flag.Parse()

	if *initKey {
		if err := initJWK(*jwkPath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}
	loadKey(*jwkPath)
	if *printToken != "" {
		fmt.Println(token(*printToken))
		return
	}

	alice, mallory := token("alice"), token("mallory")

	unary(alice, mallory)
	spoof(alice)
	internalBlocked(alice)
	noToken()
	parseError(alice)
	streaming("x", alice, *delay, true)
	streaming("y", alice, *delay, false)
	streamDenied(mallory)
	plainRoute()
	stats()

	fmt.Printf("\n=== Resultados fase 0 · edición %s · %s ===\n", strings.ToUpper(*edition), gw)
	failed := 0
	for _, c := range checks {
		fmt.Printf("%-6s %-4s %-8s %-48s %s\n", c.Resultado, c.ID, c.Supuestos, c.Escenario, c.Detalle)
		if c.Resultado == fail {
			failed++
		}
	}
	if *out != "" {
		writeReport(*out, *edition)
	}
	if failed > 0 {
		fmt.Printf("\n%d comprobaciones fallidas\n", failed)
		os.Exit(1)
	}
}

func writeReport(path, edition string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	report := map[string]any{"edition": edition, "gateway": gw, "at": time.Now().UTC(), "checks": checks}
	b, _ := json.MarshalIndent(report, "", "  ")
	_ = os.WriteFile(path, b, 0o644)
}

// --- escenarios ------------------------------------------------------------

func unary(alice, mallory string) {
	body := `{"jsonrpc":"2.0","id":"u-1","method":"SendMessage","params":{"message":{"messageId":"m-1",` +
		`"role":"ROLE_USER","parts":[{"text":"hola"}]}}}`
	reset()
	st, resp, _ := post(edgePath, alice, body, nil)
	got := received()
	const title = "SendMessage llega por la ruta interna"
	switch {
	case st != 200 || !strings.Contains(resp, `"result"`):
		add("T1", "S1", title, fail, fmt.Sprintf("estado=%d cuerpo=%s", st, trunc(resp)))
	case len(got) != 1:
		add("T1", "S1", title, fail, fmt.Sprintf("el agente recibió %d peticiones", len(got)))
	default:
		checkForwarded(got[0], body, title)
	}

	reset()
	st, resp, _ = post(edgePath, mallory, strings.Replace(body, "u-1", "u-2", 1), nil)
	n := len(received())
	const denied = "Denegación corta el pipeline (unaria)"
	if st == 200 && strings.Contains(resp, `"code":-32001`) && n == 0 {
		add("T5", "S3 S4", denied, pass, "respuesta -32001 y el agente no recibe nada")
	} else {
		add("T5", "S3 S4", denied, fail, fmt.Sprintf(agentRequests+" cuerpo=%s", st, n, trunc(resp)))
	}
}

// checkForwarded comprueba lo que recibió el agente de la petición unaria.
func checkForwarded(r rec, body, title string) {
	var problems []string
	if r.Headers[hdrOperation] != "SendMessage" || r.Headers["X-A2A-Agent"] != "x" {
		problems = append(problems, fmt.Sprintf("cabeceras de contexto=%v", r.Headers))
	}
	if r.Headers[hdrRequestID] == "" {
		problems = append(problems, "falta X-A2A-Request-Id")
	}
	if r.BodySHA256 != sha(body) {
		problems = append(problems, "el cuerpo cambió")
	}
	if len(problems) > 0 {
		add("T1", "S1", title, fail, strings.Join(problems, "; "))
	} else {
		add("T1", "S1", title, pass, "cuerpo intacto, cabeceras de contexto presentes")
	}

	const subTitle = "Sujeto del JWT propagado al endpoint"
	if r.Headers[hdrSub] == "alice" {
		add("T1b", "S4", subTitle, pass, "el agente ve X-A2A-Sub=alice")
	} else {
		add("T1b", "S4", subTitle, fail, fmt.Sprintf("X-A2A-Sub=%q", r.Headers[hdrSub]))
	}
	add("T1c", "-", "El token no llega al agente", info(!r.HasAuthz),
		fmt.Sprintf("Authorization en el agente=%v", r.HasAuthz))
}

func spoof(alice string) {
	body := `{"jsonrpc":"2.0","id":"s-1","method":"SendMessage","params":{"message":{"messageId":"m-2",` +
		`"role":"ROLE_USER","parts":[{"text":"x"}]}}}`
	reset()
	forged := map[string]string{hdrSub: "mallory", hdrRequestID: "forjado", hdrOperation: "CancelTask"}
	st, _, _ := post(edgePath, alice, body, forged)
	got := received()
	const title = "Cabeceras reservadas del cliente eliminadas"
	if st == 200 && len(got) == 1 && !forgedHeadersArrived(got[0]) {
		add("T2", "D4", title, pass, "el agente ve la identidad del token, no la enviada")
	} else {
		add("T2", "D4", title, fail, fmt.Sprintf("estado=%d recibidas=%v", st, got))
	}
}

// forgedHeadersArrived indica si alguna cabecera reservada que puso el cliente
// llegó al agente.
func forgedHeadersArrived(r rec) bool {
	return r.Headers[hdrSub] != "alice" || r.Headers[hdrOperation] != "SendMessage" ||
		r.Headers[hdrRequestID] == "forjado"
}

func internalBlocked(alice string) {
	reset()
	st, _, _ := post("/__a2a/x/unary", alice, emptySend, nil)
	n := len(received())
	ok := st == 404 && n == 0
	add("T3", "S1", "Ruta interna inaccesible desde fuera", passIf(ok), fmt.Sprintf(agentRequests, st, n))
}

func noToken() {
	reset()
	st, _, _ := post(edgePath, "", emptySend, nil)
	n := len(received())
	ok := st == 401 && n == 0
	add("T4", "-", "Sin token: 401 de la validación nativa", passIf(ok), fmt.Sprintf(agentRequests, st, n))
}

func parseError(alice string) {
	reset()
	st, resp, _ := post(edgePath, alice, `esto no es json`, nil)
	n := len(received())
	ok := st == 200 && strings.Contains(resp, `"code":-32700`) && n == 0
	add("T6", "-", "JSON inválido: -32700 sin llegar al agente", passIf(ok), fmt.Sprintf(agentRequests, st, n))
}

func streaming(agent, tok string, delay time.Duration, expectFlush bool) {
	reset()
	body := `{"jsonrpc":"2.0","id":"st-1","method":"SendStreamingMessage","params":{"message":{"messageId":"m-3",` +
		`"role":"ROLE_USER","parts":[{"text":"stream"}]}}}`
	req, _ := http.NewRequest(http.MethodPost, gw+"/a2a/"+agent, strings.NewReader(body))
	req.Header.Set(hdrContentType, contentTypeJSON)
	req.Header.Set("A2A-Version", "1.0")
	req.Header.Set("Authorization", "Bearer "+tok)
	start := time.Now()
	resp, err := client.Do(req)
	id, title := "T7", "SSE incremental con flush del plugin"
	if !expectFlush {
		id, title = "T8", "Control: mismo flujo sin flush del plugin"
	}
	const sup = "S2"
	if err != nil {
		add(id, sup, title, fail, err.Error())
		return
	}
	defer resp.Body.Close()
	ct := resp.Header.Get(hdrContentType)
	arrivals := eventArrivals(resp.Body, start)
	gaps, minGap := eventGaps(arrivals)
	detail := fmt.Sprintf("estado=%d tipo=%q eventos=%d llegadas=%v huecos=%v",
		resp.StatusCode, ct, len(arrivals), round(arrivals), gaps)
	incremental := len(arrivals) >= 2 && minGap >= time.Duration(float64(delay)*0.6)
	if expectFlush {
		ok := resp.StatusCode == 200 && strings.HasPrefix(ct, "text/event-stream") && incremental
		add(id, sup, title, passIf(ok), detail)
		return
	}
	verdict := "retenido hasta el final"
	if incremental {
		verdict = "incremental también sin flush"
	}
	add(id, sup, title, infoResult, verdict+"; "+detail)
}

// eventArrivals devuelve, para cada línea data: del flujo, el tiempo
// transcurrido desde start hasta que llegó.
func eventArrivals(r io.Reader, start time.Time) []time.Duration {
	var arrivals []time.Duration
	rd := bufio.NewReader(r)
	for {
		line, err := rd.ReadString('\n')
		if strings.HasPrefix(line, "data:") {
			arrivals = append(arrivals, time.Since(start))
		}
		if err != nil {
			return arrivals
		}
	}
}

// eventGaps devuelve los huecos entre llegadas y el menor de ellos.
func eventGaps(arrivals []time.Duration) ([]string, time.Duration) {
	gaps := []string{}
	minGap := time.Duration(1<<62 - 1)
	for i := 1; i < len(arrivals); i++ {
		g := arrivals[i] - arrivals[i-1]
		gaps = append(gaps, g.Round(10*time.Millisecond).String())
		if g < minGap {
			minGap = g
		}
	}
	return gaps, minGap
}

func streamDenied(mallory string) {
	reset()
	body := `{"jsonrpc":"2.0","id":"st-2","method":"SendStreamingMessage",` +
		`"params":{"message":{"messageId":"m-4","role":"ROLE_USER","parts":[{"text":"x"}]}}}`
	st, resp, _ := post(edgePath, mallory, body, nil)
	n := len(received())
	ok := st == 200 && strings.Contains(resp, `"code":-32001`) && n == 0
	add("T9", "S3 S4", "Denegación corta el pipeline (streaming)", passIf(ok),
		fmt.Sprintf(agentRequests+" cuerpo=%s", st, n, trunc(resp)))
}

func plainRoute() {
	const title = "Rutas no A2A siguen funcionando"
	resp, err := client.Get(gw + "/plain")
	if err != nil {
		add("T10", "S1", title, fail, err.Error())
		return
	}
	resp.Body.Close()
	add("T10", "S1", title, passIf(resp.StatusCode == 200), fmt.Sprintf("estado=%d", resp.StatusCode))
}

func stats() {
	const title = "Entradas de correlación liberadas"
	resp, err := client.Get(gw + "/__a2a_proto/stats")
	if err != nil {
		add("T11", "D3", title, fail, err.Error())
		return
	}
	defer resp.Body.Close()
	var s struct {
		LiveEntries int64 `json:"liveEntries"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&s)
	ok := resp.StatusCode == 200 && s.LiveEntries == 0
	add("T11", "D3", title, passIf(ok), fmt.Sprintf("entradas_vivas=%d", s.LiveEntries))
}

// --- utilidades ------------------------------------------------------------

func post(path, tok, body string, extra map[string]string) (int, string, error) {
	req, _ := http.NewRequest(http.MethodPost, gw+path, strings.NewReader(body))
	req.Header.Set(hdrContentType, contentTypeJSON)
	req.Header.Set("A2A-Version", "1.0")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err.Error(), err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), nil
}

func reset() {
	resp, err := client.Post(admin+"/__reset", contentTypeJSON, nil)
	if err == nil {
		resp.Body.Close()
	}
}

func received() []rec {
	resp, err := client.Get(admin + "/__received")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var out []rec
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func add(id, sup, esc, res, det string) {
	checks = append(checks, check{ID: id, Supuestos: sup, Escenario: esc, Resultado: res, Detalle: det})
}

func passIf(ok bool) string {
	if ok {
		return pass
	}
	return fail
}

func info(ok bool) string {
	if ok {
		return pass
	}
	return infoResult
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func trunc(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

func round(ds []time.Duration) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Round(10 * time.Millisecond).String()
	}
	return out
}

// --- JWT de prueba (HS256) -------------------------------------------------

func loadKey(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "no se puede leer la JWK de prueba:", err)
		os.Exit(2)
	}
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			K   string `json:"k"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(raw, &set); err != nil || len(set.Keys) == 0 {
		fmt.Fprintln(os.Stderr, "JWK de prueba inválida")
		os.Exit(2)
	}
	k, err := base64.RawURLEncoding.DecodeString(set.Keys[0].K)
	if err != nil {
		fmt.Fprintln(os.Stderr, "clave de la JWK de prueba inválida")
		os.Exit(2)
	}
	key, kid = k, set.Keys[0].Kid
}

func token(sub string) string {
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	now := time.Now().Unix()
	head := enc(map[string]any{"alg": "HS256", "typ": "JWT", "kid": kid})
	body := enc(map[string]any{
		"iss": issuer, "sub": sub, "aud": audience,
		"iat": now, "exp": now + 3600, "scope": "a2a.read a2a.write",
	})
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(head + "." + body))
	return head + "." + body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// initJWK genera una clave simétrica de prueba si no existe. Vive en .secrets,
// que está excluido de git.
func initJWK(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return err
	}
	doc, _ := json.MarshalIndent(map[string]any{"keys": []any{map[string]string{
		"kty": "oct", "kid": "proto-hs256", "alg": "HS256", "use": "sig",
		"k": base64.RawURLEncoding.EncodeToString(k),
	}}}, "", "  ")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, doc, 0o600)
}
