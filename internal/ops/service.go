package ops

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

var (
	ErrNTPNoServers  = errors.New("ops: ntp server list is empty")
	ErrNTPSyncFailed = errors.New("ops: ntp sync probe failed")
)

// Service surfaces Ops & Support metadata plus helper utilities.
type Service struct {
	options          Options
	logger           *zap.Logger
	settingsStore    SettingsStore
	settingsDefaults SystemSettings
	settingsMu       sync.RWMutex
	settingsCache    SystemSettings
	settingsLoaded   bool

	helpMu           sync.RWMutex
	helpCache        []HelpArticle
	helpFAQCache     []HelpFAQEntry
	helpReleaseCache []ReleaseNote
	helpExpiry       time.Time
	helpLoaded       bool
	scriptRunner     *ScriptRunner
	transfer         *transferManager
}

// NewService builds an Ops service from static options.
func NewService(opts Options, store SettingsStore, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	svc := &Service{
		options:          opts,
		logger:           logger.Named("ops-support"),
		settingsStore:    store,
		settingsDefaults: buildSettingsDefaults(opts),
	}
	if opts.Scripts.Enabled {
		svc.scriptRunner = NewScriptRunner(opts.Scripts, logger)
	}
	if opts.ImportExport.Enabled {
		transfer, err := newTransferManager(opts.ImportExport, logger)
		if err != nil {
			logger.Warn("ops: import/export disabled due to configuration", zap.Error(err))
		} else {
			svc.transfer = transfer
		}
	}
	return svc
}

// SystemSummary returns current system management metadata.
func (s *Service) SystemSummary(ctx context.Context) (SystemSummary, error) {
	settings, err := s.CurrentSettings(ctx)
	if err != nil {
		return SystemSummary{}, err
	}
	summary := SystemSummary{
		Enabled:                    s.options.System.Enabled,
		AllowConfigExport:          s.options.System.AllowConfigExport,
		AllowConfigImport:          s.options.System.AllowConfigImport,
		BackupLocation:             s.options.System.BackupLocation,
		MaintenanceWindow:          settings.MaintenanceWindow,
		Theme:                      settings.Theme,
		Locale:                     settings.Locale,
		MaintenanceMode:            settings.MaintenanceMode,
		Announcement:               settings.Announcement,
		AdminSessionTimeoutMinutes: settings.AdminSessionTimeoutMinutes,
		AutoLogoutEnabled:          settings.AutoLogoutEnabled,
		SystemLogRetentionDays:     settings.SystemLogRetentionDays,
		AuditLogRetentionDays:      settings.AuditLogRetentionDays,
		LogPushEnabled:             settings.LogPushEnabled,
		LogPushEndpoint:            settings.LogPushEndpoint,
		LogPushMinLevel:            settings.LogPushMinLevel,
		LogPushChannels:            settings.LogPushChannels,
		LogPushPhones:              settings.LogPushPhones,
		LogPushDingTalkEndpoint:    settings.LogPushDingTalkEndpoint,
		LogPushFeishuEndpoint:      settings.LogPushFeishuEndpoint,
		LogPushWecomEndpoint:       settings.LogPushWecomEndpoint,
		LogPushSlackEndpoint:       settings.LogPushSlackEndpoint,
		NTPEnabled:                 settings.NTPEnabled,
		NTPServers:                 settings.NTPServers,
		NTPIntervalMinutes:         settings.NTPIntervalMinutes,
		NTPTimeoutSeconds:          settings.NTPTimeoutSeconds,
		Timezone:                   settings.Timezone,
		NTPSyncStatus:              settings.NTPSyncStatus,
		NTPLastSyncAt:              settings.NTPLastSyncAt,
		UpdatedAt:                  settings.UpdatedAt,
		UpdatedBy:                  settings.UpdatedBy,
		GeneratedAt:                time.Now().UTC(),
	}
	if summary.MaintenanceWindow == "" {
		summary.MaintenanceWindow = s.options.System.MaintenanceWindow
	}
	if summary.Theme == "" {
		summary.Theme = s.options.System.Theme
	}
	if summary.Locale == "" {
		summary.Locale = s.options.System.Locale
	}
	if summary.AdminSessionTimeoutMinutes <= 0 {
		summary.AdminSessionTimeoutMinutes = s.settingsDefaults.AdminSessionTimeoutMinutes
	}
	if summary.SystemLogRetentionDays <= 0 {
		summary.SystemLogRetentionDays = s.settingsDefaults.SystemLogRetentionDays
	}
	if summary.AuditLogRetentionDays <= 0 {
		summary.AuditLogRetentionDays = s.settingsDefaults.AuditLogRetentionDays
	}
	if summary.LogPushMinLevel == "" {
		summary.LogPushMinLevel = s.settingsDefaults.LogPushMinLevel
	}
	if summary.NTPServers == "" {
		summary.NTPServers = s.settingsDefaults.NTPServers
	}
	if summary.NTPIntervalMinutes <= 0 {
		summary.NTPIntervalMinutes = s.settingsDefaults.NTPIntervalMinutes
	}
	if summary.NTPTimeoutSeconds <= 0 {
		summary.NTPTimeoutSeconds = s.settingsDefaults.NTPTimeoutSeconds
	}
	if summary.Timezone == "" {
		summary.Timezone = s.settingsDefaults.Timezone
	}
	if summary.NTPSyncStatus == "" {
		summary.NTPSyncStatus = s.settingsDefaults.NTPSyncStatus
	}
	if summary.UpdatedBy == "" {
		summary.UpdatedBy = "system"
	}
	if summary.UpdatedAt.IsZero() {
		summary.UpdatedAt = time.Now().UTC()
	}
	return summary, nil
}

// ImportExportSummary describes bulk data pipelines and recent transfers.
func (s *Service) ImportExportSummary(ctx context.Context) ImportExportSummary {
	summary := ImportExportSummary{
		Enabled:     s.options.ImportExport.Enabled,
		StoragePath: s.options.ImportExport.StoragePath,
		ObjectStore: s.options.ImportExport.ObjectStore,
		Formats:     append([]string(nil), s.options.ImportExport.Formats...),
		MaxFileSize: s.options.ImportExport.MaxFileSize,
		GeneratedAt: time.Now().UTC(),
	}
	if !summary.Enabled || summary.StoragePath == "" {
		return summary
	}
	recent, err := s.scanRecentFiles(ctx, summary.StoragePath, 10)
	if err != nil && s.logger != nil {
		s.logger.Debug("ops import/export scan failed", zap.String("path", summary.StoragePath), zap.Error(err))
	}
	if len(recent) > 0 {
		summary.RecentTransfers = recent
	}
	return summary
}

