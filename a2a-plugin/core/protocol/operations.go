// Copyright 2026 Los autores del plugin A2A para KrakenD.

package protocol

import "strings"

// Operation es una operación A2A, con independencia del nombre que use cada
// versión en el cable.
type Operation uint8

const (
	OpUnknown Operation = iota
	SendMessage
	SendStreamingMessage
	GetTask
	ListTasks
	CancelTask
	SubscribeToTask
	CreateTaskPushNotificationConfig
	GetTaskPushNotificationConfig
	ListTaskPushNotificationConfigs
	DeleteTaskPushNotificationConfig
	GetExtendedAgentCard
)

// Operations enumera las once operaciones en orden estable.
var Operations = []Operation{
	SendMessage, SendStreamingMessage, GetTask, ListTasks, CancelTask, SubscribeToTask,
	CreateTaskPushNotificationConfig, GetTaskPushNotificationConfig,
	ListTaskPushNotificationConfigs, DeleteTaskPushNotificationConfig,
	GetExtendedAgentCard,
}

// Class agrupa las operaciones por el tipo de endpoint interno que necesitan
// (design.md D5).
type Class string

const (
	ClassUnary  Class = "unary"
	ClassStream Class = "stream"
)

type opInfo struct {
	canonical string // nombre en 1.0, que es también el nombre canónico
	v03       string // nombre en 0.3; vacío si no existía
}

var opTable = map[Operation]opInfo{
	SendMessage:                      {"SendMessage", "message/send"},
	SendStreamingMessage:             {"SendStreamingMessage", "message/stream"},
	GetTask:                          {"GetTask", "tasks/get"},
	ListTasks:                        {"ListTasks", ""},
	CancelTask:                       {"CancelTask", "tasks/cancel"},
	SubscribeToTask:                  {"SubscribeToTask", "tasks/resubscribe"},
	CreateTaskPushNotificationConfig: {"CreateTaskPushNotificationConfig", "tasks/pushNotificationConfig/set"},
	GetTaskPushNotificationConfig:    {"GetTaskPushNotificationConfig", "tasks/pushNotificationConfig/get"},
	ListTaskPushNotificationConfigs:  {"ListTaskPushNotificationConfigs", "tasks/pushNotificationConfig/list"},
	DeleteTaskPushNotificationConfig: {"DeleteTaskPushNotificationConfig", "tasks/pushNotificationConfig/delete"},
	GetExtendedAgentCard:             {"GetExtendedAgentCard", "agent/getAuthenticatedExtendedCard"},
}

// byWireName indexa los nombres de cable de cada versión.
var byWireName = func() map[Version]map[string]Operation {
	m := map[Version]map[string]Operation{V10: {}, V03: {}}
	for op, info := range opTable {
		m[V10][info.canonical] = op
		if info.v03 != "" {
			m[V03][info.v03] = op
		}
	}
	return m
}()

// String devuelve el nombre canónico, que coincide con el de la versión 1.0.
func (o Operation) String() string {
	if info, ok := opTable[o]; ok {
		return info.canonical
	}
	return "Unknown"
}

// WireName devuelve el nombre del método en una versión concreta.
func (o Operation) WireName(v Version) (string, bool) {
	info, ok := opTable[o]
	if !ok {
		return "", false
	}
	switch v {
	case V10:
		return info.canonical, true
	case V03:
		return info.v03, info.v03 != ""
	default:
		return "", false
	}
}

// IsStreaming indica si la respuesta es un flujo SSE.
func (o Operation) IsStreaming() bool {
	return o == SendStreamingMessage || o == SubscribeToTask
}

// IsPushConfig indica si es una de las cuatro operaciones de configuración de
// notificaciones push.
func (o Operation) IsPushConfig() bool {
	switch o {
	case CreateTaskPushNotificationConfig, GetTaskPushNotificationConfig,
		ListTaskPushNotificationConfigs, DeleteTaskPushNotificationConfig:
		return true
	default:
		return false
	}
}

// MayCreateTask indica si la respuesta puede revelar una tarea o un contexto
// nuevos (design.md D10).
func (o Operation) MayCreateTask() bool {
	return o == SendMessage || o == SendStreamingMessage
}

// Class devuelve la clase de endpoint interno.
func (o Operation) Class() Class {
	if o.IsStreaming() {
		return ClassStream
	}
	return ClassUnary
}

// OperationByName busca una operación por su nombre canónico.
func OperationByName(canonical string) (Operation, bool) {
	op, ok := byWireName[V10][canonical]
	return op, ok
}

// ResolveMethod identifica la operación de un método en la versión ya
// negociada. Un nombre válido solo en la otra versión también es -32601.
func ResolveMethod(method string, v Version) (Operation, *Error) {
	if op, ok := byWireName[v][method]; ok {
		return op, nil
	}
	for other, names := range byWireName {
		if other == v {
			continue
		}
		if _, ok := names[method]; ok {
			return OpUnknown, newError(CodeMethodNotFound, "Method not found",
				"el método "+quote(method)+" pertenece a la versión "+string(other)+", no a la "+string(v))
		}
	}
	return OpUnknown, newError(CodeMethodNotFound, "Method not found", "método desconocido "+quote(method))
}

func quote(s string) string {
	if len(s) > 64 {
		s = s[:64] + "…"
	}
	return `"` + strings.ReplaceAll(s, `"`, `'`) + `"`
}
