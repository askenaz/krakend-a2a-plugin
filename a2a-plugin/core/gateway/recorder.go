// Copyright 2026 Los autores del plugin A2A para KrakenD.

package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"krakend-a2a.local/core/protocol"
)

// recorder envuelve la respuesta del pipeline de KrakenD.
//
//   - Las respuestas unarias se retienen, hasta limits.max_inspect_bytes, para
//     poder registrar la propiedad, reescribir la card extendida o sustituir
//     la respuesta si la autorización denegó.
//   - Los flujos SSE se retienen solo hasta el primer evento, que registra la
//     propiedad antes de llegar al cliente. Después pasan sin analizar, con
//     flush en cada escritura (supuesto S2 validado en la fase 0).
type recorder struct {
	w     http.ResponseWriter
	g     *Gateway
	e     *entry
	ctx   context.Context
	hdr   http.Header
	limit int64

	status      int
	wroteHeader bool
	mode        int
	buf         bytes.Buffer
	committed   bool // cabeceras ya enviadas al cliente
	sent        int  // estado HTTP enviado al cliente
	blocked     *verdict
	overflowed  bool
}

const (
	modeBuffer = iota
	modeStream
	modePass
	modeDiscard
)

func (r *recorder) Header() http.Header { return r.hdr }

func (r *recorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.status = code
	if v := r.g.blockedVerdict(r.e, code); v != nil {
		r.mode = modeDiscard
		r.blocked = v
		return
	}
	if r.e.op.IsStreaming() && code == http.StatusOK &&
		strings.HasPrefix(strings.ToLower(r.hdr.Get(headerContentType)), "text/event-stream") {
		r.mode = modeStream
		return
	}
	r.mode = modeBuffer
}

func (r *recorder) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	switch r.mode {
	case modePass:
		return r.passThrough(p)
	case modeBuffer:
		return r.writeBuffered(p)
	case modeStream:
		return r.writeStream(p)
	default:
		// modeDiscard: la respuesta se sustituye al terminar.
		return len(p), nil
	}
}

func (r *recorder) passThrough(p []byte) (int, error) {
	n, err := r.w.Write(p)
	r.flushUnderlying()
	return n, err
}

func (r *recorder) writeBuffered(p []byte) (int, error) {
	if int64(r.buf.Len()+len(p)) <= r.limit {
		return r.buf.Write(p)
	}
	r.overflow()
	if r.mode == modeDiscard {
		return len(p), nil
	}
	return r.passThrough(p)
}

func (r *recorder) writeStream(p []byte) (int, error) {
	if r.committed {
		return r.passThrough(p)
	}
	r.buf.Write(p)
	end := eventEnd(r.buf.Bytes())
	switch {
	case end >= 0:
		r.handleFirstEvent(r.buf.Bytes()[:end])
		r.commitStream()
	case int64(r.buf.Len()) > r.limit:
		r.g.log.Warning(logAgent, r.e.agent.id, " primer evento SSE demasiado grande: no se registra la propiedad")
		r.commitStream()
	default:
		// Falta el resto del primer evento.
	}
	return len(p), nil
}

// Flush mantiene la capacidad de flush del pipeline (KrakenD Enterprise la usa
// en sus respuestas de streaming).
func (r *recorder) Flush() {
	if r.committed {
		r.flushUnderlying()
	}
}

