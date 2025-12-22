package server

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// dhcpOptionTemplate captures editable DHCP option metadata consumed by the UI.
type dhcpOptionTemplate struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Code        int       `json:"code"`
	Scope       string    `json:"scope"`
	Format      string    `json:"format"`
	Value       string    `json:"value"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// dhcpOptionStore keeps option templates in-memory for Phase 2 console workflows.
type dhcpOptionStore struct {
	mu      sync.RWMutex
	options map[string]dhcpOptionTemplate
}

func newDHCPOptionStore() *dhcpOptionStore {
	store := &dhcpOptionStore{options: make(map[string]dhcpOptionTemplate)}
	now := time.Now().UTC()
	defaults := []dhcpOptionTemplate{
		{
			ID:          uuid.NewString(),
			Name:        "Default Gateway",
			Code:        3,
			Scope:       "GLOBAL",
			Format:      "ipv4",
			Value:       "10.10.0.1",
			Description: "Router option delivered to branch clients",
			Tags:        []string{"core", "routing"},
			UpdatedAt:   now,
		},
		{
			ID:          uuid.NewString(),
			Name:        "DNS Servers",
			Code:        6,
			Scope:       "GLOBAL",
			Format:      "ipv4-list",
			Value:       "10.10.0.53,10.10.0.54",
			Description: "Primary/secondary DNS resolvers",
			Tags:        []string{"dns", "prod"},
			UpdatedAt:   now,
		},
		{
			ID:          uuid.NewString(),
			Name:        "Timezone",
			Code:        2,
			Scope:       "SITE",
			Format:      "string",
			Value:       "UTC+8",
			Description: "East-Asia factory timezone broadcast",
			Tags:        []string{"apac", "factory"},
			UpdatedAt:   now,
		},
	}
	for _, opt := range defaults {
		store.options[opt.ID] = opt
	}
	return store
}

func (s *dhcpOptionStore) list() []dhcpOptionTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]dhcpOptionTemplate, 0, len(s.options))
	for _, opt := range s.options {
		out = append(out, opt)
	}
	sort.Slice(out, func(i, j int) bool {
		if strings.EqualFold(out[i].Name, out[j].Name) {
			return out[i].Code < out[j].Code
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (s *dhcpOptionStore) get(id string) (dhcpOptionTemplate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	opt, ok := s.options[id]
	return opt, ok
}

func (s *dhcpOptionStore) upsert(opt dhcpOptionTemplate) dhcpOptionTemplate {
	if strings.TrimSpace(opt.ID) == "" {
		opt.ID = uuid.NewString()
	}
	opt.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.options[opt.ID] = opt
	s.mu.Unlock()
	return opt
}

func (s *dhcpOptionStore) delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.options[id]; !ok {
		return false
	}
	delete(s.options, id)
	return true
}
