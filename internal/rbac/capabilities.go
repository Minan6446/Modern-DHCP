package rbac

const (
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
	CapabilityUserRead             = "auth.user.read"
	CapabilityUserManage           = "auth.user.manage"
	CapabilityAPIKeyManage         = "auth.apikey.manage"
	CapabilityAuthProviderRead     = "auth.provider.read"
	CapabilityAuthProviderManage   = "auth.provider.manage"
	CapabilityTenantQuotaRead      = "tenant.quota.read"
	CapabilityTenantQuotaWrite     = "tenant.quota.write"
	CapabilityRBACAssignmentRead   = "rbac.assignment.read"
	CapabilityRBACAssignmentWrite  = "rbac.assignment.write"
	CapabilityRBACRoleRead         = "rbac.role.read"
	CapabilityRBACRoleManage       = "rbac.role.manage"
	CapabilityIoTRegistryRead      = "iot.registry.read"
	CapabilityIoTRegistryManage    = "iot.registry.manage"
	CapabilitySecurityPolicyRead   = "security.policy.read"
	CapabilitySecurityPolicyManage = "security.policy.manage"
	CapabilitySecurityView         = "security.view"
	CapabilitySecurityManage       = "security.manage"
	CapabilityHARead               = "ha.read"
	CapabilityHAManage             = "ha.manage"
)

func AllCapabilities() []string {
	return []string{
		CapabilityPolicyRead,
		CapabilityPolicyWrite,
		CapabilityPolicyBYODManage,
		CapabilityPoolRead,
		CapabilityPoolWrite,
		CapabilityBindingRead,
		CapabilityBindingManage,
		CapabilityLeaseRead,
		CapabilityLeaseManage,
		CapabilityReportRead,
		CapabilityAuditRead,
		CapabilityUserRead,
		CapabilityUserManage,
		CapabilityAPIKeyManage,
		CapabilityAuthProviderRead,
		CapabilityAuthProviderManage,
		CapabilityTenantQuotaRead,
		CapabilityTenantQuotaWrite,
		CapabilityRBACAssignmentRead,
		CapabilityRBACAssignmentWrite,
		CapabilityRBACRoleRead,
		CapabilityRBACRoleManage,
		CapabilityIoTRegistryRead,
		CapabilityIoTRegistryManage,
		CapabilitySecurityPolicyRead,
		CapabilitySecurityPolicyManage,
		CapabilitySecurityView,
		CapabilitySecurityManage,
		CapabilityHARead,
		CapabilityHAManage,
	}
}
