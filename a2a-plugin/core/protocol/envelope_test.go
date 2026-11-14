// Copyright 2026 Los autores del plugin A2A para KrakenD.

package protocol

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestParseRequestValid(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		id     string
		method string
		params bool
	}{
		{"id de cadena",
			`{"jsonrpc":"2.0","id":"req-1","method":"SendMessage","params":{"message":{}}}`, `"req-1"`, "SendMessage", true},
		{"id numérico", `{"jsonrpc":"2.0","id":7,"method":"GetTask","params":{"id":"t"}}`, `7`, "GetTask", true},
		{"id nulo", `{"jsonrpc":"2.0","id":null,"method":"GetTask","params":{"id":"t"}}`, `null`, "GetTask", true},
		{"sin params", `{"jsonrpc":"2.0","id":1,"method":"GetExtendedAgentCard"}`, `1`, "GetExtendedAgentCard", false},
		{"params nulo equivale a ausente",
			`{"jsonrpc":"2.0","id":1,"method":"ListTasks","params":null}`, `1`, "ListTasks", false},
		{"espacios alrededor",
			" \n\t{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tasks/get\",\"params\":{\"id\":\"t\"}}\n", `1`, "tasks/get", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, err := ParseRequest([]byte(c.body))
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if string(req.ID) != c.id || req.Method != c.method || (req.Params != nil) != c.params {
				t.Fatalf("obtenido id=%s method=%q params=%v", req.ID, req.Method, req.Params != nil)
			}
		})
	}
}

func TestParseRequestErrors(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		code   int
		echoID string // id que debe repetirse en la respuesta; vacío = null
	}{
		{"JSON mal formado", `{"jsonrpc":"2.0",`, CodeParseError, ""},
		{"cuerpo vacío", ``, CodeParseError, ""},
		{"texto", `esto no es json`, CodeParseError, ""},
		{"datos tras el objeto", `{"jsonrpc":"2.0","id":1,"method":"GetTask"} {}`, CodeParseError, ""},
		{"lote JSON-RPC", `[{"jsonrpc":"2.0","id":1,"method":"GetTask"}]`, CodeInvalidRequest, ""},
		{"lote vacío", `[]`, CodeInvalidRequest, ""},
		{"escalar", `42`, CodeInvalidRequest, ""},
		{"sin id", `{"jsonrpc":"2.0","method":"GetTask","params":{"id":"t"}}`, CodeInvalidRequest, ""},
		{"id objeto", `{"jsonrpc":"2.0","id":{},"method":"GetTask"}`, CodeInvalidRequest, ""},
		{"id booleano", `{"jsonrpc":"2.0","id":true,"method":"GetTask"}`, CodeInvalidRequest, ""},
		{"sin jsonrpc", `{"id":1,"method":"GetTask"}`, CodeInvalidRequest, "1"},
		{"jsonrpc 1.0", `{"jsonrpc":"1.0","id":1,"method":"GetTask"}`, CodeInvalidRequest, "1"},
		{"jsonrpc numérico", `{"jsonrpc":2.0,"id":1,"method":"GetTask"}`, CodeInvalidRequest, "1"},
		{"method no es cadena", `{"jsonrpc":"2.0","id":"a","method":5}`, CodeInvalidRequest, `"a"`},
		{"method vacío", `{"jsonrpc":"2.0","id":"a","method":""}`, CodeInvalidRequest, `"a"`},
		{"sin method", `{"jsonrpc":"2.0","id":"a"}`, CodeInvalidRequest, `"a"`},
		{"params escalar", `{"jsonrpc":"2.0","id":1,"method":"GetTask","params":"t"}`, CodeInvalidRequest, "1"},
		{"params posicionales", `{"jsonrpc":"2.0","id":1,"method":"GetTask","params":["t"]}`, CodeInvalidParams, "1"},
		{"clave repetida", `{"jsonrpc":"2.0","id":1,"method":"SendMessage","method":"CancelTask"}`, CodeInvalidRequest, ""},
		{"claves que solo difieren en mayúsculas",
			`{"jsonrpc":"2.0","id":1,"method":"SendMessage","METHOD":"CancelTask"}`, CodeInvalidRequest, ""},
		{"clave con escape unicode repetida",
			esc(`{"jsonrpc":"2.0","id":1,"method":"SendMessage","meth%u006fd":"CancelTask"}`), CodeInvalidRequest, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { checkParseError(t, c.body, c.code, c.echoID) })
	}
}

// checkParseError comprueba el error de ParseRequest y el id que se repite.
func checkParseError(t *testing.T, body string, code int, echoID string) {
	t.Helper()
	req, err := ParseRequest([]byte(body))
	if err == nil {
		t.Fatalf("se esperaba el error %d", code)
	}
	if err.Code != code {
		t.Fatalf("código %d, se esperaba %d (%v)", err.Code, code, err)
	}
	if err.HTTPStatus != http.StatusOK {
		t.Fatalf("estado HTTP %d, se esperaba 200", err.HTTPStatus)
	}
	var id json.RawMessage
	if req != nil {
		id = req.ID
	}
	if string(id) != echoID && !(echoID == "" && len(id) == 0) {
		t.Fatalf("id a repetir %q, se esperaba %q", id, echoID)
	}
}

func TestReadBodyLimit(t *testing.T) {
	const limit = 16
	if _, err := ReadBody(strings.NewReader(strings.Repeat("a", limit)), limit); err != nil {
		t.Fatalf("un cuerpo igual al límite debe aceptarse: %v", err)
	}
	_, err := ReadBody(strings.NewReader(strings.Repeat("a", limit+1)), limit)
	if err != ErrBodyTooLarge {
		t.Fatalf("se esperaba ErrBodyTooLarge, obtenido %v", err)
	}
	if err.HTTPStatus != http.StatusRequestEntityTooLarge {
		t.Fatalf("estado HTTP %d, se esperaba 413", err.HTTPStatus)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("conexión cortada") }

func TestReadBodyReadError(t *testing.T) {
	_, err := ReadBody(failingReader{}, 1024)
	if err == nil || err.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("un fallo de lectura debe ser 400: %v", err)
	}
}

func TestErrorResponse(t *testing.T) {
	got := string(ErrorResponse(json.RawMessage(`"r-9"`), NewTaskNotFound("ajena")))
	want := `{"jsonrpc":"2.0","id":"r-9","error":{"code":-32001,"message":"Task not found"}}`
	if got != want {
		t.Fatalf("\nobtenido %s\nesperado %s", got, want)
	}
	if strings.Contains(got, "ajena") {
		t.Fatal("el detalle para el operador no debe llegar al cliente")
	}
	got = string(ErrorResponse(nil, NewInternalError("x")))
	if !strings.Contains(got, `"id":null`) {
		t.Fatalf("un id ausente debe enviarse como null: %s", got)
	}
}

// esc convierte %u en la secuencia de escape JSON, para no escribirla tal cual
// en el fichero fuente.
func esc(s string) string { return strings.ReplaceAll(s, "%u", "\\"+"u") }
