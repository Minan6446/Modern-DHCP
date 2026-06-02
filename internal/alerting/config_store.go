package alerting

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Thresholds captures alert threshold configuration per tenant.
type Thresholds struct {
	TenantID                    string    `db:"tenant_id" json:"tenantId"`
	ResourcePoolUsage           int       `db:"resource_pool_usage" json:"poolUsage"`
	ResourceLeaseUsage          int       `db:"resource_lease_usage" json:"leaseUsage"`
	ResourceRenewFail           int       `db:"resource_renew_fail" json:"renewFail"`
	ResourceLeaseTimeDrift      int       `db:"resource_lease_time_drift" json:"leaseTimeDrift"`
	ResourceFailedRequestRatio  int       `db:"resource_failed_request_ratio" json:"failedRequestRatio"`
	ResourceSubnetImbalance     int       `db:"resource_subnet_imbalance" json:"subnetImbalance"`
	ResourceLogErrorThreshold   int       `db:"resource_log_error_threshold" json:"logErrorThreshold"`
	ServerResponseTimeout       int       `db:"server_response_timeout" json:"responseTimeout"`
	ServerResponseTimeMs        int       `db:"server_response_time_ms" json:"responseTimeMs"`
	ServerCPUUsage              int       `db:"server_cpu_usage" json:"cpuUsage"`
	ServerMemoryUsage           int       `db:"server_memory_usage" json:"memoryUsage"`
	ServerProcessCheck          bool      `db:"server_process_check" json:"processCheck"`
	NetworkConflictSensitivity  string    `db:"network_conflict_sensitivity" json:"conflictSensitivity"`
	NetworkAbnormalQPS          int       `db:"network_abnormal_qps" json:"abnormalQps"`
	NetworkDuplicateIPDetection bool      `db:"network_duplicate_ip_detection" json:"duplicateIpDetection"`
	NetworkUnauthorizedDHCP     bool      `db:"network_unauthorized_server_detection" json:"unauthorizedServerDetection"`
	UpdatedAt                   time.Time `db:"updated_at" json:"updatedAt"`
	UpdatedBy                   string    `db:"updated_by" json:"updatedBy"`
}

// NotifyConfig persists alert notify channels and policies per tenant.
type NotifyConfig struct {
	TenantID   string    `db:"tenant_id" json:"tenantId"`
	Channels   Channels  `json:"channels"`
	Policies   Policies  `json:"policies"`
	WebhookURL string    `db:"webhook_url" json:"webhookUrl"`
	UpdatedAt  time.Time `db:"updated_at" json:"updatedAt"`
	UpdatedBy  string    `db:"updated_by" json:"updatedBy"`
}

// Channels represents notification switches.
type Channels struct {
	Email   bool `db:"channels_email" json:"email"`
	SMS     bool `db:"channels_sms" json:"sms"`
	Webhook bool `db:"channels_webhook" json:"webhook"`
}

// Policies captures per-severity channel fanout.
type Policies struct {
	Emergency []string `json:"emergency"`
	Critical  []string `json:"critical"`
	Info      []string `json:"info"`
}

// Template represents an alert template.
type Template struct {
	ID        string    `db:"id" json:"id"`
	TenantID  string    `db:"tenant_id" json:"tenantId"`
	Name      string    `db:"name" json:"name"`
	Lang      string    `db:"lang" json:"lang"`
	Channel   string    `db:"channel" json:"channel"`
	Subject   string    `db:"subject" json:"subject"`
	Body      string    `db:"body" json:"body"`
	Variables []string  `db:"variables" json:"variables"`
	UpdatedAt time.Time `db:"updated_at" json:"updatedAt"`
	UpdatedBy string    `db:"updated_by" json:"updatedBy"`
}