// HelpCenter returns articles and metadata, honoring the configured cache TTL.
func (s *Service) HelpCenter(ctx context.Context) HelpCenterInfo {
	meta := HelpCenterInfo{
		Enabled:           s.options.HelpCenter.Enabled,
		BaseURL:           s.options.HelpCenter.BaseURL,
		FeaturedTopics:    append([]string(nil), s.options.HelpCenter.FeaturedTopics...),
		ExternalSearchURL: s.options.HelpCenter.ExternalSearchURL,
		GeneratedAt:       time.Now().UTC(),
	}
	if !meta.Enabled {
		return meta
	}
	content := s.loadHelpContent(ctx)
	if len(content.articles) > 0 {
		meta.Articles = content.articles
	}
	if len(content.faq) > 0 {
		meta.FAQ = content.faq
	}
	if len(content.releases) > 0 {
		meta.Releases = content.releases
	}
	return meta
}

// HelpArticles returns cached help center article summaries.
func (s *Service) HelpArticles(ctx context.Context) []HelpArticle {
	content := s.loadHelpContent(ctx)
	return content.articles
}

// FAQEntries returns frequently asked questions from the configured source.
func (s *Service) FAQEntries(ctx context.Context) []HelpFAQEntry {
	content := s.loadHelpContent(ctx)
	return content.faq
}

// ReleaseNotes returns recent release notes metadata for diagnostics surfaces.
func (s *Service) ReleaseNotes(ctx context.Context) []ReleaseNote {
	content := s.loadHelpContent(ctx)
	return content.releases
}

// SupportDirectory exposes contact points and escalation metadata.
func (s *Service) SupportDirectory() SupportDirectory {
	return SupportDirectory{
		Enabled:          s.options.Support.Enabled,
		TicketURL:        s.options.Support.TicketURL,
		ChatURL:          s.options.Support.ChatURL,
		Phone:            s.options.Support.Phone,
		EscalationPolicy: s.options.Support.EscalationPolicy,
		Contacts:         append([]string(nil), s.options.Support.Contacts...),
		GeneratedAt:      time.Now().UTC(),
	}
}

// CurrentSettings returns the latest persisted platform settings, falling back to defaults when needed.
func (s *Service) CurrentSettings(ctx context.Context) (SystemSettings, error) {
	s.settingsMu.RLock()
	if s.settingsLoaded {
		current := s.settingsCache
		s.settingsMu.RUnlock()
		return current, nil
	}
	s.settingsMu.RUnlock()

	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	if s.settingsLoaded {
		return s.settingsCache, nil
	}
	settings, err := s.resolveSettings(ctx)
	if err != nil {
		return SystemSettings{}, err
	}
	s.settingsCache = settings
	s.settingsLoaded = true
	return settings, nil
}

