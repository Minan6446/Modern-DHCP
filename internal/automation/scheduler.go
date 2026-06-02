package automation

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler defines the contract automation jobs must satisfy.
type Handler interface {
	Handle(context.Context, Job) error
}

// HandlerFunc is a helper to turn functions into job handlers.
type HandlerFunc func(context.Context, Job) error

// Handle executes the wrapped handler function.
func (f HandlerFunc) Handle(ctx context.Context, job Job) error {
	return f(ctx, job)
}

// Scheduler coordinates automation jobs with lightweight in-memory workers.
type Scheduler struct {
	opts Options

	queue    chan Job
	handlers map[JobType]Handler

	handlerMu  sync.RWMutex
	recorderMu sync.RWMutex
	mu         sync.RWMutex

	ctx     context.Context
	cancel  context.CancelFunc
	running bool

	wg            sync.WaitGroup
	activeWorkers int32
	startedAt     time.Time

	recorder JobRecorder
	logger   *zap.Logger

	metrics schedulerMetrics
}

// NewScheduler constructs a scheduler with sane defaults.
func NewScheduler(opts Options, logger *zap.Logger) *Scheduler {
	opts = normalizeOptions(opts)
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Scheduler{
		opts:     opts,
		queue:    make(chan Job, opts.QueueSize),
		handlers: make(map[JobType]Handler),
		logger:   logger,
	}
}

// Start spins up worker goroutines. Calling Start twice is a no-op.
func (s *Scheduler) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.running = true
	s.startedAt = time.Now()

	for i := 0; i < s.opts.WorkerCount; i++ {
		s.wg.Add(1)
		go s.worker(s.ctx, i)
	}

	s.logger.Info("automation scheduler started", zap.Int("workers", s.opts.WorkerCount))
	return nil
}

// Stop signals workers to finish and waits for completion or context cancellation.
func (s *Scheduler) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}

	if s.cancel != nil {
		s.cancel()
	}
	s.running = false
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.wg.Wait()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		s.logger.Info("automation scheduler stopped")
		return nil
	}
}

// RegisterHandler wires a handler for a specific job type.
func (s *Scheduler) RegisterHandler(jobType JobType, handler Handler) {
	if handler == nil {
		panic("automation: nil handler")
	}

	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()
	s.handlers[jobType] = handler
}

// UseRecorder wires a JobRecorder for persistence hooks.
func (s *Scheduler) UseRecorder(recorder JobRecorder) {
	s.recorderMu.Lock()
	s.recorder = recorder
	s.recorderMu.Unlock()
}

// Enqueue submits a job to the scheduler.
func (s *Scheduler) Enqueue(ctx context.Context, job Job) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	if job.Type == "" {
		return errors.New("automation: job type required")
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	job.Status = JobStatusPending

	return s.enqueueInternal(ctx, job)
}

// Snapshot exposes current runtime metrics for diagnostics.
func (s *Scheduler) Snapshot() Snapshot {
	s.handlerMu.RLock()
	registered := len(s.handlers)
	s.handlerMu.RUnlock()

	s.mu.RLock()
	started := s.startedAt
	s.mu.RUnlock()

	waitTotal := atomic.LoadInt64(&s.metrics.waitTotal)
	waitCount := atomic.LoadInt64(&s.metrics.waitCount)
	runTotal := atomic.LoadInt64(&s.metrics.runTotal)
	runCount := atomic.LoadInt64(&s.metrics.runCount)
	completed := atomic.LoadInt64(&s.metrics.completed)
	failed := atomic.LoadInt64(&s.metrics.failed)
	retried := atomic.LoadInt64(&s.metrics.retried)

	avgWait := 0.0
	if waitCount > 0 {
		avgWait = float64(waitTotal) / float64(waitCount) / 1e6
	}
	avgRun := 0.0
	if runCount > 0 {
		avgRun = float64(runTotal) / float64(runCount) / 1e6
	}
	uptime := int64(0)
	if !started.IsZero() {
		uptime = int64(time.Since(started).Seconds())
		if uptime < 0 {
			uptime = 0
		}
	}

	return Snapshot{
		PendingJobs:        len(s.queue),
		ActiveWorkers:      int(atomic.LoadInt32(&s.activeWorkers)),
		RegisteredHandlers: registered,
		StartedAt:          started,
		UptimeSeconds:      uptime,
		AverageWaitMillis:  avgWait,
		AverageRunMillis:   avgRun,
		CompletedJobs:      completed,
		FailedJobs:         failed,
		RetryScheduled:     retried,
	}
}

