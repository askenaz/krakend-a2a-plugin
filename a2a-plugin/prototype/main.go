// Copyright 2026 Los autores del plugin A2A para KrakenD.

// Prototipo de la fase 0 del plugin A2A para KrakenD.
//
// NO es código de producción. Existe solo para validar los supuestos S1 a S7
// de design.md en KrakenD Community 2.13.11 y Enterprise 3.0.4 con un único
// código fuente. Solo usa la biblioteca estándar (design.md D13).
//
// Exporta dos símbolos desde el mismo .so:
//
//   - HandlerRegisterer: borde A2A en la capa de router (punto [1] de D2).
//     Valida el sobre JSON-RPC, crea una entrada de correlación, reescribe la
//     ruta a /__a2a/{agent}/{unary|stream} y cede el control al enrutador.
//   - ModifierRegisterer: autorización en la capa de proxy (punto [3] de D2).
//     Lee la entrada de correlación y la identidad propagada por
//     auth/validator, y deniega cortando el pipeline.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	edgeName  = "a2a-gateway-proto"
	authzName = "a2a-authz-proto"

	hdrReqID = "X-A2A-Request-Id"
	hdrOp    = "X-A2A-Operation"
	hdrAgent = "X-A2A-Agent"
	hdrSub   = "X-A2A-Sub"

	maxBody = 4 << 20
)

// Símbolos que busca el cargador de plugins de KrakenD. Son los mismos en
// Community 2.13 y en Enterprise 3.0.
var (
	HandlerRegisterer  = edgeRegisterer(edgeName)
	ModifierRegisterer = authzRegisterer(authzName)
)

func main() {
	// Sin efecto: KrakenD carga el plugin con plugin.Open y solo usa los
	// símbolos exportados.
}

// ---------------------------------------------------------------------------
// Logger

type Logger interface {
	Debug(v ...interface{})
	Info(v ...interface{})
	Warning(v ...interface{})
	Error(v ...interface{})
	Critical(v ...interface{})
	Fatal(v ...interface{})
}

type stdLogger struct{}

func (stdLogger) Debug(v ...interface{})    { fmt.Println(append([]interface{}{"DEBUG"}, v...)...) }
func (stdLogger) Info(v ...interface{})     { fmt.Println(append([]interface{}{"INFO"}, v...)...) }
func (stdLogger) Warning(v ...interface{})  { fmt.Println(append([]interface{}{"WARNING"}, v...)...) }
func (stdLogger) Error(v ...interface{})    { fmt.Println(append([]interface{}{"ERROR"}, v...)...) }
func (stdLogger) Critical(v ...interface{}) { fmt.Println(append([]interface{}{"CRITICAL"}, v...)...) }
func (stdLogger) Fatal(v ...interface{})    { fmt.Println(append([]interface{}{"FATAL"}, v...)...) }

var (
	loggerMu sync.RWMutex
	logger   Logger = stdLogger{}
)

func setLogger(v interface{}) {
	l, ok := v.(Logger)
	if !ok {
		return
	}
	loggerMu.Lock()
	logger = l
	loggerMu.Unlock()
}

func logf(level, format string, args ...interface{}) {
	loggerMu.RLock()
	l := logger
	loggerMu.RUnlock()
	msg := "[PLUGIN: a2a-proto] " + fmt.Sprintf(format, args...)
	switch level {
	case "error":
		l.Error(msg)
	case "warning":
		l.Warning(msg)
	default:
		l.Info(msg)
	}
}

// ---------------------------------------------------------------------------
// Estado compartido entre los dos símbolos (supuesto S3)

type entry struct {
	mu       sync.Mutex
	agent    string
	op       string
	created  time.Time
	authzRan bool
	denied   bool
	subHash  string
}

var (
	entries     sync.Map // id -> *entry
	liveEntries atomic.Int64
	loadedAt    = time.Now()
)

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func shortHash(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:6])
}

// ---------------------------------------------------------------------------
// Configuración (supuesto S5)
//
// Community 2.x entrega al plugin el mapa del espacio de nombres, con la
// configuración bajo la clave del nombre del plugin. Enterprise 3.x entrega
// directamente el contenido de "config". Se detecta la forma y se registra.

func pluginConfig(name string, extra map[string]interface{}) (map[string]interface{}, string) {
	if extra == nil {
		return map[string]interface{}{}, "vacia"
	}
	if v, ok := extra[name].(map[string]interface{}); ok {
		return v, "anidada-por-nombre (2.x)"
	}
	return extra, "directa (3.x)"
}

