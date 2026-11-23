// Copyright 2026 Los autores del plugin A2A para KrakenD.

// Plugin A2A para KrakenD.
//
// Un único código fuente que se compila para dos objetivos (design.md D1):
//
//   - KrakenD Community 2.13, con la imagen krakend/builder de su versión.
//   - KrakenD Enterprise 3.0.4 o superior, con krakend/builder-ee del mismo
//     número de versión.
//
// Las versiones con las que se compila y verifica están en versions.env.
//
// Exporta dos símbolos que existen en ambas ediciones:
//
//   - HandlerRegisterer registra el plugin de servidor "a2a-gateway": el borde
//     A2A, que se configura con el bloque descrito en docs/configuracion.md.
//   - ModifierRegisterer registra el modificador de petición "a2a-authz": la
//     autorización A2A, que se declara en cada endpoint interno.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"

	"krakend-a2a.local/core/config"
	"krakend-a2a.local/core/gateway"
)

// Version es la versión del plugin. La sobrescribe la compilación con
// -ldflags "-X main.Version=...".
var Version = "0.1.0-dev"

const (
	serverName   = "a2a-gateway"
	modifierName = "a2a-authz"
)

// Símbolos que busca el cargador de plugins de KrakenD.
var (
	HandlerRegisterer  = serverRegisterer(serverName)
	ModifierRegisterer = modifierRegisterer(modifierName)
)

func main() {
	// Sin efecto: KrakenD carga el plugin con plugin.Open y solo usa los
	// símbolos exportados.
}

// current es el gateway activo. El borde lo crea; el modificador lo consulta
// en cada petición, porque KrakenD puede crear los modificadores antes que el
// plugin de servidor.
var current atomic.Pointer[gateway.Gateway]

// exit permite a las pruebas sustituir la salida del proceso.
var exit = os.Exit

// ---------------------------------------------------------------------------
// Plugin de servidor

type serverRegisterer string

func (r serverRegisterer) RegisterLogger(v interface{}) { setLogger(v) }

func (r serverRegisterer) RegisterHandlers(f func(
	name string,
	handler func(context.Context, map[string]interface{}, http.Handler) (http.Handler, error),
)) {
	f(string(r), r.factory)
}

func (r serverRegisterer) factory(ctx context.Context, extra map[string]interface{},
	next http.Handler) (http.Handler, error) {
	g, err := newGateway(string(r), extra, gateway.Options{Logger: log()})
	if err != nil {
		// Una configuración inválida impide arrancar (spec a2a-edition-packaging).
		log().Critical("[PLUGIN: a2a] ", err.Error())
		log().Critical("[PLUGIN: a2a] KrakenD no arranca hasta que se corrija la configuración A2A")
		exit(1)
		return next, err
	}
	if old := current.Swap(g); old != nil {
		_ = old.Close()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_ = g.Start(ctx)
	log().Info(fmt.Sprintf("[PLUGIN: a2a] versión %s cargada con %d agente(s)", Version, len(cfgAgents(g))))
	return g.Handler(next), nil
}

func newGateway(name string, extra map[string]interface{}, opts gateway.Options) (*gateway.Gateway, error) {
	raw, form := pluginConfig(name, extra)
	log().Debug("[PLUGIN: a2a] configuración recibida en forma ", form)
	cfg, err := config.FromMap(raw)
	if err != nil {
		return nil, err
	}
	g, err := gateway.New(cfg, opts)
	if err != nil {
		return nil, fmt.Errorf("configuración A2A inválida: %w", err)
	}
	agentsOf.Store(g, agentIDs(cfg))
	return g, nil
}

var agentsOf sync.Map // *gateway.Gateway -> []string

func agentIDs(cfg *config.Config) []string {
	ids := make([]string, 0, len(cfg.Agents))
	for _, a := range cfg.Agents {
		ids = append(ids, a.ID)
	}
	sort.Strings(ids)
	return ids
}

func cfgAgents(g *gateway.Gateway) []string {
	if v, ok := agentsOf.Load(g); ok {
		return v.([]string)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Modificador de petición

type modifierRegisterer string

func (r modifierRegisterer) RegisterLogger(v interface{}) { setLogger(v) }

func (r modifierRegisterer) RegisterModifiers(f func(
	name string,
	factoryFunc func(map[string]interface{}) func(interface{}) (interface{}, error),
	appliesToRequest bool,
	appliesToResponse bool,
)) {
	f(string(r), r.factory, true, false)
}

// headerReader es lo único que el modificador necesita del RequestWrapper de
// KrakenD, que es distinto en cada edición.
type headerReader interface {
	Headers() map[string][]string
}

type contextReader interface {
	Context() context.Context
}

// errNotReady se devuelve si el borde A2A aún no se ha creado.
var errNotReady = &gateway.DenyError{
	Status: http.StatusServiceUnavailable,
	Reason: "el borde A2A no está inicializado",
}

func (r modifierRegisterer) factory(extra map[string]interface{}) func(interface{}) (interface{}, error) {
	cfg, _ := pluginConfig(string(r), extra)
	expected, err := modifierAgent(cfg)
	if err != nil {
		log().Critical("[PLUGIN: a2a] configuración de ", string(r), ": ", err.Error())
		exit(1)
	}
	return func(input interface{}) (interface{}, error) {
		req, ok := input.(headerReader)
		if !ok {
			return nil, &gateway.DenyError{Status: http.StatusInternalServerError, Reason: fmt.Sprintf("tipo de petición "+
				"inesperado %T", input)}
		}
		g := current.Load()
		if g == nil {
			// Fallo cerrado: sin borde no hay correlación.
			return nil, errNotReady
		}
		ctx := context.Background()
		if c, ok := input.(contextReader); ok && c.Context() != nil {
			ctx = c.Context()
		}
		if err := g.Authorize(ctx, req.Headers(), expected); err != nil {
			return nil, err
		}
		return input, nil
	}
}

// modifierAgent lee la única opción del modificador: el agente al que
// pertenece el endpoint interno.
func modifierAgent(cfg map[string]interface{}) (string, error) {
	var agent string
	for k, v := range cfg {
		switch k {
		case "agent":
			s, ok := v.(string)
			if !ok {
				return "", errors.New("agent debe ser una cadena")
			}
			agent = s
		default:
			return "", fmt.Errorf("campo desconocido %q; solo se admite agent", k)
		}
	}
	return agent, nil
}

// ---------------------------------------------------------------------------
// Forma de la configuración (supuesto S5, validado en la fase 0)

// pluginConfig extrae la configuración del plugin del mapa que entrega KrakenD:
//
//   - Community 2.x entrega el mapa del espacio de nombres, con la
//     configuración bajo la clave del nombre del plugin y la lista "name".
//   - Enterprise 3.x entrega directamente el contenido de "config".
func pluginConfig(name string, extra map[string]interface{}) (map[string]interface{}, string) {
	if extra == nil {
		return map[string]interface{}{}, "vacía"
	}
	if v, ok := extra[name].(map[string]interface{}); ok {
		return v, "Community 2.x"
	}
	if _, ok := extra["name"].([]interface{}); ok {
		// Forma 2.x sin configuración para este plugin.
		return map[string]interface{}{}, "Community 2.x sin configuración"
	}
	return extra, "Enterprise 3.x"
}
