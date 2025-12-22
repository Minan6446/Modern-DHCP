package monitoring

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/config"
)

// AlertRuleDescriptor exposes normalized rule metadata for APIs/UI.
type AlertRuleDescriptor struct {
	ID              string   `json:"id"`
	Expression      string   `json:"expression"`
	Severity        string   `json:"severity"`
	SummaryTemplate string   `json:"summaryTemplate"`
	DetailTemplate  string   `json:"detailTemplate"`
	Channels        []string `json:"channels"`
	TTLSeconds      int64    `json:"ttlSeconds"`
}

type compiledRule struct {
	id       string
	expr     string
	severity alerting.Severity
	summary  string
	detail   string
	channels []string
	clauses  []ruleClause
	ttl      time.Duration
}

type ruleClause struct {
	metric string
	op     string
	value  float64
}

func compileRule(cfg config.AlertRuleConfig) (compiledRule, error) {
	id := strings.TrimSpace(cfg.ID)
	if id == "" {
		return compiledRule{}, fmt.Errorf("rule id required")
	}
	expression := strings.TrimSpace(cfg.Expression)
	severity, ok := alerting.ParseSeverity(cfg.Severity)
	if !ok {
		severity = alerting.SeverityWarning
	}
	clauses, err := parseExpression(expression)
	if err != nil {
		return compiledRule{}, err
	}
	channels := make([]string, 0, len(cfg.Channels))
	for _, ch := range cfg.Channels {
		if trimmed := strings.ToLower(strings.TrimSpace(ch)); trimmed != "" {
			channels = append(channels, trimmed)
		}
	}
	return compiledRule{
		id:       id,
		expr:     expression,
		severity: severity,
		summary:  strings.TrimSpace(cfg.Summary),
		detail:   strings.TrimSpace(cfg.Detail),
		channels: channels,
		clauses:  clauses,
		ttl:      cfg.TTL,
	}, nil
}

func parseExpression(expr string) ([]ruleClause, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("alert rule expression required")
	}
	fields := tokenize(expr)
	clauses := make([]ruleClause, 0, len(fields)/3)
	for idx := 0; idx < len(fields); {
		if idx+2 >= len(fields) {
			return nil, fmt.Errorf("invalid expression segment near %s", fields[idx])
		}
		metric := strings.ToLower(fields[idx])
		op := fields[idx+1]
		valueToken := fields[idx+2]
		idx += 3
		value, err := parseNumeric(valueToken)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, ruleClause{metric: metric, op: op, value: value})
		if idx < len(fields) {
			sep := strings.ToUpper(fields[idx])
			if sep != "AND" && sep != "&&" {
				return nil, fmt.Errorf("unsupported operator %s", fields[idx])
			}
			idx++
		}
	}
	return clauses, nil
}

func tokenize(expr string) []string {
	replacer := strings.NewReplacer("<=", " <= ", ">=", " >= ", "==", " == ", "<", " < ", ">", " > ")
	prepared := replacer.Replace(expr)
	return strings.Fields(prepared)
}

func parseNumeric(token string) (float64, error) {
	token = strings.TrimSpace(strings.TrimSuffix(token, ","))
	token = strings.TrimSuffix(token, "%")
	value, err := strconv.ParseFloat(token, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric literal %s", token)
	}
	return value, nil
}

func (rule compiledRule) matches(metrics map[string]float64) bool {
	for _, clause := range rule.clauses {
		value := metrics[strings.ToLower(clause.metric)]
		if !compare(value, clause.value, clause.op) {
			return false
		}
	}
	return true
}

func compare(observed, expected float64, op string) bool {
	switch op {
	case ">":
		return observed > expected
	case ">=":
		return observed >= expected
	case "<":
		return observed < expected
	case "<=":
		return observed <= expected
	case "==":
		return math.Abs(observed-expected) < 0.0001
	default:
		return false
	}
}

var templatePattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9\._-]+)\s*\}}`)

func renderTemplate(template string, metrics map[string]float64) string {
	if template == "" {
		return ""
	}
	return templatePattern.ReplaceAllStringFunc(template, func(match string) string {
		submatches := templatePattern.FindStringSubmatch(match)
		if len(submatches) != 2 {
			return match
		}
		key := strings.ToLower(submatches[1])
		value := metrics[key]
		return strconv.FormatFloat(value, 'f', 2, 64)
	})
}

func buildRuleMetrics(pools []PoolUsageSummary, requests []RequestPhaseSnapshot, security SecuritySnapshot) map[string]float64 {
	metrics := map[string]float64{}
	var maxUtil, totalUtil float64
	minHeadroom := math.Inf(1)
	for _, pool := range pools {
		if pool.Utilization > maxUtil {
			maxUtil = pool.Utilization
		}
		totalUtil += pool.Utilization
		headroom := float64(pool.Capacity - pool.Allocated)
		if headroom < 0 {
			headroom = 0
		}
		if headroom < minHeadroom {
			minHeadroom = headroom
		}
	}
	metrics["pool.count"] = float64(len(pools))
	metrics["pool.utilization.max"] = maxUtil
	if len(pools) > 0 {
		metrics["pool.utilization.avg"] = totalUtil / float64(len(pools))
		if minHeadroom == math.Inf(1) {
			minHeadroom = 0
		}
		metrics["pool.headroom.min"] = minHeadroom
	} else {
		metrics["pool.utilization.avg"] = 0
		metrics["pool.headroom.min"] = 0
	}
	var maxLatency, totalLatency float64
	var maxErrorRate float64
	for _, req := range requests {
		if req.P95Ms > maxLatency {
			maxLatency = req.P95Ms
		}
		totalLatency += req.AverageMs
		total := req.Success + req.Failure
		if total > 0 {
			rate := (float64(req.Failure) / float64(total)) * 100
			if rate > maxErrorRate {
				maxErrorRate = rate
			}
		}
	}
	metrics["request.count"] = float64(len(requests))
	metrics["request.latency.p95.max"] = maxLatency
	if len(requests) > 0 {
		metrics["request.latency.avg"] = totalLatency / float64(len(requests))
	} else {
		metrics["request.latency.avg"] = 0
	}
	metrics["request.errorRate.max"] = maxErrorRate
	metrics["security.ratelimit.hits"] = float64(security.RateLimit.TotalHits)
	metrics["security.ratelimit.uniqueMacs"] = float64(security.RateLimit.UniqueMACs)
	metrics["security.ratelimit.uniquePorts"] = float64(security.RateLimit.UniquePorts)
	return metrics
}