// UpdateSettings persists partial updates to the platform settings store.
func (s *Service) UpdateSettings(ctx context.Context, update SystemSettingsUpdate, updatedBy string) (SystemSettings, error) {
	if s.settingsStore == nil {
		return SystemSettings{}, ErrSettingsStoreUnavailable
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()

	settings, err := s.resolveSettings(ctx)
	if err != nil {
		return SystemSettings{}, err
	}
	original := settings
	changed := false

	if update.Theme != nil {
		theme := strings.TrimSpace(*update.Theme)
		if theme == "" {
			theme = s.settingsDefaults.Theme
		}
		if theme != settings.Theme {
			settings.Theme = theme
			changed = true
		}
	}
	if update.Locale != nil {
		locale := strings.TrimSpace(*update.Locale)
		if locale == "" {
			locale = s.settingsDefaults.Locale
		}
		if locale != settings.Locale {
			settings.Locale = locale
			changed = true
		}
	}
	if update.MaintenanceMode != nil && settings.MaintenanceMode != *update.MaintenanceMode {
		settings.MaintenanceMode = *update.MaintenanceMode
		changed = true
	}
	if update.MaintenanceWindow != nil {
		window := strings.TrimSpace(*update.MaintenanceWindow)
		if window != settings.MaintenanceWindow {
			settings.MaintenanceWindow = window
			changed = true
		}
	}
	if update.Announcement != nil {
		announcement := strings.TrimSpace(*update.Announcement)
		if announcement != settings.Announcement {
			settings.Announcement = announcement
			changed = true
		}
	}
	if update.AdminSessionTimeoutMinutes != nil {
		minutes := *update.AdminSessionTimeoutMinutes
		if minutes <= 0 {
			minutes = s.settingsDefaults.AdminSessionTimeoutMinutes
		}
		if minutes != settings.AdminSessionTimeoutMinutes {
			settings.AdminSessionTimeoutMinutes = minutes
			changed = true
		}
	}
	if update.AutoLogoutEnabled != nil && settings.AutoLogoutEnabled != *update.AutoLogoutEnabled {
		settings.AutoLogoutEnabled = *update.AutoLogoutEnabled
		changed = true
	}
	if update.SystemLogRetentionDays != nil {
		days := *update.SystemLogRetentionDays
		if days <= 0 {
			days = s.settingsDefaults.SystemLogRetentionDays
		}
		if days != settings.SystemLogRetentionDays {
			settings.SystemLogRetentionDays = days
			changed = true
		}
	}
	if update.AuditLogRetentionDays != nil {
		days := *update.AuditLogRetentionDays
		if days <= 0 {
			days = s.settingsDefaults.AuditLogRetentionDays
		}
		if days != settings.AuditLogRetentionDays {
			settings.AuditLogRetentionDays = days
			changed = true
		}
	}
	if update.LogPushEnabled != nil && settings.LogPushEnabled != *update.LogPushEnabled {
		settings.LogPushEnabled = *update.LogPushEnabled
		changed = true
	}
	if update.LogPushEndpoint != nil {
		endpoint := strings.TrimSpace(*update.LogPushEndpoint)
		if endpoint != settings.LogPushEndpoint {
			settings.LogPushEndpoint = endpoint
			changed = true
		}
	}
	if update.LogPushMinLevel != nil {
		level := strings.ToLower(strings.TrimSpace(*update.LogPushMinLevel))
		if level == "" {
			level = s.settingsDefaults.LogPushMinLevel
		}
		if level != settings.LogPushMinLevel {
			settings.LogPushMinLevel = level
			changed = true
		}
	}
	if update.LogPushChannels != nil {
		channels := normalizeLogPushChannels(*update.LogPushChannels)
		if channels != settings.LogPushChannels {
			settings.LogPushChannels = channels
			changed = true
		}
	}
	if update.LogPushPhones != nil {
		phones := strings.TrimSpace(*update.LogPushPhones)
		if phones != settings.LogPushPhones {
			settings.LogPushPhones = phones
			changed = true
		}
	}
	if update.LogPushDingTalkEndpoint != nil {
		value := strings.TrimSpace(*update.LogPushDingTalkEndpoint)
		if value != settings.LogPushDingTalkEndpoint {
			settings.LogPushDingTalkEndpoint = value
			changed = true
		}
	}
	if update.LogPushFeishuEndpoint != nil {
		value := strings.TrimSpace(*update.LogPushFeishuEndpoint)
		if value != settings.LogPushFeishuEndpoint {
			settings.LogPushFeishuEndpoint = value
			changed = true
		}
	}
	if update.LogPushWecomEndpoint != nil {
		value := strings.TrimSpace(*update.LogPushWecomEndpoint)
		if value != settings.LogPushWecomEndpoint {
			settings.LogPushWecomEndpoint = value
			changed = true
		}
	}
	if update.LogPushSlackEndpoint != nil {
		value := strings.TrimSpace(*update.LogPushSlackEndpoint)
		if value != settings.LogPushSlackEndpoint {
			settings.LogPushSlackEndpoint = value
			changed = true
		}
	}
	if update.NTPEnabled != nil && settings.NTPEnabled != *update.NTPEnabled {
		settings.NTPEnabled = *update.NTPEnabled
		changed = true
	}
	if update.NTPServers != nil {
		servers := strings.TrimSpace(*update.NTPServers)
		if servers == "" {
			servers = s.settingsDefaults.NTPServers
		}
		if servers != settings.NTPServers {
			settings.NTPServers = servers
			changed = true
		}
	}
	if update.NTPIntervalMinutes != nil {
		minutes := *update.NTPIntervalMinutes
		if minutes <= 0 {
			minutes = s.settingsDefaults.NTPIntervalMinutes
		}
		if minutes != settings.NTPIntervalMinutes {
			settings.NTPIntervalMinutes = minutes
			changed = true
		}
	}
	if update.NTPTimeoutSeconds != nil {
		seconds := *update.NTPTimeoutSeconds
		if seconds <= 0 {
			seconds = s.settingsDefaults.NTPTimeoutSeconds
		}
		if seconds != settings.NTPTimeoutSeconds {
			settings.NTPTimeoutSeconds = seconds
			changed = true
		}
	}
	if update.Timezone != nil {
		timezone := strings.TrimSpace(*update.Timezone)
		if timezone == "" {
			timezone = s.settingsDefaults.Timezone
		}
		if timezone != settings.Timezone {
			settings.Timezone = timezone
			changed = true
		}
	}
	if update.NTPSyncStatus != nil {
		status := strings.TrimSpace(*update.NTPSyncStatus)
		if status == "" {
			status = s.settingsDefaults.NTPSyncStatus
		}
		if status != settings.NTPSyncStatus {
			settings.NTPSyncStatus = status
			changed = true
		}
	}
	if update.NTPLastSyncAt != nil {
		if settings.NTPLastSyncAt == nil || !settings.NTPLastSyncAt.Equal(*update.NTPLastSyncAt) {
			settings.NTPLastSyncAt = update.NTPLastSyncAt
			changed = true
		}
	}
	if !changed {
		s.settingsCache = original
		s.settingsLoaded = true
		return original, nil
	}
	if updatedBy == "" {
		updatedBy = "system"
	}
	settings.UpdatedBy = updatedBy
	saved, saveErr := s.settingsStore.Save(ctx, settings)
	if saveErr != nil {
		return SystemSettings{}, saveErr
	}
	merged := s.mergeDefaults(saved)
	s.settingsCache = merged
	s.settingsLoaded = true
	return merged, nil
}

// SyncNTP probes configured NTP servers and persists the latest probe status.
func (s *Service) SyncNTP(ctx context.Context, updatedBy string) (SystemSettings, error) {
	if s.settingsStore == nil {
		return SystemSettings{}, ErrSettingsStoreUnavailable
	}

	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()

	settings, err := s.resolveSettings(ctx)
	if err != nil {
		return SystemSettings{}, err
	}

	servers := parseNTPServers(settings.NTPServers)
	if len(servers) == 0 {
		return settings, ErrNTPNoServers
	}

	timeout := time.Duration(settings.NTPTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	now := time.Now().UTC()
	if updatedBy == "" {
		updatedBy = "system"
	}

	var lastErr error
	for _, server := range servers {
		select {
		case <-ctx.Done():
			return settings, ctx.Err()
		default:
		}
		if _, probeErr := probeNTPServer(ctx, server, timeout); probeErr == nil {
			status := "ok"
			settings.NTPSyncStatus = status
			settings.NTPLastSyncAt = &now
			settings.UpdatedBy = updatedBy
			saved, saveErr := s.settingsStore.Save(ctx, settings)
			if saveErr != nil {
				return SystemSettings{}, saveErr
			}
			merged := s.mergeDefaults(saved)
			s.settingsCache = merged
			s.settingsLoaded = true
			return merged, nil
		} else {
			lastErr = probeErr
		}
	}

	status := "failed"
	settings.NTPSyncStatus = status
	settings.UpdatedBy = updatedBy
	saved, saveErr := s.settingsStore.Save(ctx, settings)
	if saveErr != nil {
		return SystemSettings{}, saveErr
	}
	merged := s.mergeDefaults(saved)
	s.settingsCache = merged
	s.settingsLoaded = true
	if lastErr == nil {
		lastErr = ErrNTPSyncFailed
	}
	return merged, fmt.Errorf("%w: %v", ErrNTPSyncFailed, lastErr)
}

func parseNTPServers(raw string) []string {
	replacer := strings.NewReplacer(",", "\n", ";", "\n", "\t", "\n", " ", "\n")
	normalized := replacer.Replace(raw)
	parts := strings.Split(normalized, "\n")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, item := range parts {
		candidate := strings.TrimSpace(item)
		if candidate == "" {
			continue
		}
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, candidate)
	}
	return result
}

func normalizeLogPushChannels(raw string) string {
	parts := strings.Split(strings.ToLower(raw), ",")
	allowed := map[string]struct{}{
		"dingtalk": {},
		"feishu":   {},
		"wecom":    {},
		"slack":    {},
		"phone":    {},
	}
	seen := make(map[string]struct{}, len(parts))
	clean := make([]string, 0, len(parts))
	for _, item := range parts {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, ok := allowed[v]; !ok {
			continue
		}
		if _, exists := seen[v]; exists {
			continue
		}
		seen[v] = struct{}{}
		clean = append(clean, v)
	}
	return strings.Join(clean, ",")
}

func normalizeNTPAddress(server string) string {
	candidate := strings.TrimSpace(server)
	if candidate == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(candidate); err == nil {
		return candidate
	}
	if strings.Count(candidate, ":") > 1 && !strings.HasPrefix(candidate, "[") {
		return net.JoinHostPort(candidate, "123")
	}
	if strings.HasPrefix(candidate, "[") && strings.HasSuffix(candidate, "]") {
		return candidate + ":123"
	}
	if strings.Contains(candidate, ":") {
		return candidate
	}
	return net.JoinHostPort(candidate, "123")
}

func probeNTPServer(ctx context.Context, server string, timeout time.Duration) (time.Time, error) {
	addr := normalizeNTPAddress(server)
	if addr == "" {
		return time.Time{}, fmt.Errorf("empty ntp server")
	}

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "udp", addr)
	if err != nil {
		return time.Time{}, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	request := make([]byte, 48)
	request[0] = 0x1B
	if _, err := conn.Write(request); err != nil {
		return time.Time{}, err
	}

	response := make([]byte, 48)
	if _, err := conn.Read(response); err != nil {
		return time.Time{}, err
	}

	seconds := binary.BigEndian.Uint32(response[40:44])
	fraction := binary.BigEndian.Uint32(response[44:48])
	const ntpUnixOffset = 2208988800
	unixSeconds := int64(seconds) - ntpUnixOffset
	nanoseconds := (int64(fraction) * int64(time.Second)) >> 32
	return time.Unix(unixSeconds, nanoseconds).UTC(), nil
}

// ScriptCatalog describes available automation scripts.
func (s *Service) ScriptCatalog() ScriptCatalog {
	return ScriptCatalog{
		Enabled:        s.options.Scripts.Enabled,
		DefaultTimeout: s.options.Scripts.DefaultTimeout,
		MaxConcurrent:  s.options.Scripts.MaxConcurrent,
		SandboxImage:   s.options.Scripts.SandboxImage,
		Entries:        cloneScriptCatalog(s.options.Scripts.Catalog),
	}
}

func (s *Service) scanRecentFiles(ctx context.Context, basePath string, limit int) ([]TransferMetadata, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil, err
	}
	meta := make([]TransferMetadata, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			if s.logger != nil {
				s.logger.Debug("ops: stat export failed", zap.String("name", entry.Name()), zap.Error(err))
			}
			continue
		}
		meta = append(meta, TransferMetadata{
			Name:       entry.Name(),
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime().UTC(),
		})
	}
	sort.Slice(meta, func(i, j int) bool {
		return meta[i].ModifiedAt.After(meta[j].ModifiedAt)
	})
	if limit > 0 && len(meta) > limit {
		meta = meta[:limit]
	}
	return meta, nil
}

