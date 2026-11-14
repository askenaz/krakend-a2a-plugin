// Copyright 2026 Los autores del plugin A2A para KrakenD.

package protocol

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// Request es una petición JSON-RPC 2.0 individual ya validada.
type Request struct {
	// ID es el identificador tal como llegó: cadena, número o null. Se repite
	// sin cambios en las respuestas.
	ID json.RawMessage
	// Method es el nombre del método en el formato de la versión usada.
	Method string
	// Params es el objeto de parámetros, o nil si no venía.
	Params json.RawMessage
}

// msgInvalidRequest es el mensaje JSON-RPC de -32600.
const msgInvalidRequest = "Invalid Request"

// ReadBody lee el cuerpo con un límite. Si lo supera devuelve ErrBodyTooLarge
// sin leer el resto.
func ReadBody(r io.Reader, limit int64) ([]byte, *Error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		e := newError(CodeInvalidRequest, msgInvalidRequest, "no se pudo leer el cuerpo: "+err.Error())
		e.HTTPStatus = http.StatusBadRequest
		return nil, e
	}
	if int64(len(body)) > limit {
		return nil, ErrBodyTooLarge
	}
	return body, nil
}

// ParseRequest valida que el cuerpo sea una petición JSON-RPC 2.0 individual.
//
// Solo decodifica el primer nivel del objeto: el contenido de params no se
// interpreta aquí.
func ParseRequest(body []byte) (*Request, *Error) {
	if !json.Valid(body) {
		return nil, newError(CodeParseError, "Parse error", "el cuerpo no es JSON válido")
	}
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	switch {
	case len(trimmed) > 0 && trimmed[0] == '[':
		return nil, newError(CodeInvalidRequest, msgInvalidRequest+": batch requests are not supported",
			"lote JSON-RPC")
	case len(trimmed) == 0 || trimmed[0] != '{':
		return nil, newError(CodeInvalidRequest, msgInvalidRequest, "el cuerpo no es un objeto JSON")
	default:
		// Es un objeto: se decodifica a continuación.
	}

	env, err := decodeObject(body)
	if err != nil {
		return nil, newError(CodeInvalidRequest, msgInvalidRequest, "sobre: "+err.Error())
	}

	req := &Request{}

	// El id se valida primero para poder repetirlo en los demás errores.
	id, ok := env["id"]
	if !ok {
		return nil, newError(CodeInvalidRequest, msgInvalidRequest+": id is required",
			"falta id: A2A no define notificaciones del cliente")
	}
	switch jsonKind(id) {
	case "string", "number", "null":
		req.ID = id
	default:
		return nil, newError(CodeInvalidRequest, msgInvalidRequest+": id must be a string, a number or null",
			"id de tipo "+jsonKind(id))
	}

	fail := func(msg, detail string) (*Request, *Error) {
		e := newError(CodeInvalidRequest, msg, detail)
		return &Request{ID: req.ID}, e
	}

	var version string
	if raw, ok := env["jsonrpc"]; !ok || json.Unmarshal(raw, &version) != nil || version != "2.0" {
		return fail(`Invalid Request: jsonrpc must be "2.0"`, "jsonrpc ausente o distinto de 2.0")
	}

	raw, ok := env["method"]
	if !ok || jsonKind(raw) != "string" || json.Unmarshal(raw, &req.Method) != nil || req.Method == "" {
		return fail(msgInvalidRequest+": method must be a non-empty string", "method ausente, vacío o no es cadena")
	}

	if p, ok := env["params"]; ok && !isNull(p) {
		switch jsonKind(p) {
		case "object":
			req.Params = p
		case "array":
			return &Request{ID: req.ID, Method: req.Method},
				NewInvalidParams("params must be an object", "params posicionales no admitidos en A2A")
		default:
			return fail(msgInvalidRequest+": params must be an object", "params de tipo "+jsonKind(p))
		}
	}
	return req, nil
}
