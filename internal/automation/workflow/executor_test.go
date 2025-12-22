package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"modern-dhcp/internal/automation"
)

func TestBuildTraversalPlan(t *testing.T) {
	spec := Spec{
		StartNode: "start",
		Nodes: map[string]Node{
			"start":  {ID: "start", Type: NodeTypeStart},
			"finish": {ID: "finish", Type: NodeTypeEnd},
			"orph":   {ID: "orph", Type: NodeTypeWait, Wait: &WaitNode{Duration: 1}},
		},
		Edges: []Edge{
			{From: "start", To: "finish"},
		},
	}

	ordered, unreachable := buildTraversalPlan(spec)
	if len(ordered) != 2 {
		t.Fatalf("expected 2 reachable nodes, got %d", len(ordered))
	}
	if ordered[0].ID != "start" || ordered[1].ID != "finish" {
		t.Fatalf("unexpected traversal order: %+v", ordered)
	}
	if len(unreachable) != 1 || unreachable[0] != "orph" {
		t.Fatalf("expected orphaned node to be reported, got %+v", unreachable)
	}
}

func TestExecutorInvokesTaskHandler(t *testing.T) {
	var called bool
	exec := NewExecutor(ExecutorOptions{
		TaskHandlers: map[string]TaskHandler{
			"lease.reclaim": func(ctx context.Context, task TaskContext) (TaskResult, error) {
				called = true
				if task.Node.ID != "task" {
					return TaskResult{}, errors.New("unexpected node id")
				}
				return TaskResult{Summary: "ok"}, nil
			},
		},
	})

	job := automation.Job{ID: "job-1", Payload: mustExecutionPayload(t, executionPayload{
		DefinitionID: "wf",
		Version:      1,
		TenantID:     "tenant",
		Spec: Spec{
			StartNode: "start",
			Nodes: map[string]Node{
				"start": {ID: "start", Type: NodeTypeStart},
				"task": {
					ID:   "task",
					Type: NodeTypeTask,
					Task: &TaskNode{Action: "lease.reclaim"},
				},
			},
			Edges: []Edge{{From: "start", To: "task"}},
		},
	})}

	if err := exec.Handle(context.Background(), job); err != nil {
		t.Fatalf("executor returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected task handler to be invoked")
	}
}

func TestExecutorFailsWithoutHandler(t *testing.T) {
	exec := NewExecutor(ExecutorOptions{})
	job := automation.Job{ID: "job-2", Payload: mustExecutionPayload(t, executionPayload{
		DefinitionID: "wf",
		Version:      1,
		TenantID:     "tenant",
		Spec: Spec{
			StartNode: "start",
			Nodes: map[string]Node{
				"start": {ID: "start", Type: NodeTypeStart},
				"task": {
					ID:   "task",
					Type: NodeTypeTask,
					Task: &TaskNode{Action: "lease.reclaim"},
				},
			},
			Edges: []Edge{{From: "start", To: "task"}},
		},
	})}

	if err := exec.Handle(context.Background(), job); err == nil {
		t.Fatalf("expected missing handler error")
	}
}

func mustExecutionPayload(t *testing.T, payload executionPayload) []byte {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return data
}
