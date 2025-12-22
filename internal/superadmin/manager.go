package superadmin

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	DefaultUsername = "admin"
	defaultPassword = "admin123"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
	ErrPasswordReuse      = errors.New("new password matches current password")
)

type state struct {
	PasswordHash string    `json:"passwordHash"`
	MustReset    bool      `json:"mustReset"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Manager struct {
	path  string
	mu    sync.RWMutex
	state state
}

func NewManager(path string) (*Manager, error) {
	if strings.TrimSpace(path) == "" {
		path = "super_admin.json"
	}
	mgr := &Manager{path: path}
	if err := mgr.loadOrInit(); err != nil {
		return nil, err
	}
	return mgr, nil
}

func (m *Manager) Authenticate(password string) (bool, bool, error) {
	if m == nil {
		return false, false, errors.New("super admin manager unavailable")
	}
	m.mu.RLock()
	hash := m.state.PasswordHash
	mustReset := m.state.MustReset
	m.mu.RUnlock()
	if hash == "" {
		return false, false, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return false, false, ErrInvalidCredentials
	}
	return true, mustReset, nil
}

func (m *Manager) UpdatePassword(current, next string) error {
	if m == nil {
		return errors.New("super admin manager unavailable")
	}
	if len(next) < 8 {
		return ErrWeakPassword
	}
	if next == defaultPassword {
		return errors.New("new password cannot reuse the default password")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := bcrypt.CompareHashAndPassword([]byte(m.state.PasswordHash), []byte(current)); err != nil {
		return ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(m.state.PasswordHash), []byte(next)); err == nil {
		return ErrPasswordReuse
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	m.state.PasswordHash = string(hash)
	m.state.MustReset = false
	m.state.UpdatedAt = time.Now().UTC()
	return m.persistLocked()
}

func (m *Manager) ForceReset() error {
	if m == nil {
		return errors.New("super admin manager unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	hash, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	m.state.PasswordHash = string(hash)
	m.state.MustReset = true
	m.state.UpdatedAt = time.Now().UTC()
	return m.persistLocked()
}

func (m *Manager) RequiresReset() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.MustReset
}

func (m *Manager) loadOrInit() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return m.initDefault()
		}
		return err
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return err
	}
	if st.PasswordHash == "" {
		return m.initDefault()
	}
	m.state = st
	return nil
}

func (m *Manager) initDefault() error {
	if dir := filepath.Dir(m.path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	m.state = state{
		PasswordHash: string(hash),
		MustReset:    true,
		UpdatedAt:    time.Now().UTC(),
	}
	return m.persistLocked()
}

func (m *Manager) persistLocked() error {
	payload, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, payload, 0o600)
}
