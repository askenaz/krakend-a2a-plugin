// Copyright 2026 Los autores del plugin A2A para KrakenD.

// Package config define el bloque de configuración A2A, sus valores por
// defecto y su validación estricta.
//
// El bloque tiene el mismo esquema en KrakenD Community 2.13 y en Enterprise
// 3.0 (design.md D14). Los mensajes de error están en español porque los lee
// el operador al arrancar el gateway.
package config

import (
	"encoding/json"
	"fmt"
	"time"
)

// Config es el bloque A2A completo, ya con los valores por defecto aplicados.
type Config struct {
	PublicURL         string            `json:"public_url"`
	BasePath          string            `json:"base_path"`
	InternalPrefix    string            `json:"internal_prefix"`
	RouteGranularity  string            `json:"route_granularity"`
	DefaultAgent      string            `json:"default_agent"`
	Limits            Limits            `json:"limits"`
	Identity          Identity          `json:"identity"`
	Authorization     Authorization     `json:"authorization"`
	Ownership         Ownership         `json:"ownership"`
	PushNotifications PushNotifications `json:"push_notifications"`
	Challenge         Challenge         `json:"challenge"`
	Agents            []Agent           `json:"agents"`

	// Warnings recoge avisos que no impiden arrancar.
	Warnings []string `json:"-"`
}

// Limits acota la memoria que usa el gateway por petición.
type Limits struct {
	MaxBodyBytes    int64 `json:"max_body_bytes"`
	MaxInspectBytes int64 `json:"max_inspect_bytes"`
}

// Identity indica de qué cabeceras sale la identidad que KrakenD ya verificó
// (design.md D4).
type Identity struct {
	Required *bool `json:"required"`
	// Headers asocia un nombre lógico con la cabecera que lo transporta.
	// subject es obligatorio; scopes tiene un significado especial.
	Headers map[string]string `json:"headers"`
	// OwnerScope lista los nombres lógicos que forman la clave de propietario.
	OwnerScope []string `json:"owner_scope"`
}

// Authorization asocia operaciones A2A con los scopes que exigen.
type Authorization struct {
	OperationScopes map[string][]string `json:"operation_scopes"`
}

// Ownership configura el registro de propiedad de tareas y contextos.
type Ownership struct {
	Enabled      *bool       `json:"enabled"`
	Store        string      `json:"store"`
	TTL          Duration    `json:"ttl"`
	UnknownTask  string      `json:"unknown_task"`
	ListTasks    string      `json:"list_tasks"`
	OnStoreError string      `json:"on_store_error"`
	Memory       MemoryStore `json:"memory"`
	Redis        RedisStore  `json:"redis"`
}

// MemoryStore configura el almacén en memoria, válido solo para un nodo.
type MemoryStore struct {
	MaxEntries int `json:"max_entries"`
}

// RedisStore configura el almacén Redis. Las credenciales se leen de
// variables de entorno; nunca se escriben en la configuración.
type RedisStore struct {
	Address     string   `json:"address"`
	DB          int      `json:"db"`
	TLS         bool     `json:"tls"`
	UsernameEnv string   `json:"username_env"`
	PasswordEnv string   `json:"password_env"`
	Timeout     Duration `json:"timeout"`
}

// PushNotifications configura la guardia de webhooks (design.md D11).
type PushNotifications struct {
	RequireHTTPS        *bool    `json:"require_https"`
	DenyPrivateNetworks *bool    `json:"deny_private_networks"`
	AllowedHosts        []string `json:"allowed_hosts"`
}

// Challenge configura el desafío WWW-Authenticate y los metadatos de recurso
// protegido.
type Challenge struct {
	Scheme               string   `json:"scheme"`
	Realm                string   `json:"realm"`
	ResourceMetadata     bool     `json:"resource_metadata"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
}

// Agent es un agente publicado a través del gateway.
type Agent struct {
	ID                 string         `json:"id"`
	Versions           []string       `json:"versions"`
	Card               Card           `json:"card"`
	Security           *Security      `json:"security"`
	Capabilities       Capabilities   `json:"capabilities"`
	DisabledOperations []string       `json:"disabled_operations"`
	Ownership          AgentOwnership `json:"ownership"`
}

// Card indica de dónde sale la Agent Card y cómo se cachea.
type Card struct {
	Source     string          `json:"source"`
	Inline     json.RawMessage `json:"inline"`
	File       string          `json:"file"`
	URL        string          `json:"url"`
	TTL        Duration        `json:"ttl"`
	MaxStale   Duration        `json:"max_stale"`
	MaxBytes   int64           `json:"max_bytes"`
	HTTPMaxAge *Duration       `json:"http_max_age"`
	Signature  CardSignature   `json:"signature"`
}

// CardSignature activa la verificación de firma de una card remota.
type CardSignature struct {
	Verify   bool   `json:"verify"`
	JWKSFile string `json:"jwks_file"`
}

// Security sustituye los esquemas y requisitos de seguridad de la card por los
// del gateway. Se escriben en el formato de A2A 1.0.
type Security struct {
	Schemes      map[string]json.RawMessage `json:"schemes"`
	Requirements []json.RawMessage          `json:"requirements"`
}

// Capabilities habilita en el gateway capacidades que el agente declara.
type Capabilities struct {
	Streaming         *bool `json:"streaming"`
	PushNotifications *bool `json:"push_notifications"`
}

// AgentOwnership sobrescribe para un agente las políticas globales de
// propiedad. Un valor vacío hereda el global.
type AgentOwnership struct {
	Enabled     *bool  `json:"enabled"`
	UnknownTask string `json:"unknown_task"`
	ListTasks   string `json:"list_tasks"`
}

// Duration es una duración escrita como en Go, por ejemplo "5m" o "168h".
type Duration time.Duration

// D devuelve la duración como time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

func (d Duration) String() string { return time.Duration(d).String() }

// MarshalJSON escribe la duración como texto.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON lee una duración escrita como texto.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("debe ser una duración como \"5m\"")
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("duración inválida %q", s)
	}
	*d = Duration(v)
	return nil
}

// Agent devuelve el agente con ese identificador.
func (c *Config) Agent(id string) (*Agent, bool) {
	for i := range c.Agents {
		if c.Agents[i].ID == id {
			return &c.Agents[i], true
		}
	}
	return nil, false
}

// IdentityRequired indica si toda operación exige identidad.
func (c *Config) IdentityRequired() bool { return boolOr(c.Identity.Required, true) }

// StreamingEnabled indica si el gateway admite streaming para el agente.
func (a *Agent) StreamingEnabled() bool { return boolOr(a.Capabilities.Streaming, true) }

// PushEnabled indica si el gateway admite notificaciones push para el agente.
func (a *Agent) PushEnabled() bool { return boolOr(a.Capabilities.PushNotifications, false) }

// OwnershipEnabled indica si el control de propiedad se aplica al agente.
func (c *Config) OwnershipEnabled(a *Agent) bool {
	return boolOr(a.Ownership.Enabled, boolOr(c.Ownership.Enabled, true))
}

// UnknownTaskPolicy devuelve la política efectiva para tareas no registradas.
func (c *Config) UnknownTaskPolicy(a *Agent) string {
	return stringOr(a.Ownership.UnknownTask, c.Ownership.UnknownTask)
}

// ListTasksPolicy devuelve la política efectiva para ListTasks.
func (c *Config) ListTasksPolicy(a *Agent) string {
	return stringOr(a.Ownership.ListTasks, c.Ownership.ListTasks)
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func stringOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
