// Copyright 2026 Los autores del plugin A2A para KrakenD.

package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"krakend-a2a.local/core/protocol"
)

var (
	agentIDRe     = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	pathRe        = regexp.MustCompile(`^(/[A-Za-z0-9._~-]+)+$`)
	headerTokenRe = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")
	envNameRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	scopeTokenRe  = regexp.MustCompile(`^[\x21\x23-\x5B\x5D-\x7E]+$`)
	hostLabel     = `[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?`
	hostPatternRe = regexp.MustCompile(`^(\*\.)?` + hostLabel + `(\.` + hostLabel + `)*$`)
)

// Cabeceras que no pueden usarse para transportar identidad: el gateway borra
// las cabeceras de identidad de toda petición entrante (design.md D4), así que
// usar una de estas rompería la autenticación o el protocolo.
var forbiddenIdentityHeaders = []string{
	"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "Host",
	"Content-Type", "Content-Length", "Content-Encoding", "Transfer-Encoding",
	"Connection", "Upgrade", "Te", "Trailer", "Keep-Alive", "Accept",
	"Accept-Encoding", "User-Agent", "Forwarded", "X-Forwarded-For",
	"X-Forwarded-Host", "X-Forwarded-Proto", "A2A-Version", "A2A-Extensions",
}

// ReservedHeaders son cabeceras internas del gateway (design.md D3).
var ReservedHeaders = []string{"X-A2A-Request-Id", "X-A2A-Operation", "X-A2A-Agent", "X-A2A-Version"}

var securitySchemeKeys = map[string]bool{
	"apiKeySecurityScheme":        true,
	"httpAuthSecurityScheme":      true,
	"oauth2SecurityScheme":        true,
	"openIdConnectSecurityScheme": true,
	"mtlsSecurityScheme":          true,
}

// Mensajes que se repiten en varias validaciones.
const (
	msgUnsupportedOf2    = "valor %q no admitido; usa %q o %q"
	msgRepeatedFem       = "%q está repetida"
	msgRepeatedMasc      = "%q está repetido"
	msgUpstreamOnly      = "solo aplica a source upstream"
	msgOwnershipIdentity = "el control de propiedad requiere identity.required=true"
)

func (c *Config) validate(errs *errorList) {
	c.validateURLsAndPaths(errs)
	c.validateLimits(errs)
	c.validateIdentity(errs)
	c.validateAuthorization(errs)
	c.validateOwnership(errs)
	c.validatePush(errs)
	c.validateChallenge(errs)
	c.validateAgents(errs)
}

func (c *Config) validateURLsAndPaths(errs *errorList) {
	c.validatePublicURL(errs)
	c.validatePaths(errs)
	c.validateRouting(errs)
}

func (c *Config) validatePublicURL(errs *errorList) {
	if c.PublicURL == "" {
		errs.add("public_url", "es obligatorio: es la URL con la que los clientes ven el gateway")
		return
	}
	u, msg := absoluteURL(c.PublicURL)
	if msg != "" {
		errs.add("public_url", "%s", msg)
		return
	}
	if u.RawQuery != "" || (u.Path != "" && u.Path != "/") {
		errs.add("public_url", "solo puede llevar esquema, host y puerto; la ruta va en base_path")
	}
	if u.Scheme == "http" {
		c.Warnings = append(c.Warnings,
			"public_url usa http: solo es aceptable en desarrollo; A2A exige HTTPS en producción")
	}
	c.PublicURL = strings.TrimSuffix(c.PublicURL, "/")
}

func (c *Config) validatePaths(errs *errorList) {
	c.BasePath = strings.TrimSuffix(c.BasePath, "/")
	c.InternalPrefix = strings.TrimSuffix(c.InternalPrefix, "/")
	okBase := checkPath(errs, "base_path", c.BasePath)
	okInternal := checkPath(errs, "internal_prefix", c.InternalPrefix)
	if okBase && okInternal && pathsOverlap(c.BasePath, c.InternalPrefix) {
		errs.add("internal_prefix", "no puede coincidir ni solaparse con base_path %q", c.BasePath)
	}
	if okBase && hasSegmentPrefix(c.BasePath, "/.well-known") {
		errs.add("base_path", "no puede estar bajo /.well-known")
	}
}

