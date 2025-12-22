package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"modern-dhcp/internal/iot"
	"modern-dhcp/pkg/models"
)

// Defaults captures fallback cadence for sleepy IoT devices.
type Defaults struct {
	SleepInterval time.Duration
	OfflineWindow time.Duration
}

// Option customizes registry service behavior.
type Option func(*Service)

// WithDefaults overrides fallback timings for sleepy devices.
func WithDefaults(defaults Defaults) Option {
	return func(s *Service) {
		s.defaults = defaults
	}
}

// Service wires higher-level orchestration atop the IoT repository.
type Service struct {
	repo     iot.Repository
	logger   *zap.Logger
	defaults Defaults
}

// NewService constructs a registry service.
func NewService(repo iot.Repository, logger *zap.Logger, opts ...Option) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	svc := &Service{
		repo:   repo,
		logger: logger,
		defaults: Defaults{
			SleepInterval: 10 * time.Minute,
			OfflineWindow: 2 * time.Hour,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

// UpsertDevice registers or updates an IoT device binding.
func (s *Service) UpsertDevice(ctx context.Context, req DeviceUpsertRequest) (*models.IoTDevice, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("iot registry: repository not configured")
	}
	tenantID := strings.TrimSpace(req.TenantID)
	deviceID := strings.TrimSpace(req.DeviceID)
	if tenantID == "" || deviceID == "" {
		return nil, fmt.Errorf("iot registry: tenantId and deviceId are required")
	}
	var (
		existing *models.IoTDevice
		err      error
	)
	existing, err = s.repo.GetDevice(ctx, tenantID, deviceID)
	if err != nil && !errors.Is(err, iot.ErrNotFound) {
		return nil, err
	}
	now := time.Now().UTC()
	if existing == nil || errors.Is(err, iot.ErrNotFound) {
		existing = &models.IoTDevice{
			ID:        uuid.NewString(),
			TenantID:  tenantID,
			DeviceID:  deviceID,
			CreatedAt: now,
		}
	}
	existing.DisplayName = strings.TrimSpace(req.DisplayName)
	existing.HardwareAddr = normalizeHardware(req.HardwareAddr)
	existing.ProfileID = strings.TrimSpace(req.ProfileID)
	existing.LeaseProfileID = strings.TrimSpace(req.LeaseProfileID)
	existing.SleepClass = s.normalizeSleepClass(req.SleepClass, req.SleepyHint)
	existing.SleepInterval = s.ensureSleepInterval(req.SleepInterval)
	existing.OfflineWindow = s.ensureOfflineWindow(req.OfflineWindow)
	existing.SleepyHint = req.SleepyHint || strings.EqualFold(existing.SleepClass, models.IoTSleepClassSleepy)
	existing.Status = s.normalizeStatus(req.Status)
	existing.Firmware = strings.TrimSpace(req.Firmware)
	existing.Labels = s.encodeStringMap(req.Labels)
	if req.Metadata != nil {
		existing.Metadata = cloneJSON(req.Metadata)
	}
	if req.LastSeen != nil {
		existing.LastSeen = cloneTime(req.LastSeen)
	}
	existing.UpdatedAt = now
	if existing.CreatedAt.IsZero() {
		existing.CreatedAt = now
	}
	if err := s.repo.UpsertDevice(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// ListDevices returns IoT devices for a tenant.
func (s *Service) ListDevices(ctx context.Context, tenantID string, opts DeviceListOptions) ([]models.IoTDevice, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("iot registry: repository not configured")
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("iot registry: tenantId required")
	}
	filter := iot.DeviceFilter{
		ProfileID:  strings.TrimSpace(opts.ProfileID),
		SleepyOnly: opts.SleepyOnly,
		Search:     strings.TrimSpace(opts.Search),
		Limit:      opts.Limit,
		Offset:     opts.Offset,
	}
	if strings.TrimSpace(opts.Status) != "" {
		filter.Status = s.normalizeStatus(opts.Status)
	}
	return s.repo.ListDevices(ctx, tenantID, filter)
}

// GetDevice fetches a registered device.
func (s *Service) GetDevice(ctx context.Context, tenantID, deviceID string) (*models.IoTDevice, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("iot registry: repository not configured")
	}
	tenantID = strings.TrimSpace(tenantID)
	deviceID = strings.TrimSpace(deviceID)
	if tenantID == "" || deviceID == "" {
		return nil, fmt.Errorf("iot registry: tenantId and deviceId required")
	}
	return s.repo.GetDevice(ctx, tenantID, deviceID)
}

// DeleteDevice removes a device from the registry.
func (s *Service) DeleteDevice(ctx context.Context, tenantID, deviceID string) error {
	if s.repo == nil {
		return fmt.Errorf("iot registry: repository not configured")
	}
	tenantID = strings.TrimSpace(tenantID)
	deviceID = strings.TrimSpace(deviceID)
	if tenantID == "" || deviceID == "" {
		return fmt.Errorf("iot registry: tenantId and deviceId required")
	}
	return s.repo.DeleteDevice(ctx, tenantID, deviceID)
}

// RecordHeartbeat updates last seen metadata for a device.
func (s *Service) RecordHeartbeat(ctx context.Context, tenantID, deviceID, status string, seenAt time.Time) error {
	if s.repo == nil {
		return fmt.Errorf("iot registry: repository not configured")
	}
	tenantID = strings.TrimSpace(tenantID)
	deviceID = strings.TrimSpace(deviceID)
	if tenantID == "" || deviceID == "" {
		return fmt.Errorf("iot registry: tenantId and deviceId required")
	}
	if seenAt.IsZero() {
		seenAt = time.Now().UTC()
	}
	return s.repo.RecordHeartbeat(ctx, tenantID, deviceID, seenAt.UTC(), s.normalizeStatus(status))
}

// CreateProfile registers a reusable IoT profile.
func (s *Service) CreateProfile(ctx context.Context, req ProfileUpsertRequest) (*models.IoTDeviceProfile, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("iot registry: repository not configured")
	}
	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("iot registry: tenantId required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("iot registry: profile name required")
	}
	now := time.Now().UTC()
	profile := &models.IoTDeviceProfile{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		Name:           name,
		Description:    strings.TrimSpace(req.Description),
		SleepClass:     s.normalizeSleepClass(req.SleepClass, req.SleepyCapable),
		SleepInterval:  s.ensureSleepInterval(req.SleepInterval),
		OfflineWindow:  s.ensureOfflineWindow(req.OfflineWindow),
		LeaseProfileID: strings.TrimSpace(req.LeaseProfileID),
		SleepyCapable:  req.SleepyCapable,
		Metadata:       cloneJSON(req.Metadata),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.CreateProfile(ctx, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

// UpdateProfile mutates an existing profile in place.
func (s *Service) UpdateProfile(ctx context.Context, req ProfileUpsertRequest) (*models.IoTDeviceProfile, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("iot registry: repository not configured")
	}
	tenantID := strings.TrimSpace(req.TenantID)
	profileID := strings.TrimSpace(req.ProfileID)
	if tenantID == "" || profileID == "" {
		return nil, fmt.Errorf("iot registry: tenantId and profileId required")
	}
	existing, err := s.repo.GetProfile(ctx, tenantID, profileID)
	if err != nil {
		return nil, err
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		existing.Name = name
	}
	existing.Description = strings.TrimSpace(req.Description)
	if strings.TrimSpace(req.SleepClass) != "" || req.SleepyCapable {
		existing.SleepClass = s.normalizeSleepClass(req.SleepClass, req.SleepyCapable || existing.SleepyCapable)
	}
	if req.SleepInterval > 0 {
		existing.SleepInterval = s.ensureSleepInterval(req.SleepInterval)
	}
	if req.OfflineWindow > 0 {
		existing.OfflineWindow = s.ensureOfflineWindow(req.OfflineWindow)
	}
	existing.LeaseProfileID = strings.TrimSpace(req.LeaseProfileID)
	existing.SleepyCapable = req.SleepyCapable
	existing.Metadata = cloneJSON(req.Metadata)
	existing.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateProfile(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// ListProfiles enumerates profiles for a tenant.
func (s *Service) ListProfiles(ctx context.Context, tenantID string) ([]models.IoTDeviceProfile, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("iot registry: repository not configured")
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("iot registry: tenantId required")
	}
	return s.repo.ListProfiles(ctx, tenantID)
}

// DeleteProfile removes a profile by id.
func (s *Service) DeleteProfile(ctx context.Context, tenantID, profileID string) error {
	if s.repo == nil {
		return fmt.Errorf("iot registry: repository not configured")
	}
	tenantID = strings.TrimSpace(tenantID)
	profileID = strings.TrimSpace(profileID)
	if tenantID == "" || profileID == "" {
		return fmt.Errorf("iot registry: tenantId and profileId required")
	}
	return s.repo.DeleteProfile(ctx, tenantID, profileID)
}

func (s *Service) ensureSleepInterval(value time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	if s.defaults.SleepInterval > 0 {
		return s.defaults.SleepInterval
	}
	return 5 * time.Minute
}

func (s *Service) ensureOfflineWindow(value time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	if s.defaults.OfflineWindow > 0 {
		return s.defaults.OfflineWindow
	}
	return 2 * time.Hour
}

func (s *Service) normalizeStatus(status string) string {
	status = strings.ToUpper(strings.TrimSpace(status))
	switch status {
	case models.IoTDeviceStatusDormant:
		return models.IoTDeviceStatusDormant
	case models.IoTDeviceStatusRetired:
		return models.IoTDeviceStatusRetired
	default:
		return models.IoTDeviceStatusActive
	}
}

func (s *Service) normalizeSleepClass(class string, sleepy bool) string {
	class = strings.ToUpper(strings.TrimSpace(class))
	if class == "" && sleepy {
		class = models.IoTSleepClassSleepy
	}
	switch class {
	case models.IoTSleepClassSleepy:
		return models.IoTSleepClassSleepy
	default:
		return models.IoTSleepClassNormal
	}
}

func (s *Service) encodeStringMap(input map[string]string) []byte {
	if len(input) == 0 {
		return nil
	}
	data, err := json.Marshal(input)
	if err != nil {
		s.logger.Warn("iot registry: label marshal failed", zap.Error(err))
		return nil
	}
	return data
}

func normalizeHardware(hw string) string {
	hw = strings.TrimSpace(strings.ToLower(hw))
	hw = strings.ReplaceAll(hw, "-", ":")
	return hw
}

func cloneJSON(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return nil
	}
	buf := make([]byte, len(raw))
	copy(buf, raw)
	return buf
}

func cloneTime(src *time.Time) *time.Time {
	if src == nil {
		return nil
	}
	v := src.UTC()
	return &v
}
