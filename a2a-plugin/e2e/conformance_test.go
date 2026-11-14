// Copyright 2026 Los autores del plugin A2A para KrakenD.

package e2e

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

// Tarea 10.1: las once operaciones de A2A 1.0 y sus equivalentes de 0.3, con el
// cliente oficial del SDK, a través del gateway, contra el agente del SDK.
func TestConformanceSDK(t *testing.T) {
	e := env(t)
	for _, version := range []string{"1.0", "0.3"} {
		t.Run("A2A "+version, func(t *testing.T) {
			c := sdkClient(t, e.gw, "billing", token(t, "alice-app"), version)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			task := checkSendAndGet(t, ctx, c)
			if version == "1.0" {
				checkListTasks(t, ctx, c, task)
			}
			checkStreaming(t, ctx, c)
			checkSubscribe(t, ctx, c)
			checkCancel(t, ctx, c)
			checkPushConfigs(t, ctx, c, task.ID)
			checkExtendedCard(t, ctx, c)
		})
	}
}

// checkSendAndGet: SendMessage / message/send y GetTask / tasks/get.
func checkSendAndGet(t *testing.T, ctx context.Context, c *a2aclient.Client) *a2a.Task {
	t.Helper()
	res, err := c.SendMessage(ctx, &a2a.SendMessageRequest{Message: userMessage(nil)})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	task, ok := res.(*a2a.Task)
	if !ok || task.Status.State != a2a.TaskStateCompleted {
		t.Fatalf("SendMessage: resultado inesperado %T %+v", res, res)
	}
	got, err := c.GetTask(ctx, &a2a.GetTaskRequest{ID: task.ID})
	if err != nil || got.ID != task.ID {
		t.Fatalf("GetTask: %v", err)
	}
	return task
}

// checkListTasks: ListTasks, que solo existe en 1.0.
func checkListTasks(t *testing.T, ctx context.Context, c *a2aclient.Client, task *a2a.Task) {
	t.Helper()
	list, err := c.ListTasks(ctx, &a2a.ListTasksRequest{ContextID: task.ContextID})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	found := false
	for _, lt := range list.Tasks {
		found = found || lt.ID == task.ID
	}
	if !found {
		t.Errorf("ListTasks no incluye la tarea del contexto")
	}
}

// checkStreaming: SendStreamingMessage / message/stream, con eventos espaciados.
func checkStreaming(t *testing.T, ctx context.Context, c *a2aclient.Client) {
	t.Helper()
	start := time.Now()
	var arrivals []time.Duration
	var streamTask a2a.TaskID
	req := &a2a.SendMessageRequest{Message: userMessage(map[string]any{"delayMs": 400})}
	for ev, err := range c.SendStreamingMessage(ctx, req) {
		if err != nil {
			t.Fatalf("SendStreamingMessage: %v", err)
		}
		arrivals = append(arrivals, time.Since(start))
		if tk, ok := ev.(*a2a.Task); ok {
			streamTask = tk.ID
		}
	}
	if len(arrivals) < 3 || arrivals[len(arrivals)-1]-arrivals[0] < 700*time.Millisecond {
		t.Errorf("los eventos deben llegar según se emiten: %v", arrivals)
	}
	if streamTask == "" {
		t.Error("el primer evento debe ser la tarea")
	}
}

// checkSubscribe: SubscribeToTask / tasks/resubscribe sobre una tarea abierta.
func checkSubscribe(t *testing.T, ctx context.Context, c *a2aclient.Client) {
	t.Helper()
	held := heldTask(t, ctx, c, 1500)
	n := 0
	for _, err := range c.SubscribeToTask(ctx, &a2a.SubscribeToTaskRequest{ID: held}) {
		if err != nil {
			t.Fatalf("SubscribeToTask: %v", err)
		}
		n++
	}
	if n == 0 {
		t.Error("SubscribeToTask no entregó eventos")
	}
}

