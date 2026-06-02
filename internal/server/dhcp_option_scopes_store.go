package server

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type dhcpOptionScope struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Subnet      string    `json:"subnet,omitempty"`
	Range       string    `json:"range,omitempty"`
	Gateway     string    `json:"gateway,omitempty"`
	Status      string    `json:"status,omitempty"`
	ScopeType   string    `json:"scopeType"`
	Target      string    `json:"target"`
	TemplateID  string    `json:"templateId,omitempty"`
	OptionIDs   []string  `json:"optionIds,omitempty"`
	Description string    `json:"description,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type dhcpOptionScopeStore struct {
	mu     sync.RWMutex
	scopes map[string]dhcpOptionScope
}

func newDHCPOptionScopeStore() *dhcpOptionScopeStore {
	return &dhcpOptionScopeStore{scopes: make(map[string]dhcpOptionScope)}
}

func (s *dhcpOptionScopeStore) list() []dhcpOptionScope {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]dhcpOptionScope, 0, len(s.scopes))
	for _, item := range s.scopes {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if strings.EqualFold(out[i].Name, out[j].Name) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (s *dhcpOptionScopeStore) replace(items []dhcpOptionScope) {
	if s == nil {
		return
	}
	next := make(map[string]dhcpOptionScope, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		next[id] = item
	}
	s.mu.Lock()
	s.scopes = next
	s.mu.Unlock()
}

func (s *dhcpOptionScopeStore) get(id string) (dhcpOptionScope, bool) {
	if s == nil {
		return dhcpOptionScope{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.scopes[id]
	return item, ok
}

func (s *dhcpOptionScopeStore) upsert(item dhcpOptionScope) dhcpOptionScope {
	if s == nil {
		return item
	}
	if strings.TrimSpace(item.ID) == "" {
		item.ID = uuid.NewString()
	}
	item.Name = strings.TrimSpace(item.Name)
	item.Subnet = strings.TrimSpace(item.Subnet)
	item.Range = strings.TrimSpace(item.Range)
	item.Gateway = strings.TrimSpace(item.Gateway)
	status := strings.ToLower(strings.TrimSpace(item.Status))
	if status != "inactive" {
		status = "active"
	}
	item.Status = status
	item.ScopeType = normalizeOptionScope(item.ScopeType)
	if item.ScopeType == "" {
		item.ScopeType = "GLOBAL"
	}
	item.Target = strings.TrimSpace(item.Target)
	if item.Target == "" {
		item.Target = firstNonEmpty(item.Subnet, "global")
	}
	item.TemplateID = strings.TrimSpace(item.TemplateID)
	item.OptionIDs = uniqueStrings(item.OptionIDs)
	item.Description = strings.TrimSpace(item.Description)
	item.Notes = strings.TrimSpace(item.Notes)
	item.UpdatedAt = time.Now().UTC()

	s.mu.Lock()
	s.scopes[item.ID] = item
	s.mu.Unlock()
	return item
}

func (s *dhcpOptionScopeStore) delete(id string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.scopes[id]; !ok {
		return false
	}
	delete(s.scopes, id)
	return true
}

func (s *dhcpOptionScopeStore) listByTemplate(templateID string) []dhcpOptionScope {
	if s == nil {
		return nil
	}
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]dhcpOptionScope, 0)
	for _, item := range s.scopes {
		if strings.TrimSpace(item.TemplateID) == templateID {
			out = append(out, item)
		}
	}
	return out
}

func (s *dhcpOptionScopeStore) templateUsageCount(templateID string) int {
	if s == nil {
		return 0
	}
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, item := range s.scopes {
		if strings.TrimSpace(item.TemplateID) == templateID {
			count++
		}
	}
	return count
}

func (s *dhcpOptionScopeStore) optionUsageCount(optionID string) int {
	if s == nil {
		return 0
	}
	optionID = strings.TrimSpace(optionID)
	if optionID == "" {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, item := range s.scopes {
		for _, id := range item.OptionIDs {
			if strings.TrimSpace(id) == optionID {
				count++
				break
			}
		}
	}
	return count
}
