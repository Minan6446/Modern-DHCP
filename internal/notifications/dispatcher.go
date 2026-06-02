package notifications

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Dispatcher routes notifications to registered channels.
type Dispatcher struct {
	timeout  time.Duration
	logger   *zap.Logger
	mu       sync.RWMutex
	channels map[string]Sender
}

// NewDispatcher creates a dispatcher with the specified timeout per channel.
func NewDispatcher(timeout time.Duration, logger *zap.Logger) *Dispatcher {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Dispatcher{
		timeout:  timeout,
		logger:   logger,
		channels: make(map[string]Sender),
	}
}

// RegisterChannel registers a sender under the provided name.
func (d *Dispatcher) RegisterChannel(name string, sender Sender) {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	if trimmed == "" || sender == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.channels[trimmed] = sender
}

// Dispatch delivers the message to the requested channels.
func (d *Dispatcher) Dispatch(ctx context.Context, msg Message, channelNames ...string) error {
	if len(channelNames) == 0 {
		return errors.New("notifications: channel required")
	}
	sendCtx := ctx
	if sendCtx == nil {
		sendCtx = context.Background()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}

	var errs []error
	for _, raw := range channelNames {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		sender := d.lookup(name)
		if sender == nil {
			d.logger.Warn("notification channel missing", zap.String("channel", name))
			continue
		}
		execCtx, cancel := context.WithTimeout(sendCtx, d.timeout)
		if err := sender.Send(execCtx, msg); err != nil {
			errs = append(errs, fmt.Errorf("channel %s: %w", name, err))
		}
		cancel()
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (d *Dispatcher) lookup(name string) Sender {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.channels[name]
}

// Channels returns the registered channel names sorted alphabetically.
func (d *Dispatcher) Channels() []string {
	if d == nil {
		return []string{}
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if len(d.channels) == 0 {
		return []string{}
	}
	names := make([]string, 0, len(d.channels))
	for name := range d.channels {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
