package automation

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ScheduleConfig defines how a recurring automation job should run.
type ScheduleConfig struct {
	Enabled      bool              `json:"enabled"`
	Interval     time.Duration     `json:"interval"`
	InitialDelay time.Duration     `json:"initialDelay"`
	TenantID     string            `json:"tenantId"`
	Labels       map[string]string `json:"labels,omitempty"`
	Payload      json.RawMessage   `json:"payload,omitempty"`
	Channels     []string          `json:"channels,omitempty"`
}

// ScheduleSummary pairs a job type with its schedule configuration.
type ScheduleSummary struct {
	Type     JobType
	Schedule ScheduleConfig
}

// ServiceOptions configure the automation service wrapper.
type ServiceOptions struct {
	SchedulerOptions Options
	Schedules        map[JobType]ScheduleConfig
	Logger           *zap.Logger
	Store            JobStore
}

// Service wires the in-memory scheduler with recurring job schedules.
type Service struct {
	scheduler *Scheduler
	schedules map[JobType]ScheduleConfig
	logger    *zap.Logger
	store     JobStore

	mu         sync.Mutex
	runtimeCtx context.Context
	cancel     context.CancelFunc
	started    bool
	wg         sync.WaitGroup
}

// NewService builds a new automation service instance.
func NewService(opts ServiceOptions) *Service {
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	scheduler := NewScheduler(opts.SchedulerOptions, logger)
	if opts.Store != nil {
		scheduler.UseRecorder(opts.Store)
	}
	schedules := make(map[JobType]ScheduleConfig, len(opts.Schedules))
	for jobType, cfg := range opts.Schedules {
		schedules[jobType] = cfg
	}
	return &Service{
		scheduler: scheduler,
		schedules: schedules,
		logger:    logger,
		store:     opts.Store,
	}
}

// RegisterHandler attaches a handler to the underlying scheduler.
func (s *Service) RegisterHandler(jobType JobType, handler Handler) {
	s.scheduler.RegisterHandler(jobType, handler)
}

// Enqueue forwards a job to the underlying scheduler.
func (s *Service) Enqueue(ctx context.Context, job Job) error {
	return s.scheduler.Enqueue(ctx, s.normalizeJob(job))
}

// Store exposes the configured job store for read-only operations.
func (s *Service) Store() JobStore {
	if s == nil {
		return nil
	}
	return s.store
}

// Snapshot exposes the underlying scheduler snapshot.
func (s *Service) Snapshot() Snapshot {
	if s == nil || s.scheduler == nil {
		return Snapshot{}
	}
	return s.scheduler.Snapshot()
}

// Schedules returns a deterministic slice of configured schedules.
func (s *Service) Schedules() []ScheduleSummary {
	entries := make([]ScheduleSummary, 0, len(s.schedules))
	for jobType, cfg := range s.schedules {
		entries = append(entries, ScheduleSummary{Type: jobType, Schedule: cfg})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Type < entries[j].Type
	})
	return entries
}

// Start launches the scheduler and recurring job schedules.
func (s *Service) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return nil
	}

	runtimeCtx, cancel := context.WithCancel(ctx)
	if err := s.scheduler.Start(runtimeCtx); err != nil {
		cancel()
		return err
	}

	s.runtimeCtx = runtimeCtx
	s.cancel = cancel
	s.started = true

	active := 0
	for jobType, cfg := range s.schedules {
		if !cfg.Enabled || cfg.Interval <= 0 {
			continue
		}
		active++
		s.wg.Add(1)
		go s.runSchedule(runtimeCtx, jobType, cfg)
	}
	s.logger.Info("automation service started", zap.Int("schedules", active))
	return nil
}

// Stop halts all recurring schedules and stops the scheduler.
func (s *Service) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}
	cancel := s.cancel
	s.started = false
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
	return s.scheduler.Stop(ctx)
}

func (s *Service) runSchedule(ctx context.Context, jobType JobType, cfg ScheduleConfig) {
	defer s.wg.Done()

	if cfg.InitialDelay > 0 {
		timer := time.NewTimer(cfg.InitialDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}

	if err := s.enqueueScheduledJob(ctx, jobType, cfg); err != nil {
		s.logger.Warn("failed to enqueue scheduled job", zap.String("jobType", string(jobType)), zap.Error(err))
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.enqueueScheduledJob(ctx, jobType, cfg); err != nil {
				s.logger.Warn("failed to enqueue scheduled job", zap.String("jobType", string(jobType)), zap.Error(err))
			}
		}
	}
}

func (s *Service) enqueueScheduledJob(ctx context.Context, jobType JobType, cfg ScheduleConfig) error {
	job := Job{
		Type:        jobType,
		TenantID:    cfg.TenantID,
		Labels:      cloneLabels(cfg.Labels),
		Payload:     MergeChannels(clonePayload(cfg.Payload), cfg.Channels),
		Source:      JobSourceSchedule,
		TriggeredBy: JobActorScheduler,
	}
	return s.scheduler.Enqueue(ctx, job)
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		out[k] = v
	}
	return out
}

func clonePayload(payload json.RawMessage) json.RawMessage {
	if len(payload) == 0 {
		return nil
	}
	cp := make([]byte, len(payload))
	copy(cp, payload)
	return cp
}

// MergeChannels ensures the provided payload lists the desired notification channels.
func MergeChannels(payload json.RawMessage, channels []string) json.RawMessage {
	if len(channels) == 0 {
		return payload
	}
	var body map[string]any
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &body); err != nil {
			return payload
		}
	}
	if body == nil {
		body = make(map[string]any)
	}
	body["channels"] = append([]string(nil), channels...)
	raw, err := json.Marshal(body)
	if err != nil {
		return payload
	}
	return raw
}

func (s *Service) normalizeJob(job Job) Job {
	source := strings.TrimSpace(job.Source)
	if source == "" {
		job.Source = JobSourceManual
	} else {
		job.Source = source
	}
	triggeredBy := strings.TrimSpace(job.TriggeredBy)
	if triggeredBy == "" {
		job.TriggeredBy = JobActorSystem
	} else {
		job.TriggeredBy = triggeredBy
	}
	return job
}