func (c *Config) validateRouting(errs *errorList) {
	if c.RouteGranularity != GranularityClass && c.RouteGranularity != GranularityOperation {
		errs.add("route_granularity", "valor %q no admitido; usa %q u %q",
			c.RouteGranularity, GranularityClass, GranularityOperation)
	}
	if c.DefaultAgent == "" {
		return
	}
	if _, ok := c.Agent(c.DefaultAgent); !ok {
		errs.add("default_agent", "no existe ningún agente con id %q", c.DefaultAgent)
	}
}

func checkPath(errs *errorList, field, p string) bool {
	if p == "" {
		errs.add(field, "no puede ser la raíz /")
		return false
	}
	if !pathRe.MatchString(p) {
		errs.add(field, "ruta %q inválida: debe empezar por / y usar solo letras, dígitos y . _ ~ -", p)
		return false
	}
	for _, seg := range strings.Split(p[1:], "/") {
		if seg == "." || seg == ".." {
			errs.add(field, "ruta %q inválida: no puede contener segmentos . ni ..", p)
			return false
		}
	}
	return true
}

// hasSegmentPrefix indica si p es igual a prefix o empieza por prefix/.
func hasSegmentPrefix(p, prefix string) bool {
	p, prefix = strings.TrimSuffix(p, "/"), strings.TrimSuffix(prefix, "/")
	return p == prefix || strings.HasPrefix(p, prefix+"/")
}

// pathsOverlap indica si una de las dos rutas está dentro de la otra.
func pathsOverlap(a, b string) bool {
	return hasSegmentPrefix(a, b) || hasSegmentPrefix(b, a)
}

func (c *Config) validateLimits(errs *errorList) {
	checkRange(errs, "limits.max_body_bytes", c.Limits.MaxBodyBytes, 1<<10, 64<<20)
	checkRange(errs, "limits.max_inspect_bytes", c.Limits.MaxInspectBytes, 1<<10, 256<<20)
}

func checkRange(errs *errorList, field string, v, lo, hi int64) {
	if v < lo || v > hi {
		errs.add(field, "debe estar entre %d y %d", lo, hi)
	}
}

func checkDuration(errs *errorList, field string, d Duration, lo, hi time.Duration) {
	if d.D() < lo || d.D() > hi {
		errs.add(field, "debe estar entre %s y %s", lo, hi)
	}
}

func (c *Config) validateIdentity(errs *errorList) {
	h := c.Identity.Headers
	if _, ok := h[IdentitySubject]; !ok {
		errs.add("identity.headers", "falta la cabecera de subject")
	}
	seen := map[string]string{}
	for _, k := range sortedStringKeys(h) {
		name := h[k]
		path := "identity.headers." + k
		if !headerTokenRe.MatchString(name) {
			errs.add(path, "%q no es un nombre de cabecera válido", name)
			continue
		}
		checkIdentityHeader(errs, path, name)
		if prev, dup := seen[strings.ToLower(name)]; dup {
			errs.add(path, "la cabecera %q ya se usa en identity.headers.%s", name, prev)
		}
		seen[strings.ToLower(name)] = k
	}
	c.validateOwnerScope(errs)
	if !c.IdentityRequired() {
		c.Warnings = append(c.Warnings, "identity.required=false: las operaciones no exigen identidad")
	}
}

// checkIdentityHeader rechaza las cabeceras que el gateway borra o reserva.
func checkIdentityHeader(errs *errorList, path, name string) {
	switch {
	case containsFold(forbiddenIdentityHeaders, name):
		errs.add(path, "%q no puede transportar identidad: el gateway borra esta cabecera de las "+
			"peticiones entrantes", name)
	case containsFold(ReservedHeaders, name):
		errs.add(path, "%q es una cabecera interna del gateway", name)
	default:
		// La cabecera puede transportar identidad.
	}
}

