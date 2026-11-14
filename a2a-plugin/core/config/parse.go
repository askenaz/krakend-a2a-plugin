// Copyright 2026 Los autores del plugin A2A para KrakenD.

package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FieldError es un error asociado a un campo de la configuración.
type FieldError struct {
	// Path es la ruta del campo, con los agentes identificados por su id:
	// agents[billing].ownership.unknown_task.
	Path    string
	Message string
}

// Error agrupa todos los errores encontrados, para que el operador los corrija
// de una vez.
type Error struct {
	Fields []FieldError
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("configuración A2A inválida: ")
	for i, f := range e.Fields {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(f.Path)
		b.WriteString(": ")
		b.WriteString(f.Message)
	}
	return b.String()
}

type errorList struct {
	fields []FieldError
}

func (l *errorList) add(path, format string, args ...interface{}) {
	if path == "" {
		path = "(raíz)"
	}
	l.fields = append(l.fields, FieldError{Path: path, Message: fmt.Sprintf(format, args...)})
}

func (l *errorList) err() error {
	if len(l.fields) == 0 {
		return nil
	}
	return &Error{Fields: l.fields}
}

// Parse lee el bloque A2A desde JSON.
func Parse(data []byte) (*Config, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("configuración A2A inválida: JSON mal formado: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("configuración A2A inválida: datos tras el objeto JSON")
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, &Error{Fields: []FieldError{{Path: "(raíz)", Message: "debe ser un objeto"}}}
	}
	return FromMap(m)
}

// FromMap lee el bloque A2A desde el mapa que KrakenD entrega al plugin.
//
// Primero comprueba que no haya campos desconocidos ni tipos incorrectos,
// después aplica los valores por defecto y por último valida las reglas que
// relacionan unos campos con otros. Devuelve todos los errores juntos.
func FromMap(m map[string]interface{}) (*Config, error) {
	var errs errorList
	if m == nil {
		errs.add("", "falta el bloque de configuración A2A")
		return nil, errs.err()
	}
	walk(m, reflect.TypeOf(Config{}), "", &errs)
	if err := errs.err(); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("configuración A2A inválida: %w", err)
	}
	cfg := &Config{}
	if err := json.Unmarshal(raw, cfg); err != nil {
		// No debería ocurrir: walk ya comprobó los tipos.
		return nil, fmt.Errorf("configuración A2A inválida: %w", err)
	}
	cfg.applyDefaults()
	cfg.validate(&errs)
	if err := errs.err(); err != nil {
		return nil, err
	}
	return cfg, nil
}

var (
	durationType = reflect.TypeOf(Duration(0))
	rawType      = reflect.TypeOf(json.RawMessage(nil))
)

// walk recorre el mapa genérico junto al tipo de destino. Así un campo
// desconocido o de tipo incorrecto se informa con su ruta completa, cosa que
// encoding/json no hace.
func walk(v interface{}, t reflect.Type, path string, errs *errorList) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if v == nil {
		errs.add(path, "no puede ser null; omite el campo para usar el valor por defecto")
		return
	}
	switch {
	case t == rawType:
		// JSON libre: lo valida validate().
	case t == durationType:
		walkDuration(v, path, errs)
	case t.Kind() == reflect.Struct:
		walkStruct(v, t, path, errs)
	case t.Kind() == reflect.Map:
		walkMap(v, t, path, errs)
	case t.Kind() == reflect.Slice:
		walkSlice(v, t, path, errs)
	default:
		walkScalar(v, t.Kind(), path, errs)
	}
}

func walkDuration(v interface{}, path string, errs *errorList) {
	s, ok := v.(string)
	if !ok {
		errs.add(path, "debe ser una duración como \"5m\", no %s", describe(v))
		return
	}
	if _, err := time.ParseDuration(s); err != nil {
		errs.add(path, "duración inválida %q", s)
	}
}

// asObject comprueba que el valor sea un objeto JSON.
func asObject(v interface{}, path string, errs *errorList) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	if !ok {
		errs.add(path, "debe ser un objeto, no %s", describe(v))
	}
	return m, ok
}

func walkStruct(v interface{}, t reflect.Type, path string, errs *errorList) {
	m, ok := asObject(v, path, errs)
	if !ok {
		return
	}
	fields := jsonFields(t)
	for _, k := range sortedKeys(m) {
		ft, known := fields[k]
		if !known {
			errs.add(join(path, k), "campo desconocido")
			continue
		}
		walk(m[k], ft, join(path, k), errs)
	}
}

func walkMap(v interface{}, t reflect.Type, path string, errs *errorList) {
	m, ok := asObject(v, path, errs)
	if !ok {
		return
	}
	for _, k := range sortedKeys(m) {
		walk(m[k], t.Elem(), join(path, k), errs)
	}
}

func walkSlice(v interface{}, t reflect.Type, path string, errs *errorList) {
	list, ok := v.([]interface{})
	if !ok {
		errs.add(path, "debe ser una lista, no %s", describe(v))
		return
	}
	for i, e := range list {
		walk(e, t.Elem(), elemPath(path, i, e), errs)
	}
}

func walkScalar(v interface{}, kind reflect.Kind, path string, errs *errorList) {
	var ok bool
	var want string
	switch kind {
	case reflect.String:
		_, ok = v.(string)
		want = "una cadena"
	case reflect.Bool:
		_, ok = v.(bool)
		want = "true o false"
	case reflect.Int, reflect.Int64:
		ok = isInteger(v)
		want = "un número entero"
	default:
		errs.add(path, "tipo no admitido")
		return
	}
	if !ok {
		errs.add(path, "debe ser %s, no %s", want, describe(v))
	}
}

func jsonFields(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = f.Type
	}
	return out
}

func isInteger(v interface{}) bool {
	switch n := v.(type) {
	case json.Number:
		_, err := strconv.ParseInt(string(n), 10, 64)
		return err == nil
	case float64:
		return n == math.Trunc(n) && math.Abs(n) < 1<<53
	case int, int64, int32:
		return true
	default:
		return false
	}
}

func describe(v interface{}) string {
	switch v.(type) {
	case string:
		return "una cadena"
	case bool:
		return "un booleano"
	case json.Number, float64, int, int64:
		return "un número"
	case []interface{}:
		return "una lista"
	case map[string]interface{}:
		return "un objeto"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// elemPath nombra un elemento de lista por su id, si lo tiene, para que los
// errores de un agente digan de qué agente se trata.
func elemPath(path string, i int, e interface{}) string {
	if m, ok := e.(map[string]interface{}); ok {
		if id, ok := m["id"].(string); ok && id != "" {
			return fmt.Sprintf("%s[%s]", path, id)
		}
	}
	return fmt.Sprintf("%s[%d]", path, i)
}
