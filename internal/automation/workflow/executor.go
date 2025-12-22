package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"

	"modern-dhcp/internal/automation"
)

// ExecutorOptions configures how workflow executions are simulated.
type ExecutorOptions struct {
	Logger       *zap.Logger
	TaskHandlers map[string]TaskHandler
}

// Executor replays workflow specs inside automation jobs for observability.
type Executor struct {
	logger       *zap.Logger
	taskHandlers map[string]TaskHandler
	handlerMu    sync.RWMutex
}

var _ automation.Handler = (*Executor)(nil)

// NewExecutor builds an Executor instance.
func NewExecutor(opts ExecutorOptions) *Executor {
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Executor{
		logger:       logger,
		taskHandlers: cloneTaskHandlers(opts.TaskHandlers),
	}
}

// TaskHandler runs workflow task nodes that reference domain-specific actions.
type TaskHandler func(context.Context, TaskContext) (TaskResult, error)

// TaskContext exposes workflow metadata to task handlers.
type TaskContext struct {
	JobID        string
	DefinitionID string
	Version      int
	TenantID     string
	TriggeredBy  string
	Labels       map[string]string
	Context      map[string]any
	Node         Node
}

// TaskResult surfaces structured task execution metadata.
type TaskResult struct {
	Summary  string
	Metadata map[string]any
}

// Handle implements automation.Handler so workflow executions can be dispatched through automation.Service.
func (e *Executor) Handle(ctx context.Context, job automation.Job) error {
	if len(job.Payload) == 0 {
		return errors.New("workflow: execution payload required")
	}
	var payload executionPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("workflow: decode execution payload: %w", err)
	}
	if err := payload.Spec.Validate(); err != nil {
		return fmt.Errorf("workflow: invalid spec: %w", err)
	}

	order, unreachable := buildTraversalPlan(payload.Spec)
	if len(order) == 0 {
		return errors.New("workflow: traversal produced no nodes")
	}

	e.logger.Info("workflow execution planned",
		zap.String("definitionId", payload.DefinitionID),
		zap.Int("version", payload.Version),
		zap.String("tenantId", payload.TenantID),
		zap.String("jobId", job.ID),
		zap.Int("nodes", len(order)),
	)

	for idx, node := range order {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := e.executeNode(ctx, payload, job, idx, node); err != nil {
			return err
		}
	}

	if len(unreachable) > 0 {
		e.logger.Warn("workflow contains unreachable nodes",
			zap.String("definitionId", payload.DefinitionID),
			zap.Strings("nodes", unreachable),
		)
	}

	e.logger.Info("workflow execution completed",
		zap.String("definitionId", payload.DefinitionID),
		zap.String("jobId", job.ID),
		zap.Int("steps", len(order)),
	)
	return nil
}

// RegisterTaskHandler attaches or replaces a task handler at runtime.
func (e *Executor) RegisterTaskHandler(action string, handler TaskHandler) {
	if e == nil || strings.TrimSpace(action) == "" || handler == nil {
		return
	}
	e.handlerMu.Lock()
	defer e.handlerMu.Unlock()
	if e.taskHandlers == nil {
		e.taskHandlers = make(map[string]TaskHandler)
	}
	e.taskHandlers[action] = handler
}

func (e *Executor) executeNode(ctx context.Context, payload executionPayload, job automation.Job, idx int, node Node) error {
	switch node.Type {
	case NodeTypeTask:
		if node.Task == nil || strings.TrimSpace(node.Task.Action) == "" {
			return fmt.Errorf("workflow: node %s missing task action", node.ID)
		}
		result, err := e.executeTask(ctx, payload, job, node)
		if err != nil {
			return err
		}
		fields := []zap.Field{
			zap.String("definitionId", payload.DefinitionID),
			zap.String("jobId", job.ID),
			zap.Int("step", idx+1),
			zap.String("nodeId", node.ID),
			zap.String("action", node.Task.Action),
		}
		if result.Summary != "" {
			fields = append(fields, zap.String("summary", result.Summary))
		}
		if len(result.Metadata) > 0 {
			fields = append(fields, zap.Any("metadata", result.Metadata))
		}
		e.logger.Info("workflow task completed", fields...)
	default:
		e.logger.Debug("workflow node simulated",
			zap.String("definitionId", payload.DefinitionID),
			zap.String("jobId", job.ID),
			zap.Int("step", idx+1),
			zap.String("nodeId", node.ID),
			zap.String("nodeType", string(node.Type)),
		)
	}
	return nil
}

func (e *Executor) executeTask(ctx context.Context, payload executionPayload, job automation.Job, node Node) (TaskResult, error) {
	handler, ok := e.lookupTaskHandler(strings.TrimSpace(node.Task.Action))
	if !ok {
		return TaskResult{}, fmt.Errorf("workflow: no handler registered for action %s", node.Task.Action)
	}
	taskCtx := TaskContext{
		JobID:        job.ID,
		DefinitionID: payload.DefinitionID,
		Version:      payload.Version,
		TenantID:     payload.TenantID,
		TriggeredBy:  payload.TriggeredBy,
		Labels:       cloneStringMap(payload.Labels),
		Context:      payload.Context,
		Node:         node,
	}
	return handler(ctx, taskCtx)
}

func (e *Executor) lookupTaskHandler(action string) (TaskHandler, bool) {
	e.handlerMu.RLock()
	defer e.handlerMu.RUnlock()
	if e.taskHandlers == nil {
		return nil, false
	}
	handler, ok := e.taskHandlers[action]
	return handler, ok
}

func cloneTaskHandlers(in map[string]TaskHandler) map[string]TaskHandler {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]TaskHandler, len(in))
	for action, handler := range in {
		if handler == nil || strings.TrimSpace(action) == "" {
			continue
		}
		out[action] = handler
	}
	return out
}

func buildTraversalPlan(spec Spec) ([]Node, []string) {
	adjacency := make(map[string][]string)
	for _, edge := range spec.Edges {
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
	}

	visited := make(map[string]struct{}, len(spec.Nodes))
	order := make([]Node, 0, len(spec.Nodes))
	queue := []string{spec.StartNode}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := visited[current]; ok {
			continue
		}
		node, ok := spec.Nodes[current]
		if !ok {
			continue
		}
		node.ID = strings.TrimSpace(node.ID)
		if node.ID == "" {
			node.ID = current
		}
		order = append(order, node)
		visited[current] = struct{}{}
		queue = append(queue, adjacency[current]...)
	}

	unreachable := make([]string, 0)
	for id := range spec.Nodes {
		if _, ok := visited[id]; !ok {
			unreachable = append(unreachable, id)
		}
	}
	sort.Strings(unreachable)
	return order, unreachable
}
