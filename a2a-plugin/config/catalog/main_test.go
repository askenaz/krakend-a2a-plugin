// Copyright 2026 Los autores del plugin A2A para KrakenD.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssemble(t *testing.T) {
	dir := fixture(t, map[string]string{
		"a2a.json":                         `{"service":{"name":"demo"},"agents":[{"id":"legacy"}]}`,
		"agents/research/agent.json":       `{"id":"research","card":{"source":"static"}}`,
		"agents/research/gateway.json":     `{"host":"https://research.example.com"}`,
		"agents/research/versions.json":    `["1.0"]`,
		"agents/research/assets/card.json": `{"name":"Research"}`,
		"agents/billing.json":              `{"id":"billing","gateway":{"host":"https://billing.example.com"}}`,
	})
	settings, err := assemble(dir)
	if err != nil {
		t.Fatal(err)
	}
	var agents []object
	if err := json.Unmarshal(settings["agents"], &agents); err != nil {
		t.Fatal(err)
	}
	if len(agents) != 3 || string(agents[0]["id"]) != `"legacy"` ||
		string(agents[1]["id"]) != `"billing"` || string(agents[2]["id"]) != `"research"` {
		t.Fatalf("unexpected catalog: %s", settings["agents"])
	}
	research := agents[2]
	if string(research["gateway"]) != `{"host":"https://research.example.com"}` ||
		string(research["versions"]) != `["1.0"]` || string(research["card"]) != `{"source":"static"}` ||
		string(settings["service"]) != `{"name":"demo"}` {
		t.Fatalf("unexpected settings: %+v, research: %+v", settings, research)
	}
}

func TestAssemblyErrors(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"conflicting sections", map[string]string{
			"agents/billing/agent.json":   `{"id":"billing","gateway":{}}`,
			"agents/billing/gateway.json": `{}`,
		}, []string{"defined in both", "agent.json", "gateway.json"}},
		{"duplicate agents", map[string]string{
			"agents/billing.json":       `{"id":"billing"}`,
			"agents/billing/agent.json": `{"id":"billing"}`,
		}, []string{"duplicate agent id", "billing.json", "agent.json"}},
		{"duplicate legacy agent", map[string]string{
			"a2a.json":            `{"agents":[{"id":"billing"}]}`,
			"agents/billing.json": `{"id":"billing"}`,
		}, []string{"duplicate agent id", "a2a.json", "billing.json"}},
		{"id mismatch", map[string]string{
			"agents/research.json": `{"id":"billing"}`,
		}, []string{"research.json", "must match"}},
		{"malformed fragment", map[string]string{
			"agents/billing/agent.json":    `{"id":"billing"}`,
			"agents/billing/security.json": `{"bad":`,
		}, []string{"security.json", "JSON"}},
		{"wrong fragment shape", map[string]string{
			"agents/billing/agent.json":    `{"id":"billing"}`,
			"agents/billing/versions.json": `{"versions":["1.0"]}`,
		}, []string{"versions.json", "must be an array"}},
		{"missing manifest", map[string]string{
			"agents/billing/gateway.json": `{}`,
		}, []string{"agent.json"}},
		{"empty", nil, []string{"no agents defined"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := assemble(fixture(t, tc.files))
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, part := range tc.want {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("expected %q in %q", part, err)
				}
			}
		})
	}
}

func TestInlineOnly(t *testing.T) {
	dir := fixture(t, map[string]string{"a2a.json": `{"agents":[{"id":"billing"}]}`})
	settings, err := assemble(dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(settings["agents"]) != `[{"id":"billing"}]` {
		t.Fatalf("unexpected catalog: %s", settings["agents"])
	}
}

// Links inside the settings directory work, as in a Kubernetes ConfigMap
// volume. A link that leads outside it is rejected without reading the target.
func TestSymlinks(t *testing.T) {
	dir := fixture(t, map[string]string{
		"shared/billing.json": `{"id":"billing"}`,
		"agents/.keep":        ``,
	})
	link := filepath.Join(dir, "agents", "billing.json")
	if err := os.Symlink(filepath.Join("..", "shared", "billing.json"), link); err != nil {
		t.Skipf("symbolic links are not available: %v", err)
	}
	settings, err := assemble(dir)
	if err != nil {
		t.Fatalf("a link inside the settings directory must work: %v", err)
	}
	if string(settings["agents"]) != `[{"id":"billing"}]` {
		t.Fatalf("unexpected catalog: %s", settings["agents"])
	}

	outside := filepath.Join(t.TempDir(), "research.json")
	if err := os.WriteFile(outside, []byte(`{"id":"research"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "agents", "research.json")); err != nil {
		t.Fatal(err)
	}
	_, err = assemble(dir)
	if err == nil || !strings.Contains(err.Error(), "research.json") {
		t.Fatalf("a link outside the settings directory must be rejected, got %v", err)
	}
}

func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, data string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("a2a.json", `{}`)
	for name, data := range files {
		write(name, data)
	}
	return dir
}
