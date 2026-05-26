package services

import (
	"log"
	"sync"
	"time"
)

// PermissionRegistry manages customer permission definitions and bit code mappings.
// Uses hardcoded bit assignments — no IAM registration/sync needed.
type PermissionRegistry struct {
	mu         sync.RWMutex
	cusPermMap map[string]int
	lastSync   time.Time
}

// NewPermissionRegistry creates a permission registry with hardcoded bit mappings.
func NewPermissionRegistry() *PermissionRegistry {
	r := &PermissionRegistry{
		cusPermMap: make(map[string]int),
	}
	for _, p := range getTuneLoopPermissions() {
		r.cusPermMap[p.Code] = p.BitCode
	}
	return r
}

// RegisterAndSync initializes the registry from hardcoded mappings.
// No IAM calls — bit codes are fixed at compile time.
func (r *PermissionRegistry) RegisterAndSync(namespaceID string) error {
	log.Printf("[PermissionRegistry] Initialized %d hardcoded permissions", len(r.cusPermMap))
	r.lastSync = time.Now()
	return nil
}

// GetCusPermBit returns the bit position for a customer permission name.
func (r *PermissionRegistry) GetCusPermBit(code string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if bit, ok := r.cusPermMap[code]; ok {
		return bit
	}
	return -1
}

// GetSysPermBit returns the bit position for a system permission name.
func (r *PermissionRegistry) GetSysPermBit(name string) (int, bool) {
	bit, ok := middlewareSysPermMap[name]
	return bit, ok
}

// GetCusPermMapping returns the full cus_perm code-to-bit mapping for the frontend.
func (r *PermissionRegistry) GetCusPermMapping() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]int, len(r.cusPermMap))
	for k, v := range r.cusPermMap {
		result[k] = v
	}
	return result
}

// GlobalPermissionRegistry is the global permission registry instance.
// Set during startup by main.go. Used by service code to compute bitmaps.
var GlobalPermissionRegistry *PermissionRegistry
var middlewareSysPermMap = map[string]int{
	"namespace_view":       0,
	"namespace_list":       1,
	"namespace_create":     2,
	"namespace_update":     3,
	"namespace_delete":     4,
	"tenant_view":          5,
	"tenant_list":          6,
	"tenant_create":        7,
	"tenant_update":        8,
	"tenant_delete":        9,
	"organization_view":    10,
	"organization_list":    11,
	"organization_create":  12,
	"organization_update":  13,
	"organization_delete":  14,
	"user_view":            15,
	"user_list":            16,
	"user_create":          17,
	"user_update":          18,
	"user_delete":          19,
	"role_view":            20,
	"role_list":            21,
	"role_create":          22,
	"role_update":          23,
	"role_delete":          24,
}