func (c *Config) validateOwnerScope(errs *errorList) {
	scope := c.Identity.OwnerScope
	if len(scope) == 0 {
		errs.add("identity.owner_scope", "no puede estar vacío")
	}
	for i, k := range scope {
		path := fmt.Sprintf("identity.owner_scope[%d]", i)
		switch {
		case k == IdentityScopes:
			errs.add(path, "scopes no identifica a un principal")
		case c.Identity.Headers[k] == "":
			errs.add(path, "%q no está definido en identity.headers", k)
		default:
			// El componente identifica al principal.
		}
		if indexOf(scope[:i], k) >= 0 {
			errs.add(path, msgRepeatedMasc, k)
		}
	}
}

func (c *Config) validateAuthorization(errs *errorList) {
	scopes := c.Authorization.OperationScopes
	for _, op := range sortedListKeys(scopes) {
		path := "authorization.operation_scopes." + op
		if _, ok := protocol.OperationByName(op); !ok {
			errs.add(path, "operación desconocida; usa el nombre de A2A 1.0, como GetTask")
		}
		checkScopeList(errs, path, scopes[op], false)
	}
	if len(scopes) == 0 {
		return
	}
	if _, ok := c.Identity.Headers[IdentityScopes]; !ok {
		errs.add("authorization.operation_scopes", "requiere identity.headers.scopes")
	}
	if !c.IdentityRequired() {
		errs.add("authorization.operation_scopes", "requiere identity.required=true")
	}
}

func checkScopeList(errs *errorList, path string, list []string, allowEmpty bool) {
	if len(list) == 0 && !allowEmpty {
		errs.add(path, "debe incluir al menos un scope")
	}
	for i, s := range list {
		if !scopeTokenRe.MatchString(s) {
			errs.add(fmt.Sprintf("%s[%d]", path, i), "%q no es un scope válido", s)
		}
		if indexOf(list[:i], s) >= 0 {
			errs.add(fmt.Sprintf("%s[%d]", path, i), msgRepeatedMasc, s)
		}
	}
}

func (c *Config) validateOwnership(errs *errorList) {
	o := &c.Ownership
	if o.Store != StoreMemory && o.Store != StoreRedis {
		errs.add("ownership.store", msgUnsupportedOf2, o.Store, StoreMemory, StoreRedis)
	}
	checkDuration(errs, "ownership.ttl", o.TTL, time.Minute, 8760*time.Hour)
	checkPolicy(errs, "ownership.unknown_task", o.UnknownTask, PolicyDeny, PolicyPassthrough)
	checkPolicy(errs, "ownership.list_tasks", o.ListTasks, ListTasksOwnedContext, PolicyDeny, PolicyPassthrough)
	checkPolicy(errs, "ownership.on_store_error", o.OnStoreError, PolicyDeny, PolicyPassthrough)
	checkRange(errs, "ownership.memory.max_entries", int64(o.Memory.MaxEntries), 1, 10000000)
	if o.Store == StoreRedis {
		validateRedis(errs, &o.Redis)
	}
	if *o.Enabled {
		c.checkOwnershipEnabled(errs)
	}
	c.warnOwnershipPolicies()
}

func validateRedis(errs *errorList, r *RedisStore) {
	switch {
	case r.Address == "":
		errs.add("ownership.redis.address", "es obligatorio con store redis")
	case !validHostPort(r.Address):
		errs.add("ownership.redis.address", "%q debe tener la forma host:puerto", r.Address)
	default:
		// La dirección es válida.
	}
	if r.DB < 0 {
		errs.add("ownership.redis.db", "no puede ser negativo")
	}
	for _, f := range [][2]string{{"username_env", r.UsernameEnv}, {"password_env", r.PasswordEnv}} {
		if f[1] != "" && !envNameRe.MatchString(f[1]) {
			errs.add("ownership.redis."+f[0], "%q no es un nombre de variable de entorno válido", f[1])
		}
	}
	checkDuration(errs, "ownership.redis.timeout", r.Timeout, 100*time.Millisecond, 30*time.Second)
}

