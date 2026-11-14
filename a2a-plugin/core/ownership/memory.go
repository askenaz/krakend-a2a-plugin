// Copyright 2026 Los autores del plugin A2A para KrakenD.

package ownership

import (
	"container/list"
	"context"
	"sync"
	"time"
)

// Memory es un almacén en memoria con caducidad deslizante y tamaño acotado.
// Solo es válido con un único nodo del gateway.
//
// Al llenarse descarta primero lo caducado y después lo usado hace más tiempo.
// Un registro descartado pasa a ser una tarea sin propietario, que por defecto
// se deniega: el fallo es cerrado.
type Memory struct {
	mu    sync.Mutex
	max   int
	items map[string]*list.Element
	lru   *list.List // frente: uso más reciente
	now   func() time.Time
}

type memItem struct {
	key     string
	owner   string
	expires time.Time
}

// NewMemory crea un almacén en memoria con un máximo de registros.
func NewMemory(maxEntries int) *Memory {
	if maxEntries <= 0 {
		maxEntries = 1
	}
	return &Memory{max: maxEntries, items: map[string]*list.Element{}, lru: list.New(), now: time.Now}
}

// Get implementa Store.
func (m *Memory) Get(_ context.Context, key string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	el, ok := m.items[key]
	if !ok {
		return "", false, nil
	}
	it := el.Value.(*memItem)
	if !m.now().Before(it.expires) {
		m.remove(el)
		return "", false, nil
	}
	m.lru.MoveToFront(el)
	return it.owner, true, nil
}

// Claim implementa Store.
func (m *Memory) Claim(_ context.Context, key, owner string, ttl time.Duration) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	if el, ok := m.items[key]; ok {
		it := el.Value.(*memItem)
		if now.Before(it.expires) {
			if it.owner == owner {
				it.expires = now.Add(ttl)
			}
			m.lru.MoveToFront(el)
			return it.owner, nil
		}
		m.remove(el)
	}
	for len(m.items) >= m.max {
		m.evict(now)
	}
	m.items[key] = m.lru.PushFront(&memItem{key: key, owner: owner, expires: now.Add(ttl)})
	return owner, nil
}

// Touch implementa Store.
func (m *Memory) Touch(_ context.Context, key string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if el, ok := m.items[key]; ok {
		it := el.Value.(*memItem)
		if m.now().Before(it.expires) {
			it.expires = m.now().Add(ttl)
			m.lru.MoveToFront(el)
		}
	}
	return nil
}

// Close implementa Store.
func (m *Memory) Close() error { return nil }

// Len devuelve el número de registros, incluidos los caducados pendientes.
func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

func (m *Memory) evict(now time.Time) {
	// Primero, cualquier registro caducado empezando por los menos usados.
	for el := m.lru.Back(); el != nil; el = el.Prev() {
		if !now.Before(el.Value.(*memItem).expires) {
			m.remove(el)
			return
		}
	}
	if el := m.lru.Back(); el != nil {
		m.remove(el)
	}
}

func (m *Memory) remove(el *list.Element) {
	delete(m.items, el.Value.(*memItem).key)
	m.lru.Remove(el)
}
