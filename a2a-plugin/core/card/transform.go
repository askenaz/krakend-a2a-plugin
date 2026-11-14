// Copyright 2026 Los autores del plugin A2A para KrakenD.

// Package card publica las Agent Cards a través del gateway: las obtiene de la
// configuración o del agente, las reescribe para que solo anuncien el gateway,
// las cachea y sirve el descubrimiento con ETag y Cache-Control
// (spec a2a-agent-discovery, design.md D7).
//
// La card se trata como un árbol JSON genérico: solo se tocan los campos que
// el gateway gestiona y el resto se copia tal cual.
package card

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"krakend-a2a.local/core/config"
	"krakend-a2a.local/core/protocol"
)

// Transformer reescribe Agent Cards de un agente.
type Transformer struct {
	// GatewayURL es la URL pública del agente: {public_url}{base_path}/{id}.
	GatewayURL string
	// Versions son las versiones habilitadas para el agente.
	Versions []protocol.Version
	// Streaming, Push y Extended indican si el gateway admite cada capacidad.
	Streaming, Push, Extended bool
	// Security sustituye los esquemas de seguridad de la card, si no es nil.
	Security *config.Security
	// InternalOrigins son orígenes que nunca deben aparecer en la card
	// publicada, además de los que se deducen de la propia card.
	InternalOrigins []string
}

// NewTransformer construye el transformador de un agente a partir de la
// configuración. streamingAvailable indica si la edición en ejecución puede
// entregar streaming (spec a2a-edition-packaging).
func NewTransformer(cfg *config.Config, a *config.Agent, streamingAvailable bool) *Transformer {
	disabled := map[string]bool{}
	for _, op := range a.DisabledOperations {
		disabled[op] = true
	}
	versions := make([]protocol.Version, 0, len(a.Versions))
	for _, v := range a.Versions {
		versions = append(versions, protocol.Version(v))
	}
	t := &Transformer{
		GatewayURL: GatewayURL(cfg, a),
		Versions:   versions,
		Streaming: streamingAvailable && a.StreamingEnabled() &&
			!disabled[protocol.SendStreamingMessage.String()],
		Push:     a.PushEnabled() && !disabled[protocol.CreateTaskPushNotificationConfig.String()],
		Extended: !disabled[protocol.GetExtendedAgentCard.String()],
		Security: a.Security,
	}
	if a.Card.Source == config.CardUpstream {
		if o := origin(a.Card.URL); o != "" {
			t.InternalOrigins = append(t.InternalOrigins, o)
		}
	}
	return t
}

// GatewayURL devuelve la URL pública de un agente.
func GatewayURL(cfg *config.Config, a *config.Agent) string {
	return strings.TrimSuffix(cfg.PublicURL, "/") + cfg.BasePath + "/" + a.ID
}

var errNoInterface = errors.New("la card no anuncia ninguna interfaz JSON-RPC con una versión habilitada en el gateway")

// Transform reescribe una card y devuelve el JSON publicado.
func (t *Transformer) Transform(raw []byte) ([]byte, error) {
	card, err := decodeCard(raw)
	if err != nil {
		return nil, err
	}
	if err := t.apply(card); err != nil {
		return nil, err
	}
	return encode(card)
}

// TransformValue reescribe una card ya decodificada (por ejemplo, el result de
// GetExtendedAgentCard) y devuelve el JSON publicado.
func (t *Transformer) TransformValue(raw json.RawMessage) (json.RawMessage, error) {
	return t.Transform(raw)
}

func (t *Transformer) apply(card map[string]interface{}) error {
	internal := t.internalOrigins(card)

	_, has10 := card["supportedInterfaces"]
	_, has03 := card["url"].(string)
	if !has10 && !has03 {
		return errors.New("la card no tiene supportedInterfaces (1.0) ni url (0.3)")
	}

	usable10 := has10 && t.rewriteInterfaces10(card)
	usable03 := has03 && t.rewriteOrDrop03(card, usable10)
	if !usable10 && !usable03 {
		return errNoInterface
	}
	t.clampCapabilities(card)

	if t.Security != nil {
		if err := t.replaceSecurity(card, has10, usable03); err != nil {
			return err
		}
	}

	// La firma original deja de ser válida al reescribir la card.
	delete(card, "signatures")

	scrub(card, internal, origin(t.GatewayURL))
	return nil
}

// rewriteOrDrop03 reescribe los campos de 0.3 si hacen falta y, si no quedan
// utilizables, los elimina.
func (t *Transformer) rewriteOrDrop03(card map[string]interface{}, usable10 bool) bool {
	usable03 := false
	if t.enabled(protocol.V03) || !usable10 {
		usable03 = t.rewriteInterfaces03(card)
	}
	if !usable03 {
		for _, k := range []string{"url", "preferredTransport", "additionalInterfaces", "protocolVersion"} {
			delete(card, k)
		}
	}
	return usable03
}

