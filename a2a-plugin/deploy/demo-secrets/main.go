// Copyright 2026 Los autores del plugin A2A para KrakenD.

// demo-secrets genera, en local, el material secreto del entorno de
// demostración, para que nada de eso viva en el repositorio:
//
//	<dir>/agent/card-signing.key.pem     clave privada con la que el agente firma su card
//	<dir>/gateway/card-signing.jwks.json clave pública con la que el gateway la verifica
//	<dir>/idp/clients.json               clientes del IdP simulado con secretos aleatorios
//
// No sobrescribe lo que ya existe: para rotar, borra el directorio.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// SigningKID identifica la clave de firma de cards de la demostración.
const SigningKID = "demo-card-signing-1"

type client struct {
	Sub    string `json:"sub"`
	Tenant string `json:"tenant"`
	Scope  string `json:"scope"`
}

// readWrite son los scopes de los clientes que leen y escriben.
const readWrite = "a2a.read a2a.write"

// Clientes de la demostración. Los secretos se generan al azar.
var clients = []struct {
	id string
	c  client
}{
	{"alice-app", client{"alice", "acme", readWrite}},
	{"bob-app", client{"bob", "acme", readWrite}},
	{"reader-app", client{"carol", "acme", "a2a.read"}},
	{"eve-app", client{"eve", "globex", readWrite}},
}

func main() {
	dir := "/out"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := generate(dir); err != nil {
		log.Fatal(err)
	}
}

func generate(dir string) error {
	for _, sub := range []string{"agent", "gateway", "idp"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return err
		}
	}
	keyFile := filepath.Join(dir, "agent", "card-signing.key.pem")
	jwksFile := filepath.Join(dir, "gateway", "card-signing.jwks.json")
	if !exists(keyFile) || !exists(jwksFile) {
		if err := signingKeys(keyFile, jwksFile); err != nil {
			return err
		}
		log.Printf("generado el par de claves de firma de cards")
	}
	clientsFile := filepath.Join(dir, "idp", "clients.json")
	if !exists(clientsFile) {
		if err := clientsJSON(clientsFile); err != nil {
			return err
		}
		log.Printf("generados los clientes del IdP simulado")
	}
	return nil
}

func signingKeys(keyFile, jwksFile string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o644); err != nil {
		return err
	}
	jwks, err := json.MarshalIndent(map[string]interface{}{"keys": []interface{}{map[string]string{
		"kty": "EC", "crv": "P-256", "alg": "ES256", "use": "sig", "kid": SigningKID,
		"x": b64(key.X.FillBytes(make([]byte, 32))), "y": b64(key.Y.FillBytes(make([]byte, 32))),
	}}}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(jwksFile, jwks, 0o644)
}

// clientsJSON escribe un cliente por línea, para que los scripts puedan leerlo
// sin herramientas de JSON.
func clientsJSON(path string) error {
	lines := make([]string, 0, len(clients))
	for _, c := range clients {
		secret := make([]byte, 24)
		if _, err := rand.Read(secret); err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf(`  %q: {"secret": %q, "sub": %q, "tenant": %q, "scope": %q}`,
			c.id, b64(secret), c.c.Sub, c.c.Tenant, c.c.Scope))
	}
	return os.WriteFile(path, []byte("{\n"+strings.Join(lines, ",\n")+"\n}\n"), 0o644)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
