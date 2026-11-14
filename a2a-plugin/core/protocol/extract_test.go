// Copyright 2026 Los autores del plugin A2A para KrakenD.

package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Las peticiones siguen la forma de la especificación (1.0 sección 9.4, 0.3
// sección 7) tal como la serializa el SDK oficial de Go: a2a-go v2.8.0 para
// 1.0 y su paquete de compatibilidad sobre a2a-go v0.3.15 para 0.3.

const msg10 = `{"messageId":"msg-1","role":"ROLE_USER","parts":[{"text":"hola"}],"taskId":"task-1","contextId":"ctx-1"}`
const msg03 = `{"kind":"message","messageId":"msg-1","role":"user","parts":[{"kind":"text","text":"hola"}],` +
	`"taskId":"task-1","contextId":"ctx-1"}`

func TestExtractRequest(t *testing.T) {
	cases := []struct {
		name   string
		op     Operation
		v      Version
		params string
		want   RequestRefs
	}{
		// SendMessage y SendStreamingMessage.
		{"1.0 SendMessage con tarea y contexto",
			SendMessage, V10, `{"tenant":"acme","message":` + msg10 + `}`,
			RequestRefs{TaskID: "task-1", ContextID: "ctx-1", Tenant: "acme"}},
		{"1.0 SendMessage nuevo, sin ids",
			SendMessage, V10, `{"message":{"messageId":"m","role":"ROLE_USER","parts":[{"text":"x"}]}}`, RequestRefs{}},
		{"1.0 SendMessage con webhook",
			SendMessage, V10,
			`{"message":` + msg10 + `,"configuration":{"returnImmediately":true,` +
				`"taskPushNotificationConfig":{"url":"https://hooks.example.com/a2a","token":"t",` +
				`"authentication":{"scheme":"Bearer","credentials":"c"}}}}`,
			RequestRefs{TaskID: "task-1", ContextID: "ctx-1", PushURLs: []string{"https://hooks.example.com/a2a"}}},
		{"1.0 SendStreamingMessage",
			SendStreamingMessage, V10, `{"message":` + msg10 + `}`, RequestRefs{TaskID: "task-1", ContextID: "ctx-1"}},
		{"0.3 message/send con webhook",
			SendMessage, V03,
			`{"message":` + msg03 + `,"configuration":{"blocking":false,` +
				`"pushNotificationConfig":{"url":"https://hooks.example.com/v03","authentication":{"schemes":["Bearer"]}}}}`,
			RequestRefs{TaskID: "task-1", ContextID: "ctx-1", PushURLs: []string{"https://hooks.example.com/v03"}}},
		{"0.3 message/stream",
			SendStreamingMessage, V03, `{"message":` + msg03 + `}`, RequestRefs{TaskID: "task-1", ContextID: "ctx-1"}},
		{"webhook con el campo de la otra versión también se ve",
			SendMessage, V10,
			`{"message":` + msg10 + `,"configuration":{"pushNotificationConfig":{"url":"http://169.254.169.254/"}}}`,
			RequestRefs{TaskID: "task-1", ContextID: "ctx-1", PushURLs: []string{"http://169.254.169.254/"}}},

		// Operaciones sobre una tarea.
		{"1.0 GetTask", GetTask, V10, `{"id":"task-9","historyLength":3}`, RequestRefs{TaskID: "task-9"}},
		{"0.3 tasks/get", GetTask, V03, `{"id":"task-9","historyLength":3}`, RequestRefs{TaskID: "task-9"}},
		{"1.0 CancelTask", CancelTask, V10, `{"id":"task-9","metadata":{}}`, RequestRefs{TaskID: "task-9"}},
		{"0.3 tasks/cancel", CancelTask, V03, `{"id":"task-9"}`, RequestRefs{TaskID: "task-9"}},
		{"1.0 SubscribeToTask", SubscribeToTask, V10, `{"id":"task-9"}`, RequestRefs{TaskID: "task-9"}},
		{"0.3 tasks/resubscribe", SubscribeToTask, V03, `{"id":"task-9"}`, RequestRefs{TaskID: "task-9"}},

		// ListTasks, solo en 1.0.
		{"1.0 ListTasks con contexto",
			ListTasks, V10, `{"contextId":"ctx-7","pageSize":10}`, RequestRefs{ContextID: "ctx-7"}},
		{"1.0 ListTasks sin params", ListTasks, V10, ``, RequestRefs{}},

		// Configuración de notificaciones push.
		{"1.0 Create push: objeto plano",
			CreateTaskPushNotificationConfig, V10, `{"taskId":"task-3","url":"https://hooks.example.com/p","token":"tok"}`,
			RequestRefs{TaskID: "task-3", PushURLs: []string{"https://hooks.example.com/p"}}},
		{"0.3 set push: url anidada",
			CreateTaskPushNotificationConfig, V03,
			`{"taskId":"task-3","pushNotificationConfig":{"id":"cfg-1","url":"https://hooks.example.com/p"}}`,
			RequestRefs{TaskID: "task-3", PushURLs: []string{"https://hooks.example.com/p"}}},
		{"1.0 Get push: taskId es la tarea, id la configuración",
			GetTaskPushNotificationConfig, V10, `{"taskId":"task-3","id":"cfg-1"}`, RequestRefs{TaskID: "task-3"}},
		{"0.3 get push: id es la tarea",
			GetTaskPushNotificationConfig, V03, `{"id":"task-3","pushNotificationConfigId":"cfg-1"}`,
			RequestRefs{TaskID: "task-3"}},
		{"1.0 List push",
			ListTaskPushNotificationConfigs, V10, `{"taskId":"task-3","pageSize":5}`, RequestRefs{TaskID: "task-3"}},
		{"0.3 list push", ListTaskPushNotificationConfigs, V03, `{"id":"task-3"}`, RequestRefs{TaskID: "task-3"}},
		{"1.0 Delete push",
			DeleteTaskPushNotificationConfig, V10, `{"taskId":"task-3","id":"cfg-1"}`, RequestRefs{TaskID: "task-3"}},
		{"0.3 delete push",
			DeleteTaskPushNotificationConfig, V03, `{"id":"task-3","pushNotificationConfigId":"cfg-1"}`,
			RequestRefs{TaskID: "task-3"}},

		// Card extendida.
		{"1.0 GetExtendedAgentCard sin params", GetExtendedAgentCard, V10, ``, RequestRefs{}},
		{"0.3 getAuthenticatedExtendedCard", GetExtendedAgentCard, V03, ``, RequestRefs{}},
		{"1.0 GetExtendedAgentCard con tenant", GetExtendedAgentCard, V10, `{"tenant":"acme"}`, RequestRefs{Tenant: "acme"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var params json.RawMessage
			if c.params != "" {
				params = json.RawMessage(c.params)
			}
			got, err := ExtractRequest(c.op, c.v, params)
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("\nobtenido %+v\nesperado %+v", got, c.want)
			}
		})
	}
}