func (c *Config) checkOwnershipEnabled(errs *errorList) {
	if !c.IdentityRequired() {
		errs.add("ownership.enabled", msgOwnershipIdentity)
	}
	if c.Ownership.Store == StoreMemory {
		c.Warnings = append(c.Warnings, "ownership.store=memory: la propiedad no se comparte entre nodos; usa "+
			"redis con más de una instancia")
	}
}

func (c *Config) warnOwnershipPolicies() {
	if c.Ownership.UnknownTask == PolicyPassthrough {
		c.Warnings = append(c.Warnings, "ownership.unknown_task=passthrough: las tareas sin propietario "+
			"registrado son accesibles para cualquier principal")
	}
	if c.Ownership.OnStoreError == PolicyPassthrough {
		c.Warnings = append(c.Warnings, "ownership.on_store_error=passthrough: si el almacén falla, no se "+
			"comprueba la propiedad")
	}
}

func checkPolicy(errs *errorList, path, v string, allowed ...string) {
	if indexOf(allowed, v) < 0 {
		errs.add(path, "valor %q no admitido; usa uno de %s", v, strings.Join(quoteAll(allowed), ", "))
	}
}

func validPort(p string) bool {
	n, err := strconv.Atoi(p)
	return err == nil && n > 0 && n < 65536
}

func validHostPort(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	return err == nil && host != "" && validPort(port)
}

func (c *Config) validatePush(errs *errorList) {
	for i, h := range c.PushNotifications.AllowedHosts {
		if !hostPatternRe.MatchString(h) {
			errs.add(fmt.Sprintf("push_notifications.allowed_hosts[%d]", i),
				"%q no es un host válido; usa nombres en minúsculas, con *. opcional al principio", h)
		}
	}
	if !*c.PushNotifications.RequireHTTPS {
		c.Warnings = append(c.Warnings, "push_notifications.require_https=false: se aceptan webhooks sin TLS")
	}
	if !*c.PushNotifications.DenyPrivateNetworks {
		c.Warnings = append(c.Warnings, "push_notifications.deny_private_networks=false: se aceptan webhooks "+
			"hacia redes internas")
	}
}

func (c *Config) validateChallenge(errs *errorList) {
	ch := c.Challenge
	if !headerTokenRe.MatchString(ch.Scheme) {
		errs.add("challenge.scheme", "%q no es un esquema de autenticación válido", ch.Scheme)
	}
	if strings.ContainsAny(ch.Realm, "\"\\\r\n") {
		errs.add("challenge.realm", "no puede contener comillas, barras invertidas ni saltos de línea")
	}
	if ch.ResourceMetadata && len(ch.AuthorizationServers) == 0 {
		errs.add("challenge.authorization_servers", "es obligatorio con resource_metadata=true")
	}
	for i, s := range ch.AuthorizationServers {
		checkAbsoluteURL(errs, fmt.Sprintf("challenge.authorization_servers[%d]", i), s)
	}
	checkScopeList(errs, "challenge.scopes_supported", ch.ScopesSupported, true)
}

func (c *Config) validateAgents(errs *errorList) {
	if len(c.Agents) == 0 {
		errs.add("agents", "debe declarar al menos un agente")
		return
	}
	firstAt := map[string]int{}
	for i := range c.Agents {
		a := &c.Agents[i]
		path := agentPath(i, a.ID)
		validateAgentID(errs, path, a.ID)
		if prev, dup := firstAt[a.ID]; dup && a.ID != "" {
			errs.add(path+".id", "identificador duplicado %q: ya lo usa el agente en la posición %d", a.ID, prev)
		} else {
			firstAt[a.ID] = i
		}
		validateVersions(errs, path, a.Versions)
		validateDisabledOperations(errs, path, a.DisabledOperations)
		validateCard(errs, path+".card", &a.Card)
		if a.Security != nil {
			validateSecurity(errs, path+".security", a.Security)
		}
		c.validateAgentOwnership(errs, path, &a.Ownership)
	}
}