// Receiver models alert receiver preferences.
type Receiver struct {
	ID           string    `db:"id" json:"id"`
	TenantID     string    `db:"tenant_id" json:"tenantId"`
	Name         string    `db:"name" json:"name"`
	Email        string    `db:"email" json:"email"`
	Phone        string    `db:"phone" json:"phone"`
	Levels       []string  `db:"levels" json:"levels"`
	Department   string    `db:"department" json:"department"`
	Schedule     string    `db:"schedule" json:"schedule"`
	ServerGroups []string  `db:"server_groups" json:"serverGroups"`
	CreatedAt    time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time `db:"updated_at" json:"updatedAt"`
	UpdatedBy    string    `db:"updated_by" json:"updatedBy"`
}

// ConfigStore persists thresholds and notify settings.
type ConfigStore interface {
	GetThresholds(ctx context.Context, tenantID string) (Thresholds, error)
	SaveThresholds(ctx context.Context, cfg Thresholds) error
	GetNotify(ctx context.Context, tenantID string) (NotifyConfig, error)
	SaveNotify(ctx context.Context, cfg NotifyConfig) error
}

// TemplateStore persists templates.
type TemplateStore interface {
	ListTemplates(ctx context.Context, tenantID string) ([]Template, error)
	CreateTemplate(ctx context.Context, tpl *Template) error
	UpdateTemplate(ctx context.Context, tpl *Template) error
	DeleteTemplate(ctx context.Context, tenantID, id string) error
}

// ReceiverStore persists receivers.
type ReceiverStore interface {
	ListReceivers(ctx context.Context, tenantID string) ([]Receiver, error)
	CreateReceiver(ctx context.Context, r *Receiver) error
	UpdateReceiver(ctx context.Context, r *Receiver) error
	DeleteReceiver(ctx context.Context, tenantID, id string) error
}

// Ensure SQLStore implements the new stores.
var _ ConfigStore = (*SQLStore)(nil)
var _ TemplateStore = (*SQLStore)(nil)
var _ ReceiverStore = (*SQLStore)(nil)

func (s *SQLStore) GetThresholds(ctx context.Context, tenantID string) (Thresholds, error) {
	if s == nil {
		return Thresholds{}, errors.New("alert store unavailable")
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return Thresholds{}, errors.New("tenantID required")
	}
	const query = `SELECT tenant_id, resource_pool_usage, resource_lease_usage, resource_renew_fail, resource_lease_time_drift, resource_failed_request_ratio, resource_subnet_imbalance, resource_log_error_threshold, server_response_timeout, server_response_time_ms, server_cpu_usage, server_memory_usage, server_process_check, network_conflict_sensitivity, network_abnormal_qps, network_duplicate_ip_detection, network_unauthorized_server_detection, updated_at, updated_by FROM alert_thresholds WHERE tenant_id = ? LIMIT 1`
	var row Thresholds
	err := s.db.GetContext(ctx, &row, query, tenantID)
	if err != nil {
		return Thresholds{}, err
	}
	return row, nil
}

