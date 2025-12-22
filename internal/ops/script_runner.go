package ops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	// ErrScriptsDisabled indicates the script runner is not enabled.
	ErrScriptsDisabled = errors.New("ops: script runner disabled")
	// ErrScriptNotFound indicates the requested script name is unknown.
	ErrScriptNotFound = errors.New("ops: script not found")
	// ErrScriptRoleDenied indicates the caller's role is not authorized for the script.
	ErrScriptRoleDenied = errors.New("ops: role not allowed for script")
	// ErrScriptRunNotFound indicates the specific run ID was not found.
	ErrScriptRunNotFound = errors.New("ops: script run not found")
	// ErrScriptRunNotPendingApproval indicates an approval action was attempted on a run that is not awaiting approval.
	ErrScriptRunNotPendingApproval = errors.New("ops: script run not awaiting approval")
)

// ScriptRunStatus represents the lifecycle of a script execution request.
type ScriptRunStatus string

const (
	ScriptRunPendingApproval ScriptRunStatus = "pendingApproval"
	ScriptRunPending         ScriptRunStatus = "pending"
	ScriptRunRunning         ScriptRunStatus = "running"
	ScriptRunSucceeded       ScriptRunStatus = "succeeded"
	ScriptRunFailed          ScriptRunStatus = "failed"
	ScriptRunRejected        ScriptRunStatus = "rejected"
)

const (
	scriptRunnerHistorySize = 50
	maxLogBytes             = 64 * 1024
)

