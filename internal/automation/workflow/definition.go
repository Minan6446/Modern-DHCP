package workflow

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// DefinitionStatus reflects the lifecycle of a workflow definition.
type DefinitionStatus string

const (
	// DefinitionStatusDraft indicates the workflow can be edited but not executed yet.
	DefinitionStatusDraft DefinitionStatus = "draft"
	// DefinitionStatusActive indicates the workflow is available for execution.
	DefinitionStatusActive DefinitionStatus = "active"
	// DefinitionStatusArchived indicates the workflow is read-only for audit/history.
	DefinitionStatusArchived DefinitionStatus = "archived"
)

// Definition captures the persisted workflow metadata and specification.
type Definition struct {
	ID          string            `json:"id"`
	Version     int               `json:"version"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Labels      map[string]string `json:"labels,omitempty"`
	Spec        Spec              `json:"spec"`
	Status      DefinitionStatus  `json:"status"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// Spec describes the workflow graph using a simple DSL.
type Spec struct {
	StartNode string            `json:"startNode"`
	Nodes     map[string]Node   `json:"nodes"`
	Edges     []Edge            `json:"edges"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// NodeType enumerates supported workflow node kinds.
type NodeType string

const (
	NodeTypeTask     NodeType = "task"
	NodeTypeApproval NodeType = "approval"
	NodeTypeWait     NodeType = "wait"
	NodeTypeGateway  NodeType = "gateway"
	NodeTypeStart    NodeType = "start"
	NodeTypeEnd      NodeType = "end"
)

// Node describes an individual workflow step with optional typed payloads.
type Node struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Type            NodeType          `json:"type"`
	Task            *TaskNode         `json:"task,omitempty"`
	Approval        *ApprovalNode     `json:"approval,omitempty"`
	Wait            *WaitNode         `json:"wait,omitempty"`
	Gateway         *GatewayNode      `json:"gateway,omitempty"`
	ContinueOnError bool              `json:"continueOnError"`
	Timeout         time.Duration     `json:"timeout,omitempty"`
	Retries         int               `json:"retries,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// TaskNode invokes an automation action (existing job handlers).
type TaskNode struct {
	Action   string            `json:"action"`
	TenantID string            `json:"tenantId,omitempty"`
	Params   map[string]any    `json:"params,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// ApprovalNode requires a human approval before continuing.
type ApprovalNode struct {
	Role         string        `json:"role"`
	Timeout      time.Duration `json:"timeout,omitempty"`
	Channels     []string      `json:"channels,omitempty"`
	Instructions string        `json:"instructions,omitempty"`
}

// WaitNode pauses execution until a condition or duration elapses.
type WaitNode struct {
	Duration   time.Duration `json:"duration,omitempty"`
	EventTopic string        `json:"eventTopic,omitempty"`
	Expr       string        `json:"expr,omitempty"`
}

// GatewayNode introduces branching / merge semantics.
type GatewayNode struct {
	Mode GatewayMode `json:"mode"`
}

// GatewayMode enumerates branching strategies.
type GatewayMode string

const (
	GatewayModeParallel GatewayMode = "parallel"
	GatewayModeChoice   GatewayMode = "choice"
)

// EdgeType models routing semantics between nodes.
type EdgeType string

const (
	EdgeTypeAlways    EdgeType = "always"
	EdgeTypeOnSuccess EdgeType = "onSuccess"
	EdgeTypeOnFailure EdgeType = "onFailure"
	EdgeTypeExpr      EdgeType = "expression"
)

// Edge connects two nodes with optional conditions.
type Edge struct {
	From       string   `json:"from"`
	To         string   `json:"to"`
	Type       EdgeType `json:"type"`
	Expression string   `json:"expression,omitempty"`
	Priority   int      `json:"priority,omitempty"`
}

// Validate ensures the workflow specification is structurally sound.
func (s Spec) Validate() error {
	if strings.TrimSpace(s.StartNode) == "" {
		return errors.New("workflow: start node required")
	}
	if len(s.Nodes) == 0 {
		return errors.New("workflow: at least one node required")
	}
	nodes := make(map[string]Node, len(s.Nodes))
	for id, node := range s.Nodes {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			return errors.New("workflow: node id cannot be empty")
		}
		node.ID = trimmed
		if node.Type == "" {
			return fmt.Errorf("workflow: node %s missing type", trimmed)
		}
		if err := validateNode(node); err != nil {
			return fmt.Errorf("workflow: node %s invalid: %w", trimmed, err)
		}
		nodes[trimmed] = node
	}
	if _, ok := nodes[s.StartNode]; !ok {
		return fmt.Errorf("workflow: start node %s not defined", s.StartNode)
	}
	if len(s.Edges) == 0 {
		return errors.New("workflow: edges required to connect nodes")
	}
	for idx, edge := range s.Edges {
		if strings.TrimSpace(edge.From) == "" || strings.TrimSpace(edge.To) == "" {
			return fmt.Errorf("workflow: edge %d requires from/to", idx)
		}
		if _, ok := nodes[edge.From]; !ok {
			return fmt.Errorf("workflow: edge %d references unknown from node %s", idx, edge.From)
		}
		if _, ok := nodes[edge.To]; !ok {
			return fmt.Errorf("workflow: edge %d references unknown to node %s", idx, edge.To)
		}
		if edge.From == edge.To {
			return fmt.Errorf("workflow: edge %d cannot form a self loop", idx)
		}
		if edge.Type == EdgeTypeExpr && strings.TrimSpace(edge.Expression) == "" {
			return fmt.Errorf("workflow: edge %d requires expression", idx)
		}
	}
	return nil
}

func validateNode(node Node) error {
	switch node.Type {
	case NodeTypeTask:
		if node.Task == nil || strings.TrimSpace(node.Task.Action) == "" {
			return errors.New("task node requires action")
		}
	case NodeTypeApproval:
		if node.Approval == nil || strings.TrimSpace(node.Approval.Role) == "" {
			return errors.New("approval node requires role")
		}
	case NodeTypeWait:
		if node.Wait == nil || (node.Wait.Duration <= 0 && node.Wait.EventTopic == "" && strings.TrimSpace(node.Wait.Expr) == "") {
			return errors.New("wait node requires duration, eventTopic, or expr")
		}
	case NodeTypeGateway:
		if node.Gateway == nil || node.Gateway.Mode == "" {
			return errors.New("gateway node requires mode")
		}
	case NodeTypeStart, NodeTypeEnd:
		// metadata only
	default:
		return fmt.Errorf("unsupported node type %s", node.Type)
	}
	if node.Retries < 0 {
		return errors.New("retries cannot be negative")
	}
	return nil
}