func (s *SQLStore) SaveThresholds(ctx context.Context, cfg Thresholds) error {
	if s == nil {
		return errors.New("alert store unavailable")
	}
	cfg.TenantID = strings.TrimSpace(cfg.TenantID)
	if cfg.TenantID == "" {
		return errors.New("tenantID required")
	}
	if cfg.UpdatedAt.IsZero() {
		cfg.UpdatedAt = time.Now().UTC()
	}
	if cfg.UpdatedBy == "" {
		cfg.UpdatedBy = "system"
	}
	const stmt = `INSERT INTO alert_thresholds
(tenant_id, resource_pool_usage, resource_lease_usage, resource_renew_fail, resource_lease_time_drift, resource_failed_request_ratio, resource_subnet_imbalance, resource_log_error_threshold, server_response_timeout, server_response_time_ms, server_cpu_usage, server_memory_usage, server_process_check, network_conflict_sensitivity, network_abnormal_qps, network_duplicate_ip_detection, network_unauthorized_server_detection, updated_at, updated_by)
VALUES (:tenant_id, :resource_pool_usage, :resource_lease_usage, :resource_renew_fail, :resource_lease_time_drift, :resource_failed_request_ratio, :resource_subnet_imbalance, :resource_log_error_threshold, :server_response_timeout, :server_response_time_ms, :server_cpu_usage, :server_memory_usage, :server_process_check, :network_conflict_sensitivity, :network_abnormal_qps, :network_duplicate_ip_detection, :network_unauthorized_server_detection, :updated_at, :updated_by)
ON DUPLICATE KEY UPDATE
 resource_pool_usage=VALUES(resource_pool_usage),
 resource_lease_usage=VALUES(resource_lease_usage),
 resource_renew_fail=VALUES(resource_renew_fail),
 resource_lease_time_drift=VALUES(resource_lease_time_drift),
 resource_failed_request_ratio=VALUES(resource_failed_request_ratio),
 resource_subnet_imbalance=VALUES(resource_subnet_imbalance),
 resource_log_error_threshold=VALUES(resource_log_error_threshold),
 server_response_timeout=VALUES(server_response_timeout),
 server_response_time_ms=VALUES(server_response_time_ms),
 server_cpu_usage=VALUES(server_cpu_usage),
 server_memory_usage=VALUES(server_memory_usage),
 server_process_check=VALUES(server_process_check),
 network_conflict_sensitivity=VALUES(network_conflict_sensitivity),
 network_abnormal_qps=VALUES(network_abnormal_qps),
 network_duplicate_ip_detection=VALUES(network_duplicate_ip_detection),
 network_unauthorized_server_detection=VALUES(network_unauthorized_server_detection),
 updated_at=VALUES(updated_at),
 updated_by=VALUES(updated_by)`
	payload := map[string]any{
		"tenant_id":                             cfg.TenantID,
		"resource_pool_usage":                   cfg.ResourcePoolUsage,
		"resource_lease_usage":                  cfg.ResourceLeaseUsage,
		"resource_renew_fail":                   cfg.ResourceRenewFail,
		"resource_lease_time_drift":             cfg.ResourceLeaseTimeDrift,
		"resource_failed_request_ratio":         cfg.ResourceFailedRequestRatio,
		"resource_subnet_imbalance":             cfg.ResourceSubnetImbalance,
		"resource_log_error_threshold":          cfg.ResourceLogErrorThreshold,
		"server_response_timeout":               cfg.ServerResponseTimeout,
		"server_response_time_ms":               cfg.ServerResponseTimeMs,
		"server_cpu_usage":                      cfg.ServerCPUUsage,
		"server_memory_usage":                   cfg.ServerMemoryUsage,
		"server_process_check":                  boolToInt(cfg.ServerProcessCheck),
		"network_conflict_sensitivity":          strings.TrimSpace(cfg.NetworkConflictSensitivity),
		"network_abnormal_qps":                  cfg.NetworkAbnormalQPS,
		"network_duplicate_ip_detection":        boolToInt(cfg.NetworkDuplicateIPDetection),
		"network_unauthorized_server_detection": boolToInt(cfg.NetworkUnauthorizedDHCP),
		"updated_at":                            cfg.UpdatedAt,
		"updated_by":                            strings.TrimSpace(cfg.UpdatedBy),
	}
	_, err := s.db.NamedExecContext(ctx, stmt, payload)
	return err
}