// ScriptRun captures metadata about a single script execution.
type ScriptRun struct {
	ID               string            `json:"id"`
	ScriptName       string            `json:"scriptName"`
	Description      string            `json:"description,omitempty"`
	RequestedBy      string            `json:"requestedBy"`
	Role             string            `json:"role,omitempty"`
	Reason           string            `json:"reason,omitempty"`
	RequiresApproval bool              `json:"requiresApproval"`
	ApprovedBy       string            `json:"approvedBy,omitempty"`
	ApprovedAt       time.Time         `json:"approvedAt,omitempty"`
	ApprovalNote     string            `json:"approvalNote,omitempty"`
	RejectedBy       string            `json:"rejectedBy,omitempty"`
	RejectedAt       time.Time         `json:"rejectedAt,omitempty"`
	RejectionNote    string            `json:"rejectionNote,omitempty"`
	Args             []string          `json:"args,omitempty"`
	ExtraArgs        []string          `json:"extraArgs,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	Status           ScriptRunStatus   `json:"status"`
	Stdout           string            `json:"stdout,omitempty"`
	Stderr           string            `json:"stderr,omitempty"`
	Error            string            `json:"error,omitempty"`
	CreatedAt        time.Time         `json:"createdAt"`
	StartedAt        time.Time         `json:"startedAt"`
	CompletedAt      time.Time         `json:"completedAt"`
	Duration         time.Duration     `json:"duration"`
	Timeout          time.Duration     `json:"timeout"`
	Command          string            `json:"-"`
}

// StartScriptRunInput describes a script execution request.
type StartScriptRunInput struct {
	ScriptName  string
	RequestedBy string
	Role        string
	Reason      string
	Args        []string
	Env         map[string]string
	Timeout     time.Duration
}

type commandExecutor func(ctx context.Context, command string, args []string, env map[string]string) (stdout string, stderr string, err error)

// ScriptRunnerOption customizes runner construction.
type ScriptRunnerOption func(*ScriptRunner)

// WithCommandExecutor overrides the default process executor (useful for tests).
func WithCommandExecutor(exec commandExecutor) ScriptRunnerOption {
	return func(r *ScriptRunner) {
		r.exec = exec
	}
}

// ScriptRunner coordinates sandboxed script executions.
type ScriptRunner struct {
	opts        ScriptRunnerOptions
	logger      *zap.Logger
	semaphore   chan struct{}
	runsMu      sync.RWMutex
	runs        map[string]*ScriptRun
	order       []string
	catalog     map[string]ScriptDescriptor
	exec        commandExecutor
	historySize int
}

// NewScriptRunner builds a new script runner with concurrency controls.
func NewScriptRunner(opts ScriptRunnerOptions, logger *zap.Logger, runnerOpts ...ScriptRunnerOption) *ScriptRunner {
	if logger == nil {
		logger = zap.NewNop()
	}
	normalized := normalizeScriptRunnerOptions(opts)
	runner := &ScriptRunner{
		opts:        normalized,
		logger:      logger.Named("script-runner"),
		runs:        make(map[string]*ScriptRun),
		order:       make([]string, 0, scriptRunnerHistorySize),
		catalog:     buildScriptCatalog(normalized.Catalog),
		historySize: scriptRunnerHistorySize,
		exec:        defaultCommandExecutor,
	}
	if normalized.MaxConcurrent > 0 {
		runner.semaphore = make(chan struct{}, normalized.MaxConcurrent)
	}
	for _, opt := range runnerOpts {
		if opt != nil {
			opt(runner)
		}
	}
	return runner
}

// Start launches a new script run asynchronously.
func (r *ScriptRunner) Start(ctx context.Context, in StartScriptRunInput) (*ScriptRun, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	scriptName := strings.TrimSpace(in.ScriptName)
	if scriptName == "" {
		return nil, errors.New("ops: script name required")
	}
	descriptor, ok := r.catalog[strings.ToLower(scriptName)]
	if !ok {
		return nil, ErrScriptNotFound
	}
	if len(descriptor.AllowedRoles) > 0 && !roleAllowed(strings.ToLower(in.Role), descriptor.AllowedRoles) {
		return nil, ErrScriptRoleDenied
	}
	if strings.TrimSpace(descriptor.Command) == "" {
		return nil, errors.New("ops: script command missing")
	}
	timeout := in.Timeout
	if timeout <= 0 {
		timeout = r.opts.DefaultTimeout
	}
	status := ScriptRunPending
	if descriptor.RequiresApproval {
		status = ScriptRunPendingApproval
	}
	run := &ScriptRun{
		ID:               uuid.NewString(),
		ScriptName:       descriptor.Name,
		Description:      descriptor.Description,
		RequestedBy:      strings.TrimSpace(in.RequestedBy),
		Role:             strings.TrimSpace(in.Role),
		Reason:           strings.TrimSpace(in.Reason),
		RequiresApproval: descriptor.RequiresApproval,
		Args:             append([]string(nil), descriptor.Args...),
		ExtraArgs:        append([]string(nil), in.Args...),
		Env:              cloneStringMap(in.Env),
		Status:           status,
		CreatedAt:        time.Now().UTC(),
		Timeout:          timeout,
		Command:          descriptor.Command,
	}
	run.Args = append(run.Args, run.ExtraArgs...)
	r.storeRun(run)
	if descriptor.RequiresApproval {
		cloned := run.clone()
		return &cloned, nil
	}
	r.triggerExecution(run.ID)
	cloned := run.clone()
	return &cloned, nil
}

// List returns the most recent script runs up to the requested limit.
func (r *ScriptRunner) List(limit int) []ScriptRun {
	r.runsMu.RLock()
	defer r.runsMu.RUnlock()
	if limit <= 0 || limit > len(r.order) {
		limit = len(r.order)
	}
	result := make([]ScriptRun, 0, limit)
	for i := 0; i < limit; i++ {
		runID := r.order[i]
		if run, ok := r.runs[runID]; ok {
			result = append(result, run.clone())
		}
	}
	return result
}

// Get returns a specific script run by ID.
func (r *ScriptRunner) Get(runID string) (*ScriptRun, error) {
	r.runsMu.RLock()
	defer r.runsMu.RUnlock()
	run, ok := r.runs[runID]
	if !ok {
		return nil, ErrScriptRunNotFound
	}
	cloned := run.clone()
	return &cloned, nil
}

// Approve transitions a pending approval run into the execution queue.
func (r *ScriptRunner) Approve(runID, approver, note string) (*ScriptRun, error) {
	if strings.TrimSpace(approver) == "" {
		return nil, errors.New("ops: approver required")
	}
	var cloned ScriptRun
	var trigger bool
	r.runsMu.Lock()
	defer func() {
		r.runsMu.Unlock()
		if trigger {
			r.triggerExecution(runID)
		}
	}()
	run, ok := r.runs[runID]
	if !ok {
		return nil, ErrScriptRunNotFound
	}
	if !run.RequiresApproval || run.Status != ScriptRunPendingApproval {
		return nil, ErrScriptRunNotPendingApproval
	}
	run.ApprovedBy = strings.TrimSpace(approver)
	run.ApprovedAt = time.Now().UTC()
	run.ApprovalNote = strings.TrimSpace(note)
	run.Status = ScriptRunPending
	cloned = run.clone()
	trigger = true
	return &cloned, nil
}

// Reject marks a pending approval run as rejected without executing it.
func (r *ScriptRunner) Reject(runID, approver, note string) (*ScriptRun, error) {
	if strings.TrimSpace(approver) == "" {
		return nil, errors.New("ops: approver required")
	}
	r.runsMu.Lock()
	defer r.runsMu.Unlock()
	run, ok := r.runs[runID]
	if !ok {
		return nil, ErrScriptRunNotFound
	}
	if !run.RequiresApproval || run.Status != ScriptRunPendingApproval {
		return nil, ErrScriptRunNotPendingApproval
	}
	now := time.Now().UTC()
	run.RejectedBy = strings.TrimSpace(approver)
	run.RejectedAt = now
	run.RejectionNote = strings.TrimSpace(note)
	run.Status = ScriptRunRejected
	run.CompletedAt = now
	run.Error = "rejected"
	run.Duration = 0
	cloned := run.clone()
	return &cloned, nil
}

func (r *ScriptRunner) execute(run *ScriptRun) {
	if r.semaphore != nil {
		r.semaphore <- struct{}{}
		defer func() { <-r.semaphore }()
	}

	start := time.Now().UTC()
	r.updateRun(run.ID, func(stored *ScriptRun) {
		stored.Status = ScriptRunRunning
		stored.StartedAt = start
	})

	execCtx, cancel := context.WithTimeout(context.Background(), run.Timeout)
	defer cancel()

	stdout, stderr, err := r.exec(execCtx, run.Command, run.Args, run.Env)
	completed := time.Now().UTC()
	status := ScriptRunSucceeded
	errMsg := ""
	if err != nil {
		status = ScriptRunFailed
		errMsg = err.Error()
	}
	duration := completed.Sub(start)

	r.updateRun(run.ID, func(stored *ScriptRun) {
		stored.Status = status
		stored.Stdout = truncateLog(stdout)
		stored.Stderr = truncateLog(stderr)
		stored.Error = errMsg
		stored.CompletedAt = completed
		stored.Duration = duration
	})

	r.logger.Info("script run finished",
		zap.String("runId", run.ID),
		zap.String("script", run.ScriptName),
		zap.String("status", string(status)),
		zap.Duration("duration", duration),
		zap.String("error", errMsg),
	)
}

func (r *ScriptRunner) storeRun(run *ScriptRun) {
	r.runsMu.Lock()
	defer r.runsMu.Unlock()
	r.runs[run.ID] = run
	r.order = append([]string{run.ID}, r.order...)
	if len(r.order) > r.historySize {
		stale := r.order[r.historySize:]
		r.order = r.order[:r.historySize]
		for _, id := range stale {
			delete(r.runs, id)
		}
	}
}

func (r *ScriptRunner) triggerExecution(runID string) {
	r.runsMu.RLock()
	run, ok := r.runs[runID]
	r.runsMu.RUnlock()
	if !ok {
		return
	}
	go r.execute(run)
}

func (r *ScriptRunner) updateRun(runID string, fn func(*ScriptRun)) {
	r.runsMu.Lock()
	defer r.runsMu.Unlock()
	if run, ok := r.runs[runID]; ok {
		fn(run)
	}
}

func (run *ScriptRun) clone() ScriptRun {
	if run == nil {
		return ScriptRun{}
	}
	out := *run
	if len(run.Args) > 0 {
		out.Args = append([]string(nil), run.Args...)
	}
	if len(run.ExtraArgs) > 0 {
		out.ExtraArgs = append([]string(nil), run.ExtraArgs...)
	}
	out.Env = cloneStringMap(run.Env)
	return out
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func roleAllowed(role string, allowed []string) bool {
	if role == "" {
		return false
	}
	for _, candidate := range allowed {
		if strings.EqualFold(strings.TrimSpace(candidate), role) {
			return true
		}
	}
	return false
}

func normalizeScriptRunnerOptions(opts ScriptRunnerOptions) ScriptRunnerOptions {
	normalized := opts
	if normalized.DefaultTimeout <= 0 {
		normalized.DefaultTimeout = 2 * time.Minute
	}
	if normalized.MaxConcurrent <= 0 {
		normalized.MaxConcurrent = 2
	}
	return normalized
}

func buildScriptCatalog(entries []ScriptDescriptor) map[string]ScriptDescriptor {
	catalog := make(map[string]ScriptDescriptor, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.Name) == "" {
			continue
		}
		copyEntry := entry
		copyEntry.Args = append([]string(nil), entry.Args...)
		copyEntry.AllowedRoles = append([]string(nil), entry.AllowedRoles...)
		catalog[strings.ToLower(entry.Name)] = copyEntry
	}
	return catalog
}

func truncateLog(out string) string {
	if len(out) <= maxLogBytes {
		return out
	}
	return out[:maxLogBytes]
}

func defaultCommandExecutor(ctx context.Context, command string, args []string, env map[string]string) (string, string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), formatEnv(env)...)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func formatEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	vars := make([]string, 0, len(env))
	for _, key := range keys {
		vars = append(vars, fmt.Sprintf("%s=%s", key, env[key]))
	}
	return vars
}
