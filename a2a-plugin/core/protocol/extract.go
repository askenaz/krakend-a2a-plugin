// Copyright 2026 Los autores del plugin A2A para KrakenD.

package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// RequestRefs son los identificadores de una petición que intervienen en el
// control de acceso (design.md D8). El resto de params no se interpreta.
type RequestRefs struct {
	// TaskID es la tarea existente a la que se refiere la petición.
	TaskID string
	// ContextID es el contexto que indica el cliente.
	ContextID string
	// PushURLs son las URLs de webhook que el agente podría usar.
	PushURLs []string
	// Tenant es el valor de tenant de la petición (solo 1.0).
	Tenant string
}

// Rutas de campos por operación y versión, verificadas contra la
// especificación y el SDK oficial de Go (a2a-go v2.8.0 y v0.3.15):
//
//	Operación                 1.0                                         0.3
//	SendMessage, Streaming    message.taskId, message.contextId            igual
//	  webhook                 configuration.taskPushNotificationConfig.url configuration.pushNotificationConfig.url
//	GetTask, CancelTask,      id                                           id
//	SubscribeToTask
//	ListTasks                 contextId                                    no existe
//	Create push config        taskId, url                                  taskId, pushNotificationConfig.url
//	Get, List, Delete push    taskId (id es la configuración)              id (es la tarea)
//
// Las URLs de webhook se buscan en las rutas de ambas versiones a la vez: si un
// agente aceptara el campo de la otra versión, la guardia de webhooks no debe
// quedar sin ver esa URL.
func ExtractRequest(op Operation, v Version, params json.RawMessage) (RequestRefs, *Error) {
	var refs RequestRefs

	if params == nil {
		if op == ListTasks || op == GetExtendedAgentCard {
			return refs, nil
		}
		return refs, NewInvalidParams("params is required", "faltan params en "+op.String())
	}
	p, err := decodeObject(params)
	if err != nil {
		return refs, NewInvalidParams("params must be a valid object", "params: "+err.Error())
	}
	if refs.Tenant, _, err = p.optString("tenant"); err != nil {
		return refs, invalid(err)
	}
	perr := extractByOperation(op, v, p, &refs)
	return refs, perr
}

func extractByOperation(op Operation, v Version, p object, refs *RequestRefs) *Error {
	var err error
	switch op {
	case SendMessage, SendStreamingMessage:
		return extractSend(p, refs)
	case GetTask, CancelTask, SubscribeToTask:
		refs.TaskID, err = requiredString(p, "id")
	case ListTasks:
		refs.ContextID, _, err = p.optString("contextId")
	case CreateTaskPushNotificationConfig:
		return extractCreatePush(v, p, refs)
	case GetTaskPushNotificationConfig, ListTaskPushNotificationConfigs, DeleteTaskPushNotificationConfig:
		refs.TaskID, err = requiredString(p, pushConfigTaskKey(v))
	case GetExtendedAgentCard:
		// Sin identificadores.
	default:
		return NewInternalError("operación sin reglas de extracción: " + op.String())
	}
	if err != nil {
		return invalid(err)
	}
	return nil
}

func extractSend(p object, refs *RequestRefs) *Error {
	msg, ok, err := p.optObject("message")
	if err != nil {
		return invalid(err)
	}
	if !ok {
		return NewInvalidParams("params.message is required", "falta params.message")
	}
	if refs.TaskID, _, err = msg.optString("taskId"); err != nil {
		return invalid(fmt.Errorf("message.%w", err))
	}
	if refs.ContextID, _, err = msg.optString("contextId"); err != nil {
		return invalid(fmt.Errorf("message.%w", err))
	}
	cfg, ok, err := p.optObject("configuration")
	if err != nil {
		return invalid(err)
	}
	if !ok {
		return nil
	}
	for _, key := range []string{"taskPushNotificationConfig", "pushNotificationConfig"} {
		if err := collectPushURL(cfg, key, refs); err != nil {
			return invalid(fmt.Errorf("configuration.%w", err))
		}
	}
	return nil
}

func extractCreatePush(v Version, p object, refs *RequestRefs) *Error {
	var err error
	if refs.TaskID, err = requiredString(p, "taskId"); err != nil {
		return invalid(err)
	}
	webhook, ok, err := p.optString("url")
	if err != nil {
		return invalid(err)
	}
	if ok {
		refs.PushURLs = append(refs.PushURLs, webhook)
	}
	if err := collectPushURL(p, "pushNotificationConfig", refs); err != nil {
		return invalid(err)
	}
	if len(refs.PushURLs) == 0 {
		want := "params.url"
		if v == V03 {
			want = "params.pushNotificationConfig.url"
		}
		return NewInvalidParams(want+" is required", "configuración push sin URL de webhook")
	}
	return nil
}

// pushConfigTaskKey es el campo que lleva el id de la tarea en las
// operaciones Get, List y Delete de configuración push.
func pushConfigTaskKey(v Version) string {
	if v == V03 {
		return "id"
	}
	return "taskId"
}

// collectPushURL añade la URL de un objeto de configuración push, si existe.
// Un objeto de configuración sin url es un error.
func collectPushURL(parent object, key string, refs *RequestRefs) error {
	cfg, ok, err := parent.optObject(key)
	if err != nil || !ok {
		return err
	}
	url, ok, err := cfg.optString("url")
	if err != nil {
		return fmt.Errorf("%s.%w", key, err)
	}
	if !ok || url == "" {
		return fmt.Errorf("%s.url es obligatorio", key)
	}
	refs.PushURLs = append(refs.PushURLs, url)
	return nil
}