// agentPath nombra a un agente por su id o, si no lo tiene, por su posición.
func agentPath(i int, id string) string {
	if id != "" {
		return fmt.Sprintf("agents[%s]", id)
	}
	return fmt.Sprintf("agents[%d]", i)
}

func validateAgentID(errs *errorList, path, id string) {
	switch {
	case id == "":
		errs.add(path+".id", "es obligatorio")
	case !agentIDRe.MatchString(id):
		errs.add(path+".id", "%q inválido: usa minúsculas, dígitos y guiones, hasta 63 caracteres", id)
	default:
		// El identificador es válido.
	}
}

func validateVersions(errs *errorList, path string, versions []string) {
	if len(versions) == 0 {
		errs.add(path+".versions", "debe incluir al menos una versión")
	}
	for j, v := range versions {
		p := fmt.Sprintf("%s.versions[%d]", path, j)
		if v != string(protocol.V10) && v != string(protocol.V03) {
			errs.add(p, "versión %q no admitida; usa \"1.0\" o \"0.3\"", v)
		}
		if indexOf(versions[:j], v) >= 0 {
			errs.add(p, msgRepeatedFem, v)
		}
	}
}

func validateDisabledOperations(errs *errorList, path string, ops []string) {
	for j, op := range ops {
		p := fmt.Sprintf("%s.disabled_operations[%d]", path, j)
		if _, ok := protocol.OperationByName(op); !ok {
			errs.add(p, "operación desconocida %q; usa el nombre de A2A 1.0, como ListTasks", op)
		}
		if indexOf(ops[:j], op) >= 0 {
			errs.add(p, msgRepeatedFem, op)
		}
	}
}

func (c *Config) validateAgentOwnership(errs *errorList, path string, o *AgentOwnership) {
	if o.UnknownTask != "" {
		checkPolicy(errs, path+".ownership.unknown_task", o.UnknownTask, PolicyDeny, PolicyPassthrough)
	}
	if o.UnknownTask == PolicyPassthrough {
		c.Warnings = append(c.Warnings, path+".ownership.unknown_task=passthrough: las tareas sin "+
			"propietario son accesibles para cualquier principal")
	}
	if o.ListTasks != "" {
		checkPolicy(errs, path+".ownership.list_tasks", o.ListTasks,
			ListTasksOwnedContext, PolicyDeny, PolicyPassthrough)
	}
	if o.Enabled != nil && *o.Enabled && !c.IdentityRequired() {
		errs.add(path+".ownership.enabled", msgOwnershipIdentity)
	}
}

func validateCard(errs *errorList, path string, card *Card) {
	switch card.Source {
	case CardStatic:
		validateStaticCard(errs, path, card)
	case CardUpstream:
		validateUpstreamCard(errs, path, card)
	case "":
		errs.add(path+".source", "es obligatorio: usa %q o %q", CardStatic, CardUpstream)
	default:
		errs.add(path+".source", msgUnsupportedOf2, card.Source, CardStatic, CardUpstream)
	}
	checkDuration(errs, path+".http_max_age", *card.HTTPMaxAge, 0, 24*time.Hour)
	if card.Signature.Verify && card.Signature.JWKSFile == "" {
		errs.add(path+".signature.jwks_file", "es obligatorio con verify=true")
	}
}

func validateStaticCard(errs *errorList, path string, card *Card) {
	hasInline, hasFile := len(card.Inline) > 0, card.File != ""
	switch {
	case hasInline == hasFile:
		errs.add(path, "con source static indica exactamente uno de inline o file")
	case hasInline && !isJSONObject(card.Inline):
		errs.add(path+".inline", "debe ser un objeto JSON con la Agent Card")
	default:
		// Hay una sola fuente y es válida.
	}
	if card.URL != "" {
		errs.add(path+".url", msgUpstreamOnly)
	}
	if card.Signature.Verify {
		errs.add(path+".signature.verify", msgUpstreamOnly)
	}
}

