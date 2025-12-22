package snooping

import (
	"context"
	"errors"
	"time"
)

var (
	ErrBindingNotFound = errors.New("snooping: binding not found")
	ErrUntrusted       = errors.New("snooping: binding untrusted")
)

// LookupKey identifies a snooping binding lookup.
type LookupKey struct {
	TenantID string
	MAC      string
	PortID   string
	VLANID   int
}

// Binding represents a trusted port/MAC/IP tuple.
type Binding struct {
	TenantID    string
	MACAddress  string
	IPAddress   string
	PortID      string
	InterfaceID string
	VLANID      int
	Trusted     bool
	ExpiresAt   time.Time
}

// Store provides cached lookup semantics for bindings.
type Store interface {
	Lookup(ctx context.Context, key LookupKey) (*Binding, error)
	StreamChanges(ctx context.Context) (<-chan Binding, error)
}

// Loader ingests snooping feeds from switches or controllers.
type Loader interface {
	Start(ctx context.Context) error
}

type noopStore struct{}

// NewNoopStore returns a store that always misses lookups.
func NewNoopStore() Store {
	return &noopStore{}
}

func (n *noopStore) Lookup(ctx context.Context, key LookupKey) (*Binding, error) {
	return nil, ErrBindingNotFound
}

func (n *noopStore) StreamChanges(ctx context.Context) (<-chan Binding, error) {
	ch := make(chan Binding)
	close(ch)
	return ch, nil
}
