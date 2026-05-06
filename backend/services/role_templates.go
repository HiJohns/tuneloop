package services

import (
	"fmt"
	"strings"
)

// RoleTemplate defines the permission mapping for a predefined role.
type RoleTemplate struct {
	Name           string   `json:"name"`
	SysPermBits    []int    `json:"sys_perm_bits"`
	CusPermCodes   []string `json:"cus_perm_codes"`
	Description    string   `json:"description"`
}

// AllRoleTemplates maps role template code to its definition.
// These must be kept in sync with backend/services/permission_bootstrap.go.
var AllRoleTemplates = map[string]RoleTemplate{
	"namespace_admin": {
		Name:        "命名空间管理员",
		SysPermBits: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24},
		CusPermCodes: []string{},
		Description: "全部系统权限，无业务权限",
	},
	"merchant_admin": {
		Name:        "商户管理员",
		SysPermBits: []int{5, 6, 7, 8, 9},
		CusPermCodes: []string{
			"instrument:create", "instrument:edit", "instrument:delete",
			"category:manage", "property:manage",
			"inventory:view", "inventory:manage",
			"rent:setting",
			"order:view", "order:manage",
			"maintenance:view", "maintenance:assign", "maintenance:complete",
			"finance:config", "appeal:handle",
		},
		Description: "商户级管理权限，全部业务权限",
	},
	"site_admin": {
		Name:        "网点管理员",
		SysPermBits: []int{10, 11, 12, 13, 14, 15, 16, 17, 18, 19},
		CusPermCodes: []string{
			"inventory:view", "inventory:manage",
			"order:view", "order:manage",
			"maintenance:view", "maintenance:assign", "maintenance:complete",
			"appeal:handle",
		},
		Description: "网点管理权限",
	},
	"site_member": {
		Name:         "网点员工",
		SysPermBits:  []int{},
		CusPermCodes: []string{"instrument:view", "maintenance:view", "maintenance:complete"},
		Description:  "网点员工基础权限",
	},
	"worker": {
		Name:         "维修工程师",
		SysPermBits:  []int{},
		CusPermCodes: []string{"maintenance:view", "maintenance:complete"},
		Description:  "维修权限",
	},
	"customer": {
		Name:         "顾客",
		SysPermBits:  []int{},
		CusPermCodes: []string{},
		Description:  "无管理权限",
	},
}

// GetRoleTemplate returns a role template by code.
func GetRoleTemplate(code string) (RoleTemplate, bool) {
	t, ok := AllRoleTemplates[code]
	return t, ok
}

// ValidateRoleTemplate checks if a role template code is valid.
func ValidateRoleTemplate(code string) error {
	if _, ok := AllRoleTemplates[code]; !ok {
		validCodes := make([]string, 0, len(AllRoleTemplates))
		for k := range AllRoleTemplates {
			validCodes = append(validCodes, k)
		}
		return fmt.Errorf("invalid role_template '%s', valid values: %s", code, strings.Join(validCodes, ", "))
	}
	return nil
}

var BusinessRoleMapping = map[string]string{
	"merchant_admin": "owner",
	"site_admin":     "admin",
	"site_member":    "staff",
	"worker":         "worker",
}

func GetBusinessRole(roleTemplate string) string {
	role, ok := BusinessRoleMapping[roleTemplate]
	if !ok {
		return ""
	}
	return role
}

// ComputeSysPermBitmap calculates the sys_perm integer bitmap from bit positions.
func ComputeSysPermBitmap(bits []int) int64 {
	var result int64
	for _, b := range bits {
		if b >= 0 && b < 64 {
			result |= 1 << b
		}
	}
	return result
}

// ComputeCusPermBitmap calculates the cus_perm integer bitmap from permission codes using the registry.
func ComputeCusPermBitmap(codes []string, registry *PermissionRegistry) int64 {
	var result int64
	for _, code := range codes {
		bit := registry.GetCusPermBit(code)
		if bit >= 0 {
			result |= 1 << bit
		}
	}
	return result
}

// GetRoleTemplateSysPerm returns the sys_perm bitmap for a role template.
func GetRoleTemplateSysPerm(roleCode string) int64 {
	t, ok := AllRoleTemplates[roleCode]
	if !ok {
		return 0
	}
	return ComputeSysPermBitmap(t.SysPermBits)
}

// GetRoleTemplateCusPerm returns the cus_perm bitmap for a role template.
func GetRoleTemplateCusPerm(roleCode string, registry *PermissionRegistry) int64 {
	t, ok := AllRoleTemplates[roleCode]
	if !ok {
		return 0
	}
	return ComputeCusPermBitmap(t.CusPermCodes, registry)
}

// GetAllValidRoleTemplateCodes returns all valid role template codes.
func GetAllValidRoleTemplateCodes() []string {
	codes := make([]string, 0, len(AllRoleTemplates))
	for k := range AllRoleTemplates {
		codes = append(codes, k)
	}
	return codes
}
