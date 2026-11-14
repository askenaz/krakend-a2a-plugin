// Copyright 2026 Los autores del plugin A2A para KrakenD.

package main

import (
	"fmt"
	"sync"
)

// krakendLogger es el logger que KrakenD inyecta con RegisterLogger.
type krakendLogger interface {
	Debug(v ...interface{})
	Info(v ...interface{})
	Warning(v ...interface{})
	Error(v ...interface{})
	Critical(v ...interface{})
	Fatal(v ...interface{})
}

type stdoutLogger struct{}

func (stdoutLogger) Debug(...interface{}) {
	// Sin el logger de KrakenD no se emite el nivel de depuración.
}
func (stdoutLogger) Info(v ...interface{})    { fmt.Println(append([]interface{}{"INFO"}, v...)...) }
func (stdoutLogger) Warning(v ...interface{}) { fmt.Println(append([]interface{}{"WARNING"}, v...)...) }
func (stdoutLogger) Error(v ...interface{})   { fmt.Println(append([]interface{}{"ERROR"}, v...)...) }
func (stdoutLogger) Critical(v ...interface{}) {
	fmt.Println(append([]interface{}{"CRITICAL"}, v...)...)
}
func (s stdoutLogger) Fatal(v ...interface{}) { s.Critical(v...) }

var (
	loggerMu sync.RWMutex
	logger   krakendLogger = stdoutLogger{}
)

func setLogger(v interface{}) {
	l, ok := v.(krakendLogger)
	if !ok {
		return
	}
	loggerMu.Lock()
	logger = joined{l}
	loggerMu.Unlock()
}

// joined concatena los argumentos: el logger de KrakenD los separa con espacios.
type joined struct{ l krakendLogger }

func (j joined) Debug(v ...interface{})    { j.l.Debug(fmt.Sprint(v...)) }
func (j joined) Info(v ...interface{})     { j.l.Info(fmt.Sprint(v...)) }
func (j joined) Warning(v ...interface{})  { j.l.Warning(fmt.Sprint(v...)) }
func (j joined) Error(v ...interface{})    { j.l.Error(fmt.Sprint(v...)) }
func (j joined) Critical(v ...interface{}) { j.l.Critical(fmt.Sprint(v...)) }
func (j joined) Fatal(v ...interface{})    { j.l.Fatal(fmt.Sprint(v...)) }

// log devuelve el logger vigente. Satisface gateway.Logger.
func log() krakendLogger {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	return logger
}
