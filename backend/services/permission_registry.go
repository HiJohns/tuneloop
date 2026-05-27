package services

import (
	"log"
	"sync"
	"time"
)

type PermissionRegistry struct {
	mu         sync.RWMutex
	cusPermMap map[string]int
	lastSync   time.Time
}

func NewPermissionRegistry() *PermissionRegistry {
	r := &PermissionRegistry{
		cusPermMap: make(map[string]int),
	}
	for _, p := range getTuneLoopPermissions() {
		r.cusPermMap[p.Code] = p.BitCode
	}
	return r
}

func (r *PermissionRegistry) RegisterAndSync(namespaceID string) error {
	log.Printf("[PermissionRegistry] Initialized %d hardcoded permissions", len(r.cusPermMap))
	r.lastSync = time.Now()
	return nil
}

func (r *PermissionRegistry) GetCusPermBit(code string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if bit, ok := r.cusPermMap[code]; ok {
		return bit
	}
	return -1
}

func (r *PermissionRegistry) GetSysPermBit(name string) (int, bool) {
	bit, ok := middlewareSysPermMap[name]
	return bit, ok
}

func (r *PermissionRegistry) GetCusPermMapping() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]int, len(r.cusPermMap))
	for k, v := range r.cusPermMap {
		result[k] = v
	}
	return result
}

var GlobalPermissionRegistry *PermissionRegistry
var middlewareSysPermMap = map[string]int{
	"namespace_view":      0,
	"namespace_list":      1,
	"namespace_create":    2,
	"namespace_update":    3,
	"namespace_delete":    4,
	"tenant_view":         5,
	"tenant_list":         6,
	"tenant_update":       8,
	"tenant_delete":       9,
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
	"tenant:create":        25,
	"permission:manage":    26,
}

func getTuneLoopPermissions() []PermissionDef {
	return []PermissionDef{
		{Code: "instrument:create", Name: "创建乐器", BitCode: 0},
		{Code: "instrument:read", Name: "查看乐器", BitCode: 1},
		{Code: "instrument:update", Name: "编辑乐器", BitCode: 2},
		{Code: "instrument:delete", Name: "删除乐器", BitCode: 3},
		{Code: "instrument:price", Name: "乐器定价", BitCode: 4},
		{Code: "instrument:maintain", Name: "维修管理", BitCode: 5},
		{Code: "order:create", Name: "创建订单", BitCode: 6},
		{Code: "order:read", Name: "查看订单", BitCode: 7},
		{Code: "order:update", Name: "编辑订单", BitCode: 8},
		{Code: "order:cancel", Name: "取消订单", BitCode: 9},
	}
}

func (r *PermissionRegistry) UpdateGlobalRegistry() {
}
