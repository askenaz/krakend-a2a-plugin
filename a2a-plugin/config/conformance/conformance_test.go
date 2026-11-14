// Copyright 2026 Los autores del plugin A2A para KrakenD.

// Package conformance comprueba que el esquema JSON publicado, el validador del
// plugin, los ejemplos y la documentación cuentan lo mismo.
//
// Vive en un módulo aparte porque usa una biblioteca externa de JSON Schema que
// no debe entrar en el plugin (design.md D13).
package conformance

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"krakend-a2a.local/core/config"
)

const (
	schemaFile = "../schema/a2a-gateway.schema.json"
	schemaID   = "urn:krakend-a2a:config:a2a-gateway:v1"
	examples   = "../examples"
	docFile    = "../../docs/configuracion.md"
)

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	f, err := os.Open(schemaFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatalf("el esquema no es JSON válido: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(schemaID, doc); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile(schemaID)
	if err != nil {
		t.Fatalf("el esquema no compila: %v", err)
	}
	return sch
}

func rawSchema(t *testing.T) map[string]interface{} {
	t.Helper()
	b, err := os.ReadFile(schemaFile)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func instance(t *testing.T, raw []byte) interface{} {
	t.Helper()
	v, err := jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func files(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(examples, dir, "*.json"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no hay ejemplos en %s", dir)
	}
	sort.Strings(matches)
	return matches
}

// Los ejemplos válidos los aceptan el esquema y el plugin.
func TestValidExamples(t *testing.T) {
	sch := compileSchema(t)
	for _, f := range files(t, "valid") {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if err := sch.Validate(instance(t, raw)); err != nil {
				t.Errorf("el esquema rechaza un ejemplo válido: %v", err)
			}
			if _, err := config.Parse(raw); err != nil {
				t.Errorf("el plugin rechaza un ejemplo válido: %v", err)
			}
		})
	}
}

type invalidExample struct {
	Description string `json:"description"`
	Expect      struct {
		Path          string `json:"path"`
		Message       string `json:"message"`
		SchemaRejects bool   `json:"schema_rejects"`
	} `json:"expect"`
	Config json.RawMessage `json:"config"`
}

// Los ejemplos inválidos los rechaza el plugin con un error que nombra el
// campo. El esquema rechaza exactamente los que el fichero declara; el resto
// son reglas entre campos que solo comprueba el plugin.
func TestInvalidExamples(t *testing.T) {
	sch := compileSchema(t)
	for _, f := range files(t, "invalid") {
		t.Run(filepath.Base(f), func(t *testing.T) { checkInvalidExample(t, sch, f) })
	}
}

func checkInvalidExample(t *testing.T, sch *jsonschema.Schema, file string) {
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var ex invalidExample
	if err := json.Unmarshal(raw, &ex); err != nil {
		t.Fatalf("ejemplo mal formado: %v", err)
	}

	_, perr := config.Parse(ex.Config)
	var cfgErr *config.Error
	if !errors.As(perr, &cfgErr) {
		t.Fatalf("el plugin no lo rechaza con un error de campo: %v", perr)
	}
	if !hasFieldError(cfgErr, ex.Expect.Path, ex.Expect.Message) {
		t.Errorf("no hay error en %s con %q: %v", ex.Expect.Path, ex.Expect.Message, perr)
	}

	serr := sch.Validate(instance(t, ex.Config))
	if ex.Expect.SchemaRejects && serr == nil {
		t.Errorf("el esquema debería rechazarlo")
	}
	if !ex.Expect.SchemaRejects && serr != nil {
		t.Errorf("el ejemplo dice que el esquema no lo detecta, pero lo rechaza: %v", serr)
	}
}

func hasFieldError(cfgErr *config.Error, path, message string) bool {
	for _, fe := range cfgErr.Fields {
		if fe.Path == path && strings.Contains(fe.Message, message) {
			return true
		}
	}
	return false
}

// Cada campo del modelo de Go existe en el esquema y al revés.
func TestSchemaMatchesGoModel(t *testing.T) {
	root := rawSchema(t)
	compareNode(t, root, reflect.TypeOf(config.Config{}), root, "")
}

var rawType = reflect.TypeOf(json.RawMessage(nil))

func compareNode(t *testing.T, root map[string]interface{}, typ reflect.Type,
	node map[string]interface{}, path string) {
	t.Helper()
	node = resolve(t, root, node)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch {
	case typ == rawType:
		// JSON libre: el esquema no describe su contenido.
	case typ.Kind() == reflect.Struct:
		compareStruct(t, root, typ, node, path)
	case typ.Kind() == reflect.Slice:
		if items, ok := node["items"].(map[string]interface{}); ok {
			compareNode(t, root, typ.Elem(), items, path+"[]")
		}
	default:
		// Los tipos escalares no tienen estructura que comparar.
	}
}

func compareStruct(t *testing.T, root map[string]interface{}, typ reflect.Type,
	node map[string]interface{}, path string) {
	t.Helper()
	props, _ := node["properties"].(map[string]interface{})
	fields := goFields(typ)
	for name, ft := range fields {
		sub, ok := props[name].(map[string]interface{})
		if !ok {
			t.Errorf("%s: el campo %q del modelo de Go falta en el esquema", orRoot(path), name)
			continue
		}
		compareNode(t, root, ft, sub, join(path, name))
	}
	for name := range props {
		if _, ok := fields[name]; !ok {
			t.Errorf("%s: el esquema define %q, que no existe en el modelo de Go", orRoot(path), name)
		}
	}
}

func resolve(t *testing.T, root, node map[string]interface{}) map[string]interface{} {
	t.Helper()
	ref, ok := node["$ref"].(string)
	if !ok {
		return node
	}
	name := strings.TrimPrefix(ref, "#/$defs/")
	defs := root["$defs"].(map[string]interface{})
	target, ok := defs[name].(map[string]interface{})
	if !ok {
		t.Fatalf("referencia rota %s", ref)
	}
	return resolve(t, root, target)
}

func goFields(typ reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			out[name] = f.Type
		}
	}
	return out
}