func (s *Scheduler) worker(ctx context.Context, id int) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.queue:
			s.processJob(ctx, job)
		}
	}
}

func (s *Scheduler) processJob(parent context.Context, job Job) {
	if !job.NotBefore.IsZero() {
		delay := time.Until(job.NotBefore)
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-parent.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}

	start := time.Now()
	createdAt := job.CreatedAt
	if createdAt.IsZero() {
		createdAt = start
		job.CreatedAt = start
	}

	handler, ok := s.lookupHandler(job.Type)
	if !ok {
		s.logger.Warn("no automation handler registered", zap.String("jobType", string(job.Type)))
		return
	}

	waitDuration := start.Sub(createdAt)
	s.observeWait(waitDuration)

	job.Attempts++
	job.Status = JobStatusRunning
	s.recordRunning(parent, job)
	atomic.AddInt32(&s.activeWorkers, 1)
	defer atomic.AddInt32(&s.activeWorkers, -1)

	ctx, cancel := context.WithTimeout(parent, s.opts.DefaultTimeout)
	defer cancel()

	if err := handler.Handle(ctx, job); err != nil {
		s.observeRun(time.Since(start))
		s.logger.Warn("automation job failed",
			zap.String("jobId", job.ID),
			zap.String("jobType", string(job.Type)),
			zap.Error(err),
			zap.Int("attempt", job.Attempts))
		job.Status = JobStatusFailed
		s.recordFailed(parent, job, err)
		s.observeFailed()

		if job.Attempts < s.opts.MaxAttempts {
			backoff := s.backoffDuration(job.Attempts)
			job.NotBefore = time.Now().Add(backoff)
			job.Status = JobStatusPending
			s.observeRetry()
			s.enqueueAfter(job, backoff)
		}
		return
	}

	job.Status = JobStatusSucceeded
	s.recordSucceeded(parent, job)
	s.observeRun(time.Since(start))
	s.observeCompleted()
	s.logger.Info("automation job completed", zap.String("jobId", job.ID), zap.String("jobType", string(job.Type)))
}

func (s *Scheduler) enqueueAfter(job Job, delay time.Duration) {
	timer := time.NewTimer(delay)
	go func() {
		defer timer.Stop()
		select {
		case <-timer.C:
			job.CreatedAt = time.Now()
			if err := s.enqueueInternal(context.Background(), job); err != nil {
				s.logger.Error("failed to requeue automation job", zap.String("jobId", job.ID), zap.Error(err))
			}
		case <-s.runtimeDone():
			return
		}
	}()
}

func (s *Scheduler) backoffDuration(attempts int) time.Duration {
	base := time.Second
	mult := 1 << (attempts - 1)
	if mult > 32 {
		mult = 32
	}
	return time.Duration(mult) * base
}

func (s *Scheduler) lookupHandler(jobType JobType) (Handler, bool) {
	s.handlerMu.RLock()
	handler, ok := s.handlers[jobType]
	s.handlerMu.RUnlock()
	return handler, ok
}

func (s *Scheduler) enqueueInternal(ctx context.Context, job Job) error {
	runtimeCtx := s.runtimeCtx()
	var done <-chan struct{}
	if runtimeCtx != nil {
		done = runtimeCtx.Done()
	}

	select {
	case s.queue <- job:
		s.recordPending(ctx, job)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return errors.New("automation: scheduler stopped")
	}
}