func (s *SQLStore) GetNotify(ctx context.Context, tenantID string) (NotifyConfig, error) {
	if s == nil {
		return NotifyConfig{}, errors.New("alert store unavailable")
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return NotifyConfig{}, errors.New("tenantID required")
	}
	const query = `SELECT tenant_id, channels_email, channels_sms, channels_webhook, webhook_url, policies_emergency, policies_critical, policies_info, updated_at, updated_by FROM alert_notify_configs WHERE tenant_id = ? LIMIT 1`
	type row struct {
		TenantID   string         `db:"tenant_id"`
		Email      bool           `db:"channels_email"`
		SMS        bool           `db:"channels_sms"`
		Webhook    bool           `db:"channels_webhook"`
		WebhookURL string         `db:"webhook_url"`
		Emergency  sql.NullString `db:"policies_emergency"`
		Critical   sql.NullString `db:"policies_critical"`
		Info       sql.NullString `db:"policies_info"`
		UpdatedAt  time.Time      `db:"updated_at"`
		UpdatedBy  string         `db:"updated_by"`
	}
	var r row
	if err := s.db.GetContext(ctx, &r, query, tenantID); err != nil {
		return NotifyConfig{}, err
	}
	cfg := NotifyConfig{
		TenantID:   tenantID,
		Channels:   Channels{Email: r.Email, SMS: r.SMS, Webhook: r.Webhook},
		WebhookURL: strings.TrimSpace(r.WebhookURL),
		UpdatedAt:  r.UpdatedAt,
		UpdatedBy:  strings.TrimSpace(r.UpdatedBy),
	}
	_ = json.Unmarshal([]byte(r.Emergency.String), &cfg.Policies.Emergency)
	_ = json.Unmarshal([]byte(r.Critical.String), &cfg.Policies.Critical)
	_ = json.Unmarshal([]byte(r.Info.String), &cfg.Policies.Info)
	return cfg, nil
}

func (s *SQLStore) SaveNotify(ctx context.Context, cfg NotifyConfig) error {
	if s == nil {
		return errors.New("alert store unavailable")
	}
	cfg.TenantID = strings.TrimSpace(cfg.TenantID)
	if cfg.TenantID == "" {
		return errors.New("tenantID required")
	}
	if cfg.UpdatedAt.IsZero() {
		cfg.UpdatedAt = time.Now().UTC()
	}
	if cfg.UpdatedBy == "" {
		cfg.UpdatedBy = "system"
	}
	em, _ := json.Marshal(cfg.Policies.Emergency)
	cr, _ := json.Marshal(cfg.Policies.Critical)
	info, _ := json.Marshal(cfg.Policies.Info)
	const stmt = `INSERT INTO alert_notify_configs
(tenant_id, channels_email, channels_sms, channels_webhook, webhook_url, policies_emergency, policies_critical, policies_info, updated_at, updated_by)
VALUES (:tenant_id, :channels_email, :channels_sms, :channels_webhook, :webhook_url, :policies_emergency, :policies_critical, :policies_info, :updated_at, :updated_by)
ON DUPLICATE KEY UPDATE
  channels_email=VALUES(channels_email),
  channels_sms=VALUES(channels_sms),
  channels_webhook=VALUES(channels_webhook),
  webhook_url=VALUES(webhook_url),
  policies_emergency=VALUES(policies_emergency),
  policies_critical=VALUES(policies_critical),
  policies_info=VALUES(policies_info),
  updated_at=VALUES(updated_at),
  updated_by=VALUES(updated_by)`
	payload := map[string]any{
		"tenant_id":          cfg.TenantID,
		"channels_email":     boolToInt(cfg.Channels.Email),
		"channels_sms":       boolToInt(cfg.Channels.SMS),
		"channels_webhook":   boolToInt(cfg.Channels.Webhook),
		"webhook_url":        strings.TrimSpace(cfg.WebhookURL),
		"policies_emergency": string(em),
		"policies_critical":  string(cr),
		"policies_info":      string(info),
		"updated_at":         cfg.UpdatedAt,
		"updated_by":         strings.TrimSpace(cfg.UpdatedBy),
	}
	_, err := s.db.NamedExecContext(ctx, stmt, payload)
	return err
}

