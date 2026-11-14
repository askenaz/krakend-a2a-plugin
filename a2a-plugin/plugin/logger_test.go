// Copyright 2026 Los autores del plugin A2A para KrakenD.

package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// recLogger guarda cada llamada como "NIVEL|argumento1|argumento2...".
type recLogger struct{ calls []string }

func (r *recLogger) rec(level string, v []interface{}) {
	parts := []string{level}
	for _, x := range v {
		parts = append(parts, fmt.Sprint(x))
	}
	r.calls = append(r.calls, strings.Join(parts, "|"))
}

func (r *recLogger) Debug(v ...interface{})    { r.rec("DEBUG", v) }
func (r *recLogger) Info(v ...interface{})     { r.rec("INFO", v) }
func (r *recLogger) Warning(v ...interface{})  { r.rec("WARNING", v) }
func (r *recLogger) Error(v ...interface{})    { r.rec("ERROR", v) }
func (r *recLogger) Critical(v ...interface{}) { r.rec("CRITICAL", v) }
func (r *recLogger) Fatal(v ...interface{})    { r.rec("FATAL", v) }

// El logger de KrakenD separa los argumentos con espacios, así que el plugin
// le entrega cada mensaje ya unido en un solo argumento.
func TestSetLoggerJoinsArguments(t *testing.T) {
	previous := log()
	defer func() {
		loggerMu.Lock()
		logger = previous
		loggerMu.Unlock()
	}()

	setLogger("no es un logger")
	if log() != previous {
		t.Fatal("un valor que no es un logger no debe sustituir al actual")
	}

	r := &recLogger{}
	setLogger(r)
	l := log()
	l.Debug("a", 1)
	l.Info("a", 1)
	l.Warning("a", 1)
	l.Error("a", 1)
	l.Critical("a", 1)
	l.Fatal("a", 1)
	want := []string{"DEBUG|a1", "INFO|a1", "WARNING|a1", "ERROR|a1", "CRITICAL|a1", "FATAL|a1"}
	if strings.Join(r.calls, ",") != strings.Join(want, ",") {
		t.Fatalf("llamadas = %v, se esperaba %v", r.calls, want)
	}
}

// Sin logger de KrakenD, los mensajes salen por la salida estándar, salvo los
// de depuración.
func TestStdoutLogger(t *testing.T) {
	out := captureStdout(t, func() {
		var s stdoutLogger
		s.Debug("sin efecto")
		s.Info("prueba de info")
		s.Warning("prueba de aviso")
		s.Error("prueba de error")
		s.Critical("prueba crítica")
		s.Fatal("prueba fatal")
	})
	want := "INFO prueba de info\nWARNING prueba de aviso\nERROR prueba de error\n" +
		"CRITICAL prueba crítica\nCRITICAL prueba fatal\n"
	if out != want {
		t.Fatalf("salida = %q, se esperaba %q", out, want)
	}
}

func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = stdout }()
	f()
	_ = w.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
