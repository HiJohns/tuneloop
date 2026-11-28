package services

import "strings"

// #2626：角色派生的唯一实现（Decision Lock D1/D2/D4）。
//
// 权威来源 = IAM `user_org_relations`（组织角色 `role` + 功能角色 `functional_roles[]`）；
// 本地 `users.roles` 是它的多值镜像，`users.role` 只是派生主角色（兼容旧读路径）。
//
// 派生优先级（D4）：
//
//	merchant_admin > site_admin > repair_technician > merchant_staff > site_member
//
// `namespace_admin` / `sys_admin` 是平台角色，直接保留，不参与业务角色优先级。
const (
	// OrgLevelNamespace 平台级组织。
	OrgLevelNamespace = "namespace"
	// OrgLevelMerchant 商户（tenant）级组织。
	OrgLevelMerchant = "merchant"
	// OrgLevelFranchise 加盟商级组织。
	OrgLevelFranchise = "franchise"
	// OrgLevelSite 网点（branch）级组织。
	OrgLevelSite = "site"
)

// 功能角色（functional role）码 —— 唯一字面量来源（D3/D4）。
//
// 写侧只允许经 DeriveRoles 产出的镜像落库，禁止在 handler 里内联这些字面量
// （#2626 R2 静态扫描：allowlist 之外出现内联角色字面量即失败）。
const (
	// RoleMerchantAdmin 商户管理员（组织级别 merchant 上的 OWNER）。
	RoleMerchantAdmin = "merchant_admin"
	// RoleSiteAdmin 网点管理员（组织级别 site 上的 ADMIN）。
	RoleSiteAdmin = "site_admin"
	// RoleRepairTechnician 维修师傅。
	RoleRepairTechnician = "repair_technician"
	// RoleMerchantStaff 商户直属员工。
	RoleMerchantStaff = "merchant_staff"
	// RoleSiteMember 网点员工。
	RoleSiteMember = "site_member"
	// RoleNamespaceAdmin 平台管理员（不参与业务优先级）。
	RoleNamespaceAdmin = "namespace_admin"
	// RoleSysAdmin 系统管理员（不参与业务优先级）。
	RoleSysAdmin = "sys_admin"
)

// businessRolePriority 是业务功能角色的固定优先级（D4），primary 取第一个命中。
var businessRolePriority = []string{
	RoleMerchantAdmin,
	RoleSiteAdmin,
	RoleRepairTechnician,
	RoleMerchantStaff,
	RoleSiteMember,
}

// platformRoles 是平台角色（不参与业务优先级，出现即作为主角色保留）。
var platformRoles = []string{
	RoleNamespaceAdmin,
	RoleSysAdmin,
}

// DerivedRoles 是派生结果：Primary = 派生主角色（写入 users.role），
// All = 保序去重的多值镜像（写入 users.roles）。
type DerivedRoles struct {
	Primary string
	All     []string
}

func normalizeRole(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

// NormalizeOrgLevel 把 IAM 组织类型（organization.type / type 字段）收敛到派生口径。
// 未识别的一律返回空串 —— 派生函数会退化为「只看功能角色 / 关系角色」。
func NormalizeOrgLevel(level string) string {
	switch normalizeRole(level) {
	case "namespace", "platform", "system":
		return OrgLevelNamespace
	case "merchant", "tenant":
		return OrgLevelMerchant
	case "franchise":
		return OrgLevelFranchise
	case "site", "branch", "organization":
		return OrgLevelSite
	default:
		return ""
	}
}

// DeriveRoles 由（组织级别, IAM 关系角色, 功能角色集合）派生主角色与多值镜像。
//
// 空输入不报错（历史数据允许 role=”）：返回 Primary=""、All 为清洗后的功能角色集合。
func DeriveRoles(orgLevel, relationRole string, functionalRoles []string) DerivedRoles {
	all := make([]string, 0, len(functionalRoles)+1)
	seen := make(map[string]struct{}, len(functionalRoles)+1)
	for _, raw := range functionalRoles {
		code := normalizeRole(raw)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		all = append(all, code)
	}

	primary := ""
	for _, platform := range platformRoles {
		if _, ok := seen[platform]; ok {
			primary = platform
			break
		}
	}
	if primary == "" {
		for _, candidate := range businessRolePriority {
			if _, ok := seen[candidate]; ok {
				primary = candidate
				break
			}
		}
	}

	if primary == "" {
		primary = deriveByRelation(orgLevel, relationRole)
		if primary != "" {
			if _, ok := seen[primary]; !ok {
				all = append(all, primary)
			}
		}
	}

	return DerivedRoles{Primary: primary, All: all}
}

// deriveByRelation 在只有 IAM 关系角色时的回退映射（组织角色 × 组织级别 → 功能角色）。
func deriveByRelation(orgLevel, relationRole string) string {
	level := NormalizeOrgLevel(orgLevel)
	switch strings.ToUpper(strings.TrimSpace(relationRole)) {
	case IAMRoleOwner:
		return RoleMerchantAdmin
	case IAMRoleAdmin:
		if level == OrgLevelSite || level == OrgLevelFranchise {
			return RoleSiteAdmin
		}
		if level == OrgLevelNamespace {
			return RoleNamespaceAdmin
		}
		return RoleMerchantAdmin
	case IAMRoleStaff:
		if level == OrgLevelMerchant || level == OrgLevelNamespace {
			return RoleMerchantStaff
		}
		return RoleSiteMember
	case IAMRoleRepairTechnician:
		return RoleRepairTechnician
	default:
		return ""
	}
}