func (s *SQLStore) ListTemplates(ctx context.Context, tenantID string) ([]Template, error) {
	if s == nil {
		return nil, errors.New("alert store unavailable")
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, errors.New("tenantID required")
	}
	const query = `SELECT id, tenant_id, name, lang, channel, subject, body, variables, updated_at, updated_by FROM alert_templates WHERE tenant_id = ? ORDER BY updated_at DESC`
	type row struct {
		ID        string         `db:"id"`
		TenantID  string         `db:"tenant_id"`
		Name      string         `db:"name"`
		Lang      string         `db:"lang"`
		Channel   string         `db:"channel"`
		Subject   sql.NullString `db:"subject"`
		Body      sql.NullString `db:"body"`
		Variables sql.NullString `db:"variables"`
		UpdatedAt time.Time      `db:"updated_at"`
		UpdatedBy string         `db:"updated_by"`
	}
	rows := []row{}
	if err := s.db.SelectContext(ctx, &rows, query, tenantID); err != nil {
		return nil, err
	}
	templates := make([]Template, 0, len(rows))
	for _, r := range rows {
		tpl := Template{
			ID:        r.ID,
			TenantID:  r.TenantID,
			Name:      r.Name,
			Lang:      r.Lang,
			Channel:   r.Channel,
			Subject:   strings.TrimSpace(r.Subject.String),
			Body:      strings.TrimSpace(r.Body.String),
			UpdatedAt: r.UpdatedAt,
			UpdatedBy: strings.TrimSpace(r.UpdatedBy),
		}
		_ = json.Unmarshal([]byte(r.Variables.String), &tpl.Variables)
		templates = append(templates, tpl)
	}
	return templates, nil
}

func (s *SQLStore) CreateTemplate(ctx context.Context, tpl *Template) error {
	if s == nil {
		return errors.New("alert store unavailable")
	}
	if tpl == nil {
		return errors.New("template required")
	}
	tpl.TenantID = strings.TrimSpace(tpl.TenantID)
	tpl.ID = strings.TrimSpace(tpl.ID)
	tpl.Name = strings.TrimSpace(tpl.Name)
	tpl.Lang = strings.TrimSpace(tpl.Lang)
	tpl.Channel = strings.ToLower(strings.TrimSpace(tpl.Channel))
	if tpl.TenantID == "" || tpl.ID == "" || tpl.Name == "" || tpl.Lang == "" || tpl.Channel == "" {
		return errors.New("id, tenantId, name, lang, channel required")
	}
	if tpl.UpdatedAt.IsZero() {
		tpl.UpdatedAt = time.Now().UTC()
	}
	if tpl.UpdatedBy == "" {
		tpl.UpdatedBy = "system"
	}
	vars, _ := json.Marshal(tpl.Variables)
	const stmt = `INSERT INTO alert_templates (id, tenant_id, name, lang, channel, subject, body, variables, updated_at, updated_by)
VALUES (:id, :tenant_id, :name, :lang, :channel, :subject, :body, :variables, :updated_at, :updated_by)`
	payload := map[string]any{
		"id":         tpl.ID,
		"tenant_id":  tpl.TenantID,
		"name":       tpl.Name,
		"lang":       tpl.Lang,
		"channel":    tpl.Channel,
		"subject":    strings.TrimSpace(tpl.Subject),
		"body":       strings.TrimSpace(tpl.Body),
		"variables":  string(vars),
		"updated_at": tpl.UpdatedAt,
		"updated_by": strings.TrimSpace(tpl.UpdatedBy),
	}
	_, err := s.db.NamedExecContext(ctx, stmt, payload)
	return err
}

