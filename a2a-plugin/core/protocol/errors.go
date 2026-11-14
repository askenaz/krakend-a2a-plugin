// Copyright 2026 Los autores del plugin A2A para KrakenD.

// Package protocol implementa la parte del binding JSON-RPC de A2A que el
// gateway necesita: el sobre de la petición, las operaciones de las versiones
// 1.0 y 0.3, la negociación de versión y la extracción de los pocos campos que
// intervienen en el control de acceso.
//
// El paquete no interpreta el contenido de los mensajes. Solo usa la
// biblioteca estándar (design.md D13).
//
// Los mensajes de error que recibe el cliente están en inglés, como los
// mensajes estándar de JSON-RPC. El campo Detail, en español, es solo para los
// registros del operador.
package protocol

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Códigos de error JSON-RPC estándar y de A2A.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603

	CodeTaskNotFound                   = -32001
	CodeTaskNotCancelable              = -32002
	CodePushNotificationNotSupported   = -32003
	CodeUnsupportedOperation           = -32004
	CodeContentTypeNotSupported        = -32005
	CodeInvalidAgentResponse           = -32006
	CodeExtendedAgentCardNotConfigured = -32007
	CodeExtensionSupportRequired       = -32008
	CodeVersionNotSupported            = -32009
)

// Error es un error de protocolo que el gateway devuelve sin contactar con el
// agente.
type Error struct {
	// Code es el código JSON-RPC.
	Code int
	// Message es el texto que recibe el cliente.
	Message string
	// HTTPStatus es el estado HTTP de la respuesta. Los errores JSON-RPC
	// viajan con 200; solo algunos casos, como el cuerpo demasiado grande,
	// usan otro estado (design.md D12).
	HTTPStatus int
	// Detail explica el error al operador. Nunca se envía al cliente.
	Detail string
}

func (e *Error) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("a2a %d %s: %s", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("a2a %d %s", e.Code, e.Message)
}

func newError(code int, message, detail string) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: http.StatusOK, Detail: detail}
}

// Errores con nombre propio que otros paquetes comparan o reutilizan.
var (
	// ErrBodyTooLarge se devuelve cuando el cuerpo supera el límite
	// configurado. Viaja con HTTP 413.
	ErrBodyTooLarge = &Error{
		Code: CodeInvalidRequest, Message: "Request too large",
		HTTPStatus: http.StatusRequestEntityTooLarge, Detail: "el cuerpo supera limits.max_body_bytes",
	}
)

// NewTaskNotFound construye el error que se usa también cuando el llamante no
// es propietario de la tarea, para no revelar su existencia (design.md D12).
func NewTaskNotFound(detail string) *Error {
	return newError(CodeTaskNotFound, "Task not found", detail)
}

// NewInvalidParams construye un error -32602 con un mensaje para el cliente.
func NewInvalidParams(clientMsg, detail string) *Error {
	msg := "Invalid params"
	if clientMsg != "" {
		msg += ": " + clientMsg
	}
	return newError(CodeInvalidParams, msg, detail)
}

// NewInternalError construye un error -32603.
func NewInternalError(detail string) *Error {
	return newError(CodeInternalError, "Internal error", detail)
}

// NewUnsupportedOperation construye un error -32004.
func NewUnsupportedOperation(detail string) *Error {
	return newError(CodeUnsupportedOperation, "Unsupported operation", detail)
}

// NewPushNotificationNotSupported construye un error -32003.
func NewPushNotificationNotSupported(detail string) *Error {
	return newError(CodePushNotificationNotSupported, "Push notifications are not supported", detail)
}

// ErrorResponse serializa la respuesta JSON-RPC 2.0 de un error. Un id vacío
// se envía como null.
func ErrorResponse(id json.RawMessage, e *Error) []byte {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	b, err := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Error: struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{e.Code, e.Message},
	})
	if err != nil {
		// Solo ocurre si id no es JSON válido, y ParseRequest lo impide.
		return []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"Internal error"}}`)
	}
	return b
}
