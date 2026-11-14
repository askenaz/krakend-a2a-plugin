// Copyright 2026 Los autores del plugin A2A para KrakenD.

package protocol

import (
	"net/http"
	"net/url"
	"testing"
)

// Nombres de cable tal como los define la especificación y el SDK oficial.
var wireNames = []struct {
	op  Operation
	v10 string
	v03 string
}{
	{SendMessage, "SendMessage", "message/send"},
	{SendStreamingMessage, "SendStreamingMessage", "message/stream"},
	{GetTask, "GetTask", "tasks/get"},
	{ListTasks, "ListTasks", ""},
	{CancelTask, "CancelTask", "tasks/cancel"},
	{SubscribeToTask, "SubscribeToTask", "tasks/resubscribe"},
	{CreateTaskPushNotificationConfig, "CreateTaskPushNotificationConfig", "tasks/pushNotificationConfig/set"},
	{GetTaskPushNotificationConfig, "GetTaskPushNotificationConfig", "tasks/pushNotificationConfig/get"},
	{ListTaskPushNotificationConfigs, "ListTaskPushNotificationConfigs", "tasks/pushNotificationConfig/list"},
	{DeleteTaskPushNotificationConfig, "DeleteTaskPushNotificationConfig", "tasks/pushNotificationConfig/delete"},
	{GetExtendedAgentCard, "GetExtendedAgentCard", "agent/getAuthenticatedExtendedCard"},
}

func TestOperationTableIsComplete(t *testing.T) {
	if len(Operations) != 11 || len(wireNames) != 11 {
		t.Fatalf("se esperaban 11 operaciones, hay %d", len(Operations))
	}
	for _, w := range wireNames {
		if w.op.String() != w.v10 {
			t.Errorf("%v: nombre canónico %q, se esperaba %q", w.op, w.op.String(), w.v10)
		}
		if got, ok := OperationByName(w.v10); !ok || got != w.op {
			t.Errorf("OperationByName(%q) = %v", w.v10, got)
		}
	}
}

func TestResolveMethod(t *testing.T) {
	for _, w := range wireNames {
		expectResolves(t, w.v10, V10, w.op)
		// Los nombres de una versión no existen en la otra: -32601.
		expectMethodNotFound(t, w.v10, V03)
		if w.v03 != "" {
			expectResolves(t, w.v03, V03, w.op)
			expectMethodNotFound(t, w.v03, V10)
		}
	}
	for _, m := range []string{"DoSomething", "sendmessage", "SendMessage ", "tasks/list", ""} {
		for _, v := range SupportedVersions {
			expectMethodNotFound(t, m, v)
		}
	}
}

func expectResolves(t *testing.T, method string, v Version, want Operation) {
	t.Helper()
	if op, err := ResolveMethod(method, v); err != nil || op != want {
		t.Errorf("%s %q: op=%v err=%v", v, method, op, err)
	}
}

func expectMethodNotFound(t *testing.T, method string, v Version) {
	t.Helper()
	if _, err := ResolveMethod(method, v); err == nil || err.Code != CodeMethodNotFound {
		t.Errorf("%q con %s debe ser -32601: %v", method, v, err)
	}
}

func TestWireNameRoundTrip(t *testing.T) {
	for _, w := range wireNames {
		if got, ok := w.op.WireName(V10); !ok || got != w.v10 {
			t.Errorf("%v.WireName(1.0) = %q %v", w.op, got, ok)
		}
		got, ok := w.op.WireName(V03)
		if ok != (w.v03 != "") || got != w.v03 {
			t.Errorf("%v.WireName(0.3) = %q %v", w.op, got, ok)
		}
	}
	if _, ok := OpUnknown.WireName(V10); ok {
		t.Error("OpUnknown no tiene nombre de cable")
	}
	if _, ok := SendMessage.WireName("9.9"); ok {
		t.Error("una versión desconocida no tiene nombres")
	}
}

func TestMayCreateTask(t *testing.T) {
	for _, op := range Operations {
		want := op == SendMessage || op == SendStreamingMessage
		if op.MayCreateTask() != want {
			t.Errorf("%v.MayCreateTask() = %v", op, op.MayCreateTask())
		}
	}
}

func TestOperationClasses(t *testing.T) {
	for _, op := range Operations {
		wantStream := op == SendStreamingMessage || op == SubscribeToTask
		if op.IsStreaming() != wantStream {
			t.Errorf("%v.IsStreaming() = %v", op, op.IsStreaming())
		}
		if (op.Class() == ClassStream) != wantStream {
			t.Errorf("%v.Class() = %v", op, op.Class())
		}
	}
	push := 0
	for _, op := range Operations {
		if op.IsPushConfig() {
			push++
		}
	}
	if push != 4 {
		t.Errorf("se esperaban 4 operaciones push, hay %d", push)
	}
}

func TestNegotiate(t *testing.T) {
	both := []Version{V10, V03}
	only10 := []Version{V10}
	only03 := []Version{V03}
	cases := []struct {
		name      string
		requested string
		enabled   []Version
		want      Version
		code      int
	}{
		{"1.0 habilitada", "1.0", both, V10, 0},
		{"0.3 habilitada", "0.3", both, V03, 0},
		{"sin versión se asume 0.3", "", both, V03, 0},
		{"parche ignorado", "1.0.2", both, V10, 0},
		{"espacios", " 1.0 ", both, V10, 0},
		{"sin versión y solo 1.0", "", only10, "", CodeVersionNotSupported},
		{"1.0 no habilitada", "1.0", only03, "", CodeVersionNotSupported},
		{"versión desconocida", "2.0", both, "", CodeVersionNotSupported},
		{"0.2 no soportada", "0.2", both, "", CodeVersionNotSupported},
		{"solo mayor", "1", both, "", CodeVersionNotSupported},
		{"texto", "latest", both, "", CodeVersionNotSupported},
		{"varios valores", "1.0,0.3", both, "", CodeVersionNotSupported},
		{"signo", "+1.0", both, "", CodeVersionNotSupported},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Negotiate(c.requested, c.enabled)
			if c.code != 0 {
				if err == nil || err.Code != c.code {
					t.Fatalf("se esperaba %d, obtenido %v %v", c.code, got, err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("obtenido %q %v, se esperaba %q", got, err, c.want)
			}
		})
	}
}

func TestRequestedVersion(t *testing.T) {
	h := http.Header{}
	q := url.Values{}
	if got := RequestedVersion(h, q); got != "" {
		t.Fatalf("sin cabecera ni consulta: %q", got)
	}
	q.Set("a2a-version", "1.0")
	if got := RequestedVersion(h, q); got != "1.0" {
		t.Fatalf("la consulta no distingue mayúsculas: %q", got)
	}
	h.Set("a2a-version", "0.3")
	if got := RequestedVersion(h, q); got != "0.3" {
		t.Fatalf("la cabecera tiene prioridad: %q", got)
	}
	h.Add("A2A-Version", "1.0")
	if got := RequestedVersion(h, q); got != "0.3,1.0" {
		t.Fatalf("valores distintos deben quedar visibles: %q", got)
	}
	h = http.Header{"A2a-Version": {"1.0", "1.0"}}
	if got := RequestedVersion(h, nil); got != "1.0" {
		t.Fatalf("valores repetidos iguales: %q", got)
	}
}
