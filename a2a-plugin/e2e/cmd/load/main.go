// Copyright 2026 Los autores del plugin A2A para KrakenD.

// Prueba de carga del gateway A2A (tarea 10.3): peticiones unarias y flujos
// SSE concurrentes contra uno o varios nodos.
//
//	go run ./cmd/load -gw http://localhost:18080,http://localhost:18081 -idp http://localhost:18090
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"krakend-a2a.local/e2e/internal/secrets"
)

const headerContentType = "Content-Type"

func main() {
	gws := flag.String("gw", "http://localhost:18080", "nodos del gateway, separados por comas")
	idp := flag.String("idp", "http://localhost:18090", "IdP simulado")
	agent := flag.String("agent", "billing", "agente")
	duration := flag.Duration("duration", 20*time.Second, "duración")
	workers := flag.Int("concurrency", 16, "peticiones simultáneas")
	streamEvery := flag.Int("stream-every", 4, "uno de cada N es un flujo SSE")
	clientsFile := flag.String("clients", secrets.ClientsFile(),
		"clientes del IdP simulado que genera deploy/demo-secrets")
	flag.Parse()

	tok, err := token(*idp, *clientsFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	lt := &loadTest{
		nodes:       strings.Split(*gws, ","),
		agent:       *agent,
		token:       tok,
		streamEvery: *streamEvery,
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{MaxIdleConnsPerHost: *workers},
		},
	}
	st := lt.run(*duration, *workers)

	out, _ := json.MarshalIndent(st.summary(*duration, *workers, lt.nodes), "", "  ")
	fmt.Println(string(out))
	if st.failed() {
		os.Exit(1)
	}
}

// loadTest envía peticiones SendMessage y SendStreamingMessage al gateway.
type loadTest struct {
	nodes       []string
	agent       string
	token       string
	streamEvery int
	client      *http.Client
}

// stats acumula latencias y resultados por tipo de petición.
type stats struct {
	mu        sync.Mutex
	latencies map[string][]time.Duration
	statuses  map[string]int
}

func (lt *loadTest) run(d time.Duration, workers int) *stats {
	st := &stats{latencies: map[string][]time.Duration{}, statuses: map[string]int{}}
	var seq atomic.Int64
	deadline := time.Now().Add(d)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				lt.once(seq.Add(1), st)
			}
		}()
	}
	wg.Wait()
	return st
}

// once envía la petición número n y registra su resultado.
func (lt *loadTest) once(n int64, st *stats) {
	node := lt.nodes[int(n)%len(lt.nodes)]
	kind, method := "unaria", "SendMessage"
	if lt.streamEvery > 0 && n%int64(lt.streamEvery) == 0 {
		kind, method = "flujo", "SendStreamingMessage"
	}
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":{"message":{"messageId":"load-%d",`+
		`"role":"ROLE_USER","parts":[{"text":"carga"}]}}}`, n, method, n)
	req, _ := http.NewRequest(http.MethodPost, node+"/a2a/"+lt.agent, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+lt.token)
	req.Header.Set("A2A-Version", "1.0")
	req.Header.Set(headerContentType, "application/json")
	start := time.Now()
	result := lt.send(req, kind == "flujo")
	st.record(kind, result, time.Since(start))
}

// send hace la petición, lee la respuesta completa y la resume.
func (lt *loadTest) send(req *http.Request, stream bool) string {
	res, err := lt.client.Do(req)
	if err != nil {
		return "error-red"
	}
	defer res.Body.Close()
	result := fmt.Sprint(res.StatusCode)
	if stream {
		if countEvents(res.Body) == 0 {
			result += "-sin-eventos"
		}
		return result
	}
	b, _ := io.ReadAll(res.Body)
	if strings.Contains(string(b), `"error"`) {
		result += "-error-jsonrpc"
	}
	return result
}

func countEvents(r io.Reader) int {
	events := 0
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "data:") {
			events++
		}
	}
	return events
}

func (s *stats) record(kind, result string, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latencies[kind] = append(s.latencies[kind], d)
	s.statuses[kind+" "+result]++
}

func (s *stats) summary(d time.Duration, workers int, nodes []string) map[string]interface{} {
	summary := map[string]interface{}{
		"duracion":     d.String(),
		"concurrencia": workers,
		"nodos":        nodes,
		"resultados":   s.statuses,
	}
	for kind, l := range s.latencies {
		sort.Slice(l, func(i, j int) bool { return l[i] < l[j] })
		summary["latencia_"+kind] = map[string]string{
			"peticiones": fmt.Sprint(len(l)),
			"p50":        l[len(l)/2].Round(time.Millisecond).String(),
			"p95":        l[len(l)*95/100].Round(time.Millisecond).String(),
			"max":        l[len(l)-1].Round(time.Millisecond).String(),
		}
	}
	return summary
}

// failed indica si alguna petición no terminó con 200 y sin errores.
func (s *stats) failed() bool {
	for k := range s.statuses {
		_, result, _ := strings.Cut(k, " ")
		if !strings.HasPrefix(result, "200") || strings.Contains(k, "error") || strings.Contains(k, "sin-eventos") {
			return true
		}
	}
	return false
}

func token(idp, clientsFile string) (string, error) {
	secret, err := secrets.ClientSecret(clientsFile, "alice-app")
	if err != nil {
		return "", err
	}
	req, _ := http.NewRequest(http.MethodPost, idp+"/token", strings.NewReader("grant_type=client_credentials"))
	req.Header.Set(headerContentType, "application/x-www-form-urlencoded")
	req.SetBasicAuth("alice-app", secret)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var b struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&b); err != nil || b.AccessToken == "" {
		return "", fmt.Errorf("no se obtuvo token: %v", err)
	}
	return b.AccessToken, nil
}
