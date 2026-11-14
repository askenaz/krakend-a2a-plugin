// Copyright 2026 Los autores del plugin A2A para KrakenD.

// Package ownership guarda qué principal es propietario de cada tarea y cada
// contexto A2A (design.md D10).
//
// El valor guardado es un resumen no reversible del principal; nunca el
// sujeto en claro. Gana el primero que escribe: un segundo intento con otro
// propietario no sobrescribe.
package ownership

import (
	"context"
	"time"
)

// Store es un almacén de propiedad.
type Store interface {
	// Get devuelve el propietario de una clave, si existe y no ha caducado.
	Get(ctx context.Context, key string) (owner string, found bool, err error)
	// Claim registra owner si la clave no tiene propietario y devuelve el
	// propietario resultante, que puede ser otro si ya existía.
	Claim(ctx context.Context, key, owner string, ttl time.Duration) (current string, err error)
	// Touch renueva la caducidad de una clave existente.
	Touch(ctx context.Context, key string, ttl time.Duration) error
	// Close libera los recursos del almacén.
	Close() error
}

// TaskKey es la clave de propiedad de una tarea.
func TaskKey(agent, taskID string) string { return "a2a:own:" + agent + ":t:" + taskID }

// ContextKey es la clave de propiedad de un contexto.
func ContextKey(agent, contextID string) string { return "a2a:own:" + agent + ":c:" + contextID }
