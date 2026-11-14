// Copyright 2026 Los autores del plugin A2A para KrakenD.

package card

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"krakend-a2a.local/core/config"
)

// Logger es el subconjunto del logger de KrakenD que usa el paquete.
type Logger interface {
	Info(v ...interface{})
	Warning(v ...interface{})
	Error(v ...interface{})
}

// nopLogger descarta los mensajes cuando no se inyecta un logger.
type nopLogger struct{}

// Info descarta el mensaje.
func (nopLogger) Info(...interface{}) {
	// Sin logger no hay a dónde escribir.
}

// Warning descarta el mensaje.
func (nopLogger) Warning(...interface{}) {
	// Sin logger no hay a dónde escribir.
}

// Error descarta el mensaje.
func (nopLogger) Error(...interface{}) {
	// Sin logger no hay a dónde escribir.
}

// ErrUnavailable indica que no hay ninguna card publicable.
var ErrUnavailable = errors.New("agent card no disponible")

const (
	contentTypeJSON    = "application/json"
	headerCacheControl = "Cache-Control"
)

// Published es una card lista para servir.
type Published struct {
	Body   []byte
	ETag   string
	MaxAge time.Duration
}

// Manager gestiona la card de un agente.
type Manager struct {
	agentID     string
	transformer *Transformer
	source      string
	upstreamURL string
	ttl         time.Duration
	maxStale    time.Duration
	maxBytes    int64
	maxAge      time.Duration
	keys        *KeySet
	client      *http.Client
	now         func() time.Time
	log         Logger

	// Pausa tras un fallo antes de volver a contactar con el agente.
	failureBackoff time.Duration

	static *Published

	mu          sync.Mutex
	cur         *Published
	fetchedAt   time.Time
	upstreamTag string
	lastFailure time.Time
}

// Options ajusta un Manager; los campos vacíos usan valores por defecto.
type Options struct {
	StreamingAvailable bool
	HTTPClient         *http.Client
	Now                func() time.Time
	Logger             Logger
}

// NewManager prepara la card de un agente. Una card estática se carga y se
// transforma aquí, de modo que un error impide arrancar.
func NewManager(cfg *config.Config, a *config.Agent, opts Options) (*Manager, error) {
	m := &Manager{
		agentID:        a.ID,
		transformer:    NewTransformer(cfg, a, opts.StreamingAvailable),
		source:         a.Card.Source,
		upstreamURL:    a.Card.URL,
		ttl:            a.Card.TTL.D(),
		maxStale:       a.Card.MaxStale.D(),
		maxBytes:       a.Card.MaxBytes,
		maxAge:         a.Card.HTTPMaxAge.D(),
		client:         opts.HTTPClient,
		now:            opts.Now,
		log:            opts.Logger,
		failureBackoff: 5 * time.Second,
	}
	if m.client == nil {
		m.client = &http.Client{Timeout: 5 * time.Second}
	}
	if m.now == nil {
		m.now = time.Now
	}
	if m.log == nil {
		m.log = nopLogger{}
	}
	if a.Card.Signature.Verify {
		ks, err := LoadKeySet(a.Card.Signature.JWKSFile)
		if err != nil {
			return nil, fmt.Errorf("agents[%s].card.signature: %w", a.ID, err)
		}
		m.keys = ks
	}

	if m.source == config.CardStatic {
		raw := []byte(a.Card.Inline)
		if a.Card.File != "" {
			b, err := os.ReadFile(a.Card.File)
			if err != nil {
				return nil, fmt.Errorf("agents[%s].card.file: %w", a.ID, err)
			}
			raw = b
		}
		p, err := m.publish(raw)
		if err != nil {
			return nil, fmt.Errorf("agents[%s].card: %w", a.ID, err)
		}
		m.static = p
	}
	return m, nil
}

// Transformer devuelve el transformador del agente, que también se usa con el
// resultado de GetExtendedAgentCard.
func (m *Manager) Transformer() *Transformer { return m.transformer }

func (m *Manager) publish(raw []byte) (*Published, error) {
	body, err := m.transformer.Transform(raw)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	return &Published{Body: body, ETag: `"a2a-` + hex.EncodeToString(sum[:12]) + `"`, MaxAge: m.maxAge}, nil
}

// Get devuelve la card vigente. Con origen remoto, la renueva cuando vence el
// TTL; si el agente no responde sirve la última copia mientras no supere
// max_stale.
func (m *Manager) Get(ctx context.Context) (*Published, error) {
	if m.static != nil {
		return m.static, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	age := now.Sub(m.fetchedAt)
	if m.cur != nil && age < m.ttl {
		return m.cur, nil
	}
	if !m.lastFailure.IsZero() && now.Sub(m.lastFailure) < m.failureBackoff {
		return m.staleOr(age)
	}
	if err := m.refresh(ctx); err != nil {
		m.lastFailure = now
		m.log.Warning(fmt.Sprintf("[PLUGIN: a2a] agente=%s no se pudo obtener la agent card: %v", m.agentID, err))
		return m.staleOr(age)
	}
	m.lastFailure = time.Time{}
	return m.cur, nil
}

func (m *Manager) staleOr(age time.Duration) (*Published, error) {
	if m.cur != nil && age <= m.maxStale {
		return m.cur, nil
	}
	return nil, ErrUnavailable
}

func (m *Manager) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.upstreamURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", contentTypeJSON)
	if m.cur != nil && m.upstreamTag != "" {
		req.Header.Set("If-None-Match", m.upstreamTag)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified && m.cur != nil {
		m.fetchedAt = m.now()
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("el agente respondió %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, m.maxBytes+1))
	if err != nil {
		return err
	}
	if int64(len(raw)) > m.maxBytes {
		return fmt.Errorf("la card supera max_bytes (%d)", m.maxBytes)
	}
	if m.keys != nil {
		if err := m.keys.VerifyCard(raw); err != nil {
			return err
		}
	}
	p, err := m.publish(raw)
	if err != nil {
		return err
	}
	m.cur = p
	m.fetchedAt = m.now()
	m.upstreamTag = resp.Header.Get("ETag")
	return nil
}

// Prefetch intenta obtener la card en segundo plano al arrancar.
func (m *Manager) Prefetch(ctx context.Context) {
	if m.static != nil {
		return
	}
	go func() {
		if _, err := m.Get(ctx); err != nil {
			m.log.Warning(fmt.Sprintf("[PLUGIN: a2a] agente=%s agent card no disponible al arrancar; se reintentará con la "+
				"primera petición", m.agentID))
		}
	}()
}

// ServeHTTP sirve el descubrimiento de la card (spec a2a-agent-discovery).
func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p, err := m.Get(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", contentTypeJSON)
		w.Header().Set(headerCacheControl, "no-store")
		w.Header().Set("Retry-After", strconv.Itoa(int(m.failureBackoff.Seconds())))
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":"agent card unavailable"}`)
		return
	}
	h := w.Header()
	h.Set("ETag", p.ETag)
	if p.MaxAge > 0 {
		h.Set(headerCacheControl, "public, max-age="+strconv.Itoa(int(p.MaxAge.Seconds())))
	} else {
		h.Set(headerCacheControl, "no-cache")
	}
	if matchesETag(r.Header.Get("If-None-Match"), p.ETag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", contentTypeJSON)
	h.Set("Content-Length", strconv.Itoa(len(p.Body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(p.Body)
	}
}

func matchesETag(header, etag string) bool {
	if header == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || strings.TrimPrefix(part, "W/") == etag {
			return true
		}
	}
	return false
}
