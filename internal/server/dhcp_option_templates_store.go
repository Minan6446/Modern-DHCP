package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// dhcpConfigTemplate groups multiple DHCP options under a reusable template.
type dhcpConfigTemplate struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Icon        string               `json:"icon,omitempty"`
	Options     []dhcpOptionTemplate `json:"options"`
	UpdatedAt   time.Time            `json:"updatedAt"`
}

// dhcpTemplateStore caches config templates in memory.
type dhcpTemplateStore struct {
	mu        sync.RWMutex
	templates map[string]dhcpConfigTemplate
}

func newDHCPTemplateStore() *dhcpTemplateStore {
	return &dhcpTemplateStore{templates: make(map[string]dhcpConfigTemplate)}
}

func (s *dhcpTemplateStore) list() []dhcpConfigTemplate {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]dhcpConfigTemplate, 0, len(s.templates))
	for _, tpl := range s.templates {
		out = append(out, tpl)
	}
	return out
}

func (s *dhcpTemplateStore) get(id string) (dhcpConfigTemplate, bool) {
	if s == nil {
		return dhcpConfigTemplate{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	tpl, ok := s.templates[id]
	return tpl, ok
}

func (s *dhcpTemplateStore) upsert(tpl dhcpConfigTemplate) dhcpConfigTemplate {
	if s == nil {
		return tpl
	}
	if strings.TrimSpace(tpl.ID) == "" {
		tpl.ID = uuid.NewString()
	}
	tpl.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.templates[tpl.ID] = tpl
	s.mu.Unlock()
	return tpl
}

func (s *dhcpTemplateStore) delete(id string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.templates[id]; !ok {
		return false
	}
	delete(s.templates, id)
	return true
}

func (s *dhcpTemplateStore) apply(ctx context.Context, id string, optionStore *dhcpOptionStore, optionRepo OptionRepository) error {
	if s == nil {
		return errors.New("template store unavailable")
	}
	tpl, ok := s.get(id)
	if !ok {
		return errors.New("template not found")
	}
	for _, opt := range tpl.Options {
		normalized := normalizeTemplateOption(opt)
		if optionRepo != nil {
			saved, err := optionRepo.Upsert(ctx, normalized, "")
			if err != nil {
				return err
			}
			normalized = saved
		}
		if optionStore != nil {
			optionStore.upsert(normalized)
		}
	}
	return nil
}

func normalizeTemplateOption(opt dhcpOptionTemplate) dhcpOptionTemplate {
	normalized := opt
	if strings.TrimSpace(normalized.ID) == "" {
		normalized.ID = uuid.NewString()
	}
	normalized.Name = firstNonEmpty(normalized.Name, fmt.Sprintf("Option %d", normalized.Code))
	normalized.Scope = normalizeOptionScope(normalized.Scope)
	if normalized.Scope == "" {
		normalized.Scope = "GLOBAL"
	}
	normalized.Format = resolveOptionFormat(normalized.Format, normalized.DataType)
	if normalized.Format == "" {
		normalized.Format = "string"
	}
	normalized.DataType = resolveOptionDataType(normalized.Format, normalized.DataType)
	normalized.Value = firstNonEmpty(strings.TrimSpace(normalized.Value), strings.TrimSpace(normalized.SampleValue), strings.TrimSpace(normalized.ValueExample), "placeholder")
	normalized.ValueExample = strings.TrimSpace(normalized.ValueExample)
	normalized.SampleValue = strings.TrimSpace(normalized.SampleValue)
	normalized.Description = strings.TrimSpace(normalized.Description)
	normalized.Tags = uniqueStrings(normalized.Tags)
	normalized.AllowedValues = normalizeAllowedValues(normalized.AllowedValues)
	return normalized
}
