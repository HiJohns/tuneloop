package services

import (
	"fmt"
	"strings"
)

type RoleTemplate struct {
	Name         string   `json:"name"`
	SysPermBits  []int    `json:"sys_perm_bits"`
	CusPermCodes []string `json:"cus_perm_codes"`
	Description  string   `json:"description"`
}

var AllRoleTemplates = map[string]RoleTemplate{
	"namespace_admin": {
		Name:         "命名空间管理员",
		SysPermBits:  []int{5, 6, 7, 8, 9, 15, 16, 17, 18, 19},
		CusPermCodes: []string{"category:manage", "attribute:manage"},
		Description:  "命名空间管理员，管理商户和人员",
	},
		"merchant_admin": {
			Name:         "商户管理员",
			SysPermBits:  []int{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29},
			CusPermCodes: []string{
				"instrument:create", "instrument:read", "instrument:update", "instrument:delete", "instrument:price", "instrument:price_config",
				"instrument:maintain",
			"order:create", "order:read", "order:update", "order:cancel",
			"appeal:create", "appeal:read", "appeal:handle",
			"audit_log:read",
		},
		Description: "商户级管理权限，全部业务权限",
	},
	"site_admin": {
		Name:         "网点管理员",
		SysPermBits:  []int{15, 16, 17},
		CusPermCodes: []string{
			"instrument:create", "instrument:read", "instrument:update", "instrument:delete", "instrument:price", "instrument:maintain",
			"order:create", "order:read", "order:update", "order:cancel",
			"appeal:read", "appeal:handle",
			"audit_log:read",
		},
		Description: "网点管理权限",
	},
	"site_member": {
		Name:         "网点员工",
		SysPermBits:  []int{},
		CusPermCodes: []string{"instrument:create", "instrument:read", "instrument:update", "instrument:maintain", "order:create", "order:read", "order:update", "audit_log:read"},
		Description:  "网点员工基础权限",
	},
	"worker": {
		Name:         "维修工程师",
		SysPermBits:  []int{},
		CusPermCodes: []string{"instrument:read", "instrument:maintain"},
		Description:  "维修权限",
	},
	"customer": {
		Name:         "顾客",
		SysPermBits:  []int{},
		CusPermCodes: []string{"order:create", "order:read", "order:cancel", "appeal:create"},
		Description:  "顾客基础权限（下单/查看/取消/申诉）",
	},
}

var CustomRoleTemplates = map[string]RoleTemplate{
	"worker": {
		Name:         "维修工程师",
		SysPermBits:  []int{},
		CusPermCodes: []string{"instrument:read", "instrument:maintain"},
		Description:  "维修权限",
	},
}

func GetRoleTemplate(code string) (RoleTemplate, bool) {
	t, ok := AllRoleTemplates[code]
	return t, ok
}

func ValidateRoleTemplate(code string) error {
	if _, ok := AllRoleTemplates[code]; !ok {
		validNames := make([]string, 0, len(AllRoleTemplates))
		for _, v := range AllRoleTemplates {
			validNames = append(validNames, v.Name)
		}
		return fmt.Errorf("invalid role_template '%s', valid values: %s", code, strings.Join(validNames, ", "))
	}
	return nil
}

var BusinessRoleMapping = map[string]string{
	"merchant_admin": "tenant_admin",
	"site_admin":     "organization_admin",
	"site_member":    "user",
	"worker":         "worker",
}

func GetBusinessRole(roleTemplate string) string {
	role, ok := BusinessRoleMapping[roleTemplate]
	if !ok {
		return ""
	}
	return role
}

func ComputeSysPermBitmap(bits []int) int64 {
	var result int64
	for _, b := range bits {
		if b >= 0 && b < 64 {
			result |= 1 << b
		}
	}
	return result
}

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

func ComputeCusPermBitmapExt(codes []string, getBit func(string) int) (int64, []byte) {
	var cusPerm int64
	var cusPermExt []byte
	for _, code := range codes {
		bit := getBit(code)
		if bit >= 0 && bit < 64 {
			cusPerm |= 1 << bit
		} else if bit >= 64 {
			extIndex := (bit - 64) / 8
			extOffset := (bit - 64) % 8
			for len(cusPermExt) <= extIndex {
				cusPermExt = append(cusPermExt, 0)
			}
			cusPermExt[extIndex] |= (1 << extOffset)
		}
	}
	return cusPerm, cusPermExt
}

func GetRoleTemplateSysPerm(roleCode string) int64 {
	t, ok := AllRoleTemplates[roleCode]
	if !ok {
		return 0
	}
	return ComputeSysPermBitmap(t.SysPermBits)
}

func GetRoleTemplateCusPerm(roleCode string, registry *PermissionRegistry) int64 {
	t, ok := AllRoleTemplates[roleCode]
	if !ok {
		return 0
	}
	return ComputeCusPermBitmap(t.CusPermCodes, registry)
}

func GetAllValidRoleTemplateCodes() []string {
	codes := make([]string, 0, len(AllRoleTemplates)+len(CustomRoleTemplates))
	for k := range AllRoleTemplates {
		codes = append(codes, k)
	}
	for k := range CustomRoleTemplates {
		codes = append(codes, k)
	}
	return codes
}