// checkCancel: CancelTask / tasks/cancel.
func checkCancel(t *testing.T, ctx context.Context, c *a2aclient.Client) {
	t.Helper()
	toCancel := heldTask(t, ctx, c, 5000)
	canceled, err := c.CancelTask(ctx, &a2a.CancelTaskRequest{ID: toCancel})
	if err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	if canceled.Status.State != a2a.TaskStateCanceled {
		t.Errorf("CancelTask: estado %s", canceled.Status.State)
	}
}

// checkPushConfigs: configuración de notificaciones push; crear, leer, listar
// y borrar.
func checkPushConfigs(t *testing.T, ctx context.Context, c *a2aclient.Client, taskID a2a.TaskID) {
	t.Helper()
	cfg, err := c.CreateTaskPushConfig(ctx, &a2a.PushConfig{
		TaskID: taskID,
		ID:     "cfg-e2e",
		URL:    "https://hooks.example.com/a2a",
	})
	if err != nil {
		t.Fatalf("CreateTaskPushNotificationConfig: %v", err)
	}
	if _, err := c.GetTaskPushConfig(ctx, &a2a.GetTaskPushConfigRequest{TaskID: taskID, ID: cfg.ID}); err != nil {
		t.Fatalf("GetTaskPushNotificationConfig: %v", err)
	}
	cfgs, err := c.ListTaskPushConfigs(ctx, &a2a.ListTaskPushConfigRequest{TaskID: taskID})
	if err != nil || len(cfgs) == 0 {
		t.Fatalf("ListTaskPushNotificationConfigs: %v %d", err, len(cfgs))
	}
	if err := c.DeleteTaskPushConfig(ctx, &a2a.DeleteTaskPushConfigRequest{TaskID: taskID, ID: cfg.ID}); err != nil {
		t.Fatalf("DeleteTaskPushNotificationConfig: %v", err)
	}
}

// checkExtendedCard: GetExtendedAgentCard / agent/getAuthenticatedExtendedCard,
// reescrita por el gateway.
func checkExtendedCard(t *testing.T, ctx context.Context, c *a2aclient.Client) {
	t.Helper()
	ext, err := c.GetExtendedAgentCard(ctx, &a2a.GetExtendedAgentCardRequest{})
	if err != nil {
		t.Fatalf("GetExtendedAgentCard: %v", err)
	}
	if !strings.Contains(ext.Name, "extendida") {
		t.Errorf("card extendida inesperada: %s", ext.Name)
	}
	for _, i := range ext.SupportedInterfaces {
		if strings.Contains(i.URL, "billing:9001") {
			t.Errorf("la card extendida filtra una URL interna: %s", i.URL)
		}
	}
	if strings.Contains(ext.DocumentationURL, "billing:9001") {
		t.Errorf("la card extendida filtra documentationUrl interna: %s", ext.DocumentationURL)
	}
}

// heldTask crea una tarea que sigue abierta holdMs milisegundos.
func heldTask(t *testing.T, ctx context.Context, c *a2aclient.Client, holdMs int) a2a.TaskID {
	t.Helper()
	res, err := c.SendMessage(ctx, &a2a.SendMessageRequest{
		Message: userMessage(map[string]any{"holdMs": holdMs}),
		Config:  &a2a.SendMessageConfig{ReturnImmediately: true},
	})
	if err != nil {
		t.Fatalf("SendMessage con returnImmediately: %v", err)
	}
	task, ok := res.(*a2a.Task)
	if !ok {
		t.Fatalf("se esperaba una tarea, llegó %T", res)
	}
	return task.ID
}

// Otro principal no puede ver la tarea, ni con el cliente del SDK.
func TestConformanceOwnershipWithSDK(t *testing.T) {
	e := env(t)
	ctx := context.Background()
	alice := sdkClient(t, e.gw, "billing", token(t, "alice-app"), "1.0")
	bob := sdkClient(t, e.gw, "billing", token(t, "bob-app"), "1.0")
	res, err := alice.SendMessage(ctx, &a2a.SendMessageRequest{Message: userMessage(nil)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = bob.GetTask(ctx, &a2a.GetTaskRequest{ID: res.(*a2a.Task).ID})
	if !errors.Is(err, a2a.ErrTaskNotFound) {
		t.Fatalf("el SDK debe ver TaskNotFound para una tarea ajena: %v", err)
	}
}
