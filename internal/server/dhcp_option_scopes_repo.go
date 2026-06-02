package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type ScopeRepository interface {
	List(ctx context.Context, tenantID string) ([]dhcpOptionScope, error)
	Upsert(ctx context.Context, item dhcpOptionScope, tenantID string) (dhcpOptionScope, error)
	Delete(ctx context.Context, scopeID string, tenantID string) error
}

type SQLScopeRepository struct {
	db *sqlx.DB
}

func NewSQLScopeRepository(db *sqlx.DB) *SQLScopeRepository {
	if db == nil {
		return nil
	}
	return &SQLScopeRepository{db: db}
}

type scopeRecord struct {
	ID          string    `db:"id"`
	Name        string    `db:"name"`
	Subnet      string    `db:"subnet"`
	Range       string    `db:"range_value"`
	Gateway     string    `db:"gateway"`
	Status      string    `db:"status"`
	ScopeType   string    `db:"scope_type"`
	Target      string    `db:"target"`
	TemplateID  string    `db:"template_id"`
	OptionIDs   string    `db:"option_ids"`
	Description string    `db:"description"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

func encodeScopeOptionIDs(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	b, err := json.Marshal(uniqueStrings(values))
	if err != nil {
		return "[]"
	}
	return string(b)
}

func decodeScopeOptionIDs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return uniqueStrings(out)
}

func (r *SQLScopeRepository) List(ctx context.Context, tenantID string) ([]dhcpOptionScope, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows := make([]scopeRecord, 0)
	plans := scopeColumnPlans()
	for i, plan := range plans {
		query := r.db.Rebind(scopeListQuery(plan))
		err := r.db.SelectContext(ctx, &rows, query)
		if err == nil {
			break
		}
		if !isUnknownScopeSchemaColumnError(err) {
			return nil, err
		}
		if i == len(plans)-1 {
			// Legacy schemas may miss many columns; keep service available with in-memory scopes.
			return []dhcpOptionScope{}, nil
		}
	}
	out := make([]dhcpOptionScope, 0, len(rows))
	for _, rec := range rows {
		out = append(out, dhcpOptionScope{
			ID:          rec.ID,
			Name:        rec.Name,
			Subnet:      rec.Subnet,
			Range:       rec.Range,
			Gateway:     rec.Gateway,
			Status:      rec.Status,
			ScopeType:   rec.ScopeType,
			Target:      rec.Target,
			TemplateID:  strings.TrimSpace(rec.TemplateID),
			OptionIDs:   decodeScopeOptionIDs(rec.OptionIDs),
			Description: rec.Description,
			Notes:       firstNonEmpty(rec.Description),
			UpdatedAt:   rec.UpdatedAt,
		})
	}
	return out, nil
}

func (r *SQLScopeRepository) Upsert(ctx context.Context, item dhcpOptionScope, tenantID string) (dhcpOptionScope, error) {
	if r == nil || r.db == nil {
		return item, nil
	}
	now := time.Now().UTC()
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
	item.UpdatedAt = now

	rec := scopeRecord{
		ID:          item.ID,
		Name:        item.Name,
		Subnet:      item.Subnet,
		Range:       item.Range,
		Gateway:     item.Gateway,
		Status:      item.Status,
		ScopeType:   item.ScopeType,
		Target:      item.Target,
		TemplateID:  item.TemplateID,
		OptionIDs:   encodeScopeOptionIDs(item.OptionIDs),
		Description: firstNonEmpty(item.Description, item.Notes),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	plans := scopeColumnPlans()
	usedPlan := plans[len(plans)-1]
	updated := false
	for _, plan := range plans {
		stmt := scopeUpsertUpdateStmt(plan)
		res, err := r.db.NamedExecContext(ctx, stmt, rec)
		if err != nil {
			if isUnknownScopeSchemaColumnError(err) {
				continue
			}
			return item, err
		}
		usedPlan = plan
		if rows, _ := res.RowsAffected(); rows > 0 {
			updated = true
			break
		}
		// MySQL may return 0 when values are unchanged; treat existing row as updated.
		exists, existErr := r.scopeExists(ctx, item.ID)
		if existErr != nil {
			return item, existErr
		}
		if exists {
			updated = true
			break
		}
		break
	}
	if updated {
		return item, nil
	}

	if _, err := r.db.NamedExecContext(ctx, scopeUpsertInsertStmt(usedPlan), rec); err != nil {
		if !isUnknownScopeSchemaColumnError(err) {
			return item, err
		}
		inserted := false
		for _, plan := range plans {
			if _, retryErr := r.db.NamedExecContext(ctx, scopeUpsertInsertStmt(plan), rec); retryErr == nil {
				inserted = true
				break
			} else if !isUnknownScopeSchemaColumnError(retryErr) {
				return item, retryErr
			}
		}
		if !inserted {
			// If storage schema is too old, continue with in-memory scope store only.
			if isUnknownScopeSchemaColumnError(err) {
				return item, nil
			}
			return item, err
		}
	}
	return item, nil
}

func (r *SQLScopeRepository) scopeExists(ctx context.Context, scopeID string) (bool, error) {
	if r == nil || r.db == nil {
		return false, nil
	}
	var exists int
	query := r.db.Rebind(`SELECT 1 FROM dhcp_option_scopes WHERE id = ? LIMIT 1`)
	err := r.db.GetContext(ctx, &exists, query, scopeID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return exists == 1, nil
}

type scopeColumnPlan struct {
	subnetColumn  string
	rangeColumn   string
	gatewayColumn string
	hasRange      bool
	hasGateway    bool
	hasStatus     bool
	hasScopeType  bool
}

func scopeColumnPlans() []scopeColumnPlan {
	return []scopeColumnPlan{
		{subnetColumn: "subnet", rangeColumn: "range_value", gatewayColumn: "gateway", hasRange: true, hasGateway: true, hasStatus: true, hasScopeType: true},
		{subnetColumn: "subnet", rangeColumn: "range_value", gatewayColumn: "gateway", hasRange: true, hasGateway: true, hasStatus: false, hasScopeType: false},
		{subnetColumn: "subnet", rangeColumn: "range", gatewayColumn: "gateway", hasRange: true, hasGateway: true, hasStatus: true, hasScopeType: true},
		{subnetColumn: "subnet", rangeColumn: "range", gatewayColumn: "gateway", hasRange: true, hasGateway: true, hasStatus: false, hasScopeType: false},
		{subnetColumn: "subnet", gatewayColumn: "gateway", hasRange: false, hasGateway: true, hasStatus: true, hasScopeType: true},
		{subnetColumn: "subnet", gatewayColumn: "gateway", hasRange: false, hasGateway: true, hasStatus: false, hasScopeType: false},
		{subnetColumn: "cidr", rangeColumn: "range_value", gatewayColumn: "gateway", hasRange: true, hasGateway: true, hasStatus: true, hasScopeType: true},
		{subnetColumn: "cidr", rangeColumn: "range_value", gatewayColumn: "gateway", hasRange: true, hasGateway: true, hasStatus: false, hasScopeType: false},
		{subnetColumn: "cidr", rangeColumn: "range", gatewayColumn: "gateway", hasRange: true, hasGateway: true, hasStatus: true, hasScopeType: true},
		{subnetColumn: "cidr", rangeColumn: "range", gatewayColumn: "gateway", hasRange: true, hasGateway: true, hasStatus: false, hasScopeType: false},
		{subnetColumn: "cidr", gatewayColumn: "gateway", hasRange: false, hasGateway: true, hasStatus: true, hasScopeType: true},
		{subnetColumn: "cidr", gatewayColumn: "gateway", hasRange: false, hasGateway: true, hasStatus: false, hasScopeType: false},
		{subnetColumn: "target", rangeColumn: "range_value", gatewayColumn: "gateway", hasRange: true, hasGateway: true, hasStatus: true, hasScopeType: true},
		{subnetColumn: "target", rangeColumn: "range_value", gatewayColumn: "gateway", hasRange: true, hasGateway: true, hasStatus: false, hasScopeType: false},
		{subnetColumn: "target", rangeColumn: "range", gatewayColumn: "gateway", hasRange: true, hasGateway: true, hasStatus: true, hasScopeType: true},
		{subnetColumn: "target", rangeColumn: "range", gatewayColumn: "gateway", hasRange: true, hasGateway: true, hasStatus: false, hasScopeType: false},
		{subnetColumn: "target", gatewayColumn: "gateway", hasRange: false, hasGateway: true, hasStatus: true, hasScopeType: true},
		{subnetColumn: "target", gatewayColumn: "gateway", hasRange: false, hasGateway: true, hasStatus: false, hasScopeType: false},
		{subnetColumn: "target", hasRange: false, hasGateway: false, hasStatus: false, hasScopeType: false},
	}
}

func scopeListQuery(plan scopeColumnPlan) string {
	rangeExpr := `''`
	if plan.hasRange {
		rangeExpr = scopeColumnExpr(plan.rangeColumn)
	}
	gatewayExpr := `''`
	if plan.hasGateway {
		gatewayExpr = scopeColumnExpr(plan.gatewayColumn)
	}
	statusExpr := `'active'`
	if plan.hasStatus {
		statusExpr = "status"
	}
	scopeTypeExpr := `'GLOBAL'`
	if plan.hasScopeType {
		scopeTypeExpr = "scope_type"
	}
	return fmt.Sprintf(`SELECT id, name, %s AS subnet, %s AS range_value, %s AS gateway, %s AS status, %s AS scope_type, target, template_id, option_ids, description, created_at, updated_at FROM dhcp_option_scopes ORDER BY updated_at DESC, name ASC`, scopeColumnExpr(plan.subnetColumn), rangeExpr, gatewayExpr, statusExpr, scopeTypeExpr)
}

func scopeUpsertUpdateStmt(plan scopeColumnPlan) string {
	sets := []string{"name=:name"}
	if !strings.EqualFold(plan.subnetColumn, "target") {
		sets = append(sets, fmt.Sprintf("%s=:subnet", scopeColumnExpr(plan.subnetColumn)))
	}
	if plan.hasRange {
		sets = append(sets, fmt.Sprintf("%s=:range_value", scopeColumnExpr(plan.rangeColumn)))
	}
	if plan.hasGateway {
		sets = append(sets, fmt.Sprintf("%s=:gateway", scopeColumnExpr(plan.gatewayColumn)))
	}
	if plan.hasStatus {
		sets = append(sets, "status=:status")
	}
	if plan.hasScopeType {
		sets = append(sets, "scope_type=:scope_type")
	}
	sets = append(sets,
		"target=:target",
		"template_id=:template_id",
		"option_ids=:option_ids",
		"description=:description",
		"updated_at=:updated_at",
	)
	return fmt.Sprintf(`UPDATE dhcp_option_scopes SET %s WHERE id=:id`, strings.Join(sets, ", "))
}

func scopeUpsertInsertStmt(plan scopeColumnPlan) string {
	columns := []string{"id", "name"}
	values := []string{":id", ":name"}
	if !strings.EqualFold(plan.subnetColumn, "target") {
		columns = append(columns, scopeColumnExpr(plan.subnetColumn))
		values = append(values, ":subnet")
	}
	if plan.hasRange {
		columns = append(columns, scopeColumnExpr(plan.rangeColumn))
		values = append(values, ":range_value")
	}
	if plan.hasGateway {
		columns = append(columns, scopeColumnExpr(plan.gatewayColumn))
		values = append(values, ":gateway")
	}
	if plan.hasStatus {
		columns = append(columns, "status")
		values = append(values, ":status")
	}
	if plan.hasScopeType {
		columns = append(columns, "scope_type")
		values = append(values, ":scope_type")
	}
	columns = append(columns,
		"target",
		"template_id",
		"option_ids",
		"description",
		"created_at",
		"updated_at",
	)
	values = append(values,
		":target",
		":template_id",
		":option_ids",
		":description",
		":created_at",
		":updated_at",
	)
	return fmt.Sprintf(`INSERT INTO dhcp_option_scopes (%s) VALUES (%s)`, strings.Join(columns, ", "), strings.Join(values, ", "))
}

func scopeColumnExpr(column string) string {
	trimmed := strings.TrimSpace(column)
	if strings.EqualFold(trimmed, "range") {
		// MySQL treats RANGE as a reserved keyword, so quote legacy column name.
		return "`range`"
	}
	return trimmed
}

func isUnknownScopeSchemaColumnError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !(strings.Contains(msg, "subnet") ||
		strings.Contains(msg, "cidr") ||
		strings.Contains(msg, "range_value") ||
		strings.Contains(msg, "range") ||
		strings.Contains(msg, "gateway") ||
		strings.Contains(msg, "status") ||
		strings.Contains(msg, "scope_type") ||
		strings.Contains(msg, "target") ||
		strings.Contains(msg, "template_id") ||
		strings.Contains(msg, "option_ids") ||
		strings.Contains(msg, "description") ||
		strings.Contains(msg, "notes")) {
		return false
	}
	return strings.Contains(msg, "unknown column") ||
		strings.Contains(msg, "no such column") ||
		strings.Contains(msg, "column does not exist")
}

func (r *SQLScopeRepository) Delete(ctx context.Context, scopeID string, tenantID string) error {
	if r == nil || r.db == nil {
		return nil
	}
	stmt := `DELETE FROM dhcp_option_scopes WHERE id = ?`
	stmt = r.db.Rebind(stmt)
	_, err := r.db.ExecContext(ctx, stmt, scopeID)
	return err
}