type helpContent struct {
	articles []HelpArticle
	faq      []HelpFAQEntry
	releases []ReleaseNote
}

func (s *Service) loadHelpContent(ctx context.Context) helpContent {
	cacheTTL := s.options.HelpCenter.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = 5 * time.Minute
	}
	s.helpMu.RLock()
	if s.helpLoaded && time.Now().Before(s.helpExpiry) {
		content := helpContent{
			articles: cloneArticles(s.helpCache),
			faq:      cloneFAQEntries(s.helpFAQCache),
			releases: cloneReleaseNotes(s.helpReleaseCache),
		}
		s.helpMu.RUnlock()
		return content
	}
	s.helpMu.RUnlock()

	s.helpMu.Lock()
	defer s.helpMu.Unlock()
	if s.helpLoaded && time.Now().Before(s.helpExpiry) {
		return helpContent{
			articles: cloneArticles(s.helpCache),
			faq:      cloneFAQEntries(s.helpFAQCache),
			releases: cloneReleaseNotes(s.helpReleaseCache),
		}
	}
	articles, articleErr := readHelpArticles(ctx, s.options.HelpCenter)
	if articleErr != nil && !errors.Is(articleErr, fs.ErrNotExist) && s.logger != nil {
		s.logger.Debug("ops: help center scan failed", zap.String("path", s.options.HelpCenter.ArticlesPath), zap.Error(articleErr))
	}
	faq, faqErr := readFAQEntries(ctx, s.options.HelpCenter)
	if faqErr != nil && !errors.Is(faqErr, fs.ErrNotExist) && s.logger != nil {
		s.logger.Debug("ops: faq scan failed", zap.String("path", s.options.HelpCenter.FAQPath), zap.Error(faqErr))
	}
	releases, releaseErr := readReleaseNotes(ctx, s.options.HelpCenter)
	if releaseErr != nil && !errors.Is(releaseErr, fs.ErrNotExist) && s.logger != nil {
		s.logger.Debug("ops: release notes scan failed", zap.String("path", s.options.HelpCenter.ReleaseNotesPath), zap.Error(releaseErr))
	}
	s.helpCache = articles
	s.helpFAQCache = faq
	s.helpReleaseCache = releases
	s.helpExpiry = time.Now().Add(cacheTTL)
	s.helpLoaded = true
	return helpContent{
		articles: cloneArticles(articles),
		faq:      cloneFAQEntries(faq),
		releases: cloneReleaseNotes(releases),
	}
}

func readHelpArticles(ctx context.Context, opts HelpCenterOptions) ([]HelpArticle, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if strings.TrimSpace(opts.ArticlesPath) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(opts.ArticlesPath)
	if err != nil {
		return nil, err
	}
	articles := make([]HelpArticle, 0, len(entries))
	baseURL := strings.TrimRight(opts.BaseURL, "/")
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !(strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".markdown") || strings.HasSuffix(name, ".txt")) {
			continue
		}
		path := filepath.Join(opts.ArticlesPath, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}
		article, err := parseHelpArticle(path, baseURL, info.ModTime())
		if err != nil {
			continue
		}
		articles = append(articles, article)
	}
	sort.Slice(articles, func(i, j int) bool {
		return articles[i].UpdatedAt.After(articles[j].UpdatedAt)
	})
	return articles, nil
}

func readFAQEntries(ctx context.Context, opts HelpCenterOptions) ([]HelpFAQEntry, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	path := strings.TrimSpace(opts.FAQPath)
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Entries []HelpFAQEntry `yaml:"faq"`
	}
	if err := yaml.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	sort.Slice(payload.Entries, func(i, j int) bool {
		return payload.Entries[i].UpdatedAt.After(payload.Entries[j].UpdatedAt)
	})
	return payload.Entries, nil
}

func readReleaseNotes(ctx context.Context, opts HelpCenterOptions) ([]ReleaseNote, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	path := strings.TrimSpace(opts.ReleaseNotesPath)
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Releases []ReleaseNote `yaml:"releases"`
	}
	if err := yaml.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	sort.Slice(payload.Releases, func(i, j int) bool {
		return payload.Releases[i].PublishedAt.After(payload.Releases[j].PublishedAt)
	})
	return payload.Releases, nil
}

func parseHelpArticle(path, baseURL string, modTime time.Time) (HelpArticle, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return HelpArticle{}, err
	}
	lines := strings.Split(string(raw), "\n")
	title := filepath.Base(path)
	summary := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if title == filepath.Base(path) && strings.HasPrefix(trimmed, "#") {
			title = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			continue
		}
		if summary == "" && trimmed != "" {
			summary = trimmed
		}
		if title != filepath.Base(path) && summary != "" {
			break
		}
	}
	id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	article := HelpArticle{
		ID:        id,
		Title:     title,
		Summary:   summary,
		Path:      path,
		UpdatedAt: modTime.UTC(),
	}
	if baseURL != "" {
		article.URL = baseURL + "/" + id
	}
	return article, nil
}