func validateUpstreamCard(errs *errorList, path string, card *Card) {
	if card.URL == "" {
		errs.add(path+".url", "es obligatorio con source upstream")
	} else {
		checkAbsoluteURL(errs, path+".url", card.URL)
	}
	if len(card.Inline) > 0 || card.File != "" {
		errs.add(path, "inline y file solo aplican a source static")
	}
	checkDuration(errs, path+".ttl", card.TTL, time.Second, 24*time.Hour)
	if card.MaxStale.D() < card.TTL.D() {
		errs.add(path+".max_stale", "no puede ser menor que ttl (%s)", card.TTL)
	} else {
		checkDuration(errs, path+".max_stale", card.MaxStale, time.Second, 168*time.Hour)
	}
	checkRange(errs, path+".max_bytes", card.MaxBytes, 1<<10, 16<<20)
}

func validateSecurity(errs *errorList, path string, s *Security) {
	if len(s.Schemes) == 0 {
		errs.add(path+".schemes", "debe definir al menos un esquema")
	}
	for _, name := range sortedRawKeys(s.Schemes) {
		validateSecurityScheme(errs, path+".schemes."+name, s.Schemes[name])
	}
	for i, raw := range s.Requirements {
		validateSecurityRequirement(errs, fmt.Sprintf("%s.requirements[%d]", path, i), raw, s.Schemes)
	}
}

func validateSecurityScheme(errs *errorList, path string, raw json.RawMessage) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		errs.add(path, "debe ser un objeto SecurityScheme de A2A 1.0")
		return
	}
	kinds := 0
	for _, k := range sortedRawKeys(obj) {
		if securitySchemeKeys[k] {
			kinds++
		} else {
			errs.add(path+"."+k, "tipo de esquema desconocido")
		}
	}
	if kinds != 1 {
		errs.add(path, "debe contener exactamente un tipo de esquema, como httpAuthSecurityScheme")
	}
}

func validateSecurityRequirement(errs *errorList, path string, raw json.RawMessage,
	schemes map[string]json.RawMessage) {
	var req struct {
		Schemes map[string]json.RawMessage `json:"schemes"`
	}
	if json.Unmarshal(raw, &req) != nil || req.Schemes == nil {
		errs.add(path, "debe ser un objeto SecurityRequirement de A2A 1.0 con el campo schemes")
		return
	}
	for _, name := range sortedRawKeys(req.Schemes) {
		if _, ok := schemes[name]; !ok {
			errs.add(path+".schemes."+name, "el esquema no está definido en schemes")
		}
	}
}

// checkAbsoluteURL añade un error si s no es una URL absoluta http(s) válida.
func checkAbsoluteURL(errs *errorList, path, s string) {
	if _, msg := absoluteURL(s); msg != "" {
		errs.add(path, "%s", msg)
	}
}

func absoluteURL(s string) (*url.URL, string) {
	u, err := url.Parse(s)
	switch {
	case err != nil:
		return nil, fmt.Sprintf("URL inválida %q", s)
	case u.Scheme != "http" && u.Scheme != "https":
		return nil, fmt.Sprintf("%q debe usar http o https", s)
	case u.Host == "" || u.Hostname() == "":
		return nil, fmt.Sprintf("%q no tiene host", s)
	case u.User != nil:
		return nil, fmt.Sprintf("%q no puede llevar credenciales", s)
	case u.Fragment != "":
		return nil, fmt.Sprintf("%q no puede llevar fragmento", s)
	default:
		return u, ""
	}
}

func isJSONObject(raw json.RawMessage) bool {
	t := bytes.TrimLeft(raw, " \t\r\n")
	return len(t) > 0 && t[0] == '{' && json.Valid(raw)
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

func quoteAll(list []string) []string {
	out := make([]string, len(list))
	for i, v := range list {
		out[i] = strconv.Quote(v)
	}
	return out
}
