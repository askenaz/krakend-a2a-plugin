// Copyright 2026 Los autores del plugin A2A para KrakenD.

package ownership

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Respuestas RESP que usa el servidor simulado.
const (
	replyOK  = "+OK\r\n"
	replyNil = "$-1\r\n"
)

// fakeRedis es un servidor RESP mínimo en memoria, con reloj controlable, que
// implementa los comandos que usa el cliente: AUTH, SELECT, PING, GET, SET NX
// PX y PEXPIRE. GET de la clave "boom" devuelve un error de Redis.
type fakeRedis struct {
	ln         net.Listener
	user, pass string

	mu    sync.Mutex
	now   time.Time
	data  map[string]fakeEntry
	conns int
}

type fakeEntry struct {
	val     string
	expires time.Time
}

func newFakeRedis(t *testing.T, user, pass string) *fakeRedis {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeRedis{ln: ln, user: user, pass: pass, now: time.Unix(0, 0), data: map[string]fakeEntry{}}
	go f.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return f
}

// testSecret genera una contraseña aleatoria para el servidor simulado.
func testSecret(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func (f *fakeRedis) addr() string { return f.ln.Addr().String() }

func (f *fakeRedis) connections() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.conns
}

func (f *fakeRedis) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

func (f *fakeRedis) serve() {
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		f.conns++
		f.mu.Unlock()
		go f.handle(c)
	}
}

func (f *fakeRedis) handle(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	authed := f.pass == ""
	for {
		args, err := readCommand(r)
		if err != nil {
			return
		}
		var reply string
		if strings.EqualFold(args[0], "AUTH") {
			reply, authed = f.auth(args)
		} else if authed {
			reply = f.exec(args)
		} else {
			reply = "-NOAUTH Authentication required.\r\n"
		}
		if _, err := io.WriteString(c, reply); err != nil {
			return
		}
	}
}

// readCommand lee una orden RESP: una lista de cadenas.
func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "*")))
	if err != nil {
		return nil, err
	}
	args := make([]string, n)
	for i := range args {
		head, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		size, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(head, "$")))
		if err != nil {
			return nil, err
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		args[i] = string(buf[:size])
	}
	return args, nil
}

func (f *fakeRedis) auth(args []string) (string, bool) {
	user, pass := "default", args[len(args)-1]
	if len(args) == 3 {
		user = args[1]
	}
	if pass != f.pass || (f.user != "" && user != f.user) {
		return "-WRONGPASS invalid username-password pair\r\n", false
	}
	return replyOK, true
}

func (f *fakeRedis) exec(args []string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch strings.ToUpper(args[0]) {
	case "PING":
		return "+PONG\r\n"
	case "SELECT":
		return replyOK
	case "GET":
		return f.get(args[1])
	case "SET":
		return f.setNX(args[1], args[2], args[5])
	case "PEXPIRE":
		return f.pexpire(args[1], args[2])
	default:
		return "-ERR unknown command\r\n"
	}
}

func (f *fakeRedis) live(key string) (fakeEntry, bool) {
	e, ok := f.data[key]
	if !ok || !f.now.Before(e.expires) {
		delete(f.data, key)
		return fakeEntry{}, false
	}
	return e, true
}

func (f *fakeRedis) get(key string) string {
	if key == "boom" {
		return "-ERR fallo simulado\r\n"
	}
	e, ok := f.live(key)
	if !ok {
		return replyNil
	}
	return fmt.Sprintf("$%d\r\n%s\r\n", len(e.val), e.val)
}

func (f *fakeRedis) setNX(key, val, ms string) string {
	if _, ok := f.live(key); ok {
		return replyNil
	}
	n, _ := strconv.Atoi(ms)
	f.data[key] = fakeEntry{val: val, expires: f.now.Add(time.Duration(n) * time.Millisecond)}
	return replyOK
}

func (f *fakeRedis) pexpire(key, ms string) string {
	e, ok := f.live(key)
	if !ok {
		return ":0\r\n"
	}
	n, _ := strconv.Atoi(ms)
	e.expires = f.now.Add(time.Duration(n) * time.Millisecond)
	f.data[key] = e
	return ":1\r\n"
}

// El cliente Redis cumple el comportamiento común de los almacenes, con
// usuario, contraseña y base de datos, sin necesitar un Redis real.
func TestRedisStoreWithFakeServer(t *testing.T) {
	pass := testSecret(t)
	f := newFakeRedis(t, "gateway", pass)
	r := NewRedis(RedisConfig{Address: f.addr(), Username: "gateway", Password: pass, DB: 2})
	defer r.Close()
	if err := r.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	exerciseStore(t, r, f.advance)
	if n := f.connections(); n != 1 {
		t.Errorf("las operaciones secuenciales deben reutilizar una conexión, se abrieron %d", n)
	}
}

func TestRedisAuthRejected(t *testing.T) {
	f := newFakeRedis(t, "", testSecret(t))
	r := NewRedis(RedisConfig{Address: f.addr(), Password: testSecret(t)})
	defer r.Close()
	_, _, err := r.Get(context.Background(), "k")
	if err == nil || !strings.Contains(err.Error(), "autenticación rechazada") {
		t.Fatalf("se esperaba un rechazo de autenticación: %v", err)
	}
}

// Un error de Redis llega al llamante sin descartar la conexión; un servidor
// inaccesible es un error de conexión.
func TestRedisErrors(t *testing.T) {
	f := newFakeRedis(t, "", "")
	r := NewRedis(RedisConfig{Address: f.addr()})
	defer r.Close()
	ctx := context.Background()
	_, _, err := r.Get(ctx, "boom")
	var re redisError
	if !errors.As(err, &re) {
		t.Fatalf("se esperaba un error de Redis: %v", err)
	}
	if _, _, err := r.Get(ctx, "otra"); err != nil {
		t.Fatal(err)
	}
	if n := f.connections(); n != 1 {
		t.Errorf("un error de Redis no debe descartar la conexión, se abrieron %d", n)
	}

	bad := NewRedis(RedisConfig{Address: "127.0.0.1:1", Timeout: 200 * time.Millisecond})
	if _, _, err := bad.Get(ctx, "k"); err == nil {
		t.Fatal("un Redis inaccesible debe devolver error")
	}
}

func TestRESPParser(t *testing.T) {
	cases := []struct {
		in   string
		want interface{}
		ok   bool
	}{
		{"+OK\r\n", "OK", true},
		{":42\r\n", int64(42), true},
		{"$3\r\nabc\r\n", "abc", true},
		{replyNil, nil, true},
		{"*-1\r\n", nil, true},
		{"*2\r\n$1\r\na\r\n:7\r\n", []interface{}{"a", int64(7)}, true},
		{"-ERR mal\r\n", nil, false},
		{"?raro\r\n", nil, false},
		{"\r\n", nil, false},
		{"$x\r\n", nil, false},
		{"*x\r\n", nil, false},
		{"*1\r\n?\r\n", nil, false},
		{"$5\r\nab", nil, false},
		{"", nil, false},
	}
	for _, c := range cases {
		conn := &redisConn{r: bufio.NewReader(strings.NewReader(c.in))}
		got, err := conn.read()
		if (err == nil) != c.ok || (c.ok && !reflect.DeepEqual(got, c.want)) {
			t.Errorf("%q: obtenido %#v, %v", c.in, got, err)
		}
	}
}
