// Copyright 2026 Los autores del plugin A2A para KrakenD.

// Package gateway une las piezas del núcleo en los dos puntos de control del
// plugin (design.md D2 y D3):
//
//   - El borde, un envoltorio de http.Handler que se ejecuta antes del
//     enrutador de KrakenD. Sirve el descubrimiento, valida la petición A2A, la
//     encamina a la ruta interna y trata la respuesta.
//   - La autorización, que se ejecuta como modificador de petición después de
//     la autenticación nativa de KrakenD y decide con la identidad verificada.
//
// Ambos comparten una entrada de correlación por petición, identificada por la
// cabecera interna X-A2A-Request-Id.
package gateway

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"krakend-a2a.local/core/card"
	"krakend-a2a.local/core/config"
	"krakend-a2a.local/core/ownership"
	"krakend-a2a.local/core/protocol"
	"krakend-a2a.local/core/pushguard"
)

// Cabeceras internas del gateway. El borde las borra de toda petición
// entrante.
const (
	HeaderRequestID = "X-A2A-Request-Id"
	HeaderOperation = "X-A2A-Operation"
	HeaderAgent     = "X-A2A-Agent"
	HeaderVersion   = "X-A2A-Version"
)

// ProtectedResourcePrefix es el prefijo de los metadatos de recurso protegido
// (RFC 9728).
const ProtectedResourcePrefix = "/.well-known/oauth-protected-resource"

// Logger es el logger que KrakenD inyecta en los plugins.
type Logger interface {
	Debug(v ...interface{})
	Info(v ...interface{})
	Warning(v ...interface{})
	Error(v ...interface{})
	Critical(v ...interface{})
}

// logAgent encabeza los mensajes de log que se refieren a un agente.
const logAgent = "[PLUGIN: a2a] agente="

// nopLogger descarta los mensajes cuando no se inyecta un logger.
type nopLogger struct{}

// Debug descarta el mensaje.
func (nopLogger) Debug(...interface{}) {
	// Sin logger no hay a dónde escribir.
}

// Info descarta el mensaje.
func (nopLogger) Info(...interface{}) {
	// Sin logger no hay a dónde escribir.
}

// Warning descarta el mensaje.
func (nopLogger) Warning(...interface{}) {
	// Sin logger no hay a dónde escribir.
}

// Error descarta el mensaje.
func (nopLogger) Error(...interface{}) {
	// Sin logger no hay a dónde escribir.
}

// Critical descarta el mensaje.
func (nopLogger) Critical(...interface{}) {
	// Sin logger no hay a dónde escribir.
}

// Options ajusta el gateway. Los campos vacíos usan valores por defecto.
type Options struct {
	Logger Logger
	// Store sustituye al almacén de propiedad que indica la configuración.
	Store ownership.Store
	// StreamingUnavailable indica que la edición en ejecución no puede
	// entregar streaming (spec a2a-edition-packaging).
	StreamingUnavailable bool
	// HTTPClient se usa para obtener las Agent Cards remotas.
	HTTPClient *http.Client
	// PushGuard permite sustituir la resolución DNS de la guardia de webhooks.
	PushGuard pushguard.Config
	Getenv    func(string) string
}

// Gateway es el estado del plugin.
type Gateway struct {
	cfg       *config.Config
	log       Logger
	agents    map[string]*agent
	def       *agent
	store     ownership.Store
	guard     *pushguard.Guard
	registry  registry
	opScopes  map[protocol.Operation][]string
	streaming bool
	internal  string
}

type agent struct {
	id          string
	url         string
	versions    []protocol.Version
	disabled    map[protocol.Operation]bool
	streaming   bool
	push        bool
	ownership   bool
	unknownTask string
	listTasks   string
	card        *card.Manager
}

