// Copyright 2026 Los autores del plugin A2A para KrakenD.

package config

import "time"

// Valores por defecto (design.md D14).
const (
	DefaultBasePath         = "/a2a"
	DefaultInternalPrefix   = "/__a2a"
	DefaultRouteGranularity = GranularityClass

	DefaultMaxBodyBytes    = 4 << 20
	DefaultMaxInspectBytes = 8 << 20

	DefaultOwnershipStore = StoreMemory
	DefaultOwnershipTTL   = 168 * time.Hour
	DefaultMaxEntries     = 100000
	DefaultRedisTimeout   = 2 * time.Second

	DefaultChallengeScheme = "Bearer"

	DefaultCardTTL        = 5 * time.Minute
	DefaultCardMaxStale   = time.Hour
	DefaultCardMaxBytes   = 1 << 20
	DefaultCardHTTPMaxAge = 5 * time.Minute
)

// Valores admitidos en los campos enumerados.
const (
	GranularityClass     = "class"
	GranularityOperation = "operation"

	StoreMemory = "memory"
	StoreRedis  = "redis"

	PolicyDeny        = "deny"
	PolicyPassthrough = "passthrough"

	ListTasksOwnedContext = "owned_context"

	CardStatic   = "static"
	CardUpstream = "upstream"

	IdentityIssuer  = "issuer"
	IdentitySubject = "subject"
	IdentityScopes  = "scopes"
)

// DefaultIdentityHeaders son las cabeceras de identidad cuando no se indican.
func DefaultIdentityHeaders() map[string]string {
	return map[string]string{
		IdentityIssuer:  "X-A2A-Iss",
		IdentitySubject: "X-A2A-Sub",
		IdentityScopes:  "X-A2A-Scope",
	}
}

func (c *Config) applyDefaults() {
	c.BasePath = stringOr(c.BasePath, DefaultBasePath)
	c.InternalPrefix = stringOr(c.InternalPrefix, DefaultInternalPrefix)
	c.RouteGranularity = stringOr(c.RouteGranularity, DefaultRouteGranularity)

	setInt64(&c.Limits.MaxBodyBytes, DefaultMaxBodyBytes)
	setInt64(&c.Limits.MaxInspectBytes, DefaultMaxInspectBytes)
	c.applyIdentityDefaults()
	c.applyOwnershipDefaults()

	setBool(&c.PushNotifications.RequireHTTPS, true)
	setBool(&c.PushNotifications.DenyPrivateNetworks, true)

	c.Challenge.Scheme = stringOr(c.Challenge.Scheme, DefaultChallengeScheme)

	for i := range c.Agents {
		c.Agents[i].applyDefaults()
	}
}

func (c *Config) applyIdentityDefaults() {
	setBool(&c.Identity.Required, true)
	if c.Identity.Headers == nil {
		// Si el operador da sus propias cabeceras se usan tal cual, sin
		// mezclarlas con las de por defecto.
		c.Identity.Headers = DefaultIdentityHeaders()
	}
	if c.Identity.OwnerScope == nil {
		for _, k := range []string{IdentityIssuer, IdentitySubject} {
			if _, ok := c.Identity.Headers[k]; ok {
				c.Identity.OwnerScope = append(c.Identity.OwnerScope, k)
			}
		}
	}
}

func (c *Config) applyOwnershipDefaults() {
	o := &c.Ownership
	setBool(&o.Enabled, true)
	o.Store = stringOr(o.Store, DefaultOwnershipStore)
	setDuration(&o.TTL, DefaultOwnershipTTL)
	o.UnknownTask = stringOr(o.UnknownTask, PolicyDeny)
	o.ListTasks = stringOr(o.ListTasks, ListTasksOwnedContext)
	o.OnStoreError = stringOr(o.OnStoreError, PolicyDeny)
	if o.Memory.MaxEntries == 0 {
		o.Memory.MaxEntries = DefaultMaxEntries
	}
	setDuration(&o.Redis.Timeout, DefaultRedisTimeout)
}

func (a *Agent) applyDefaults() {
	if a.Versions == nil {
		a.Versions = []string{"1.0"}
	}
	setDuration(&a.Card.TTL, DefaultCardTTL)
	setDuration(&a.Card.MaxStale, DefaultCardMaxStale)
	setInt64(&a.Card.MaxBytes, DefaultCardMaxBytes)
	if a.Card.HTTPMaxAge == nil {
		d := Duration(DefaultCardHTTPMaxAge)
		a.Card.HTTPMaxAge = &d
	}
	setBool(&a.Capabilities.Streaming, true)
	// Las notificaciones push quedan deshabilitadas salvo que se activen:
	// configurar webhooks expone al agente a destinos elegidos por el cliente.
	setBool(&a.Capabilities.PushNotifications, false)
	// a.Ownership se queda como está: lo vacío hereda la política global.
}

func setInt64(p *int64, def int64) {
	if *p == 0 {
		*p = def
	}
}

func setBool(p **bool, def bool) {
	if *p == nil {
		v := def
		*p = &v
	}
}

func setDuration(d *Duration, def time.Duration) {
	if *d == 0 {
		*d = Duration(def)
	}
}