// clampCapabilities desactiva las capacidades que el gateway no admite.
func (t *Transformer) clampCapabilities(card map[string]interface{}) {
	if caps, ok := card["capabilities"].(map[string]interface{}); ok {
		clamp(caps, "streaming", t.Streaming)
		clamp(caps, "pushNotifications", t.Push)
		clamp(caps, "extendedAgentCard", t.Extended)
	}
	clamp(card, "supportsAuthenticatedExtendedCard", t.Extended)
}

func (t *Transformer) enabled(v protocol.Version) bool {
	for _, e := range t.Versions {
		if e == v {
			return true
		}
	}
	return false
}

// rewriteInterfaces10 deja solo las interfaces JSON-RPC con una versión
// habilitada, apuntando al gateway. Conserva tenant.
func (t *Transformer) rewriteInterfaces10(card map[string]interface{}) bool {
	list, _ := card["supportedInterfaces"].([]interface{})
	out := make([]interface{}, 0, len(list))
	seen := map[string]bool{}
	for _, item := range list {
		iface, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		binding, _ := iface["protocolBinding"].(string)
		if !strings.EqualFold(binding, "JSONRPC") {
			continue
		}
		pv, _ := iface["protocolVersion"].(string)
		v, ok := protocol.ParseVersion(pv)
		if !ok || !t.enabled(v) {
			continue
		}
		tenant, _ := iface["tenant"].(string)
		key := string(v) + "|" + tenant
		if seen[key] {
			continue
		}
		seen[key] = true
		iface["url"] = t.GatewayURL
		out = append(out, iface)
	}
	card["supportedInterfaces"] = out
	return len(out) > 0
}

// rewriteInterfaces03 reescribe los campos de interfaz de A2A 0.3. Devuelve
// false si la card no anuncia JSON-RPC en esos campos.
func (t *Transformer) rewriteInterfaces03(card map[string]interface{}) bool {
	jsonrpc := false
	preferred, hasPreferred := card["preferredTransport"].(string)
	if !hasPreferred || strings.EqualFold(preferred, "JSONRPC") {
		jsonrpc = true
	}
	if list, ok := card["additionalInterfaces"].([]interface{}); ok {
		for _, item := range list {
			iface, _ := item.(map[string]interface{})
			if tr, _ := iface["transport"].(string); strings.EqualFold(tr, "JSONRPC") {
				jsonrpc = true
			}
		}
		if jsonrpc {
			card["additionalInterfaces"] = []interface{}{
				map[string]interface{}{"url": t.GatewayURL, "transport": "JSONRPC"},
			}
		}
	}
	if !jsonrpc {
		return false
	}
	card["url"] = t.GatewayURL
	if hasPreferred {
		card["preferredTransport"] = "JSONRPC"
	}
	return true
}

// clamp pone a false una capacidad declarada que el gateway no admite.
func clamp(m map[string]interface{}, key string, allowed bool) {
	if v, ok := m[key].(bool); ok && v && !allowed {
		m[key] = false
	}
}

func (t *Transformer) replaceSecurity(card map[string]interface{}, has10, has03 bool) error {
	schemes, err := t.publishedSchemes(has10, has03)
	if err != nil {
		return err
	}
	card["securitySchemes"] = schemes
	if err := t.replaceRequirements(card, has10, has03); err != nil {
		return err
	}
	dropSkillSecurity(card)
	return nil
}

// publishedSchemes devuelve los esquemas en la forma de las versiones que la
// card anuncia.
func (t *Transformer) publishedSchemes(has10, has03 bool) (map[string]interface{}, error) {
	schemes := map[string]interface{}{}
	for name, raw := range t.Security.Schemes {
		v10, err := decodeObject(raw)
		if err != nil {
			return nil, fmt.Errorf("security.schemes.%s: %w", name, err)
		}
		if !has03 {
			schemes[name] = v10
			continue
		}
		dual, err := toDualScheme(v10)
		if err != nil {
			return nil, fmt.Errorf("security.schemes.%s: %w", name, err)
		}
		if !has10 {
			// Solo forma 0.3: se publica sin la clave de 1.0.
			for k := range v10 {
				delete(dual, k)
			}
		}
		schemes[name] = dual
	}
	return schemes, nil
}

func (t *Transformer) replaceRequirements(card map[string]interface{}, has10, has03 bool) error {
	reqs10 := make([]interface{}, 0, len(t.Security.Requirements))
	reqs03 := make([]interface{}, 0, len(t.Security.Requirements))
	for i, raw := range t.Security.Requirements {
		r, err := decodeObject(raw)
		if err != nil {
			return fmt.Errorf("security.requirements[%d]: %w", i, err)
		}
		reqs10 = append(reqs10, r)
		reqs03 = append(reqs03, toRequirement03(r))
	}
	delete(card, "securityRequirements")
	delete(card, "security")
	if len(reqs10) > 0 {
		if has10 {
			card["securityRequirements"] = reqs10
		}
		if has03 {
			card["security"] = reqs03
		}
	}
	return nil
}

