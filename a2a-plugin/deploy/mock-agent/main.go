// Copyright 2026 Los autores del plugin A2A para KrakenD.

// Agente A2A simulado para pruebas. No implementa A2A completo: responde lo
// justo para validar el gateway y registra cada petición recibida, de modo que
// las pruebas puedan comprobar si una petición llegó o no al agente.
//
// Variables de entorno:
//
//	PORT           puerto de escucha (8080)
//	STREAM_EVENTS  eventos por flujo SSE (3)
//	STREAM_DELAY   pausa entre eventos SSE (1500ms)
//	PUBLIC_URL     URL que anuncia la Agent Card (http://mock-agent:8080/)
//
// Rutas de administración, solo para pruebas:
//
//	GET  /__received  lista de peticiones A2A recibidas
//	POST /__reset     vacía la lista
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

type received struct {
	At         time.Time         `json:"at"`
	HTTPMethod string            `json:"httpMethod"`
	Path       string            `json:"path"`
	RPCMethod  string            `json:"rpcMethod"`
	BodySHA256 string            `json:"bodySha256"`
	BodyLen    int               `json:"bodyLen"`
	Headers    map[string]string `json:"headers"`
	HasAuthz   bool              `json:"hasAuthorizationHeader"`
}

// headerContentType es la cabecera del tipo de contenido.
const headerContentType = "Content-Type"

// Cabeceras que interesa observar en las pruebas.
var watched = []string{
	headerContentType, "A2A-Version", "A2A-Extensions",
	"X-A2A-Request-Id", "X-A2A-Operation", "X-A2A-Agent",
	"X-A2A-Sub", "X-A2A-Iss", "X-A2A-Scope",
}

var (
	mu   sync.Mutex
	log_ []received
	seq  int
)

func main() {
	port := env("PORT", "8080")
	events := envInt("STREAM_EVENTS", 3)
	delay := envDuration("STREAM_DELAY", 1500*time.Millisecond)
	publicURL := env("PUBLIC_URL", "http://mock-agent:8080/")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /__received", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		writeJSON(w, http.StatusOK, log_)
	})
	mux.HandleFunc("POST /__reset", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		log_ = nil
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /.well-known/agent-card.json", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, agentCard(publicURL))
	})
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		handleRPC(w, r, events, delay)
	})

	log.Printf("mock-agent escuchando en :%s (eventos=%d, pausa=%s)", port, events, delay)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func handleRPC(w http.ResponseWriter, r *http.Request, events int, delay time.Duration) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			ID      string `json:"id"`
			Message struct {
				TaskID    string `json:"taskId"`
				ContextID string `json:"contextId"`
			} `json:"message"`
		} `json:"params"`
	}
	_ = json.Unmarshal(body, &req)

	sum := sha256.Sum256(body)
	rec := received{
		At: time.Now().UTC(), HTTPMethod: r.Method, Path: r.URL.Path, RPCMethod: req.Method,
		BodySHA256: hex.EncodeToString(sum[:]), BodyLen: len(body),
		Headers: map[string]string{}, HasAuthz: r.Header.Get("Authorization") != "",
	}
	for _, h := range watched {
		if v := r.Header.Get(h); v != "" {
			rec.Headers[h] = v
		}
	}
	mu.Lock()
	log_ = append(log_, rec)
	seq++
	n := seq
	mu.Unlock()
	log.Printf("recibido %s %s rpc=%s bytes=%d", r.Method, r.URL.Path, req.Method, len(body))

	id := req.ID
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	taskID := fmt.Sprintf("task-%d", n)
	ctxID := req.Params.Message.ContextID
	if ctxID == "" {
		ctxID = fmt.Sprintf("ctx-%d", n)
	}

	switch req.Method {
	case "SendMessage", "message/send":
		writeJSON(w, http.StatusOK, rpcResult(id, map[string]any{"task": task(taskID, ctxID, "TASK_STATE_COMPLETED")}))
	case "GetTask", "tasks/get":
		writeJSON(w, http.StatusOK, rpcResult(id, task(req.Params.ID, ctxID, "TASK_STATE_COMPLETED")))
	case "SendStreamingMessage", "message/stream", "SubscribeToTask", "tasks/resubscribe":
		stream(w, id, taskID, ctxID, events, delay)
	default:
		writeJSON(w, http.StatusOK, map[string]any{
			"jsonrpc": "2.0", "id": id,
			"error": map[string]any{"code": -32601, "message": "Method not found"},
		})
	}
}

// stream emite eventos SSE espaciados. Cada evento lleva emittedAt para que las
// pruebas puedan distinguir una entrega incremental de una retenida.
func stream(w http.ResponseWriter, id json.RawMessage, taskID, ctxID string, events int, delay time.Duration) {
	flusher, _ := w.(http.Flusher)
	w.Header().Set(headerContentType, "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	for i := 0; i < events; i++ {
		var result map[string]any
		switch {
		case i == 0:
			result = map[string]any{"task": task(taskID, ctxID, "TASK_STATE_SUBMITTED")}
		case i == events-1:
			result = map[string]any{"statusUpdate": statusUpdate(taskID, ctxID, "TASK_STATE_COMPLETED")}
		default:
			result = map[string]any{"statusUpdate": statusUpdate(taskID, ctxID, "TASK_STATE_WORKING")}
		}
		result["emittedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
		result["seq"] = i
		b, _ := json.Marshal(rpcResult(id, result))
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			log.Printf("flujo %s cortado por el cliente en el evento %d", taskID, i)
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		log.Printf("flujo %s evento %d emitido", taskID, i)
		if i < events-1 {
			time.Sleep(delay)
		}
	}
}

func task(id, ctxID, state string) map[string]any {
	return map[string]any{"id": id, "contextId": ctxID, "status": map[string]any{"state": state}}
}

func statusUpdate(taskID, ctxID, state string) map[string]any {
	return map[string]any{"taskId": taskID, "contextId": ctxID, "status": map[string]any{"state": state}}
}

func rpcResult(id json.RawMessage, result any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
}

func agentCard(url string) map[string]any {
	return map[string]any{
		"name":        "Mock Agent",
		"description": "Agente simulado para pruebas del gateway A2A",
		"version":     "0.0.1",
		"supportedInterfaces": []any{
			map[string]any{"url": url, "protocolBinding": "JSONRPC", "protocolVersion": "1.0"},
		},
		"capabilities":       map[string]any{"streaming": true, "pushNotifications": false},
		"defaultInputModes":  []string{"text/plain"},
		"defaultOutputModes": []string{"text/plain"},
		"skills": []any{
			map[string]any{"id": "echo", "name": "Echo", "description": "Devuelve una tarea " +
				"completada", "tags": []string{"test"}},
		},
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set(headerContentType, "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(k)); err == nil && n > 0 {
		return n
	}
	return def
}

func envDuration(k string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(k)); err == nil && d > 0 {
		return d
	}
	return def
}