func requiredString(o object, key string) (string, error) {
	s, ok, err := o.optString(key)
	if err != nil {
		return "", err
	}
	if !ok || s == "" {
		return "", fmt.Errorf("%s es obligatorio", key)
	}
	return s, nil
}

func invalid(err error) *Error {
	return NewInvalidParams("", "params."+err.Error())
}

// ---------------------------------------------------------------------------
// Respuestas del agente

// Tipos de resultado y de evento.
const (
	KindTask           = "task"
	KindMessage        = "message"
	KindStatusUpdate   = "statusUpdate"
	KindArtifactUpdate = "artifactUpdate"
)

// ResultRefs son los identificadores que revela un resultado o un evento.
type ResultRefs struct {
	Kind      string
	TaskID    string
	ContextID string
}

// Response es el sobre de una respuesta JSON-RPC del agente.
type Response struct {
	ID     json.RawMessage
	Result json.RawMessage
	Error  *ResponseError
}

// ResponseError es el error de una respuesta del agente.
type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ParseResponse lee el sobre de una respuesta JSON-RPC: la de una operación
// unaria o el contenido de una línea data: de un flujo SSE.
func ParseResponse(data []byte) (*Response, error) {
	env, err := decodeObject(data)
	if err != nil {
		return nil, fmt.Errorf("respuesta del agente: %w", err)
	}
	resp := &Response{ID: env["id"]}
	if raw, ok := env["error"]; ok && !isNull(raw) {
		var e ResponseError
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, fmt.Errorf("respuesta del agente: error mal formado: %w", err)
		}
		resp.Error = &e
		return resp, nil
	}
	raw, ok := env["result"]
	if !ok {
		return nil, errors.New("respuesta del agente sin result ni error")
	}
	resp.Result = raw
	return resp, nil
}

var wrapperKeys = []string{KindTask, KindMessage, KindStatusUpdate, KindArtifactUpdate}

var kindByV03 = map[string]string{
	"task":            KindTask,
	"message":         KindMessage,
	"status-update":   KindStatusUpdate,
	"artifact-update": KindArtifactUpdate,
}

// ExtractResult obtiene la tarea y el contexto de un resultado o un evento.
//
// Acepta las dos formas, sin depender de la versión:
//   - 1.0: un envoltorio con una sola clave task, message, statusUpdate o
//     artifactUpdate (resultado de SendMessage y eventos), o una tarea sin
//     envoltorio (GetTask, CancelTask).
//   - 0.3: el objeto sin envoltorio con el discriminador kind.
//
// El agente es de confianza, así que la tolerancia aquí no abre ningún hueco:
// si la forma no se reconoce se devuelve un error y no se registra propiedad.
func ExtractResult(op Operation, result json.RawMessage) (ResultRefs, error) {
	obj, err := decodeObject(result)
	if err != nil {
		return ResultRefs{}, fmt.Errorf("resultado: %w", err)
	}
	kind, inner, err := classify(obj)
	if err != nil {
		return ResultRefs{}, err
	}
	if !allowedKind(op, kind) {
		return ResultRefs{}, fmt.Errorf("resultado de tipo %s inesperado para %s", kind, op)
	}

	refs := ResultRefs{Kind: kind}
	idKey := "taskId"
	if kind == KindTask {
		idKey = "id"
	}
	if refs.TaskID, _, err = inner.optString(idKey); err != nil {
		return ResultRefs{}, fmt.Errorf("resultado %s: %w", kind, err)
	}
	if refs.ContextID, _, err = inner.optString("contextId"); err != nil {
		return ResultRefs{}, fmt.Errorf("resultado %s: %w", kind, err)
	}
	if kind != KindMessage && refs.TaskID == "" {
		return ResultRefs{}, fmt.Errorf("resultado %s sin %s", kind, idKey)
	}
	return refs, nil
}

func classify(obj object) (string, object, error) {
	var found []string
	for _, k := range wrapperKeys {
		if raw, ok := obj[k]; ok && jsonKind(raw) == "object" {
			found = append(found, k)
		}
	}
	if len(found) == 1 && len(obj) == 1 {
		inner, err := decodeObject(obj[found[0]])
		return found[0], inner, err
	}
	if len(found) > 1 {
		return "", nil, fmt.Errorf("resultado con varios tipos a la vez: %v", found)
	}

	if k, ok, err := obj.optString("kind"); err != nil {
		return "", nil, err
	} else if ok {
		kind, known := kindByV03[k]
		if !known {
			return "", nil, fmt.Errorf("kind desconocido %q", k)
		}
		return kind, obj, nil
	}

	// Sin envoltorio ni kind: una tarea 1.0 sin envoltorio, o un agente 0.3
	// que omite kind.
	_, hasStatus := obj["status"]
	_, hasID := obj["id"]
	_, hasTaskID := obj["taskId"]
	_, hasArtifact := obj["artifact"]
	_, hasMessageID := obj["messageId"]
	switch {
	case hasID && hasStatus:
		return KindTask, obj, nil
	case hasTaskID && hasArtifact:
		return KindArtifactUpdate, obj, nil
	case hasTaskID && hasStatus:
		return KindStatusUpdate, obj, nil
	case hasMessageID:
		return KindMessage, obj, nil
	default:
		return "", nil, errors.New("forma de resultado no reconocida")
	}
}

func allowedKind(op Operation, kind string) bool {
	switch op {
	case SendMessage:
		return kind == KindTask || kind == KindMessage
	case GetTask, CancelTask:
		return kind == KindTask
	case SendStreamingMessage, SubscribeToTask:
		return true
	default:
		return false
	}
}