// dropSkillSecurity quita los requisitos por skill, que nombran esquemas del
// agente que ya no existen.
func dropSkillSecurity(card map[string]interface{}) {
	skills, _ := card["skills"].([]interface{})
	for _, s := range skills {
		if skill, ok := s.(map[string]interface{}); ok {
			delete(skill, "securityRequirements")
			delete(skill, "security")
		}
	}
}

// toDualScheme añade a un SecurityScheme de 1.0 los campos equivalentes de
// 0.3, como hace el SDK oficial en sus cards compatibles.
func toDualScheme(v10 map[string]interface{}) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	for k, v := range v10 {
		out[k] = v
	}
	for kind, rawBody := range v10 {
		body, _ := rawBody.(map[string]interface{})
		copyField := func(from, to string) {
			if v, ok := body[from]; ok {
				out[to] = v
			}
		}
		copyField("description", "description")
		switch kind {
		case "apiKeySecurityScheme":
			out["type"] = "apiKey"
			copyField("location", "in")
			copyField("name", "name")
		case "httpAuthSecurityScheme":
			out["type"] = "http"
			copyField("scheme", "scheme")
			copyField("bearerFormat", "bearerFormat")
		case "oauth2SecurityScheme":
			out["type"] = "oauth2"
			copyField("flows", "flows")
			copyField("oauth2MetadataUrl", "oauth2MetadataUrl")
		case "openIdConnectSecurityScheme":
			out["type"] = "openIdConnect"
			copyField("openIdConnectUrl", "openIdConnectUrl")
		case "mtlsSecurityScheme":
			out["type"] = "mutualTLS"
		default:
			return nil, fmt.Errorf("tipo de esquema desconocido %q", kind)
		}
	}
	return out, nil
}

// toRequirement03 convierte {"schemes": {n: {"list": [...]}}} en {n: [...]}.
func toRequirement03(r map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	schemes, _ := r["schemes"].(map[string]interface{})
	for name, v := range schemes {
		scopes := []interface{}{}
		if obj, ok := v.(map[string]interface{}); ok {
			if list, ok := obj["list"].([]interface{}); ok {
				scopes = list
			}
		}
		out[name] = scopes
	}
	return out
}

// internalOrigins reúne los orígenes de las URLs de interfaz originales, antes
// de reescribirlas, junto con los configurados.
func (t *Transformer) internalOrigins(card map[string]interface{}) []string {
	set := map[string]bool{}
	for _, o := range t.InternalOrigins {
		set[o] = true
	}
	addOrigin(set, card["url"])
	for _, key := range []string{"supportedInterfaces", "additionalInterfaces"} {
		list, _ := card[key].([]interface{})
		for _, item := range list {
			if iface, ok := item.(map[string]interface{}); ok {
				addOrigin(set, iface["url"])
			}
		}
	}
	out := make([]string, 0, len(set))
	for o := range set {
		out = append(out, o)
	}
	return out
}

// addOrigin añade al conjunto el origen de v si es una URL absoluta.
func addOrigin(set map[string]bool, v interface{}) {
	s, _ := v.(string)
	if o := origin(s); o != "" {
		set[o] = true
	}
}

// scrub elimina cualquier campo cuyo valor sea una URL de un origen interno,
// a cualquier profundidad. El origen público del gateway nunca se elimina.
func scrub(v interface{}, internal []string, public string) {
	switch n := v.(type) {
	case map[string]interface{}:
		scrubObject(n, internal, public)
	case []interface{}:
		for _, child := range n {
			scrub(child, internal, public)
		}
	default:
		// Los valores escalares no contienen campos.
	}
}

func scrubObject(obj map[string]interface{}, internal []string, public string) {
	for k, child := range obj {
		if s, ok := child.(string); ok && isInternal(s, internal, public) {
			delete(obj, k)
			continue
		}
		scrub(child, internal, public)
	}
}

func isInternal(s string, internal []string, public string) bool {
	o := origin(s)
	if o == "" || o == public {
		return false
	}
	for _, in := range internal {
		if o == in {
			return true
		}
	}
	return false
}

// origin devuelve esquema://host[:puerto] en minúsculas, o "" si no es una URL
// absoluta http, https o grpc.
func origin(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Host == "" {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "grpc", "grpcs":
		return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
	default:
		return ""
	}
}

func decodeCard(raw []byte) (map[string]interface{}, error) {
	card, err := decodeObject(raw)
	if err != nil {
		return nil, fmt.Errorf("agent card inválida: %w", err)
	}
	return card, nil
}

func decodeObject(raw []byte) (map[string]interface{}, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("datos tras el objeto JSON")
	}
	obj, ok := v.(map[string]interface{})
	if !ok {
		return nil, errors.New("no es un objeto JSON")
	}
	return obj, nil
}

func encode(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