func cloneArticles(src []HelpArticle) []HelpArticle {
	if len(src) == 0 {
		return nil
	}
	cpy := make([]HelpArticle, len(src))
	copy(cpy, src)
	return cpy
}

func cloneFAQEntries(src []HelpFAQEntry) []HelpFAQEntry {
	if len(src) == 0 {
		return nil
	}
	cpy := make([]HelpFAQEntry, len(src))
	copy(cpy, src)
	return cpy
}

func cloneReleaseNotes(src []ReleaseNote) []ReleaseNote {
	if len(src) == 0 {
		return nil
	}
	cpy := make([]ReleaseNote, len(src))
	copy(cpy, src)
	return cpy
}

func cloneScriptCatalog(src []ScriptDescriptor) []ScriptDescriptor {
	if len(src) == 0 {
		return nil
	}
	cpy := make([]ScriptDescriptor, len(src))
	for i, entry := range src {
		cpy[i] = ScriptDescriptor{
			Name:             entry.Name,
			Description:      entry.Description,
			Command:          entry.Command,
			Args:             append([]string(nil), entry.Args...),
			AllowedRoles:     append([]string(nil), entry.AllowedRoles...),
			RequiresApproval: entry.RequiresApproval,
		}
	}
	return cpy
}

func (s *Service) resolveSettings(ctx context.Context) (SystemSettings, error) {
	if s.settingsStore == nil {
		return s.mergeDefaults(s.settingsDefaults), nil
	}
	settings, err := s.settingsStore.Load(ctx)
	if err != nil {
		return SystemSettings{}, err
	}
	return s.mergeDefaults(settings), nil
}

func (s *Service) mergeDefaults(settings SystemSettings) SystemSettings {
	merged := settings
	if merged.Theme == "" {
		merged.Theme = s.settingsDefaults.Theme
	}
	if merged.Locale == "" {
		merged.Locale = s.settingsDefaults.Locale
	}
	if merged.MaintenanceWindow == "" {
		merged.MaintenanceWindow = s.settingsDefaults.MaintenanceWindow
	}
	if merged.Announcement == "" {
		merged.Announcement = s.settingsDefaults.Announcement
	}
	if merged.AdminSessionTimeoutMinutes <= 0 {
		merged.AdminSessionTimeoutMinutes = s.settingsDefaults.AdminSessionTimeoutMinutes
	}
	if merged.SystemLogRetentionDays <= 0 {
		merged.SystemLogRetentionDays = s.settingsDefaults.SystemLogRetentionDays
	}
	if merged.AuditLogRetentionDays <= 0 {
		merged.AuditLogRetentionDays = s.settingsDefaults.AuditLogRetentionDays
	}
	if merged.LogPushMinLevel == "" {
		merged.LogPushMinLevel = s.settingsDefaults.LogPushMinLevel
	}
	if merged.LogPushChannels == "" {
		merged.LogPushChannels = s.settingsDefaults.LogPushChannels
	}
	if merged.LogPushPhones == "" {
		merged.LogPushPhones = s.settingsDefaults.LogPushPhones
	}
	if merged.NTPServers == "" {
		merged.NTPServers = s.settingsDefaults.NTPServers
	}
	if merged.NTPIntervalMinutes <= 0 {
		merged.NTPIntervalMinutes = s.settingsDefaults.NTPIntervalMinutes
	}
	if merged.NTPTimeoutSeconds <= 0 {
		merged.NTPTimeoutSeconds = s.settingsDefaults.NTPTimeoutSeconds
	}
	if merged.Timezone == "" {
		merged.Timezone = s.settingsDefaults.Timezone
	}
	if merged.NTPSyncStatus == "" {
		merged.NTPSyncStatus = s.settingsDefaults.NTPSyncStatus
	}
	if merged.UpdatedBy == "" {
		merged.UpdatedBy = s.settingsDefaults.UpdatedBy
	}
	if merged.UpdatedAt.IsZero() {
		merged.UpdatedAt = s.settingsDefaults.UpdatedAt
	}
	return merged
}

func buildSettingsDefaults(opts Options) SystemSettings {
	return SystemSettings{
		Theme:                      strings.TrimSpace(opts.System.Theme),
		Locale:                     strings.TrimSpace(opts.System.Locale),
		MaintenanceMode:            opts.System.MaintenanceMode,
		MaintenanceWindow:          strings.TrimSpace(opts.System.MaintenanceWindow),
		Announcement:               strings.TrimSpace(opts.System.Announcement),
		AdminSessionTimeoutMinutes: 30,
		AutoLogoutEnabled:          true,
		SystemLogRetentionDays:     30,
		AuditLogRetentionDays:      180,
		LogPushEnabled:             false,
		LogPushEndpoint:            "",
		LogPushMinLevel:            "warning",
		LogPushChannels:            "",
		LogPushPhones:              "",
		LogPushDingTalkEndpoint:    "",
		LogPushFeishuEndpoint:      "",
		LogPushWecomEndpoint:       "",
		LogPushSlackEndpoint:       "",
		NTPEnabled:                 true,
		NTPServers:                 "pool.ntp.org\nntp.aliyun.com\ntime.cloudflare.com",
		NTPIntervalMinutes:         30,
		NTPTimeoutSeconds:          5,
		Timezone:                   "Asia/Shanghai",
		NTPSyncStatus:              "idle",
		UpdatedAt:                  time.Now().UTC(),
		UpdatedBy:                  "system",
	}
}

// SystemSettingsUpdate captures partial updates to platform settings.
type SystemSettingsUpdate struct {
	Theme                      *string
	Locale                     *string
	MaintenanceMode            *bool
	MaintenanceWindow          *string
	Announcement               *string
	AdminSessionTimeoutMinutes *int
	AutoLogoutEnabled          *bool
	SystemLogRetentionDays     *int
	AuditLogRetentionDays      *int
	LogPushEnabled             *bool
	LogPushEndpoint            *string
	LogPushMinLevel            *string
	LogPushChannels            *string
	LogPushPhones              *string
	LogPushDingTalkEndpoint    *string
	LogPushFeishuEndpoint      *string
	LogPushWecomEndpoint       *string
	LogPushSlackEndpoint       *string
	NTPEnabled                 *bool
	NTPServers                 *string
	NTPIntervalMinutes         *int
	NTPTimeoutSeconds          *int
	Timezone                   *string
	NTPSyncStatus              *string
	NTPLastSyncAt              *time.Time
}

// StartScriptRun executes an approved script if the runner is enabled.
func (s *Service) StartScriptRun(ctx context.Context, input StartScriptRunInput) (*ScriptRun, error) {
	runner := s.scriptRunner
	if runner == nil {
		return nil, ErrScriptsDisabled
	}
	return runner.Start(ctx, input)
}