// getTuneLoopPermissions returns the cus_perm definitions with hardcoded bit codes.
// Bit codes are frozen from IAM allocations (0-69, continuous, no gaps).
func getTuneLoopPermissions() []PermissionDef {
	return []PermissionDef{
		{Code: "instrument:create", Name: "创建乐器", BitCode: 0},
		{Code: "instrument:edit", Name: "编辑乐器", BitCode: 1},
		{Code: "instrument:delete", Name: "删除乐器", BitCode: 2},
		{Code: "instrument:view", Name: "查看乐器", BitCode: 3},
		{Code: "category:manage", Name: "分类管理", BitCode: 4},
		{Code: "property:manage", Name: "属性管理", BitCode: 5},
		{Code: "inventory:view", Name: "库存查看", BitCode: 6},
		{Code: "inventory:manage", Name: "库存管理/调拨", BitCode: 7},
		{Code: "rent:setting", Name: "租金设定", BitCode: 8},
		{Code: "order:view", Name: "订单/租约查看", BitCode: 9},
		{Code: "order:manage", Name: "订单管理", BitCode: 10},
		{Code: "maintenance:view", Name: "维修查看", BitCode: 11},
		{Code: "maintenance:assign", Name: "维修派单", BitCode: 12},
		{Code: "maintenance:complete", Name: "维修完成", BitCode: 13},
		{Code: "finance:config", Name: "财务配置", BitCode: 14},
		{Code: "appeal:handle", Name: "申诉处理", BitCode: 15},
		{Code: "instrument:list", Name: "乐器列表", BitCode: 16},
		{Code: "category:list", Name: "分类列表", BitCode: 17},
		{Code: "category:create", Name: "新建分类", BitCode: 18},
		{Code: "category:edit", Name: "编辑分类", BitCode: 19},
		{Code: "category:delete", Name: "删除分类", BitCode: 20},
		{Code: "property:list", Name: "属性列表", BitCode: 21},
		{Code: "property:create", Name: "新建属性", BitCode: 22},
		{Code: "property:edit", Name: "编辑属性", BitCode: 23},
		{Code: "property:delete", Name: "删除属性", BitCode: 24},
		{Code: "property:merge", Name: "合并属性", BitCode: 25},
		{Code: "inventory:list", Name: "库存列表", BitCode: 26},
		{Code: "inventory:transfer", Name: "库存调拨", BitCode: 27},
		{Code: "rent:view", Name: "租金查看", BitCode: 28},
		{Code: "rent:edit", Name: "租金编辑", BitCode: 29},
		{Code: "order:list", Name: "订单列表", BitCode: 30},
		{Code: "order:create", Name: "新建订单", BitCode: 31},
		{Code: "order:pay", Name: "订单支付", BitCode: 32},
		{Code: "order:pickup", Name: "订单取件", BitCode: 33},
		{Code: "order:return", Name: "订单归还", BitCode: 34},
		{Code: "order:cancel", Name: "取消订单", BitCode: 35},
		{Code: "order:transfer", Name: "转移归属", BitCode: 36},
		{Code: "order:terminate", Name: "终止订单", BitCode: 37},
		{Code: "maintenance:list", Name: "维修列表", BitCode: 38},
		{Code: "maintenance:create", Name: "提交维修", BitCode: 39},
		{Code: "maintenance:accept", Name: "接受维修", BitCode: 40},
		{Code: "maintenance:quote", Name: "维修报价", BitCode: 41},
		{Code: "maintenance:start", Name: "开始维修", BitCode: 42},
		{Code: "maintenance:inspect", Name: "检验维修", BitCode: 43},
		{Code: "maintenance:cancel", Name: "取消维修", BitCode: 44},
		{Code: "lease:list", Name: "租赁列表", BitCode: 45},
		{Code: "lease:view", Name: "租赁查看", BitCode: 46},
		{Code: "lease:create", Name: "新建租赁", BitCode: 47},
		{Code: "lease:edit", Name: "编辑租赁", BitCode: 48},
		{Code: "lease:terminate", Name: "终止租赁", BitCode: 49},
		{Code: "deposit:list", Name: "押金列表", BitCode: 50},
		{Code: "deposit:view", Name: "押金查看", BitCode: 51},
		{Code: "deposit:create", Name: "新建押金", BitCode: 52},
		{Code: "deposit:edit", Name: "编辑押金", BitCode: 53},
		{Code: "appeal:list", Name: "申诉列表", BitCode: 54},
		{Code: "appeal:view", Name: "申诉查看", BitCode: 55},
		{Code: "appeal:create", Name: "提交申诉", BitCode: 56},
		{Code: "appeal:resolve", Name: "裁定申诉", BitCode: 57},
		{Code: "label:list", Name: "标签列表", BitCode: 58},
		{Code: "label:create", Name: "新建标签", BitCode: 59},
		{Code: "label:approve", Name: "批准标签", BitCode: 60},
		{Code: "label:reject", Name: "拒绝标签", BitCode: 61},
		{Code: "label:merge", Name: "合并标签", BitCode: 62},
		{Code: "worker:list", Name: "师傅列表", BitCode: 63},
		{Code: "worker:create", Name: "新建师傅", BitCode: 64},
		{Code: "worker:delete", Name: "删除师傅", BitCode: 65},
		{Code: "audit_log:view", Name: "审计日志查看", BitCode: 66},
		{Code: "audit_log:export", Name: "审计日志导出", BitCode: 67},
		{Code: "organization:import", Name: "批量导入组织", BitCode: 68},
		{Code: "account:import", Name: "批量导入账号", BitCode: 69},
	}
}

// UpdateGlobalRegistry is kept for API compatibility.
func (r *PermissionRegistry) UpdateGlobalRegistry() {
}
