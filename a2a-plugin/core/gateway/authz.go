// Copyright 2026 Los autores del plugin A2A para KrakenD.

package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"krakend-a2a.local/core/config"
	"krakend-a2a.local/core/ownership"
	"krakend-a2a.local/core/protocol"
)

// entry correlaciona el borde y la autorización de una misma petición.
type entry struct {
	id      string
	agent   *agent
	op      protocol.Operation
	version protocol.Version
	refs    protocol.RequestRefs
	rpcID   json.RawMessage

	mu        sync.Mutex
	authzRan  bool
	verdict   *verdict
	owner     string // clave de propietario, resumen no reversible
	principal string // prefijo del resumen, para los registros
}

type registry struct {
	m    sync.Map
	live atomic.Int64
}

func (r *registry) add(e *entry) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	e.id = hex.EncodeToString(b)
	r.m.Store(e.id, e)
	r.live.Add(1)
}

func (r *registry) remove(e *entry) {
	if _, ok := r.m.LoadAndDelete(e.id); ok {
		r.live.Add(-1)
	}
}

func (r *registry) get(id string) *entry {
	if id == "" {
		return nil
	}
	v, ok := r.m.Load(id)
	if !ok {
		return nil
	}
	return v.(*entry)
}

// verdict es una denegación ya decidida.
type verdict struct {
	httpStatus int
	rpcCode    int
	message    string
	reason     string // para los registros
	challenge  bool
}

func deny401(reason string) *verdict {
	return &verdict{http.StatusUnauthorized, protocol.CodeInternalError, "Unauthorized", reason, true}
}

func deny403(reason string) *verdict {
	return &verdict{http.StatusForbidden, protocol.CodeInternalError, "Forbidden", reason, false}
}

func denyRPC(e *protocol.Error, reason string) *verdict {
	return &verdict{http.StatusOK, e.Code, e.Message, reason, false}
}

// DenyError es el error que el modificador devuelve a KrakenD para cortar el
// pipeline. El borde sustituye después la respuesta por la correcta.
type DenyError struct {
	Status int
	Reason string
}

func (e *DenyError) Error() string   { return "a2a: " + e.Reason }
func (e *DenyError) StatusCode() int { return e.Status }

// Authorize decide sobre una petición con la identidad que KrakenD ya
// verificó. headers son las cabeceras que ve el modificador de petición;
// expectedAgent, si no está vacío, es el agente del endpoint interno.
func (g *Gateway) Authorize(ctx context.Context, headers map[string][]string, expectedAgent string) error {
	e := g.registry.get(header(headers, HeaderRequestID))
	if e == nil {
		// La petición no pasó por el borde: fallo cerrado.
		g.log.Warning("[PLUGIN: a2a] autorización sin entrada de correlación: se deniega")
		return &DenyError{http.StatusForbidden, "request did not pass the A2A edge"}
	}

	var v *verdict
	if expectedAgent != "" && expectedAgent != e.agent.id {
		g.log.Critical("[PLUGIN: a2a] el endpoint interno del agente ", expectedAgent,
			" recibió una petición del agente ", e.agent.id, ": revisa la configuración")
		v = denyRPC(protocol.NewInternalError("agente cruzado"), "endpoint de otro agente")
	} else {
		v = g.decide(ctx, e, headers)
	}

	e.mu.Lock()
	e.authzRan = true
	e.verdict = v
	e.mu.Unlock()
	if v != nil {
		status := v.httpStatus
		if status == http.StatusOK {
			status = http.StatusForbidden
		}
		return &DenyError{status, v.reason}
	}
	return nil
}

type identity struct {
	subject string
	scopes  []string
	owner   string
	ownerOK bool
}

func (g *Gateway) identity(headers map[string][]string) identity {
	hc := g.cfg.Identity.Headers
	id := identity{subject: header(headers, hc[config.IdentitySubject])}
	if name, ok := hc[config.IdentityScopes]; ok {
		raw := strings.Join(headerValues(headers, name), " ")
		id.scopes = strings.FieldsFunc(raw, func(r rune) bool { return r == ' ' || r == ',' })
	}
	h := sha256.New()
	id.ownerOK = true
	for _, k := range g.cfg.Identity.OwnerScope {
		v := header(headers, hc[k])
		if v == "" {
			id.ownerOK = false
		}
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(v))
		h.Write([]byte{0})
	}
	id.owner = hex.EncodeToString(h.Sum(nil))
	return id
}

func (g *Gateway) decide(ctx context.Context, e *entry, headers map[string][]string) *verdict {
	id := g.identity(headers)
	e.setPrincipal(id)

	if g.authzRequired(e.agent, e.op) && id.subject == "" {
		return deny401("sin identidad")
	}
	if v := g.checkScopes(e.op, id.scopes); v != nil {
		return v
	}
	if !e.agent.ownership {
		return nil
	}
	if !id.ownerOK {
		return deny401("identidad incompleta para owner_scope")
	}
	return g.checkOwnership(ctx, e)
}

// setPrincipal anota el propietario y la etiqueta del principal en la entrada.
func (e *entry) setPrincipal(id identity) {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case id.ownerOK:
		e.owner = id.owner
		e.principal = id.owner[:12]
	case id.subject != "":
		sum := sha256.Sum256([]byte(id.subject))
		e.principal = hex.EncodeToString(sum[:6])
	default:
		// Petición anónima: no hay principal que anotar.
	}
}

