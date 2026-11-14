// Copyright 2026 Los autores del plugin A2A para KrakenD.

package ownership

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// RedisConfig configura el cliente Redis mínimo.
type RedisConfig struct {
	Address  string
	Username string
	Password string
	DB       int
	TLS      bool
	// TLSConfig permite ajustar TLS; si es nil se usa la configuración por
	// defecto con el nombre del host como ServerName.
	TLSConfig *tls.Config
	Timeout   time.Duration
	PoolSize  int
}

// Redis es un almacén de propiedad sobre Redis, compartido entre nodos.
//
// Implementa solo lo necesario sobre la biblioteca estándar: AUTH, SELECT,
// GET, SET NX PX y PEXPIRE con el protocolo RESP2. KrakenD Community 2.13 no
// ofrece Redis a los plugins y enlazar una biblioteca externa arriesga fallos
// de carga (design.md D10 y D13). Admite un único servidor o un punto de
// entrada único; no admite Redis Cluster ni Sentinel.
type Redis struct {
	cfg  RedisConfig
	pool chan *redisConn
}

type redisConn struct {
	net.Conn
	r *bufio.Reader
	w *bufio.Writer
}

// NewRedis crea el cliente. No conecta hasta la primera operación.
func NewRedis(cfg RedisConfig) *Redis {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Second
	}
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 8
	}
	return &Redis{cfg: cfg, pool: make(chan *redisConn, cfg.PoolSize)}
}

// Ping comprueba la conexión.
func (r *Redis) Ping(ctx context.Context) error {
	v, err := r.do(ctx, "PING")
	if err != nil {
		return err
	}
	if v != "PONG" {
		return fmt.Errorf("redis: respuesta inesperada a PING: %v", v)
	}
	return nil
}

// Get implementa Store.
func (r *Redis) Get(ctx context.Context, key string) (string, bool, error) {
	v, err := r.do(ctx, "GET", key)
	if err != nil {
		return "", false, err
	}
	if v == nil {
		return "", false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", false, fmt.Errorf("redis: GET devolvió %T", v)
	}
	return s, true, nil
}

// Claim implementa Store.
func (r *Redis) Claim(ctx context.Context, key, owner string, ttl time.Duration) (string, error) {
	ms := strconv.FormatInt(ttl.Milliseconds(), 10)
	for attempt := 0; attempt < 2; attempt++ {
		v, err := r.do(ctx, "SET", key, owner, "NX", "PX", ms)
		if err != nil {
			return "", err
		}
		if v == "OK" {
			return owner, nil
		}
		cur, found, err := r.Get(ctx, key)
		if err != nil {
			return "", err
		}
		if found {
			if cur == owner {
				_ = r.Touch(ctx, key, ttl)
			}
			return cur, nil
		}
		// La clave caducó entre SET y GET: se reintenta una vez.
	}
	return "", errors.New("redis: no se pudo registrar la propiedad")
}

// Touch implementa Store.
func (r *Redis) Touch(ctx context.Context, key string, ttl time.Duration) error {
	_, err := r.do(ctx, "PEXPIRE", key, strconv.FormatInt(ttl.Milliseconds(), 10))
	return err
}

// Close implementa Store.
func (r *Redis) Close() error {
	for {
		select {
		case c := <-r.pool:
			_ = c.Close()
		default:
			return nil
		}
	}
}

func (r *Redis) do(ctx context.Context, args ...string) (interface{}, error) {
	c, err := r.get(ctx)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(r.cfg.Timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = c.SetDeadline(deadline)
	v, err := c.call(args...)
	if err != nil {
		var re redisError
		if !errors.As(err, &re) {
			// Error de red o de protocolo: la conexión no se reutiliza.
			_ = c.Close()
			return nil, err
		}
	}
	r.put(c)
	return v, err
}

func (r *Redis) get(ctx context.Context) (*redisConn, error) {
	select {
	case c := <-r.pool:
		return c, nil
	default:
	}
	d := net.Dialer{Timeout: r.cfg.Timeout}
	var conn net.Conn
	var err error
	if r.cfg.TLS {
		tc := r.cfg.TLSConfig
		if tc == nil {
			host, _, _ := net.SplitHostPort(r.cfg.Address)
			tc = &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		}
		conn, err = (&tls.Dialer{NetDialer: &d, Config: tc}).DialContext(ctx, "tcp", r.cfg.Address)
	} else {
		conn, err = d.DialContext(ctx, "tcp", r.cfg.Address)
	}
	if err != nil {
		return nil, fmt.Errorf("redis: no se puede conectar con %s: %w", r.cfg.Address, err)
	}
	c := &redisConn{Conn: conn, r: bufio.NewReader(conn), w: bufio.NewWriter(conn)}
	_ = c.SetDeadline(time.Now().Add(r.cfg.Timeout))
	if r.cfg.Password != "" {
		args := []string{"AUTH", r.cfg.Password}
		if r.cfg.Username != "" {
			args = []string{"AUTH", r.cfg.Username, r.cfg.Password}
		}
		if _, err := c.call(args...); err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("redis: autenticación rechazada: %w", err)
		}
	}
	if r.cfg.DB != 0 {
		if _, err := c.call("SELECT", strconv.Itoa(r.cfg.DB)); err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("redis: SELECT %d: %w", r.cfg.DB, err)
		}
	}
	return c, nil
}

func (r *Redis) put(c *redisConn) {
	select {
	case r.pool <- c:
	default:
		_ = c.Close()
	}
}

type redisError string

func (e redisError) Error() string { return "redis: " + string(e) }

func (c *redisConn) call(args ...string) (interface{}, error) {
	fmt.Fprintf(c.w, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(c.w, "$%d\r\n%s\r\n", len(a), a)
	}
	if err := c.w.Flush(); err != nil {
		return nil, err
	}
	return c.read()
}

func (c *redisConn) read() (interface{}, error) {
	line, err := c.r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(line, "\r\n")
	if line == "" {
		return nil, errors.New("redis: respuesta vacía")
	}
	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return nil, redisError(line[1:])
	case ':':
		return strconv.ParseInt(line[1:], 10, 64)
	case '$':
		return c.readBulk(line[1:])
	case '*':
		return c.readArray(line[1:])
	default:
		return nil, fmt.Errorf("redis: respuesta no reconocida %q", line)
	}
}

// readBulk lee una cadena RESP de la longitud indicada; -1 es nil.
func (c *redisConn) readBulk(size string) (interface{}, error) {
	n, err := strconv.Atoi(size)
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	buf := make([]byte, n+2)
	if _, err := io.ReadFull(c.r, buf); err != nil {
		return nil, err
	}
	return string(buf[:n]), nil
}

// readArray lee una lista RESP con el número de elementos indicado; -1 es nil.
func (c *redisConn) readArray(size string) (interface{}, error) {
	n, err := strconv.Atoi(size)
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	out := make([]interface{}, n)
	for i := range out {
		if out[i], err = c.read(); err != nil {
			return nil, err
		}
	}
	return out, nil
}
