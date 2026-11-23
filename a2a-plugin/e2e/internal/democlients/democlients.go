// Copyright 2026 Los autores del plugin A2A para KrakenD.

// Package democlients lee los clientes del IdP simulado que genera
// deploy/demo-secrets, con sus secretos. Nunca están en el repositorio.
//
// El paquete no se llama "secrets" a propósito: .gitignore excluye cualquier
// carpeta con ese nombre como red de seguridad, y el código quedaría fuera de
// git.
package democlients

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultClientsFile es la ubicación por defecto, relativa al módulo e2e.
const DefaultClientsFile = "../deploy/.secrets/idp/clients.json"

// ClientsFile devuelve el fichero de clientes: A2A_CLIENTS_FILE o el de por defecto.
func ClientsFile() string {
	if f := os.Getenv("A2A_CLIENTS_FILE"); f != "" {
		return f
	}
	return filepath.FromSlash(DefaultClientsFile)
}

// ClientSecret devuelve el secreto de un cliente del IdP simulado.
func ClientSecret(file, clientID string) (string, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("no se pueden leer los clientes de la demostración (%w); levanta antes "+
			"el entorno de deploy/compose", err)
	}
	var clients map[string]struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(raw, &clients); err != nil {
		return "", fmt.Errorf("%s no es válido: %w", file, err)
	}
	c, ok := clients[clientID]
	if !ok || c.Secret == "" {
		return "", fmt.Errorf("el cliente %s no está en %s", clientID, file)
	}
	return c.Secret, nil
}
