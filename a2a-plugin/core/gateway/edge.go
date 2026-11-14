// Copyright 2026 Los autores del plugin A2A para KrakenD.

package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"krakend-a2a.local/core/protocol"
)

// Cabeceras y valores HTTP que el borde usa en varios sitios.
const (
	headerContentType   = "Content-Type"
	headerContentLength = "Content-Length"
	headerCacheControl  = "Cache-Control"
	headerWWWAuth       = "WWW-Authenticate"
	contentTypeJSON     = "application/json"
	wellKnownCard       = "/.well-known/agent-card.json"
)

// Handler envuelve el handler de KrakenD con el borde A2A.
func (g *Gateway) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.serve(w, r, next)
	})
}

func (g *Gateway) serve(w http.ResponseWriter, r *http.Request, next http.Handler) {
	g.stripReserved(r.Header)

	p := r.URL.Path
	clean := strings.ToLower(path.Clean("/" + p))
	metadataPrefix := ProtectedResourcePrefix + g.cfg.BasePath + "/"
	switch {
	case clean == g.internal || strings.HasPrefix(clean, g.internal+"/"):
		g.serveInternal(w, r, clean)
	case p == wellKnownCard && g.def != nil:
		g.def.card.ServeHTTP(w, r)
	case strings.HasPrefix(p, metadataPrefix) && g.cfg.Challenge.ResourceMetadata:
		g.serveMetadataFor(w, r, strings.TrimPrefix(p, metadataPrefix))
	case strings.HasPrefix(p, g.cfg.BasePath+"/"):
		g.serveAgent(w, r, strings.TrimPrefix(p, g.cfg.BasePath+"/"), next)
	default:
		next.ServeHTTP(w, r)
	}
}

// serveInternal responde a las rutas internas, que solo se alcanzan tras la
// reescritura del borde. La única excepción son las estadísticas en loopback.
func (g *Gateway) serveInternal(w http.ResponseWriter, r *http.Request, clean string) {
	if clean == g.internal+"/_stats" && isLoopback(r.RemoteAddr) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"liveEntries": g.LiveEntries()})
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (g *Gateway) serveMetadataFor(w http.ResponseWriter, r *http.Request, id string) {
	a, ok := g.agents[id]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown agent")
		return
	}
	g.serveMetadata(w, r, a)
}