// Los valores por defecto que publica el esquema coinciden con los que aplica
// el plugin.
func TestSchemaDefaultsMatchPlugin(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(examples, "valid", "minimal.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	a := &cfg.Agents[0]
	root := rawSchema(t)
	cases := []struct {
		pointer string
		got     interface{}
	}{
		{"properties/base_path", cfg.BasePath},
		{"properties/internal_prefix", cfg.InternalPrefix},
		{"properties/route_granularity", cfg.RouteGranularity},
		{"properties/limits/properties/max_body_bytes", float64(cfg.Limits.MaxBodyBytes)},
		{"properties/limits/properties/max_inspect_bytes", float64(cfg.Limits.MaxInspectBytes)},
		{"properties/identity/properties/required", cfg.IdentityRequired()},
		{"properties/ownership/properties/enabled", *cfg.Ownership.Enabled},
		{"properties/ownership/properties/store", cfg.Ownership.Store},
		{"properties/ownership/properties/ttl", cfg.Ownership.TTL.D()},
		{"properties/ownership/properties/unknown_task", cfg.Ownership.UnknownTask},
		{"properties/ownership/properties/list_tasks", cfg.Ownership.ListTasks},
		{"properties/ownership/properties/on_store_error", cfg.Ownership.OnStoreError},
		{"properties/ownership/properties/memory/properties/max_entries", float64(cfg.Ownership.Memory.MaxEntries)},
		{"properties/ownership/properties/redis/properties/timeout", cfg.Ownership.Redis.Timeout.D()},
		{"properties/push_notifications/properties/require_https", *cfg.PushNotifications.RequireHTTPS},
		{"properties/push_notifications/properties/deny_private_networks", *cfg.PushNotifications.DenyPrivateNetworks},
		{"properties/challenge/properties/scheme", cfg.Challenge.Scheme},
		{"properties/challenge/properties/resource_metadata", cfg.Challenge.ResourceMetadata},
		{"$defs/agent/properties/capabilities/properties/streaming", a.StreamingEnabled()},
		{"$defs/agent/properties/capabilities/properties/push_notifications", a.PushEnabled()},
		{"$defs/card/properties/ttl", a.Card.TTL.D()},
		{"$defs/card/properties/max_stale", a.Card.MaxStale.D()},
		{"$defs/card/properties/max_bytes", float64(a.Card.MaxBytes)},
		{"$defs/card/properties/http_max_age", a.Card.HTTPMaxAge.D()},
	}
	for _, c := range cases {
		node := lookup(t, root, c.pointer)
		def, ok := node["default"]
		if !ok {
			t.Errorf("%s: el esquema no publica valor por defecto", c.pointer)
			continue
		}
		if d, isDur := c.got.(time.Duration); isDur {
			parsed, err := time.ParseDuration(def.(string))
			if err != nil || parsed != d {
				t.Errorf("%s: el esquema dice %v y el plugin aplica %v", c.pointer, def, d)
			}
			continue
		}
		if def != c.got {
			t.Errorf("%s: el esquema dice %v y el plugin aplica %v", c.pointer, def, c.got)
		}
	}

	headers := lookup(t, root, "properties/identity/properties/headers")["default"].(map[string]interface{})
	for k, v := range config.DefaultIdentityHeaders() {
		if headers[k] != v {
			t.Errorf("identity.headers.%s: el esquema dice %v y el plugin aplica %v", k, headers[k], v)
		}
	}
}

func lookup(t *testing.T, root map[string]interface{}, pointer string) map[string]interface{} {
	t.Helper()
	node := root
	for _, part := range strings.Split(pointer, "/") {
		next, ok := node[part].(map[string]interface{})
		if !ok {
			t.Fatalf("ruta %s no existe en el esquema", pointer)
		}
		node = next
	}
	return node
}

// Cada campo del esquema aparece documentado en docs/configuracion.md
// (tarea 3.6).
func TestDocumentationCoversSchema(t *testing.T) {
	doc, err := os.ReadFile(docFile)
	if err != nil {
		t.Fatalf("falta la documentación de la configuración: %v", err)
	}
	text := string(doc)
	names := map[string]bool{}
	collectProperties(rawSchema(t), names)
	if len(names) < 50 {
		t.Fatalf("solo se encontraron %d campos en el esquema: la prueba no está mirando donde debe", len(names))
	}
	var missing []string
	for name := range names {
		if !strings.Contains(text, "`"+name+"`") {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("campos del esquema sin documentar en %s: %v", docFile, missing)
	}
}

func collectProperties(node interface{}, out map[string]bool) {
	switch n := node.(type) {
	case map[string]interface{}:
		props, _ := n["properties"].(map[string]interface{})
		for name := range props {
			out[name] = true
		}
		for _, v := range n {
			collectProperties(v, out)
		}
	case []interface{}:
		for _, v := range n {
			collectProperties(v, out)
		}
	default:
		// Los escalares no definen propiedades.
	}
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func orRoot(path string) string {
	if path == "" {
		return "(raíz)"
	}
	return path
}
