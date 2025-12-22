package ha

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/failover"
)

var (
	// ErrControllerUnavailable indicates HA control endpoints are disabled.
	ErrControllerUnavailable = errors.New("ha: controller unavailable")
	// ErrInvalidLoadPolicy indicates the payload could not be mapped to a strategy.
	ErrInvalidLoadPolicy = errors.New("ha: invalid load balancer policy")
)

// Controller abstracts the failover manager functions required by the HA service.
type Controller interface {
	failover.StatusReporter
	AllowManualFailback()
	TriggerFailover(context.Context, failover.ManualFailoverRequest) error
	UpdateIngressPolicy(context.Context, failover.IngressPolicy) error
	Nodes() []failover.NodeStatus
}

// Options configures the HA service wiring.
type Options struct {
	Controller   Controller
	RunbookPaths []string
	Logger       *zap.Logger
}

// Service backs the HA control surface APIs.
type Service struct {
	controller   Controller
	logger       *zap.Logger
	runbookOnce  sync.Once
	runbookPaths []string
	runbooks     []Runbook
}

// Runbook captures lightweight metadata about recovery procedures.
type Runbook struct {
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary,omitempty"`
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// FailoverRequest describes a manual failover intent.
type FailoverRequest struct {
	TargetRole string
	Reason     string
	DryRun     bool
	Force      bool
}

// LoadPolicy describes ingress/load-balancer adjustments.
type LoadPolicy struct {
	Strategy      string
	Weights       map[string]int
	StickySeconds int64
}

// NewService builds a HA helper surface.
func NewService(opts Options) *Service {
	svc := &Service{
		controller:   opts.Controller,
		logger:       opts.Logger,
		runbookPaths: append([]string(nil), opts.RunbookPaths...),
	}
	if svc.logger == nil {
		svc.logger = zap.NewNop()
	}
	return svc
}

// Snapshot returns the current failover snapshot.
func (s *Service) Snapshot() failover.StatusSnapshot {
	if s == nil || s.controller == nil {
		return failover.StatusSnapshot{}
	}
	return s.controller.Snapshot()
}

// Nodes reports currently known cluster members.
func (s *Service) Nodes(ctx context.Context) []failover.NodeStatus {
	if s == nil || s.controller == nil {
		return nil
	}
	return append([]failover.NodeStatus(nil), s.controller.Nodes()...)
}

// RequestFailover attempts to promote/demote nodes per the request payload.
func (s *Service) RequestFailover(ctx context.Context, req FailoverRequest) error {
	if s == nil || s.controller == nil {
		return ErrControllerUnavailable
	}
	target := strings.TrimSpace(strings.ToLower(req.TargetRole))
	role := failover.RolePrimary
	if target == "standby" {
		role = failover.RoleStandby
	}
	payload := failover.ManualFailoverRequest{
		TargetRole: role,
		Reason:     strings.TrimSpace(req.Reason),
		DryRun:     req.DryRun,
		Force:      req.Force,
	}
	return s.controller.TriggerFailover(ctx, payload)
}

// UpdateLoadPolicy pushes ingress/load-balancer adjustments to the controller.
func (s *Service) UpdateLoadPolicy(ctx context.Context, policy LoadPolicy) error {
	if s == nil || s.controller == nil {
		return ErrControllerUnavailable
	}
	strategy := strings.TrimSpace(strings.ToLower(policy.Strategy))
	if strategy == "" {
		return ErrInvalidLoadPolicy
	}
	duration := time.Duration(policy.StickySeconds) * time.Second
	payload := failover.IngressPolicy{Strategy: strategy, Weights: policy.Weights, StickyDuration: duration}
	return s.controller.UpdateIngressPolicy(ctx, payload)
}

// Runbooks returns cached metadata for registered runbook files.
func (s *Service) Runbooks(ctx context.Context) []Runbook {
	if s == nil {
		return nil
	}
	s.runbookOnce.Do(func() {
		s.runbooks = s.loadRunbooks()
	})
	return append([]Runbook(nil), s.runbooks...)
}

func (s *Service) loadRunbooks() []Runbook {
	if len(s.runbookPaths) == 0 {
		return nil
	}
	runbooks := make([]Runbook, 0, len(s.runbookPaths))
	for _, path := range s.runbookPaths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		data, err := os.ReadFile(trimmed)
		if err != nil {
			if s.logger != nil {
				s.logger.Debug("ha: runbook read failed", zap.String("path", trimmed), zap.Error(err))
			}
			continue
		}
		info, err := os.Stat(trimmed)
		if err != nil && s.logger != nil {
			s.logger.Debug("ha: runbook stat failed", zap.String("path", trimmed), zap.Error(err))
		}
		title, summary := parseRunbookContent(string(data))
		if title == "" {
			title = filepath.Base(trimmed)
		}
		runbook := Runbook{
			Slug:      slugify(trimmed),
			Title:     title,
			Summary:   summary,
			Source:    trimmed,
			UpdatedAt: time.Now().UTC(),
		}
		if err == nil && info != nil {
			runbook.UpdatedAt = info.ModTime().UTC()
		}
		runbooks = append(runbooks, runbook)
	}
	return runbooks
}

var slugSanitizer = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(path string) string {
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	base = slugSanitizer.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		return "runbook"
	}
	return base
}

func parseRunbookContent(body string) (string, string) {
	if body == "" {
		return "", ""
	}
	lines := strings.Split(body, "\n")
	title := ""
	summary := ""
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if title == "" {
			title = strings.Trim(line, "#* ")
			continue
		}
		summary = sanitizeSummary(line)
		if summary != "" {
			break
		}
		if i > 20 {
			break
		}
	}
	return title, summary
}

func sanitizeSummary(line string) string {
	if line == "" {
		return ""
	}
	// Strip markdown links [text](url)
	summary := linkPattern.ReplaceAllString(line, "$1")
	summary = strings.Trim(summary, "#* _`")
	return summary
}

var linkPattern = regexp.MustCompile(`\[([^\]]+)\]\([^\)]+\)`)