// ListScriptRuns returns the most recent script runs.
func (s *Service) ListScriptRuns(_ context.Context, limit int) ([]ScriptRun, error) {
	runner := s.scriptRunner
	if runner == nil {
		return nil, ErrScriptsDisabled
	}
	return runner.List(limit), nil
}

// GetScriptRun fetches a specific script run by ID.
func (s *Service) GetScriptRun(_ context.Context, runID string) (*ScriptRun, error) {
	runner := s.scriptRunner
	if runner == nil {
		return nil, ErrScriptsDisabled
	}
	return runner.Get(runID)
}

// ApproveScriptRun authorizes a pending script run and dispatches it for execution.
func (s *Service) ApproveScriptRun(_ context.Context, runID, approver, note string) (*ScriptRun, error) {
	runner := s.scriptRunner
	if runner == nil {
		return nil, ErrScriptsDisabled
	}
	return runner.Approve(runID, approver, note)
}

// RejectScriptRun denies a pending script run.
func (s *Service) RejectScriptRun(_ context.Context, runID, approver, note string) (*ScriptRun, error) {
	runner := s.scriptRunner
	if runner == nil {
		return nil, ErrScriptsDisabled
	}
	return runner.Reject(runID, approver, note)
}

// StartTransferJob dispatches a bulk import/export job.
func (s *Service) StartTransferJob(ctx context.Context, req TransferRequest) (*TransferJob, error) {
	if s.transfer == nil {
		return nil, ErrImportExportDisabled
	}
	return s.transfer.StartTransfer(ctx, req)
}

// ListTransferJobs returns the most recent transfer jobs.
func (s *Service) ListTransferJobs(_ context.Context, limit int) ([]TransferJob, error) {
	if s.transfer == nil {
		return nil, ErrImportExportDisabled
	}
	return s.transfer.List(limit), nil
}

// GetTransferJob returns a specific transfer job by ID.
func (s *Service) GetTransferJob(_ context.Context, jobID string) (*TransferJob, error) {
	if s.transfer == nil {
		return nil, ErrImportExportDisabled
	}
	return s.transfer.Get(jobID)
}

// MaintenancePlan returns maintenance playbooks, windows, and upgrade plans.
func (s *Service) MaintenancePlan(_ context.Context) MaintenancePlan {
	plan := MaintenancePlan{
		Enabled:     s.options.Maintenance.Enabled,
		GeneratedAt: time.Now().UTC(),
	}
	for _, playbook := range s.options.Maintenance.Playbooks {
		plan.Playbooks = append(plan.Playbooks, maintenancePlaybookFromOption(playbook))
	}
	for _, window := range s.options.Maintenance.Windows {
		plan.Windows = append(plan.Windows, maintenanceWindowFromOption(window))
	}
	for _, upgrade := range s.options.Maintenance.UpgradePlans {
		plan.UpgradePlans = append(plan.UpgradePlans, maintenanceUpgradeFromOption(upgrade))
	}
	return plan
}

// BackupWizard returns the configured backup schedules and restore workflows.
func (s *Service) BackupWizard(_ context.Context) BackupWizard {
	wizard := BackupWizard{
		Enabled:     s.options.Backups.Enabled,
		GeneratedAt: time.Now().UTC(),
	}
	for _, schedule := range s.options.Backups.Schedules {
		wizard.Schedules = append(wizard.Schedules, backupScheduleFromOption(schedule))
	}
	for _, destination := range s.options.Backups.Destinations {
		wizard.Destinations = append(wizard.Destinations, backupDestinationFromOption(destination))
	}
	for _, flow := range s.options.Backups.RestoreFlows {
		wizard.RestoreFlows = append(wizard.RestoreFlows, restoreWorkflowFromOption(flow))
	}
	wizard.Verification = backupVerificationFromOption(s.options.Backups.Verification)
	return wizard
}

// PerformanceDiagnostics returns configured probes and remediation guidance.
func (s *Service) PerformanceDiagnostics(_ context.Context) PerformanceDiagnostics {
	diagnostics := PerformanceDiagnostics{
		Enabled:     s.options.Performance.Enabled,
		GeneratedAt: time.Now().UTC(),
	}
	for _, probe := range s.options.Performance.Probes {
		diagnostics.Probes = append(diagnostics.Probes, performanceProbeFromOption(probe))
	}
	for _, indicator := range s.options.Performance.Indicators {
		diagnostics.Indicators = append(diagnostics.Indicators, performanceIndicatorFromOption(indicator))
	}
	for _, playbook := range s.options.Performance.Recommendations {
		diagnostics.Recommendations = append(diagnostics.Recommendations, performancePlaybookFromOption(playbook))
	}
	return diagnostics
}

// SystemSummary captures management metadata for the platform.
type SystemSummary struct {
	Enabled                    bool       `json:"enabled"`
	AllowConfigExport          bool       `json:"allowConfigExport"`
	AllowConfigImport          bool       `json:"allowConfigImport"`
	BackupLocation             string     `json:"backupLocation,omitempty"`
	MaintenanceWindow          string     `json:"maintenanceWindow,omitempty"`
	Theme                      string     `json:"theme"`
	Locale                     string     `json:"locale"`
	MaintenanceMode            bool       `json:"maintenanceMode"`
	Announcement               string     `json:"announcement,omitempty"`
	AdminSessionTimeoutMinutes int        `json:"adminSessionTimeoutMinutes"`
	AutoLogoutEnabled          bool       `json:"autoLogoutEnabled"`
	SystemLogRetentionDays     int        `json:"systemLogRetentionDays"`
	AuditLogRetentionDays      int        `json:"auditLogRetentionDays"`
	LogPushEnabled             bool       `json:"logPushEnabled"`
	LogPushEndpoint            string     `json:"logPushEndpoint,omitempty"`
	LogPushMinLevel            string     `json:"logPushMinLevel,omitempty"`
	LogPushChannels            string     `json:"logPushChannels,omitempty"`
	LogPushPhones              string     `json:"logPushPhones,omitempty"`
	LogPushDingTalkEndpoint    string     `json:"logPushDingTalkEndpoint,omitempty"`
	LogPushFeishuEndpoint      string     `json:"logPushFeishuEndpoint,omitempty"`
	LogPushWecomEndpoint       string     `json:"logPushWecomEndpoint,omitempty"`
	LogPushSlackEndpoint       string     `json:"logPushSlackEndpoint,omitempty"`
	NTPEnabled                 bool       `json:"ntpEnabled"`
	NTPServers                 string     `json:"ntpServers,omitempty"`
	NTPIntervalMinutes         int        `json:"ntpIntervalMinutes"`
	NTPTimeoutSeconds          int        `json:"ntpTimeoutSeconds"`
	Timezone                   string     `json:"timezone,omitempty"`
	NTPSyncStatus              string     `json:"ntpSyncStatus,omitempty"`
	NTPLastSyncAt              *time.Time `json:"ntpLastSyncAt,omitempty"`
	UpdatedAt                  time.Time  `json:"updatedAt"`
	UpdatedBy                  string     `json:"updatedBy"`
	GeneratedAt                time.Time  `json:"generatedAt"`
}