func TestExtractRequestErrors(t *testing.T) {
	cases := []struct {
		name   string
		op     Operation
		v      Version
		params string
	}{
		{"GetTask sin params", GetTask, V10, ``},
		{"GetTask sin id", GetTask, V10, `{"historyLength":1}`},
		{"GetTask con id vacío", GetTask, V10, `{"id":""}`},
		{"GetTask con id numérico", GetTask, V10, `{"id":5}`},
		{"SendMessage sin message", SendMessage, V10, `{"configuration":{}}`},
		{"SendMessage con message no objeto", SendMessage, V10, `{"message":"hola"}`},
		{"taskId no es cadena", SendMessage, V10, `{"message":{"taskId":["a","b"]}}`},
		{"webhook sin url", SendMessage, V10, `{"message":{},"configuration":{"taskPushNotificationConfig":{"token":"t"}}}`},
		{"webhook con url no cadena",
			SendMessage, V03, `{"message":{},"configuration":{"pushNotificationConfig":{"url":1}}}`},
		{"Create push sin url", CreateTaskPushNotificationConfig, V10, `{"taskId":"t"}`},
		{"Create push sin tarea", CreateTaskPushNotificationConfig, V10, `{"url":"https://h.example.com"}`},
		{"0.3 get push con taskId en vez de id", GetTaskPushNotificationConfig, V03, `{"taskId":"t"}`},
		{"1.0 get push con id en vez de taskId", GetTaskPushNotificationConfig, V10, `{"id":"t"}`},
		{"claves que solo difieren en mayúsculas", GetTask, V10, `{"id":"mia","ID":"ajena"}`},
		{"repetición dentro de message", SendMessage, V10, `{"message":{"taskId":"mia","taskid":"ajena"}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var params json.RawMessage
			if c.params != "" {
				params = json.RawMessage(c.params)
			}
			_, err := ExtractRequest(c.op, c.v, params)
			if err == nil || err.Code != CodeInvalidParams {
				t.Fatalf("se esperaba -32602, obtenido %v", err)
			}
		})
	}
}

// Resultados y eventos con la forma que produce el SDK oficial.
func TestExtractResult(t *testing.T) {
	cases := []struct {
		name   string
		op     Operation
		result string
		want   ResultRefs
	}{
		{"1.0 SendMessage devuelve una tarea",
			SendMessage, `{"task":{"id":"task-1","contextId":"ctx-1","status":{"state":"TASK_STATE_SUBMITTED"}}}`,
			ResultRefs{KindTask, "task-1", "ctx-1"}},
		{"1.0 SendMessage devuelve un mensaje",
			SendMessage, `{"message":{"messageId":"m-2","role":"ROLE_AGENT","parts":[{"text":"ok"}],"contextId":"ctx-1"}}`,
			ResultRefs{KindMessage, "", "ctx-1"}},
		{"1.0 GetTask: tarea sin envoltorio",
			GetTask,
			`{"id":"task-1","contextId":"ctx-1","status":{"state":"TASK_STATE_COMPLETED"},"history":[{"messageId":"m",` +
				`"role":"ROLE_USER","parts":[]}]}`, ResultRefs{KindTask, "task-1", "ctx-1"}},
		{"1.0 evento statusUpdate",
			SendStreamingMessage,
			`{"statusUpdate":{"taskId":"task-1","contextId":"ctx-1","status":{"state":"TASK_STATE_WORKING"}}}`,
			ResultRefs{KindStatusUpdate, "task-1", "ctx-1"}},
		{"1.0 evento artifactUpdate",
			SubscribeToTask,
			`{"artifactUpdate":{"taskId":"task-1","contextId":"ctx-1","artifact":{"artifactId":"a","parts":[{"text":"x"}]},` +
				`"append":false,"lastChunk":true}}`, ResultRefs{KindArtifactUpdate, "task-1", "ctx-1"}},
		{"0.3 message/send devuelve una tarea",
			SendMessage, `{"kind":"task","id":"task-1","contextId":"ctx-1","status":{"state":"submitted"}}`,
			ResultRefs{KindTask, "task-1", "ctx-1"}},
		{"0.3 message/send devuelve un mensaje",
			SendMessage,
			`{"kind":"message","messageId":"m-2","role":"agent","parts":[{"kind":"text","text":"ok"}],"contextId":"ctx-1"}`,
			ResultRefs{KindMessage, "", "ctx-1"}},
		{"0.3 evento status-update",
			SendStreamingMessage,
			`{"kind":"status-update","taskId":"task-1","contextId":"ctx-1","status":{"state":"completed"},"final":true}`,
			ResultRefs{KindStatusUpdate, "task-1", "ctx-1"}},
		{"0.3 evento artifact-update",
			SendStreamingMessage,
			`{"kind":"artifact-update","taskId":"task-1","contextId":"ctx-1","artifact":{"artifactId":"a","parts":[]}}`,
			ResultRefs{KindArtifactUpdate, "task-1", "ctx-1"}},
		{"0.3 tasks/get",
			GetTask, `{"kind":"task","id":"task-1","contextId":"ctx-1","status":{"state":"working"}}`,
			ResultRefs{KindTask, "task-1", "ctx-1"}},
		{"0.3 sin kind: tarea",
			CancelTask, `{"id":"task-1","contextId":"ctx-1","status":{"state":"canceled"}}`,
			ResultRefs{KindTask, "task-1", "ctx-1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ExtractResult(c.op, json.RawMessage(c.result))
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if got != c.want {
				t.Fatalf("\nobtenido %+v\nesperado %+v", got, c.want)
			}
		})
	}
}

func TestExtractResultErrors(t *testing.T) {
	cases := []struct {
		name   string
		op     Operation
		result string
	}{
		{"no es objeto", SendMessage, `"x"`},
		{"forma desconocida", SendMessage, `{"foo":1}`},
		{"varios tipos a la vez", SendStreamingMessage, `{"task":{"id":"a","status":{}},"message":{"messageId":"m"}}`},
		{"kind desconocido", SendMessage, `{"kind":"thing","id":"a"}`},
		{"tarea sin id", SendMessage, `{"task":{"contextId":"c","status":{}}}`},
		{"evento sin taskId", SendStreamingMessage, `{"statusUpdate":{"contextId":"c","status":{}}}`},
		{"tipo no admitido por la operación", GetTask, `{"message":{"messageId":"m"}}`},
		{"id no es cadena", GetTask, `{"id":5,"status":{}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, err := ExtractResult(c.op, json.RawMessage(c.result)); err == nil {
				t.Fatalf("se esperaba error, obtenido %+v", got)
			}
		})
	}
}

func TestParseResponse(t *testing.T) {
	r, err := ParseResponse([]byte(`{"jsonrpc":"2.0","id":"s-1","result":{"statusUpdate":{"taskId":"t","contextId":"c",` +
		`"status":{}}}}`))
	if err != nil || r.Error != nil || string(r.ID) != `"s-1"` || len(r.Result) == 0 {
		t.Fatalf("respuesta correcta mal leída: %+v %v", r, err)
	}
	r, err = ParseResponse([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32002,"message":"Task not cancelable"}}`))
	if err != nil || r.Error == nil || r.Error.Code != CodeTaskNotCancelable {
		t.Fatalf("error del agente mal leído: %+v %v", r, err)
	}
	if _, err := ParseResponse([]byte(`{"jsonrpc":"2.0","id":1}`)); err == nil {
		t.Fatal("una respuesta sin result ni error debe rechazarse")
	}
}