func (r *recorder) flushUnderlying() {
	if f, ok := r.w.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap permite a http.ResponseController llegar al writer original.
func (r *recorder) Unwrap() http.ResponseWriter { return r.w }

// CloseNotify delega en el writer original si lo admite.
func (r *recorder) CloseNotify() <-chan bool {
	//nolint:staticcheck // interfaz obsoleta que algunos componentes aún usan
	if cn, ok := r.w.(http.CloseNotifier); ok {
		return cn.CloseNotify()
	}
	ch := make(chan bool, 1)
	go func() {
		<-r.ctx.Done()
		ch <- true
	}()
	return ch
}

func (r *recorder) commitHeaders(status int) {
	dst := r.w.Header()
	for k, vs := range r.hdr {
		dst[k] = append([]string(nil), vs...)
	}
	r.w.WriteHeader(status)
	r.committed = true
	r.sent = status
}

func (r *recorder) overflow() {
	r.overflowed = true
	if r.e.op == protocol.GetExtendedAgentCard {
		// Sin poder reescribirla, la card extendida filtraría URLs internas.
		r.mode = modeDiscard
		r.blocked = denyRPC(protocol.NewInternalError("card extendida demasiado grande"),
			"card extendida mayor que max_inspect_bytes")
		return
	}
	r.g.log.Warning(logAgent, r.e.agent.id, " op=", r.e.op.String(),
		" respuesta mayor que max_inspect_bytes: se entrega sin inspeccionar")
	r.hdr.Del(headerContentLength)
	r.commitHeaders(r.status)
	_, _ = r.w.Write(r.buf.Bytes())
	r.buf.Reset()
	r.mode = modePass
}

func (r *recorder) commitStream() {
	r.hdr.Del(headerContentLength)
	r.commitHeaders(r.status)
	_, _ = r.w.Write(r.buf.Bytes())
	r.buf.Reset()
	r.flushUnderlying()
}

func (r *recorder) handleFirstEvent(event []byte) {
	if !r.e.op.MayCreateTask() || !r.e.agent.ownership {
		return
	}
	data := sseData(event)
	resp, err := protocol.ParseResponse(data)
	if err != nil || resp.Error != nil {
		return
	}
	r.g.register(r.ctx, r.e, resp.Result)
}

// finish completa la respuesta cuando el pipeline termina y devuelve el
// resultado para el registro.
func (r *recorder) finish() string {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	switch r.mode {
	case modePass:
		return "ok-sin-inspeccionar"
	case modeStream:
		if !r.committed {
			r.commitStream()
		}
		return "ok-streaming"
	case modeDiscard:
		return r.finishDiscarded()
	default:
		return r.finishBuffered()
	}
}

// finishDiscarded sustituye la respuesta descartada por el veredicto.
func (r *recorder) finishDiscarded() string {
	v := r.blocked
	if v == nil {
		v = denyRPC(protocol.NewInternalError("respuesta descartada"), "descartada")
	}
	r.writeVerdict(v)
	if v.httpStatus == http.StatusOK && v.rpcCode == protocol.CodeInternalError && v.reason == reasonAuthzNotRun {
		return "error-configuracion"
	}
	return "denegado:" + v.reason
}

func (r *recorder) writeVerdict(v *verdict) {
	if v.challenge {
		r.w.Header().Set(headerWWWAuth, r.g.challenge(r.e.agent))
	}
	writeRPCError(r.w, v.httpStatus, r.e.rpcID, &protocol.Error{Code: v.rpcCode, Message: v.message})
	r.sent = v.httpStatus
}

func (r *recorder) finishBuffered() string {
	body := r.buf.Bytes()
	resp := asJSONRPC(body)
	ok2xx := r.status >= 200 && r.status < 300

	if resp == nil {
		return r.finishNonJSONRPC(body, ok2xx)
	}
	if ok2xx && resp.Error == nil {
		var outcome string
		if body, outcome = r.processResult(resp, body); outcome != "" {
			return outcome
		}
	}
	if r.status == http.StatusUnauthorized && r.hdr.Get(headerWWWAuth) == "" {
		r.hdr.Set(headerWWWAuth, r.g.challenge(r.e.agent))
	}
	if r.hdr.Get(headerContentLength) != "" {
		r.hdr.Set(headerContentLength, strconv.Itoa(len(body)))
	}
	r.commitHeaders(r.status)
	_, _ = r.w.Write(body)
	if resp.Error != nil {
		return "error-agente:" + strconv.Itoa(resp.Error.Code)
	}
	return "ok"
}

// finishNonJSONRPC entrega una respuesta que no es JSON-RPC.
func (r *recorder) finishNonJSONRPC(body []byte, ok2xx bool) string {
	if ok2xx {
		// Respuesta del agente que no es JSON-RPC: se entrega sin cambios.
		r.commitHeaders(r.status)
		_, _ = r.w.Write(body)
		return "ok-no-jsonrpc"
	}
	// Error del pipeline de KrakenD sin cuerpo JSON-RPC (spec
	// a2a-protocol-gateway): se conserva el estado y Retry-After.
	if ra := r.hdr.Get("Retry-After"); ra != "" {
		r.w.Header().Set("Retry-After", ra)
	}
	wa := r.hdr.Get(headerWWWAuth)
	if wa == "" && r.status == http.StatusUnauthorized {
		wa = r.g.challenge(r.e.agent)
	}
	if wa != "" {
		r.w.Header().Set(headerWWWAuth, wa)
	}
	msg := http.StatusText(r.status)
	if msg == "" {
		msg = "Internal error"
	}
	writeRPCError(r.w, r.status, r.e.rpcID, &protocol.Error{Code: protocol.CodeInternalError, Message: msg})
	r.sent = r.status
	return "error-pipeline:" + strconv.Itoa(r.status)
}

// processResult registra la propiedad y reescribe la card extendida. Si la
// card no es publicable responde con un error y devuelve el resultado final.
func (r *recorder) processResult(resp *protocol.Response, body []byte) ([]byte, string) {
	if r.e.op.MayCreateTask() && r.e.agent.ownership {
		r.g.register(r.ctx, r.e, resp.Result)
	}
	if r.e.op != protocol.GetExtendedAgentCard {
		return body, ""
	}
	rewritten, err := r.e.agent.card.Transformer().TransformValue(resp.Result)
	if err != nil {
		r.g.log.Error(logAgent, r.e.agent.id, " card extendida no publicable: ", err.Error())
		r.writeVerdict(denyRPC(protocol.NewInternalError("card extendida inválida"), "card extendida inválida"))
		return nil, "error-card-extendida"
	}
	r.hdr.Del(headerContentLength)
	return rpcResult(resp.ID, rewritten), ""
}

// reasonAuthzNotRun es el motivo de bloqueo cuando falta el modificador.
const reasonAuthzNotRun = "autorización no ejecutada"

// blockedVerdict decide, al llegar la respuesta del pipeline, si hay que
// descartarla: la autorización denegó, o no llegó a ejecutarse cuando debía y
// el agente respondió (design.md D3, fallo cerrado).
func (g *Gateway) blockedVerdict(e *entry, status int) *verdict {
	e.mu.Lock()
	v, ran := e.verdict, e.authzRan
	e.mu.Unlock()
	if v != nil {
		return v
	}
	if !ran && g.authzRequired(e.agent, e.op) && status >= 200 && status < 300 {
		g.log.Critical(logAgent, e.agent.id, " op=", e.op.String(),
			" el agente respondió sin pasar por la autorización A2A: falta el modificador a2a-authz en el "+
				"endpoint interno")
		return denyRPC(protocol.NewInternalError(reasonAuthzNotRun), reasonAuthzNotRun)
	}
	return nil
}

// asJSONRPC devuelve la respuesta si el cuerpo es un sobre JSON-RPC 2.0.
func asJSONRPC(body []byte) *protocol.Response {
	t := bytes.TrimSpace(body)
	if len(t) == 0 || t[0] != '{' {
		return nil
	}
	var probe struct {
		JSONRPC string `json:"jsonrpc"`
	}
	if json.Unmarshal(t, &probe) != nil || probe.JSONRPC != "2.0" {
		return nil
	}
	resp, err := protocol.ParseResponse(t)
	if err != nil {
		return nil
	}
	return resp
}

func rpcResult(id, result json.RawMessage) []byte {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	b, _ := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
	}{"2.0", id, result})
	return b
}

// eventEnd devuelve la longitud del primer evento SSE completo, o -1.
func eventEnd(b []byte) int {
	best := -1
	for _, sep := range []string{"\n\n", "\r\n\r\n", "\r\r"} {
		if i := bytes.Index(b, []byte(sep)); i >= 0 && (best < 0 || i+len(sep) < best) {
			best = i + len(sep)
		}
	}
	return best
}

// sseData une las líneas data: de un evento SSE.
func sseData(event []byte) []byte {
	var parts []string
	sc := bufio.NewScanner(bytes.NewReader(event))
	sc.Buffer(make([]byte, 64*1024), len(event)+1)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if v, ok := strings.CutPrefix(line, "data:"); ok {
			parts = append(parts, strings.TrimPrefix(v, " "))
		}
	}
	return []byte(strings.Join(parts, "\n"))
}