func keys(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func cfgString(cfg map[string]interface{}, k, def string) string {
	if v, ok := cfg[k].(string); ok && v != "" {
		return v
	}
	return def
}

func cfgStrings(cfg map[string]interface{}, k string) []string {
	raw, _ := cfg[k].([]interface{})
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Borde A2A: plugin de servidor (supuestos S1, S2)

type edgeRegisterer string

func (r edgeRegisterer) RegisterLogger(v interface{}) { setLogger(v) }

func (r edgeRegisterer) RegisterHandlers(f func(
	name string,
	handler func(context.Context, map[string]interface{}, http.Handler) (http.Handler, error),
)) {
	f(string(r), r.factory)
}

func (r edgeRegisterer) factory(_ context.Context, extra map[string]interface{},
	next http.Handler) (http.Handler, error) {
	cfg, form := pluginConfig(string(r), extra)
	logf("info", "borde cargado: forma de config=%s claves recibidas=%v claves de config=%v", form, keys(extra), keys(cfg))
	e := &edge{
		next:     next,
		base:     strings.TrimSuffix(cfgString(cfg, "base_path", "/a2a"), "/"),
		internal: strings.TrimSuffix(cfgString(cfg, "internal_prefix", "/__a2a"), "/"),
		stats:    cfgString(cfg, "stats_path", "/__a2a_proto/stats"),
		noFlush:  map[string]bool{},
	}
	for _, a := range cfgStrings(cfg, "no_flush_agents") {
		e.noFlush[a] = true
	}
	return e, nil
}

type edge struct {
	next     http.Handler
	base     string
	internal string
	stats    string
	noFlush  map[string]bool
}

func (e *edge) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// D4: ninguna cabecera reservada puede venir del cliente.
	for k := range req.Header {
		if strings.HasPrefix(strings.ToLower(k), "x-a2a-") {
			req.Header.Del(k)
		}
	}

	p := req.URL.Path
	switch {
	case p == e.stats:
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"liveEntries": liveEntries.Load(), "loadedAt": loadedAt.UTC(),
		})
		return
	case p == e.internal || strings.HasPrefix(p, e.internal+"/"):
		// Las rutas internas solo se alcanzan tras la reescritura.
		http.NotFound(w, req)
		return
	case !strings.HasPrefix(p, e.base+"/"):
		e.next.ServeHTTP(w, req)
		return
	default:
		// Ruta del borde A2A: se procesa a continuación.
	}

	agent := strings.TrimPrefix(p, e.base+"/")
	if agent == "" || strings.Contains(agent, "/") {
		e.next.ServeHTTP(w, req)
		return
	}
	if req.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(req.Body, maxBody+1))
	if err != nil {
		writeRPCError(w, nil, -32603, "cannot read request")
		return
	}
	if len(body) > maxBody {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	var env struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  json.RawMessage `json:"method"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		writeRPCError(w, nil, -32700, "Parse error")
		return
	}
	var method string
	if env.JSONRPC != "2.0" || json.Unmarshal(env.Method, &method) != nil || method == "" {
		writeRPCError(w, env.ID, -32600, "Invalid Request")
		return
	}

	class := "unary"
	if isStream(method) {
		class = "stream"
	}

	id := newID()
	ent := &entry{agent: agent, op: method, created: time.Now()}
	entries.Store(id, ent)
	liveEntries.Add(1)
	defer func() {
		entries.Delete(id)
		liveEntries.Add(-1)
	}()

	req.Header.Set(hdrReqID, id)
	req.Header.Set(hdrOp, method)
	req.Header.Set(hdrAgent, agent)

	// S1: reescritura de la ruta antes de ceder el control al enrutador.
	target := e.internal + "/" + agent + "/" + class
	req.URL.Path = target
	req.URL.RawPath = ""
	req.RequestURI = target
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))

	rw := &respWriter{w: w, hdr: http.Header{}, flush: !e.noFlush[agent]}
	start := time.Now()
	e.next.ServeHTTP(rw, req)

	ent.mu.Lock()
	ran, denied, subHash := ent.authzRan, ent.denied, ent.subHash
	ent.mu.Unlock()
	logf("info", "agente=%s op=%s ruta=%s estado=%d authz_ejecutada=%v denegada=%v sujeto_sha=%s streaming=%v "+
		"flushes=%d dur=%s",
		agent, method, target, rw.status, ran, denied, subHash, rw.streaming, rw.flushes,
		time.Since(start).Round(time.Millisecond))

	if rw.streaming {
		return // ya entregado al cliente evento a evento
	}
	if denied {
		// D3: se descarta lo que produjo el pipeline y se responde como si la
		// tarea no existiera (D12).
		writeRPCError(w, env.ID, -32001, "Task not found")
		return
	}
	rw.commit()
}

func isStream(method string) bool {
	switch method {
	case "SendStreamingMessage", "SubscribeToTask", "message/stream", "tasks/resubscribe":
		return true
	default:
		return false
	}
}

// respWriter retiene las respuestas unarias, para poder sustituirlas si la
// autorización deniega, y deja pasar las SSE haciendo flush en cada escritura
// (supuesto S2).
type respWriter struct {
	w           http.ResponseWriter
	hdr         http.Header
	status      int
	wroteHeader bool
	streaming   bool
	flush       bool
	flushes     int
	buf         bytes.Buffer
}

func (r *respWriter) Header() http.Header { return r.hdr }

func (r *respWriter) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.status = code
	if strings.HasPrefix(r.hdr.Get("Content-Type"), "text/event-stream") {
		r.streaming = true
		copyHeader(r.w.Header(), r.hdr)
		r.w.Header().Del("Content-Length")
		r.w.WriteHeader(code)
	}
}

func (r *respWriter) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	if !r.streaming {
		return r.buf.Write(p)
	}
	n, err := r.w.Write(p)
	if r.flush {
		r.Flush()
	}
	return n, err
}

func (r *respWriter) Flush() {
	if !r.streaming {
		return
	}
	if f, ok := r.w.(http.Flusher); ok {
		f.Flush()
		r.flushes++
	}
}

func (r *respWriter) Unwrap() http.ResponseWriter { return r.w }

func (r *respWriter) commit() {
	copyHeader(r.w.Header(), r.hdr)
	if r.status == 0 {
		r.status = http.StatusOK
	}
	r.w.WriteHeader(r.status)
	_, _ = r.w.Write(r.buf.Bytes())
}

func copyHeader(dst, src http.Header) {
	for k, vs := range src {
		dst[k] = append([]string(nil), vs...)
	}
}

// ---------------------------------------------------------------------------
// Autorización: modificador de petición (supuestos S3, S4)

type authzRegisterer string

func (r authzRegisterer) RegisterLogger(v interface{}) { setLogger(v) }

func (r authzRegisterer) RegisterModifiers(f func(
	name string,
	factoryFunc func(map[string]interface{}) func(interface{}) (interface{}, error),
	appliesToRequest bool,
	appliesToResponse bool,
)) {
	f(string(r), r.factory, true, false)
}

type headerer interface {
	Headers() map[string][]string
}

type statusErr struct {
	code int
	msg  string
}

func (e *statusErr) Error() string   { return e.msg }
func (e *statusErr) StatusCode() int { return e.code }

func (r authzRegisterer) factory(extra map[string]interface{}) func(interface{}) (interface{}, error) {
	cfg, form := pluginConfig(string(r), extra)
	denySub := cfgString(cfg, "deny_subject", "")
	logf("info", "autorizacion cargada: forma de config=%s claves recibidas=%v claves de "+
		"config=%v", form, keys(extra), keys(cfg))

	return func(input interface{}) (interface{}, error) {
		req, ok := input.(headerer)
		if !ok {
			logf("error", "autorizacion: tipo de entrada inesperado %T", input)
			return nil, errors.New("a2a: unexpected request type")
		}
		h := req.Headers()
		names := make([]string, 0, len(h))
		for k := range h {
			names = append(names, k)
		}
		sort.Strings(names)

		id := header(h, hdrReqID)
		v, found := entries.Load(id)
		if !found {
			logf("warning", "autorizacion: sin entrada de correlacion, se deniega (cabeceras=%v)", names)
			return nil, &statusErr{http.StatusForbidden, "a2a: request did not pass the edge"}
		}
		ent := v.(*entry)
		sub := header(h, hdrSub)

		ent.mu.Lock()
		defer ent.mu.Unlock()
		ent.authzRan = true
		ent.subHash = shortHash(sub)
		logf("info", "autorizacion: entrada encontrada agente=%s op=%s sujeto_presente=%v sujeto_sha=%s cabeceras=%v",
			ent.agent, ent.op, sub != "", ent.subHash, names)

		if sub == "" || (denySub != "" && sub == denySub) {
			ent.denied = true
			return nil, &statusErr{http.StatusForbidden, "a2a: access denied"}
		}
		return input, nil
	}
}

func header(h map[string][]string, name string) string {
	for k, vs := range h {
		if strings.EqualFold(k, name) && len(vs) > 0 {
			return vs[0]
		}
	}
	return ""
}

// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]interface{}{"code": code, "message": msg},
	})
}