// ImportExportSummary captures import/export settings and recent transfers.
type ImportExportSummary struct {
	Enabled         bool               `json:"enabled"`
	StoragePath     string             `json:"storagePath,omitempty"`
	ObjectStore     string             `json:"objectStore,omitempty"`
	Formats         []string           `json:"formats,omitempty"`
	MaxFileSize     int64              `json:"maxFileSize"`
	RecentTransfers []TransferMetadata `json:"recentTransfers,omitempty"`
	GeneratedAt     time.Time          `json:"generatedAt"`
}

// TransferMetadata describes a single import/export artifact.
type TransferMetadata struct {
	Name       string    `json:"name"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

// HelpCenterInfo captures knowledge base metadata.
type HelpCenterInfo struct {
	Enabled           bool           `json:"enabled"`
	BaseURL           string         `json:"baseUrl,omitempty"`
	FeaturedTopics    []string       `json:"featuredTopics,omitempty"`
	ExternalSearchURL string         `json:"externalSearchUrl,omitempty"`
	Articles          []HelpArticle  `json:"articles,omitempty"`
	FAQ               []HelpFAQEntry `json:"faq,omitempty"`
	Releases          []ReleaseNote  `json:"releases,omitempty"`
	GeneratedAt       time.Time      `json:"generatedAt"`
}

// HelpArticle summarizes a single help center article.
type HelpArticle struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary,omitempty"`
	URL       string    `json:"url,omitempty"`
	Path      string    `json:"path,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// HelpFAQEntry captures frequently asked question metadata.
type HelpFAQEntry struct {
	ID        string    `json:"id" yaml:"id"`
	Question  string    `json:"question" yaml:"question"`
	Answer    string    `json:"answer" yaml:"answer"`
	Category  string    `json:"category,omitempty" yaml:"category"`
	UpdatedAt time.Time `json:"updatedAt" yaml:"updatedAt"`
}

// ReleaseNote represents a platform release entry.
type ReleaseNote struct {
	Version     string            `json:"version" yaml:"version"`
	Title       string            `json:"title" yaml:"title"`
	Highlights  []string          `json:"highlights" yaml:"highlights"`
	Metadata    map[string]string `json:"metadata,omitempty" yaml:"metadata"`
	PublishedAt time.Time         `json:"publishedAt" yaml:"publishedAt"`
}

// SupportDirectory exposes support contact metadata.
type SupportDirectory struct {
	Enabled          bool      `json:"enabled"`
	TicketURL        string    `json:"ticketUrl,omitempty"`
	ChatURL          string    `json:"chatUrl,omitempty"`
	Phone            string    `json:"phone,omitempty"`
	EscalationPolicy string    `json:"escalationPolicy,omitempty"`
	Contacts         []string  `json:"contacts,omitempty"`
	GeneratedAt      time.Time `json:"generatedAt"`
}

// ScriptCatalog summarizes script runner metadata.
type ScriptCatalog struct {
	Enabled        bool               `json:"enabled"`
	DefaultTimeout time.Duration      `json:"defaultTimeout"`
	MaxConcurrent  int                `json:"maxConcurrent"`
	SandboxImage   string             `json:"sandboxImage,omitempty"`
	Entries        []ScriptDescriptor `json:"entries,omitempty"`
}

// MaintenancePlan aggregates maintenance playbooks and schedules.
type MaintenancePlan struct {
	Enabled      bool                     `json:"enabled"`
	Playbooks    []MaintenancePlaybook    `json:"playbooks,omitempty"`
	Windows      []MaintenanceWindow      `json:"windows,omitempty"`
	UpgradePlans []MaintenanceUpgradePlan `json:"upgradePlans,omitempty"`
	GeneratedAt  time.Time                `json:"generatedAt"`
}

// MaintenancePlaybook represents a wizard for maintenance execution.
type MaintenancePlaybook struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Steps       []MaintenanceStep `json:"steps,omitempty"`
}

// MaintenanceWindow captures a recurring maintenance window.
type MaintenanceWindow struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Cron            string `json:"cron"`
	DurationMinutes int64  `json:"durationMinutes"`
	Timezone        string `json:"timezone,omitempty"`
}

// MaintenanceUpgradePlan captures upgrade execution metadata.
type MaintenanceUpgradePlan struct {
	ID            string            `json:"id"`
	Version       string            `json:"version"`
	Summary       string            `json:"summary,omitempty"`
	ScheduledFor  *time.Time        `json:"scheduledFor,omitempty"`
	Prerequisites []string          `json:"prerequisites,omitempty"`
	Steps         []MaintenanceStep `json:"steps,omitempty"`
	RollbackPlan  []MaintenanceStep `json:"rollbackPlan,omitempty"`
	ReleaseNotes  string            `json:"releaseNotes,omitempty"`
}

// MaintenanceStep describes a single maintenance action.
type MaintenanceStep struct {
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	Summary           string   `json:"summary,omitempty"`
	DurationSeconds   int64    `json:"durationSeconds,omitempty"`
	Responsible       string   `json:"responsible,omitempty"`
	RequiresApproval  bool     `json:"requiresApproval"`
	DependsOn         []string `json:"dependsOn,omitempty"`
	AutomationJobType string   `json:"automationJobType,omitempty"`
}

// BackupWizard aggregates backup policies and restore flows.
type BackupWizard struct {
	Enabled      bool                      `json:"enabled"`
	Schedules    []BackupSchedule          `json:"schedules,omitempty"`
	Destinations []BackupDestination       `json:"destinations,omitempty"`
	RestoreFlows []RestoreWorkflow         `json:"restoreWorkflows,omitempty"`
	Verification BackupVerificationSummary `json:"verification"`
	GeneratedAt  time.Time                 `json:"generatedAt"`
}

// BackupSchedule describes a recurring backup job.
type BackupSchedule struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Cron           string   `json:"cron"`
	RetentionHours int64    `json:"retentionHours"`
	WindowMinutes  int64    `json:"windowMinutes"`
	Type           string   `json:"type"`
	Enabled        bool     `json:"enabled"`
	Destinations   []string `json:"destinations,omitempty"`
}

// BackupDestination summarizes a backup target.
type BackupDestination struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Kind     string            `json:"kind"`
	Endpoint string            `json:"endpoint,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RestoreWorkflow captures guided restore actions.
type RestoreWorkflow struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Steps       []MaintenanceStep `json:"steps,omitempty"`
	Checks      []string          `json:"checks,omitempty"`
	Approvals   []string          `json:"approvals,omitempty"`
}