func (g *Gateway) checkScopes(op protocol.Operation, scopes []string) *verdict {
	for _, need := range g.opScopes[op] {
		if !contains(scopes, need) {
			return deny403("falta el scope " + need)
		}
	}
	return nil
}

// checkOwnership aplica el control de propiedad según la operación.
func (g *Gateway) checkOwnership(ctx context.Context, e *entry) *verdict {
	switch e.op {
	case protocol.SendMessage, protocol.SendStreamingMessage:
		return g.checkSend(ctx, e)
	case protocol.GetTask, protocol.CancelTask, protocol.SubscribeToTask,
		protocol.CreateTaskPushNotificationConfig, protocol.GetTaskPushNotificationConfig,
		protocol.ListTaskPushNotificationConfigs, protocol.DeleteTaskPushNotificationConfig:
		return g.checkTask(ctx, e, e.refs.TaskID)
	case protocol.ListTasks:
		return g.checkListTasks(ctx, e)
	default:
		return nil
	}
}

func (g *Gateway) checkSend(ctx context.Context, e *entry) *verdict {
	if e.refs.TaskID != "" {
		if v := g.checkTask(ctx, e, e.refs.TaskID); v != nil {
			return v
		}
	}
	if e.refs.ContextID != "" {
		return g.checkContext(ctx, e, e.refs.ContextID, true)
	}
	return nil
}

func (g *Gateway) checkListTasks(ctx context.Context, e *entry) *verdict {
	switch e.agent.listTasks {
	case config.PolicyPassthrough:
		return nil
	case config.PolicyDeny:
		return denyRPC(protocol.NewUnsupportedOperation("ListTasks denegado por list_tasks"), "list_tasks=deny")
	default:
		// owned_context: solo se listan las tareas de un contexto propio.
	}
	if e.refs.ContextID == "" {
		return denyRPC(protocol.NewInvalidParams("params.contextId is required", "ListTasks sin contextId"),
			"ListTasks sin contexto")
	}
	return g.checkContext(ctx, e, e.refs.ContextID, false)
}

func (g *Gateway) checkTask(ctx context.Context, e *entry, taskID string) *verdict {
	return g.checkOwner(ctx, e, ownership.TaskKey(e.agent.id, taskID), false, "tarea")
}

// checkContext comprueba un contexto. Con newAllowed, un contexto no registrado
// se trata como nuevo; sin él, se aplica la política de tareas no registradas.
func (g *Gateway) checkContext(ctx context.Context, e *entry, contextID string, newAllowed bool) *verdict {
	return g.checkOwner(ctx, e, ownership.ContextKey(e.agent.id, contextID), newAllowed, "contexto")
}

func (g *Gateway) checkOwner(ctx context.Context, e *entry, key string, newAllowed bool, what string) *verdict {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	owner, found, err := g.store.Get(ctx, key)
	if err != nil {
		g.log.Error("[PLUGIN: a2a] almacén de propiedad: ", err.Error())
		if g.cfg.Ownership.OnStoreError == config.PolicyPassthrough {
			return nil
		}
		return denyRPC(protocol.NewInternalError("almacén de propiedad no disponible"), "almacén no disponible")
	}
	switch {
	case !found && newAllowed:
		return nil
	case !found && e.agent.unknownTask == config.PolicyPassthrough:
		return nil
	case !found:
		return denyRPC(protocol.NewTaskNotFound(what+" sin propietario registrado"), what+" sin propietario")
	case owner != e.owner:
		return denyRPC(protocol.NewTaskNotFound(what+" de otro principal"), what+" de otro principal")
	default:
		_ = g.store.Touch(ctx, key, g.cfg.Ownership.TTL.D())
		return nil
	}
}

// register registra como propietario de lo que revela un resultado al
// principal de la petición (spec a2a-access-control).
func (g *Gateway) register(ctx context.Context, e *entry, result json.RawMessage) {
	refs, err := protocol.ExtractResult(e.op, result)
	if err != nil {
		g.log.Warning(logAgent, e.agent.id, " no se registró la propiedad: ", err.Error())
		return
	}
	e.mu.Lock()
	owner := e.owner
	e.mu.Unlock()
	if owner == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ttl := g.cfg.Ownership.TTL.D()
	claim := func(key, what string) {
		cur, err := g.store.Claim(ctx, key, owner, ttl)
		switch {
		case err != nil:
			g.log.Error(logAgent, e.agent.id, " no se pudo registrar la propiedad de la ", what, ": ", err.Error())
		case cur != owner:
			g.log.Warning(logAgent, e.agent.id, " la ", what,
				" ya pertenece a otro principal; se conserva el propietario original")
		default:
			// La propiedad queda registrada.
		}
	}
	if refs.TaskID != "" {
		claim(ownership.TaskKey(e.agent.id, refs.TaskID), "tarea")
	}
	if refs.ContextID != "" {
		claim(ownership.ContextKey(e.agent.id, refs.ContextID), "contexto")
	}
}

func header(h map[string][]string, name string) string {
	if name == "" {
		return ""
	}
	if vs := h[http.CanonicalHeaderKey(name)]; len(vs) > 0 {
		return strings.TrimSpace(vs[0])
	}
	for k, vs := range h {
		if strings.EqualFold(k, name) && len(vs) > 0 {
			return strings.TrimSpace(vs[0])
		}
	}
	return ""
}

func headerValues(h map[string][]string, name string) []string {
	var out []string
	for k, vs := range h {
		if strings.EqualFold(k, name) {
			out = append(out, vs...)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
