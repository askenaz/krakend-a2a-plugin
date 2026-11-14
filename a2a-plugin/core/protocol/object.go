// Copyright 2026 Los autores del plugin A2A para KrakenD.

package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// object es un objeto JSON decodificado un solo nivel, con las claves tal como
// llegan.
type object map[string]json.RawMessage

var errNotObject = errors.New("no es un objeto JSON")

// decodeObject decodifica un único nivel de un objeto JSON.
//
// Rechaza las claves repetidas, también las que solo difieren en mayúsculas.
// encoding/json, que usan el SDK oficial de A2A y muchos agentes en Go, asocia
// los campos sin distinguir mayúsculas y se queda con el último valor. Sin esta
// comprobación, {"method":"SendMessage","METHOD":"CancelTask"} se autorizaría
// como SendMessage en el gateway y se ejecutaría como CancelTask en el agente.
func decodeObject(raw json.RawMessage) (object, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errNotObject
	}
	out := object{}
	seen := map[string]string{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, errNotObject
		}
		fold := strings.ToLower(key)
		if prev, dup := seen[fold]; dup {
			if prev == key {
				return nil, fmt.Errorf("clave repetida %q", key)
			}
			return nil, fmt.Errorf("claves %q y %q solo difieren en mayúsculas", prev, key)
		}
		seen[fold] = key
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		out[key] = v
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return out, nil
}

// jsonKind devuelve el tipo JSON de un valor ya validado.
func jsonKind(raw json.RawMessage) string {
	raw = bytes.TrimLeft(raw, " \t\r\n")
	if len(raw) == 0 {
		return "vacío"
	}
	switch raw[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

func isNull(raw json.RawMessage) bool {
	return jsonKind(raw) == "null"
}

// optString lee un campo de texto opcional. Un campo presente con otro tipo es
// un error; null equivale a ausente.
func (o object) optString(key string) (string, bool, error) {
	raw, ok := o[key]
	if !ok || isNull(raw) {
		return "", false, nil
	}
	if jsonKind(raw) != "string" {
		return "", false, fmt.Errorf("%s debe ser una cadena", key)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false, err
	}
	return s, true, nil
}

// optObject lee un objeto opcional. null equivale a ausente.
func (o object) optObject(key string) (object, bool, error) {
	raw, ok := o[key]
	if !ok || isNull(raw) {
		return nil, false, nil
	}
	if jsonKind(raw) != "object" {
		return nil, false, fmt.Errorf("%s debe ser un objeto", key)
	}
	obj, err := decodeObject(raw)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", key, err)
	}
	return obj, true, nil
}