func (s *Scheduler) runtimeCtx() context.Context {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ctx
}

func (s *Scheduler) runtimeDone() <-chan struct{} {
	ctx := s.runtimeCtx()
	if ctx == nil {
		return nil
	}
	return ctx.Done()
}

func normalizeOptions(opts Options) Options {
	if opts.QueueSize <= 0 {
		opts.QueueSize = 128
	}
	if opts.WorkerCount <= 0 {
		opts.WorkerCount = 4
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 3
	}
	if opts.DefaultTimeout <= 0 {
		opts.DefaultTimeout = 30 * time.Second
	}
	return opts
}

// DebugString renders a concise state string useful inside health endpoints.
func (s *Scheduler) DebugString() string {
	snap := s.Snapshot()
	return fmt.Sprintf("jobs=%d workers=%d handlers=%d", snap.PendingJobs, snap.ActiveWorkers, snap.RegisteredHandlers)
}

func (s *Scheduler) jobRecorder() JobRecorder {
	s.recorderMu.RLock()
	recorder := s.recorder
	s.recorderMu.RUnlock()
	return recorder
}

func (s *Scheduler) recordPending(ctx context.Context, job Job) {
	if recorder := s.jobRecorder(); recorder != nil {
		if err := recorder.RecordPending(ctx, job); err != nil {
			s.logger.Warn("automation job pending record failed", zap.String("jobId", job.ID), zap.String("jobType", string(job.Type)), zap.Error(err))
		}
	}
}

func (s *Scheduler) recordRunning(ctx context.Context, job Job) {
	if recorder := s.jobRecorder(); recorder != nil {
		if err := recorder.MarkRunning(ctx, job); err != nil {
			s.logger.Warn("automation job running record failed", zap.String("jobId", job.ID), zap.String("jobType", string(job.Type)), zap.Error(err))
		}
	}
}

func (s *Scheduler) recordSucceeded(ctx context.Context, job Job) {
	if recorder := s.jobRecorder(); recorder != nil {
		summary := fmt.Sprintf("%s completed", job.Type)
		if err := recorder.MarkSucceeded(ctx, job, summary); err != nil {
			s.logger.Warn("automation job success record failed", zap.String("jobId", job.ID), zap.String("jobType", string(job.Type)), zap.Error(err))
		}
	}
}

func (s *Scheduler) recordFailed(ctx context.Context, job Job, runErr error) {
	if recorder := s.jobRecorder(); recorder != nil {
		summary := fmt.Sprintf("attempt %d failed", job.Attempts)
		errMsg := ""
		if runErr != nil {
			errMsg = runErr.Error()
		}
		if err := recorder.MarkFailed(ctx, job, summary, errMsg); err != nil {
			s.logger.Warn("automation job failure record failed", zap.String("jobId", job.ID), zap.String("jobType", string(job.Type)), zap.Error(err))
		}
	}
}

type schedulerMetrics struct {
	waitTotal int64
	waitCount int64
	runTotal  int64
	runCount  int64
	completed int64
	failed    int64
	retried   int64
}

func (s *Scheduler) observeWait(d time.Duration) {
	if d < 0 {
		d = 0
	}
	atomic.AddInt64(&s.metrics.waitTotal, d.Nanoseconds())
	atomic.AddInt64(&s.metrics.waitCount, 1)
}

func (s *Scheduler) observeRun(d time.Duration) {
	if d < 0 {
		d = 0
	}
	atomic.AddInt64(&s.metrics.runTotal, d.Nanoseconds())
	atomic.AddInt64(&s.metrics.runCount, 1)
}

func (s *Scheduler) observeCompleted() {
	atomic.AddInt64(&s.metrics.completed, 1)
}

func (s *Scheduler) observeFailed() {
	atomic.AddInt64(&s.metrics.failed, 1)
}

func (s *Scheduler) observeRetry() {
	atomic.AddInt64(&s.metrics.retried, 1)
}