// BackupVerificationSummary captures verification settings.
type BackupVerificationSummary struct {
	Enabled      bool   `json:"enabled"`
	Schedule     string `json:"schedule,omitempty"`
	Retention    int    `json:"retention,omitempty"`
	Sandbox      string `json:"sandbox,omitempty"`
	AlertChannel string `json:"alertChannel,omitempty"`
}

// PerformanceDiagnostics aggregates probes and remediation playbooks.
type PerformanceDiagnostics struct {
	Enabled         bool                   `json:"enabled"`
	Probes          []PerformanceProbe     `json:"probes,omitempty"`
	Indicators      []PerformanceIndicator `json:"indicators,omitempty"`
	Recommendations []PerformancePlaybook  `json:"recommendations,omitempty"`
	GeneratedAt     time.Time              `json:"generatedAt"`
}

// PerformanceProbe describes a diagnostic probe configuration.
type PerformanceProbe struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Description     string             `json:"description,omitempty"`
	Command         string             `json:"command,omitempty"`
	IntervalSeconds int64              `json:"intervalSeconds,omitempty"`
	SLO             float64            `json:"slo,omitempty"`
	Units           string             `json:"units,omitempty"`
	Thresholds      map[string]float64 `json:"thresholds,omitempty"`
}

// PerformanceIndicator captures KPI targets for diagnostics.
type PerformanceIndicator struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Target      float64 `json:"target,omitempty"`
	Units       string  `json:"units,omitempty"`
}

// PerformancePlaybook documents remediation guidance.
type PerformancePlaybook struct {
	ID      string            `json:"id"`
	Title   string            `json:"title"`
	Summary string            `json:"summary,omitempty"`
	Impact  string            `json:"impact,omitempty"`
	Steps   []MaintenanceStep `json:"steps,omitempty"`
	Signals []string          `json:"signals,omitempty"`
}

func maintenancePlaybookFromOption(opt MaintenancePlaybookOption) MaintenancePlaybook {
	playbook := MaintenancePlaybook{
		ID:          opt.ID,
		Title:       opt.Title,
		Description: opt.Description,
		Tags:        cloneStringSlice(opt.Tags),
	}
	for _, step := range opt.Steps {
		playbook.Steps = append(playbook.Steps, maintenanceStepFromOption(step))
	}
	return playbook
}

func maintenanceWindowFromOption(opt MaintenanceWindowOption) MaintenanceWindow {
	return MaintenanceWindow{
		ID:              opt.ID,
		Name:            opt.Name,
		Cron:            opt.Cron,
		DurationMinutes: int64(opt.Duration / time.Minute),
		Timezone:        opt.Timezone,
	}
}

func maintenanceUpgradeFromOption(opt MaintenanceUpgradePlanOption) MaintenanceUpgradePlan {
	plan := MaintenanceUpgradePlan{
		ID:            opt.ID,
		Version:       opt.Version,
		Summary:       opt.Summary,
		Prerequisites: cloneStringSlice(opt.Prerequisites),
		ReleaseNotes:  opt.ReleaseNotes,
	}
	if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(opt.ScheduledFor)); err == nil {
		plan.ScheduledFor = &ts
	}
	for _, step := range opt.Steps {
		plan.Steps = append(plan.Steps, maintenanceStepFromOption(step))
	}
	for _, step := range opt.RollbackPlan {
		plan.RollbackPlan = append(plan.RollbackPlan, maintenanceStepFromOption(step))
	}
	return plan
}

func maintenanceStepFromOption(opt MaintenanceStepOption) MaintenanceStep {
	return MaintenanceStep{
		ID:                opt.ID,
		Title:             opt.Title,
		Summary:           opt.Summary,
		DurationSeconds:   int64(opt.Duration / time.Second),
		Responsible:       opt.Responsible,
		RequiresApproval:  opt.RequiresApproval,
		DependsOn:         cloneStringSlice(opt.DependsOn),
		AutomationJobType: opt.AutomationJobType,
	}
}

func backupScheduleFromOption(opt BackupScheduleOption) BackupSchedule {
	return BackupSchedule{
		ID:             opt.ID,
		Name:           opt.Name,
		Cron:           opt.Cron,
		RetentionHours: int64(opt.Retention / time.Hour),
		WindowMinutes:  int64(opt.Window / time.Minute),
		Type:           opt.Type,
		Enabled:        opt.Enabled,
		Destinations:   cloneStringSlice(opt.Destinations),
	}
}

func backupDestinationFromOption(opt BackupDestinationOption) BackupDestination {
	return BackupDestination{
		ID:       opt.ID,
		Name:     opt.Name,
		Kind:     opt.Kind,
		Endpoint: opt.Endpoint,
		Metadata: cloneStringMap(opt.Metadata),
	}
}

func restoreWorkflowFromOption(opt RestoreWorkflowOption) RestoreWorkflow {
	flow := RestoreWorkflow{
		ID:          opt.ID,
		Name:        opt.Name,
		Description: opt.Description,
		Checks:      cloneStringSlice(opt.Checks),
		Approvals:   cloneStringSlice(opt.Approvals),
	}
	for _, step := range opt.Steps {
		flow.Steps = append(flow.Steps, maintenanceStepFromOption(step))
	}
	return flow
}

func backupVerificationFromOption(opt BackupVerificationOption) BackupVerificationSummary {
	return BackupVerificationSummary{
		Enabled:      opt.Enabled,
		Schedule:     opt.Schedule,
		Retention:    opt.Retention,
		Sandbox:      opt.Sandbox,
		AlertChannel: opt.AlertChannel,
	}
}

func performanceProbeFromOption(opt PerformanceProbeOption) PerformanceProbe {
	return PerformanceProbe{
		ID:              opt.ID,
		Name:            opt.Name,
		Description:     opt.Description,
		Command:         opt.Command,
		IntervalSeconds: int64(opt.Interval / time.Second),
		SLO:             opt.SLO,
		Units:           opt.Units,
		Thresholds:      cloneFloatMap(opt.Thresholds),
	}
}

func performanceIndicatorFromOption(opt PerformanceIndicatorOption) PerformanceIndicator {
	return PerformanceIndicator{
		ID:          opt.ID,
		Name:        opt.Name,
		Description: opt.Description,
		Target:      opt.Target,
		Units:       opt.Units,
	}
}

func performancePlaybookFromOption(opt PerformancePlaybookOption) PerformancePlaybook {
	playbook := PerformancePlaybook{
		ID:      opt.ID,
		Title:   opt.Title,
		Summary: opt.Summary,
		Impact:  opt.Impact,
		Signals: cloneStringSlice(opt.Signals),
	}
	for _, step := range opt.Steps {
		playbook.Steps = append(playbook.Steps, maintenanceStepFromOption(step))
	}
	return playbook
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func cloneFloatMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]float64, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}
