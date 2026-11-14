// Copyright 2026 Los autores del plugin A2A para KrakenD.

// Package pushguard valida las URLs de webhook de las notificaciones push
// antes de que lleguen al agente (design.md D11).
//
// Es defensa en profundidad: quien llama al webhook es el agente, que podría
// resolver el nombre a otra dirección más tarde. El control de salida de red
// del agente sigue siendo necesario.
package pushguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// Config configura la guardia.
type Config struct {
	RequireHTTPS        bool
	DenyPrivateNetworks bool
	// AllowedHosts son nombres en minúsculas, con *. opcional al principio.
	// Vacío permite cualquier host público.
	AllowedHosts []string
	// Lookup resuelve un nombre. Por defecto, el resolutor del sistema.
	Lookup func(ctx context.Context, host string) ([]netip.Addr, error)
	// Timeout acota la resolución de nombres.
	Timeout time.Duration
}

// Guard valida URLs de webhook.
type Guard struct {
	cfg Config
}

// New crea una guardia.
func New(cfg Config) *Guard {
	if cfg.Lookup == nil {
		cfg.Lookup = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Second
	}
	return &Guard{cfg: cfg}
}

// Rejection explica por qué se rechazó una URL.
type Rejection struct {
	URL    string
	Reason string
}

func (r *Rejection) Error() string { return fmt.Sprintf("webhook %q rechazado: %s", r.URL, r.Reason) }

// Prefijos que nunca son destinos válidos de un webhook, además de los que
// netip ya clasifica (loopback, privadas, link-local, multicast).
var deniedPrefixes = mustPrefixes(
	"0.0.0.0/8",       // red actual
	"100.64.0.0/10",   // CGNAT
	"192.0.0.0/24",    // asignaciones de protocolo IETF
	"192.0.2.0/24",    // documentación
	"198.18.0.0/15",   // pruebas de rendimiento
	"198.51.100.0/24", // documentación
	"203.0.113.0/24",  // documentación
	"240.0.0.0/4",     // reservada, incluye 255.255.255.255
	"64:ff9b::/96",    // NAT64: se comprueba también la IPv4 embebida
	"2001:db8::/32",   // documentación
)

func mustPrefixes(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

// Check valida una URL de webhook.
func (g *Guard) Check(ctx context.Context, raw string) error {
	host, reason := g.checkURL(raw)
	if reason == "" {
		reason = g.checkHost(ctx, host)
	}
	if reason != "" {
		return &Rejection{URL: raw, Reason: reason}
	}
	return nil
}

// checkURL valida la forma de la URL y devuelve el host normalizado, o el
// motivo del rechazo.
func (g *Guard) checkURL(raw string) (string, string) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "URL inválida"
	}
	if reason := g.checkScheme(u.Scheme); reason != "" {
		return "", reason
	}
	if u.User != nil {
		return "", "la URL no puede llevar credenciales"
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return "", "falta el host"
	}
	return host, ""
}

func (g *Guard) checkScheme(scheme string) string {
	switch strings.ToLower(scheme) {
	case "https":
		return ""
	case "http":
		if g.cfg.RequireHTTPS {
			return "se exige HTTPS"
		}
		return ""
	default:
		return "esquema no admitido"
	}
}

// checkHost aplica la lista de hosts permitidos y, si procede, la denegación
// de redes internas.
func (g *Guard) checkHost(ctx context.Context, host string) string {
	allowlisted, ok := g.allowlisted(host)
	if !ok {
		return "el host no está en push_notifications.allowed_hosts"
	}
	if !g.cfg.DenyPrivateNetworks {
		return ""
	}
	return g.checkDestination(ctx, host, allowlisted)
}

// allowlisted indica si el host está en la lista de hosts permitidos y si es
// aceptable: sin lista, todos lo son.
func (g *Guard) allowlisted(host string) (listed, ok bool) {
	if len(g.cfg.AllowedHosts) == 0 {
		return false, true
	}
	for _, p := range g.cfg.AllowedHosts {
		if matchHost(p, host) {
			return true, true
		}
	}
	return false, false
}

// checkDestination rechaza los destinos internos: direcciones literales,
// localhost y nombres que resuelven a redes no públicas.
func (g *Guard) checkDestination(ctx context.Context, host string, allowlisted bool) string {
	if addr, err := netip.ParseAddr(host); err == nil {
		return deniedAddr(addr)
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "loopback"
	}

	lctx, cancel := context.WithTimeout(ctx, g.cfg.Timeout)
	defer cancel()
	addrs, err := g.cfg.Lookup(lctx, host)
	if err != nil || len(addrs) == 0 {
		if allowlisted {
			// Un host que el operador permite de forma explícita se acepta
			// aunque el gateway no pueda resolverlo.
			return ""
		}
		return "no se puede resolver el host"
	}
	for _, a := range addrs {
		if why := deniedAddr(a); why != "" {
			return why + " (" + a.String() + ")"
		}
	}
	return ""
}

func matchHost(pattern, host string) bool {
	if suffix, ok := strings.CutPrefix(pattern, "*."); ok {
		return strings.HasSuffix(host, "."+suffix)
	}
	return host == pattern
}

// deniedAddr devuelve el motivo si la dirección no es un destino público.
func deniedAddr(a netip.Addr) string {
	a = a.Unmap()
	switch {
	case a.IsLoopback():
		return "loopback"
	case a.IsUnspecified():
		return "dirección no especificada"
	case a.IsPrivate():
		return "red privada"
	case a.IsLinkLocalUnicast():
		return "link-local o metadatos de nube"
	case a.IsMulticast(), a.IsLinkLocalMulticast(), a.IsInterfaceLocalMulticast():
		return "multicast"
	default:
		// Se comprueba contra los rangos reservados.
	}
	for _, p := range deniedPrefixes {
		if p.Contains(a) {
			if p.Addr().Is6() && p.Bits() == 96 {
				// NAT64: decide la IPv4 embebida.
				b := a.As16()
				v4 := netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]})
				return deniedAddr(v4)
			}
			return "rango reservado"
		}
	}
	return ""
}

// IsRejection indica si el error es un rechazo de la guardia.
func IsRejection(err error) bool {
	var r *Rejection
	return errors.As(err, &r)
}
