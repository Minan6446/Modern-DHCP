package server

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

const (
	RoleReader = "reader"
	RoleAdmin  = "admin"

	CapabilityPolicyRead           = "policy.read"
	CapabilityPolicyWrite          = "policy.write"
	CapabilityPolicyBYODManage     = "byod.policy.manage"
	CapabilityPoolRead             = "pool.read"
	CapabilityPoolWrite            = "pool.write"
	CapabilityBindingRead          = "binding.read"
	CapabilityBindingManage        = "binding.manage"
	CapabilityLeaseRead            = "lease.read"
	CapabilityLeaseManage          = "lease.manage"
	CapabilityReportRead           = "report.read"
	CapabilityAuditRead            = "audit.read"
	CapabilityTenantQuotaRead      = "tenant.quota.read"
	CapabilityTenantQuotaWrite     = "tenant.quota.write"
	CapabilityRBACAssignmentRead   = "rbac.assignment.read"
	CapabilityRBACAssignmentWrite  = "rbac.assignment.write"
	CapabilityIoTRegistryRead      = "iot.registry.read"
	CapabilityIoTRegistryManage    = "iot.registry.manage"
	CapabilitySecurityPolicyRead   = "security.policy.read"
	CapabilitySecurityPolicyManage = "security.policy.manage"
	CapabilityHARead               = "ha.read"
	CapabilityHAManage             = "ha.manage"
)

var roleWeights = map[string]int{
	RoleReader: 1,
	RoleAdmin:  2,
}

func normalizeRole(role string) string {
	r := strings.ToLower(strings.TrimSpace(role))
	if _, ok := roleWeights[r]; !ok || r == "" {
		return RoleAdmin
	}
	return r
}

func hasRequiredRole(current, required string) bool {
	if required == "" {
		return true
	}
	currWeight, ok := roleWeights[current]
	if !ok {
		currWeight = roleWeights[RoleReader]
	}
	reqWeight, ok := roleWeights[required]
	if !ok {
		reqWeight = roleWeights[RoleReader]
	}
	return currWeight >= reqWeight
}

// RequireRole ensures the caller has at least the specified role.
func RequireRole(required string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			role, _ := c.Get(contextRoleKey).(string)
			if !hasRequiredRole(role, required) {
				return echo.NewHTTPError(http.StatusForbidden, "insufficient role")
			}
			return next(c)
		}
	}
}

// RequireCapability enforces that the resolved capability set contains the desired value.
func RequireCapability(capability string) echo.MiddlewareFunc {
	return RequireCapabilities(capability)
}

// RequireCapabilities ensures all listed capabilities are granted before executing the handler.
func RequireCapabilities(capabilities ...string) echo.MiddlewareFunc {
	fixed := make([]string, 0, len(capabilities))
	seen := make(map[string]struct{}, len(capabilities))
	for _, capName := range capabilities {
		capName = strings.TrimSpace(capName)
		if capName == "" {
			continue
		}
		if _, ok := seen[capName]; ok {
			continue
		}
		seen[capName] = struct{}{}
		fixed = append(fixed, capName)
	}
	if len(fixed) == 0 {
		return func(next echo.HandlerFunc) echo.HandlerFunc { return next }
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			caps := capabilitiesFromContext(c)
			missing := firstMissingCapability(caps, fixed)
			if missing == "" {
				return next(c)
			}
			if capabilityStrictlyEnforced(c) {
				return echo.NewHTTPError(http.StatusForbidden, "missing capability: "+missing)
			}
			return next(c)
		}
	}
}

func firstMissingCapability(current map[string]struct{}, required []string) string {
	for _, capName := range required {
		if capName == "" {
			continue
		}
		if _, ok := current[capName]; ok {
			continue
		}
		return capName
	}
	return ""
}

func capabilitiesFromContext(c echo.Context) map[string]struct{} {
	if c == nil {
		return map[string]struct{}{}
	}
	if caps, ok := c.Get(contextCapabilitiesKey).(map[string]struct{}); ok && caps != nil {
		return caps
	}
	return map[string]struct{}{}
}

func capabilityStrictlyEnforced(c echo.Context) bool {
	if c == nil {
		return false
	}
	strict, _ := c.Get(contextCapabilityStrictKey).(bool)
	return strict
}
