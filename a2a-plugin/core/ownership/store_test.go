// Copyright 2026 Los autores del plugin A2A para KrakenD.

package ownership

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

// Comportamiento común que deben cumplir todos los almacenes.
func exerciseStore(t *testing.T, s Store, advance func(time.Duration)) {
	ctx := context.Background()
	key := TaskKey("billing", "task-"+strconv.FormatInt(time.Now().UnixNano(), 36))

	if _, found, err := s.Get(ctx, key); err != nil || found {
		t.Fatalf("clave nueva: found=%v err=%v", found, err)
	}
	owner, err := s.Claim(ctx, key, "alice", time.Hour)
	if err != nil || owner != "alice" {
		t.Fatalf("primer registro: %q %v", owner, err)
	}
	// Gana el primero: un segundo principal no sobrescribe.
	owner, err = s.Claim(ctx, key, "mallory", time.Hour)
	if err != nil || owner != "alice" {
		t.Fatalf("un segundo registro no debe sobrescribir: %q %v", owner, err)
	}
	if got, found, err := s.Get(ctx, key); err != nil || !found || got != "alice" {
		t.Fatalf("Get: %q %v %v", got, found, err)
	}

	if advance == nil {
		return
	}
	// Caducidad deslizante.
	short := TaskKey("billing", "short")
	if _, err := s.Claim(ctx, short, "alice", 10*time.Second); err != nil {
		t.Fatal(err)
	}
	advance(8 * time.Second)
	if err := s.Touch(ctx, short, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	advance(8 * time.Second)
	if _, found, _ := s.Get(ctx, short); !found {
		t.Fatal("Touch debe renovar la caducidad")
	}
	advance(11 * time.Second)
	if _, found, _ := s.Get(ctx, short); found {
		t.Fatal("un registro sin uso más allá del TTL debe caducar")
	}
	// Tras caducar, otro principal puede registrar la clave.
	if owner, _ := s.Claim(ctx, short, "bob", time.Hour); owner != "bob" {
		t.Fatalf("clave caducada: propietario %q", owner)
	}
}

func TestMemoryStore(t *testing.T) {
	m := NewMemory(100)
	now := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	exerciseStore(t, m, func(d time.Duration) { now = now.Add(d) })
}

func TestMemoryConcurrentClaims(t *testing.T) {
	m := NewMemory(1000)
	ctx := context.Background()
	var wg sync.WaitGroup
	owners := make([]string, 50)
	for i := range owners {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			owners[i], _ = m.Claim(ctx, "race", fmt.Sprintf("p%d", i), time.Hour)
		}(i)
	}
	wg.Wait()
	for _, o := range owners {
		if o != owners[0] {
			t.Fatalf("todas las reclamaciones concurrentes deben ver al mismo propietario: %v", owners)
		}
	}
}

// Integración con un Redis real: A2A_TEST_REDIS=host:puerto.
func TestRedisStore(t *testing.T) {
	addr := os.Getenv("A2A_TEST_REDIS")
	if addr == "" {
		t.Skip("define A2A_TEST_REDIS=host:puerto para probar contra Redis")
	}
	r := NewRedis(RedisConfig{Address: addr, DB: 1, Timeout: time.Second, Password: os.Getenv("A2A_TEST_REDIS_PASSWORD")})
	defer r.Close()
	if err := r.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	exerciseStore(t, r, nil)

	ctx := context.Background()
	key := TaskKey("billing", "ttl-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if _, err := r.Claim(ctx, key, "alice", 300*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if _, found, _ := r.Get(ctx, key); found {
		t.Fatal("Redis debe aplicar el TTL")
	}

	// Dos clientes independientes, como dos nodos del gateway.
	other := NewRedis(RedisConfig{Address: addr, DB: 1, Password: os.Getenv("A2A_TEST_REDIS_PASSWORD")})
	defer other.Close()
	shared := TaskKey("billing", "shared-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	_, _ = r.Claim(ctx, shared, "alice", time.Minute)
	if owner, _ := other.Claim(ctx, shared, "bob", time.Minute); owner != "alice" {
		t.Fatalf("el segundo nodo debe ver al propietario registrado por el primero: %q", owner)
	}

	bad := NewRedis(RedisConfig{Address: "127.0.0.1:1", Timeout: 200 * time.Millisecond})
	if _, _, err := bad.Get(ctx, key); err == nil {
		t.Fatal("un Redis inaccesible debe devolver error")
	}
}

func TestMemoryEviction(t *testing.T) {
	ctx := context.Background()
	m := NewMemory(3)
	now := time.Now()
	m.now = func() time.Time { return now }
	// has consulta el mapa directamente para no alterar el orden de uso.
	has := func(k string) bool {
		_, ok := m.items[k]
		return ok
	}

	_, _ = m.Claim(ctx, "a", "x", time.Hour)
	_, _ = m.Claim(ctx, "b", "x", time.Second)
	_, _ = m.Claim(ctx, "c", "x", time.Hour)
	now = now.Add(2 * time.Second) // b caduca
	_, _, _ = m.Get(ctx, "a")      // orden de uso: a, c, b

	_, _ = m.Claim(ctx, "d", "x", time.Hour)
	if has("b") || !has("a") || !has("c") || !has("d") {
		t.Fatal("con un registro caducado disponible debe descartarse ese y no uno vigente")
	}
	// Orden de uso: d, a, c. Lleno y sin caducados: sale el menos usado, c.
	_, _ = m.Claim(ctx, "e", "x", time.Hour)
	if m.Len() != 3 {
		t.Fatalf("el tamaño debe quedar acotado: %d", m.Len())
	}
	if has("c") || !has("a") || !has("d") || !has("e") {
		t.Fatal("debe descartarse el usado hace más tiempo")
	}
}