func (s *SQLStore) UpdateTemplate(ctx context.Context, tpl *Template) error {
	if s == nil {
		return errors.New("alert store unavailable")
	}
	if tpl == nil {
		return errors.New("template required")
	}
	tpl.TenantID = strings.TrimSpace(tpl.TenantID)
	tpl.ID = strings.TrimSpace(tpl.ID)
	tpl.Name = strings.TrimSpace(tpl.Name)
	tpl.Lang = strings.TrimSpace(tpl.Lang)
	tpl.Channel = strings.ToLower(strings.TrimSpace(tpl.Channel))
	if tpl.TenantID == "" || tpl.ID == "" || tpl.Name == "" || tpl.Lang == "" || tpl.Channel == "" {
		return errors.New("id, tenantId, name, lang, channel required")
	}
	if tpl.UpdatedAt.IsZero() {
		tpl.UpdatedAt = time.Now().UTC()
	}
	if tpl.UpdatedBy == "" {
		tpl.UpdatedBy = "system"
	}
	vars, _ := json.Marshal(tpl.Variables)
	const stmt = `UPDATE alert_templates SET name = :name, lang = :lang, channel = :channel, subject = :subject, body = :body, variables = :variables, updated_at = :updated_at, updated_by = :updated_by WHERE tenant_id = :tenant_id AND id = :id`
	payload := map[string]any{
		"id":         tpl.ID,
		"tenant_id":  tpl.TenantID,
		"name":       tpl.Name,
		"lang":       tpl.Lang,
		"channel":    tpl.Channel,
		"subject":    strings.TrimSpace(tpl.Subject),
		"body":       strings.TrimSpace(tpl.Body),
		"variables":  string(vars),
		"updated_at": tpl.UpdatedAt,
		"updated_by": strings.TrimSpace(tpl.UpdatedBy),
	}
	res, err := s.db.NamedExecContext(ctx, stmt, payload)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SQLStore) DeleteTemplate(ctx context.Context, tenantID, id string) error {
	if s == nil {
		return errors.New("alert store unavailable")
	}
	tenantID = strings.TrimSpace(tenantID)
	id = strings.TrimSpace(id)
	if tenantID == "" || id == "" {
		return errors.New("tenantId and id required")
	}
	const stmt = `DELETE FROM alert_templates WHERE tenant_id = ? AND id = ?`
	res, err := s.db.ExecContext(ctx, stmt, tenantID, id)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SQLStore) ListReceivers(ctx context.Context, tenantID string) ([]Receiver, error) {
	if s == nil {
		return nil, errors.New("alert store unavailable")
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, errors.New("tenantID required")
	}
	const query = `SELECT id, tenant_id, name, email, phone, levels, department, schedule, server_groups, created_at, updated_at, updated_by FROM alert_receivers WHERE tenant_id = ? ORDER BY updated_at DESC`
	type row struct {
		ID           string         `db:"id"`
		TenantID     string         `db:"tenant_id"`
		Name         string         `db:"name"`
		Email        string         `db:"email"`
		Phone        sql.NullString `db:"phone"`
		Levels       sql.NullString `db:"levels"`
		Department   sql.NullString `db:"department"`
		Schedule     sql.NullString `db:"schedule"`
		ServerGroups sql.NullString `db:"server_groups"`
		CreatedAt    time.Time      `db:"created_at"`
		UpdatedAt    time.Time      `db:"updated_at"`
		UpdatedBy    string         `db:"updated_by"`
	}
	rows := []row{}
	if err := s.db.SelectContext(ctx, &rows, query, tenantID); err != nil {
		return nil, err
	}
	receivers := make([]Receiver, 0, len(rows))
	for _, r := range rows {
		recv := Receiver{
			ID:         r.ID,
			TenantID:   r.TenantID,
			Name:       r.Name,
			Email:      r.Email,
			Phone:      strings.TrimSpace(r.Phone.String),
			Department: strings.TrimSpace(r.Department.String),
			Schedule:   strings.TrimSpace(r.Schedule.String),
			CreatedAt:  r.CreatedAt,
			UpdatedAt:  r.UpdatedAt,
			UpdatedBy:  strings.TrimSpace(r.UpdatedBy),
		}
		_ = json.Unmarshal([]byte(r.Levels.String), &recv.Levels)
		_ = json.Unmarshal([]byte(r.ServerGroups.String), &recv.ServerGroups)
		receivers = append(receivers, recv)
	}
	return receivers, nil
}

