// Copyright 2026 Los autores del plugin A2A para KrakenD.

// IdP simulado para el entorno de demostración y las pruebas de integración.
//
// Emite JWT RS256 con el flujo client_credentials de OAuth 2.0 y publica su
// JWKS. NO es un IdP de producción: la clave se genera al arrancar, los
// clientes y secretos son valores de prueba de clients.json y no hay rotación,
// revocación ni consentimiento.
//
// Rutas:
//
//	POST /token                              client_credentials
//	GET  /.well-known/jwks.json              claves públicas
//	GET  /.well-known/openid-configuration   metadatos
package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"
)

type client struct {
	Secret string `json:"secret"`
	Sub    string `json:"sub"`
	Tenant string `json:"tenant"`
	Scope  string `json:"scope"`
}

const kid = "mock-idp-1"

func main() {
	port := env("PORT", "8080")
	issuer := env("ISSUER", "http://mock-idp:"+port)
	audience := env("AUDIENCE", "a2a-gateway")
	clients := loadClients(env("CLIENTS_FILE", "/etc/mock-idp/clients.json"))

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal(err)
	}
	jwks, _ := json.Marshal(map[string]interface{}{"keys": []interface{}{map[string]string{
		"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig",
		"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes()),
	}}})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	})
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"issuer": issuer, "jwks_uri": issuer + "/.well-known/jwks.json",
			"token_endpoint": issuer + "/token", "grant_types_supported": []string{"client_credentials"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.PostForm.Get("grant_type") != "client_credentials" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
			return
		}
		id, secret, ok := r.BasicAuth()
		if !ok {
			id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
		}
		c, known := clients[id]
		if !known || subtle.ConstantTimeCompare([]byte(c.Secret), []byte(secret)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
			return
		}
		scope := c.Scope
		if req := r.PostForm.Get("scope"); req != "" {
			scope = intersect(c.Scope, req)
		}
		now := time.Now()
		claims := map[string]interface{}{
			"iss": issuer, "sub": c.Sub, "aud": audience, "iat": now.Unix(), "nbf": now.Unix(),
			"exp": now.Add(time.Hour).Unix(), "scope": scope, "tenant": c.Tenant, "client_id": id,
			"jti": b64(randBytes(12)),
		}
		token, err := sign(key, claims)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"access_token": token,
			"token_type":   "Bearer",
			"expires_in":   3600,
			"scope":        scope,
		})
	})

	log.Printf("mock-idp escuchando en :%s (issuer %s, %d clientes)", port, issuer, len(clients))
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func sign(key *rsa.PrivateKey, claims map[string]interface{}) (string, error) {
	h, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid})
	c, _ := json.Marshal(claims)
	input := b64(h) + "." + b64(c)
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return input + "." + b64(sig), nil
}

func intersect(allowed, requested string) string {
	ok := map[string]bool{}
	for _, s := range strings.Fields(allowed) {
		ok[s] = true
	}
	var out []string
	for _, s := range strings.Fields(requested) {
		if ok[s] {
			out = append(out, s)
		}
	}
	return strings.Join(out, " ")
}

func loadClients(path string) map[string]client {
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("no se puede leer %s: %v", path, err)
	}
	var m map[string]client
	if err := json.Unmarshal(raw, &m); err != nil {
		log.Fatalf("%s inválido: %v", path, err)
	}
	return m
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
