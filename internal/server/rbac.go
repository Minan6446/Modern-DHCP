package server

import (
	"net/http"
	"strings"

	rbaccore "modern-dhcp/internal/rbac"

	"github.com/labstack/echo/v4"
)

const (
	RoleReader = "reader"
	RoleAdmin  = "admin"

	CapabilityPolicyRead           = rbaccore.CapabilityPolicyRead
	CapabilityPolicyWrite          = rbaccore.CapabilityPolicyWrite
	CapabilityPolicyBYODManage     = rbaccore.CapabilityPolicyBYODManage
	CapabilityPoolRead             = rbaccore.CapabilityPoolRead
	CapabilityPoolWrite            = rbaccore.CapabilityPoolWrite
	CapabilityBindingRead          = rbaccore.CapabilityBindingRead
	CapabilityBindingManage        = rbaccore.CapabilityBindingManage
	CapabilityLeaseRead            = rbaccore.CapabilityLeaseRead
	CapabilityLeaseManage          = rbaccore.CapabilityLeaseManage
	CapabilityReportRead           = rbaccore.CapabilityReportRead
	CapabilityAuditRead            = rbaccore.CapabilityAuditRead
	CapabilityUserRead             = rbaccore.CapabilityUserRead
	CapabilityUserManage           = rbaccore.CapabilityUserManage
	CapabilityAPIKeyManage         = rbaccore.CapabilityAPIKeyManage
	CapabilityAuthProviderRead     = rbaccore.CapabilityAuthProviderRead
	CapabilityAuthProviderManage   = rbaccore.CapabilityAuthProviderManage
	CapabilityTenantQuotaRead      = rbaccore.CapabilityTenantQuotaRead
	CapabilityTenantQuotaWrite     = rbaccore.CapabilityTenantQuotaWrite
	CapabilityRBACAssignmentRead   = rbaccore.CapabilityRBACAssignmentRead
	CapabilityRBACAssignmentWrite  = rbaccore.CapabilityRBACAssignmentWrite
	CapabilityRBACRoleRead         = rbaccore.CapabilityRBACRoleRead
	CapabilityRBACRoleManage       = rbaccore.CapabilityRBACRoleManage
	CapabilityIoTRegistryRead      = rbaccore.CapabilityIoTRegistryRead
	CapabilityIoTRegistryManage    = rbaccore.CapabilityIoTRegistryManage
	CapabilitySecurityPolicyRead   = rbaccore.CapabilitySecurityPolicyRead
	CapabilitySecurityPolicyManage = rbaccore.CapabilitySecurityPolicyManage
	CapabilitySecurityView         = rbaccore.CapabilitySecurityView
	CapabilitySecurityManage       = rbaccore.CapabilitySecurityManage
	CapabilityHARead               = rbaccore.CapabilityHARead
	CapabilityHAManage             = rbaccore.CapabilityHAManage
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

func allCapabilities() []string {
	return rbaccore.AllCapabilities()
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