func (s *SQLStore) CreateReceiver(ctx context.Context, r *Receiver) error {
	if s == nil {
		return errors.New("alert store unavailable")
	}
	if r == nil {
		return errors.New("receiver required")
	}
	r.TenantID = strings.TrimSpace(r.TenantID)
	r.ID = strings.TrimSpace(r.ID)
	r.Name = strings.TrimSpace(r.Name)
	r.Email = strings.TrimSpace(r.Email)
	if r.TenantID == "" || r.ID == "" || r.Name == "" || r.Email == "" {
		return errors.New("id, tenantId, name, email required")
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = r.CreatedAt
	}
	if r.UpdatedBy == "" {
		r.UpdatedBy = "system"
	}
	levels, _ := json.Marshal(r.Levels)
	groups, _ := json.Marshal(r.ServerGroups)
	const stmt = `INSERT INTO alert_receivers (id, tenant_id, name, email, phone, levels, department, schedule, server_groups, created_at, updated_at, updated_by)
VALUES (:id, :tenant_id, :name, :email, :phone, :levels, :department, :schedule, :server_groups, :created_at, :updated_at, :updated_by)`
	payload := map[string]any{
		"id":            r.ID,
		"tenant_id":     r.TenantID,
		"name":          r.Name,
		"email":         r.Email,
		"phone":         strings.TrimSpace(r.Phone),
		"levels":        string(levels),
		"department":    strings.TrimSpace(r.Department),
		"schedule":      strings.TrimSpace(r.Schedule),
		"server_groups": string(groups),
		"created_at":    r.CreatedAt,
		"updated_at":    r.UpdatedAt,
		"updated_by":    strings.TrimSpace(r.UpdatedBy),
	}
	_, err := s.db.NamedExecContext(ctx, stmt, payload)
	return err
}

func (s *SQLStore) UpdateReceiver(ctx context.Context, r *Receiver) error {
	if s == nil {
		return errors.New("alert store unavailable")
	}
	if r == nil {
		return errors.New("receiver required")
	}
	r.TenantID = strings.TrimSpace(r.TenantID)
	r.ID = strings.TrimSpace(r.ID)
	r.Name = strings.TrimSpace(r.Name)
	r.Email = strings.TrimSpace(r.Email)
	if r.TenantID == "" || r.ID == "" || r.Name == "" || r.Email == "" {
		return errors.New("id, tenantId, name, email required")
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = time.Now().UTC()
	}
	if r.UpdatedBy == "" {
		r.UpdatedBy = "system"
	}
	levels, _ := json.Marshal(r.Levels)
	groups, _ := json.Marshal(r.ServerGroups)
	const stmt = `UPDATE alert_receivers SET name = :name, email = :email, phone = :phone, levels = :levels, department = :department, schedule = :schedule, server_groups = :server_groups, updated_at = :updated_at, updated_by = :updated_by WHERE tenant_id = :tenant_id AND id = :id`
	payload := map[string]any{
		"id":            r.ID,
		"tenant_id":     r.TenantID,
		"name":          r.Name,
		"email":         r.Email,
		"phone":         strings.TrimSpace(r.Phone),
		"levels":        string(levels),
		"department":    strings.TrimSpace(r.Department),
		"schedule":      strings.TrimSpace(r.Schedule),
		"server_groups": string(groups),
		"updated_at":    r.UpdatedAt,
		"updated_by":    strings.TrimSpace(r.UpdatedBy),
	}
	res, err := s.db.NamedExecContext(ctx, stmt, payload)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SQLStore) DeleteReceiver(ctx context.Context, tenantID, id string) error {
	if s == nil {
		return errors.New("alert store unavailable")
	}
	tenantID = strings.TrimSpace(tenantID)
	id = strings.TrimSpace(id)
	if tenantID == "" || id == "" {
		return errors.New("tenantId and id required")
	}
	const stmt = `DELETE FROM alert_receivers WHERE tenant_id = ? AND id = ?`
	res, err := s.db.ExecContext(ctx, stmt, tenantID, id)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
