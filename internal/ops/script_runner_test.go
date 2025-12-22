package ops

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestScriptRunnerCompletesRun(t *testing.T) {
	runner := NewScriptRunner(ScriptRunnerOptions{
		Enabled:        true,
		DefaultTimeout: time.Second,
		MaxConcurrent:  1,
		Catalog: []ScriptDescriptor{
			{Name: "cleanup", Description: "cleanup temp", Command: "echo"},
		},
	}, zap.NewNop(), WithCommandExecutor(func(ctx context.Context, command string, args []string, env map[string]string) (string, string, error) {
		return "ok", "", nil
	}))

	run, err := runner.Start(context.Background(), StartScriptRunInput{
		ScriptName:  "cleanup",
		RequestedBy: "tester",
		Args:        []string{"hello"},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}

	waitForStatus(t, runner, run.ID, ScriptRunSucceeded)

	runs := runner.List(5)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Stdout != "ok" {
		t.Fatalf("expected stdout ok, got %q", runs[0].Stdout)
	}
}

func TestScriptRunnerRoleEnforcement(t *testing.T) {
	runner := NewScriptRunner(ScriptRunnerOptions{
		Enabled:        true,
		DefaultTimeout: time.Second,
		MaxConcurrent:  1,
		Catalog: []ScriptDescriptor{
			{Name: "sync", Command: "echo", AllowedRoles: []string{"admin"}},
		},
	}, zap.NewNop(), WithCommandExecutor(func(ctx context.Context, command string, args []string, env map[string]string) (string, string, error) {
		return "", "", nil
	}))

	_, err := runner.Start(context.Background(), StartScriptRunInput{
		ScriptName:  "sync",
		RequestedBy: "bob",
		Role:        "viewer",
	})
	if err == nil {
		t.Fatalf("expected role enforcement error")
	}
	if err != ErrScriptRoleDenied {
		t.Fatalf("expected ErrScriptRoleDenied, got %v", err)
	}
}

func TestScriptRunnerRequiresApprovalFlow(t *testing.T) {
	executed := make(chan struct{}, 1)
	runner := NewScriptRunner(ScriptRunnerOptions{
		Enabled:        true,
		DefaultTimeout: time.Second,
		MaxConcurrent:  1,
		Catalog: []ScriptDescriptor{
			{Name: "danger", Command: "echo", RequiresApproval: true},
		},
	}, zap.NewNop(), WithCommandExecutor(func(ctx context.Context, command string, args []string, env map[string]string) (string, string, error) {
		executed <- struct{}{}
		return "ok", "", nil
	}))

	run, err := runner.Start(context.Background(), StartScriptRunInput{
		ScriptName:  "danger",
		RequestedBy: "analyst",
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if run.Status != ScriptRunPendingApproval {
		t.Fatalf("expected pendingApproval, got %s", run.Status)
	}
	select {
	case <-executed:
		t.Fatalf("script should not execute before approval")
	case <-time.After(50 * time.Millisecond):
	}

	approved, err := runner.Approve(run.ID, "alice", "looks good")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.ApprovedBy != "alice" {
		t.Fatalf("expected approver alice, got %s", approved.ApprovedBy)
	}
	waitForStatus(t, runner, run.ID, ScriptRunSucceeded)
	select {
	case <-executed:
	case <-time.After(time.Second):
		t.Fatalf("script never executed after approval")
	}
}

func TestScriptRunnerRejectsRun(t *testing.T) {
	executed := make(chan struct{}, 1)
	runner := NewScriptRunner(ScriptRunnerOptions{
		Enabled:        true,
		DefaultTimeout: time.Second,
		MaxConcurrent:  1,
		Catalog: []ScriptDescriptor{
			{Name: "danger", Command: "echo", RequiresApproval: true},
		},
	}, zap.NewNop(), WithCommandExecutor(func(ctx context.Context, command string, args []string, env map[string]string) (string, string, error) {
		executed <- struct{}{}
		return "ok", "", nil
	}))

	run, err := runner.Start(context.Background(), StartScriptRunInput{ScriptName: "danger", RequestedBy: "analyst"})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := runner.Reject(run.ID, "alice", "unsafe"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	runAfter, err := runner.Get(run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runAfter.Status != ScriptRunRejected {
		t.Fatalf("expected rejected status, got %s", runAfter.Status)
	}
	select {
	case <-executed:
		t.Fatalf("script should not execute after rejection")
	case <-time.After(50 * time.Millisecond):
	}
}

func waitForStatus(t *testing.T, runner *ScriptRunner, runID string, status ScriptRunStatus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := runner.Get(runID)
		if err == nil && run.Status == status {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach status %s", runID, status)
}
