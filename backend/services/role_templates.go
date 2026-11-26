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
		CusPermCodes: []string{"category:manage", "attribute:manage", "banner:manage", "mall_image:manage", "rebate:manage", "promo:manage", "points:manage", "membership:manage"},
		Description:  "命名空间管理员，管理平台配置（分类/属性/轮播图/商城图片/返点/折扣/点数/会员等级）",
	},
	"sys_admin": {
		Name:         "系统管理员",
		SysPermBits:  []int{5, 6, 7, 8, 9, 15, 16, 17, 18, 19},
		CusPermCodes: []string{"category:manage", "attribute:manage", "banner:manage", "mall_image:manage", "rebate:manage", "promo:manage", "points:manage", "membership:manage"},
		Description:  "TuneLoop 系统管理员，管理商户、人员与商城图片",
	},
	"merchant_admin": {
		Name:        "商户管理员",
		SysPermBits: []int{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29},
		CusPermCodes: []string{
			"instrument:create", "instrument:read", "instrument:update", "instrument:delete", "instrument:price", "instrument:price_config",
			"instrument:maintain",
			"repair:read", // #2129：PC 服务单列表（商户管理员）
			"repair:start", "repair:complete", "repair:accept",
			"instrument:media_upload", "instrument:media_display", "instrument:media_delete",
			"order:create", "order:read", "order:update", "order:cancel",
			"appeal:create", "appeal:read", "appeal:handle",
			"audit_log:read",
			"promo:manage", "points:manage",
			"member:invite", // #2114 邀请非本商户成员加入 + 审批网点申请
		},
		Description: "商户级管理权限，全部业务权限",
	},
	"site_admin": {
		Name:        "网点管理员",
		SysPermBits: []int{15, 16, 17},
		CusPermCodes: []string{
			"instrument:create", "instrument:read", "instrument:update", "instrument:delete", "instrument:price", "instrument:maintain",
			"repair:read", // #2229：网点管理员查看/转派本商户维修服务单（PC /repair/requests）
			"repair:start", "repair:complete", "repair:accept",
			"instrument:media_upload", "instrument:media_display", "instrument:media_delete",
			"order:create", "order:read", "order:update", "order:cancel",
			"appeal:read", "appeal:handle",
			"audit_log:read",
			"promo:override", "points:manage",
		},
		Description: "网点管理权限",
	},
	"site_member": {
		Name:         "网点员工",
		SysPermBits:  []int{},
		CusPermCodes: []string{"instrument:create", "instrument:read", "instrument:update", "instrument:delete", "instrument:maintain", "repair:start", "repair:complete", "repair:accept", "instrument:media_upload", "instrument:media_delete", "order:create", "order:read", "order:update", "audit_log:read"},
		Description:  "网点员工基础权限",
	},
	"merchant_staff": {
		Name:         "商户直属员工",
		SysPermBits:  []int{},
		CusPermCodes: []string{"instrument:create", "instrument:read", "instrument:update", "instrument:delete", "instrument:maintain", "repair:read", "repair:start", "repair:complete", "repair:accept", "instrument:media_upload", "instrument:media_delete", "order:create", "order:read", "order:update", "audit_log:read"},
		Description:  "商户直属员工（不挂靠网点；含看维修服务单 repair:read，#2129）",
	},
	"repair_technician": {
		Name:         "维修师傅",
		SysPermBits:  []int{},
		CusPermCodes: []string{"instrument:read", "instrument:maintain", "repair:start", "repair:complete"},
		Description:  "维修权限",
	},
	"customer": {
		Name:         "顾客",
		SysPermBits:  []int{},
		CusPermCodes: []string{},
		Description:  "顾客基础权限（所有权模型，不依赖 cus_perm）",
	},
}

var CustomRoleTemplates = map[string]RoleTemplate{
	"repair_technician": {
		Name:         "维修师傅",
		SysPermBits:  []int{},
		CusPermCodes: []string{"instrument:read", "instrument:maintain", "repair:start", "repair:complete"},
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
	"merchant_admin":    "tenant_admin",
	"site_admin":        "organization_admin",
	"site_member":       "user",
	"repair_technician": "repair_technician",
}

// IAM 关系角色（organization relation role）。这是 beaconiam 侧真正识别的取值集合，
// 由 backend 侧 roleToPermission 判定（OWNER/ADMIN/NAMESPACE_ADMIN/CLIENT），
// repair_technician 由本系统自带语义（#2466/#2526）。
const (
	IAMRoleOwner            = "OWNER"
	IAMRoleAdmin            = "ADMIN"
	IAMRoleStaff            = "STAFF"
	IAMRoleRepairTechnician = "repair_technician"
)

// IAMRelationRoles 是合法的 IAM 关系角色白名单（D3）。写入 IAM 前必须落在集合内。
var IAMRelationRoles = map[string]struct{}{
	IAMRoleOwner:            {},
	IAMRoleAdmin:            {},
	IAMRoleStaff:            {},
	IAMRoleRepairTechnician: {},
}

// ToIAMRole 把 tuneloop 的业务角色/角色模板码映射为 IAM 关系角色（#2467/#2466）。
//
// 注意：这是**映射**而非**校验**——未知码原样返回，因此调用方在写入 IAM 前
// 必须再经过 NormalizeIAMRelationRole 兜底，否则会把业务角色当 IAM 角色写入
// （#2467 根因，`site_member` → `"user"`）。
func ToIAMRole(tuneloopCode string) string {
	switch tuneloopCode {
	case "merchant_admin":
		return IAMRoleOwner
	case "site_admin":
		return IAMRoleAdmin
	case "site_member":
		return IAMRoleStaff
	case "repair_technician":
		return IAMRoleRepairTechnician
	case "merchant_staff":
		return IAMRoleStaff // #2129：商户直属员工 → 商户组织 STAFF 关系
	default:
		return tuneloopCode
	}
}

// NormalizeIAMRelationRole 把任意角色码收敛到 IAM 关系角色白名单，无法识别时回落 STAFF。
// 空值/业务角色（`user`/`tenant_admin`/`organization_admin`）一律视为不可识别。
func NormalizeIAMRelationRole(role string) string {
	code := strings.TrimSpace(role)
	if _, ok := IAMRelationRoles[code]; ok {
		return code
	}
	return IAMRoleStaff
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
