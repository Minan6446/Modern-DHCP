package ops

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
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
		Enabled:           s.options.System.Enabled,
		AllowConfigExport: s.options.System.AllowConfigExport,
		AllowConfigImport: s.options.System.AllowConfigImport,
		BackupLocation:    s.options.System.BackupLocation,
		MaintenanceWindow: settings.MaintenanceWindow,
		Theme:             settings.Theme,
		Locale:            settings.Locale,
		MaintenanceMode:   settings.MaintenanceMode,
		Announcement:      settings.Announcement,
		UpdatedAt:         settings.UpdatedAt,
		UpdatedBy:         settings.UpdatedBy,
		GeneratedAt:       time.Now().UTC(),
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
		Theme:             strings.TrimSpace(opts.System.Theme),
		Locale:            strings.TrimSpace(opts.System.Locale),
		MaintenanceMode:   opts.System.MaintenanceMode,
		MaintenanceWindow: strings.TrimSpace(opts.System.MaintenanceWindow),
		Announcement:      strings.TrimSpace(opts.System.Announcement),
		UpdatedAt:         time.Now().UTC(),
		UpdatedBy:         "system",
	}
}

// SystemSettingsUpdate captures partial updates to platform settings.
type SystemSettingsUpdate struct {
	Theme             *string
	Locale            *string
	MaintenanceMode   *bool
	MaintenanceWindow *string
	Announcement      *string
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

// SystemSummary captures management metadata for the platform.
type SystemSummary struct {
	Enabled           bool      `json:"enabled"`
	AllowConfigExport bool      `json:"allowConfigExport"`
	AllowConfigImport bool      `json:"allowConfigImport"`
	BackupLocation    string    `json:"backupLocation,omitempty"`
	MaintenanceWindow string    `json:"maintenanceWindow,omitempty"`
	Theme             string    `json:"theme"`
	Locale            string    `json:"locale"`
	MaintenanceMode   bool      `json:"maintenanceMode"`
	Announcement      string    `json:"announcement,omitempty"`
	UpdatedAt         time.Time `json:"updatedAt"`
	UpdatedBy         string    `json:"updatedBy"`
	GeneratedAt       time.Time `json:"generatedAt"`
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