func (g *Gateway) serveAgent(w http.ResponseWriter, r *http.Request, rest string, next http.Handler) {
	id, sub, _ := strings.Cut(rest, "/")
	a, ok := g.agents[id]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown agent")
		return
	}
	switch sub {
	case "":
		g.serveRPC(w, r, a, next)
	case wellKnownCard[1:]:
		a.card.ServeHTTP(w, r)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

// stripReserved borra las cabeceras internas y las de identidad: solo pueden
// llegar al endpoint interno si las pone el gateway o KrakenD (design.md D4).
func (g *Gateway) stripReserved(h http.Header) {
	for k := range h {
		if strings.HasPrefix(strings.ToLower(k), "x-a2a-") {
			delete(h, k)
		}
	}
	for _, name := range g.cfg.Identity.Headers {
		h.Del(name)
		for k := range h {
			if strings.EqualFold(k, name) {
				delete(h, k)
			}
		}
	}
}

// rpcCall es una petición JSON-RPC que el borde ya ha validado.
type rpcCall struct {
	body    []byte
	id      json.RawMessage
	op      protocol.Operation
	version protocol.Version
	refs    protocol.RequestRefs
}

// rejection es una petición que el borde rechaza sin reenviarla.
type rejection struct {
	id      json.RawMessage
	op      string
	version protocol.Version
	err     *protocol.Error
}

func (g *Gateway) serveRPC(w http.ResponseWriter, r *http.Request, a *agent, next http.Handler) {
	start := time.Now()
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	c, rej := g.parseCall(r, a)
	if rej != nil {
		writeRPCError(w, rej.err.HTTPStatus, rej.id, rej.err)
		g.log.Info(fmt.Sprintf("[PLUGIN: a2a] agente=%s op=%s version=%s resultado=rechazado codigo=%d estado=%d "+
			"motivo=%q dur=%s", a.id, rej.op, rej.version, rej.err.Code, rej.err.HTTPStatus, rej.err.Detail,
			time.Since(start).Round(time.Millisecond)))
		return
	}

	e := &entry{agent: a, op: c.op, version: c.version, refs: c.refs, rpcID: c.id}
	g.registry.add(e)
	defer g.registry.remove(e)
	g.rewriteToInternal(r, e, c.body)

	rec := &recorder{w: w, g: g, e: e, hdr: http.Header{}, limit: g.cfg.Limits.MaxInspectBytes, ctx: r.Context()}
	next.ServeHTTP(rec, r)
	outcome := rec.finish()

	e.mu.Lock()
	principal := e.principal
	e.mu.Unlock()
	if principal == "" {
		principal = "-"
	}
	g.log.Info(fmt.Sprintf("[PLUGIN: a2a] agente=%s op=%s version=%s resultado=%s estado=%d principal=%s dur=%s",
		a.id, c.op, c.version, outcome, rec.sent, principal, time.Since(start).Round(time.Millisecond)))
}

// parseCall lee, valida y resuelve la petición JSON-RPC.
func (g *Gateway) parseCall(r *http.Request, a *agent) (*rpcCall, *rejection) {
	body, perr := protocol.ReadBody(r.Body, g.cfg.Limits.MaxBodyBytes)
	if perr != nil {
		return nil, &rejection{op: "-", err: perr}
	}
	req, perr := protocol.ParseRequest(body)
	if perr != nil {
		rej := &rejection{op: "-", err: perr}
		if req != nil {
			rej.id = req.ID
		}
		return nil, rej
	}
	version, perr := protocol.Negotiate(protocol.RequestedVersion(r.Header, r.URL.Query()), a.versions)
	if perr != nil {
		return nil, &rejection{id: req.ID, op: req.Method, err: perr}
	}
	op, perr := protocol.ResolveMethod(req.Method, version)
	if perr != nil {
		return nil, &rejection{id: req.ID, op: req.Method, version: version, err: perr}
	}
	refs, perr := g.checkOperation(r, a, op, version, req.Params)
	if perr != nil {
		return nil, &rejection{id: req.ID, op: op.String(), version: version, err: perr}
	}
	return &rpcCall{body: body, id: req.ID, op: op, version: version, refs: refs}, nil
}

// checkOperation comprueba que la operación está habilitada y que sus
// parámetros, incluidas las URLs de webhook, son aceptables.
func (g *Gateway) checkOperation(r *http.Request, a *agent, op protocol.Operation, v protocol.Version,
	params json.RawMessage) (protocol.RequestRefs, *protocol.Error) {
	if perr := g.checkEnabled(a, op); perr != nil {
		return protocol.RequestRefs{}, perr
	}
	refs, perr := protocol.ExtractRequest(op, v, params)
	if perr != nil {
		return refs, perr
	}
	return refs, g.checkPushURLs(r, a, refs.PushURLs)
}

func (g *Gateway) checkPushURLs(r *http.Request, a *agent, urls []string) *protocol.Error {
	if len(urls) == 0 {
		return nil
	}
	if !a.push {
		return protocol.NewPushNotificationNotSupported("configuración push con push deshabilitado")
	}
	for _, u := range urls {
		if err := g.guard.Check(r.Context(), u); err != nil {
			return protocol.NewInvalidParams("webhook URL not allowed", err.Error())
		}
	}
	return nil
}

// rewriteToInternal anota la petición con las cabeceras internas y la dirige
// a la ruta interna de su clase u operación.
func (g *Gateway) rewriteToInternal(r *http.Request, e *entry, body []byte) {
	r.Header.Set(HeaderRequestID, e.id)
	r.Header.Set(HeaderOperation, e.op.String())
	r.Header.Set(HeaderAgent, e.agent.id)
	r.Header.Set(HeaderVersion, string(e.version))

	target := g.cfg.InternalPrefix + "/" + e.agent.id + "/" + string(e.op.Class())
	if g.cfg.RouteGranularity == "operation" {
		target = g.cfg.InternalPrefix + "/" + e.agent.id + "/" + e.op.String()
	}
	r.URL.Path = target
	r.URL.RawPath = ""
	r.RequestURI = target
	if r.URL.RawQuery != "" {
		r.RequestURI += "?" + r.URL.RawQuery
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	r.Header.Del(headerContentLength)
}

// checkEnabled aplica las operaciones habilitadas del agente
// (spec a2a-protocol-gateway).
func (g *Gateway) checkEnabled(a *agent, op protocol.Operation) *protocol.Error {
	switch {
	case a.disabled[op]:
		return protocol.NewUnsupportedOperation(op.String() + " deshabilitada para el agente")
	case op.IsStreaming() && !a.streaming:
		return protocol.NewUnsupportedOperation("streaming deshabilitado para el agente")
	case op.IsPushConfig() && !a.push:
		return protocol.NewPushNotificationNotSupported("notificaciones push deshabilitadas para el agente")
	default:
		return nil
	}
}

func (g *Gateway) serveMetadata(w http.ResponseWriter, r *http.Request, a *agent) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	doc := map[string]interface{}{
		"resource":                 a.url,
		"authorization_servers":    g.cfg.Challenge.AuthorizationServers,
		"bearer_methods_supported": []string{"header"},
	}
	if len(g.cfg.Challenge.ScopesSupported) > 0 {
		doc["scopes_supported"] = g.cfg.Challenge.ScopesSupported
	}
	w.Header().Set(headerCacheControl, "public, max-age=300")
	writeJSON(w, http.StatusOK, doc)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	b, _ := json.Marshal(v)
	w.Header().Set(headerContentType, contentTypeJSON)
	w.Header().Set(headerContentLength, strconv.Itoa(len(b)))
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func writeRPCError(w http.ResponseWriter, status int, id json.RawMessage, e *protocol.Error) {
	if status == 0 {
		status = http.StatusOK
	}
	b := protocol.ErrorResponse(id, e)
	w.Header().Set(headerContentType, contentTypeJSON)
	w.Header().Set(headerCacheControl, "no-store")
	w.Header().Set(headerContentLength, strconv.Itoa(len(b)))
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func isLoopback(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
