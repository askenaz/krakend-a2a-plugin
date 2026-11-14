// Copyright 2026 Los autores del plugin A2A para KrakenD.

package pushguard

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func fakeDNS(table map[string][]string) func(context.Context, string) ([]netip.Addr, error) {
	return func(_ context.Context, host string) ([]netip.Addr, error) {
		ips, ok := table[host]
		if !ok {
			return nil, errors.New("no such host")
		}
		out := make([]netip.Addr, len(ips))
		for i, s := range ips {
			out[i] = netip.MustParseAddr(s)
		}
		return out, nil
	}
}

var dns = fakeDNS(map[string][]string{
	"hooks.example.com":  {"93.184.215.14"},
	"api.partner.io":     {"2606:4700::1111"},
	"rebind.example.com": {"93.184.215.14", "10.0.0.5"},
	"internal.corp":      {"192.168.1.10"},
	"metadata.example":   {"169.254.169.254"},
})

func TestDenyPrivateNetworks(t *testing.T) {
	g := New(Config{RequireHTTPS: true, DenyPrivateNetworks: true, Lookup: dns})
	cases := []struct {
		url string
		ok  bool
	}{
		{"https://hooks.example.com/a2a", true},
		{"https://api.partner.io:8443/x", true},
		{"https://93.184.215.14/hook", true},
		{"https://[2606:4700::1111]/hook", true},
		// Escenario: dirección privada.
		{"https://127.0.0.1/", false},
		{"https://localhost/", false},
		{"https://app.localhost/", false},
		{"https://10.1.2.3/", false},
		{"https://172.16.0.1/", false},
		{"https://192.168.0.1/", false},
		{"https://169.254.169.254/latest/meta-data", false},
		{"https://[::1]/", false},
		{"https://[fe80::1]/", false},
		{"https://[fd00:ec2::254]/", false},
		{"https://[::ffff:127.0.0.1]/", false},
		{"https://[64:ff9b::a00:1]/", false},
		{"https://100.64.0.1/", false},
		{"https://0.0.0.0/", false},
		{"https://255.255.255.255/", false},
		{"https://224.0.0.1/", false},
		{"https://internal.corp/", false},
		{"https://metadata.example/", false},
		// Un nombre que resuelve a una dirección pública y a una privada se rechaza.
		{"https://rebind.example.com/", false},
		{"https://no-resuelve.example/", false},
		// Escenario: esquema no seguro y otras URLs inválidas.
		{"http://hooks.example.com/", false},
		{"ftp://hooks.example.com/", false},
		{"https://user:pass@hooks.example.com/", false},
		{"https:///sin-host", false},
		{"no es una url", false},
	}
	for _, c := range cases {
		err := g.Check(context.Background(), c.url)
		if (err == nil) != c.ok {
			t.Errorf("%s: se esperaba ok=%v, obtenido %v", c.url, c.ok, err)
		}
		if err != nil && !IsRejection(err) {
			t.Errorf("%s: el error debe ser un rechazo: %v", c.url, err)
		}
	}
}

// Escenarios: host fuera de la lista y destino permitido.
func TestAllowedHosts(t *testing.T) {
	g := New(Config{RequireHTTPS: true, DenyPrivateNetworks: true, Lookup: dns,
		AllowedHosts: []string{"*.example.com", "hooks.partner.example"}})
	cases := []struct {
		url string
		ok  bool
	}{
		{"https://hooks.example.com/x", true},
		{"https://HOOKS.Example.com./x", true},
		{"https://a.b.example.com/x", true}, // no resuelve, pero está permitido de forma explícita
		{"https://hooks.partner.example/x", true},
		{"https://example.com/x", false}, // *. no incluye el dominio raíz
		{"https://api.partner.io/x", false},
		{"https://evil-example.com/x", false},
		{"https://rebind.example.com/x", false}, // permitido, pero resuelve a una red privada
	}
	for _, c := range cases {
		err := g.Check(context.Background(), c.url)
		if (err == nil) != c.ok {
			t.Errorf("%s: se esperaba ok=%v, obtenido %v", c.url, c.ok, err)
		}
	}
}

func TestRelaxedGuard(t *testing.T) {
	g := New(Config{Lookup: dns})
	for _, u := range []string{"http://10.0.0.1/", "http://localhost:8080/hook"} {
		if err := g.Check(context.Background(), u); err != nil {
			t.Errorf("con las comprobaciones desactivadas %s debe aceptarse: %v", u, err)
		}
	}
}