// New construye el gateway a partir de una configuración ya validada.
func New(cfg *config.Config, opts Options) (*Gateway, error) {
	g := &Gateway{
		cfg:       cfg,
		log:       opts.Logger,
		agents:    map[string]*agent{},
		opScopes:  map[protocol.Operation][]string{},
		streaming: !opts.StreamingUnavailable,
		internal:  strings.ToLower(cfg.InternalPrefix),
	}
	if g.log == nil {
		g.log = nopLogger{}
	}
	g.log = joinLogger{g.log}
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	for name, scopes := range cfg.Authorization.OperationScopes {
		op, _ := protocol.OperationByName(name)
		g.opScopes[op] = scopes
	}

	needStore := false
	for i := range cfg.Agents {
		ac := &cfg.Agents[i]
		a := &agent{
			id:          ac.ID,
			disabled:    map[protocol.Operation]bool{},
			streaming:   ac.StreamingEnabled() && g.streaming,
			push:        ac.PushEnabled(),
			ownership:   cfg.OwnershipEnabled(ac),
			unknownTask: cfg.UnknownTaskPolicy(ac),
			listTasks:   cfg.ListTasksPolicy(ac),
		}
		for _, v := range ac.Versions {
			a.versions = append(a.versions, protocol.Version(v))
		}
		for _, name := range ac.DisabledOperations {
			op, _ := protocol.OperationByName(name)
			a.disabled[op] = true
		}
		m, err := card.NewManager(cfg, ac, card.Options{
			StreamingAvailable: g.streaming,
			HTTPClient:         opts.HTTPClient,
			Logger:             g.log,
		})
		if err != nil {
			return nil, err
		}
		a.card = m
		a.url = card.GatewayURL(cfg, ac)
		g.agents[a.id] = a
		needStore = needStore || a.ownership
	}
	if cfg.DefaultAgent != "" {
		g.def = g.agents[cfg.DefaultAgent]
	}

	guardCfg := opts.PushGuard
	guardCfg.RequireHTTPS = *cfg.PushNotifications.RequireHTTPS
	guardCfg.DenyPrivateNetworks = *cfg.PushNotifications.DenyPrivateNetworks
	guardCfg.AllowedHosts = cfg.PushNotifications.AllowedHosts
	g.guard = pushguard.New(guardCfg)

	switch {
	case opts.Store != nil:
		g.store = opts.Store
	case !needStore:
		// Ningún agente controla la propiedad: no hace falta almacén.
	case cfg.Ownership.Store == config.StoreRedis:
		r, err := newRedisStore(cfg.Ownership.Redis, getenv)
		if err != nil {
			return nil, err
		}
		g.store = r
	default:
		g.store = ownership.NewMemory(cfg.Ownership.Memory.MaxEntries)
	}
	return g, nil
}

func newRedisStore(rc config.RedisStore, getenv func(string) string) (*ownership.Redis, error) {
	r := ownership.RedisConfig{Address: rc.Address, DB: rc.DB, TLS: rc.TLS, Timeout: rc.Timeout.D()}
	var err error
	if r.Username, err = secret(getenv, rc.UsernameEnv); err != nil {
		return nil, err
	}
	if r.Password, err = secret(getenv, rc.PasswordEnv); err != nil {
		return nil, err
	}
	return ownership.NewRedis(r), nil
}

func secret(getenv func(string) string, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	v := getenv(name)
	if v == "" {
		return "", fmt.Errorf("la variable de entorno %s está vacía o no existe", name)
	}
	return v, nil
}

// Start comprueba el almacén y precarga las Agent Cards en segundo plano.
func (g *Gateway) Start(ctx context.Context) error {
	if r, ok := g.store.(*ownership.Redis); ok {
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := r.Ping(pctx); err != nil {
			// No impide arrancar: la política on_store_error decide por petición.
			g.log.Error("[PLUGIN: a2a] el almacén de propiedad Redis no responde: ", err.Error())
		}
	}
	for _, a := range g.agents {
		a.card.Prefetch(ctx)
	}
	for _, w := range g.cfg.Warnings {
		g.log.Warning("[PLUGIN: a2a] aviso de configuración: ", w)
	}
	return nil
}

// Close libera los recursos.
func (g *Gateway) Close() error {
	if g.store != nil {
		return g.store.Close()
	}
	return nil
}

// LiveEntries devuelve las entradas de correlación activas.
func (g *Gateway) LiveEntries() int64 { return g.registry.live.Load() }

// authzRequired indica si la operación necesita el punto de autorización.
func (g *Gateway) authzRequired(a *agent, op protocol.Operation) bool {
	return g.cfg.IdentityRequired() || a.ownership || op == protocol.GetExtendedAgentCard || len(g.opScopes[op]) > 0
}

// challenge construye la cabecera WWW-Authenticate.
func (g *Gateway) challenge(a *agent) string {
	ch := g.cfg.Challenge
	parts := []string{}
	if ch.Realm != "" {
		parts = append(parts, `realm="`+ch.Realm+`"`)
	}
	if ch.ResourceMetadata && a != nil {
		parts = append(parts, `resource_metadata="`+g.metadataURL(a)+`"`)
	}
	if len(parts) == 0 {
		return ch.Scheme
	}
	return ch.Scheme + " " + strings.Join(parts, ", ")
}

func (g *Gateway) metadataPath(a *agent) string {
	return ProtectedResourcePrefix + g.cfg.BasePath + "/" + a.id
}

func (g *Gateway) metadataURL(a *agent) string {
	return strings.TrimSuffix(g.cfg.PublicURL, "/") + g.metadataPath(a)
}

// joinLogger concatena los argumentos antes de pasarlos al logger de KrakenD,
// que los separa con espacios.
type joinLogger struct{ l Logger }

func (j joinLogger) Debug(v ...interface{})    { j.l.Debug(fmt.Sprint(v...)) }
func (j joinLogger) Info(v ...interface{})     { j.l.Info(fmt.Sprint(v...)) }
func (j joinLogger) Warning(v ...interface{})  { j.l.Warning(fmt.Sprint(v...)) }
func (j joinLogger) Error(v ...interface{})    { j.l.Error(fmt.Sprint(v...)) }
func (j joinLogger) Critical(v ...interface{}) { j.l.Critical(fmt.Sprint(v...)) }
