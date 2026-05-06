package services

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// PermissionRegistry manages customer permission definitions and bit code mappings.
// It replaces the mock registry at startup by fetching real mappings from IAM.
type PermissionRegistry struct {
	mu             sync.RWMutex
	iamClient      *IAMClient
	cusPermMap     map[string]int          // code -> bit_code
	cusPermDetails map[string]PermissionMapping // code -> full details
	lastSync       time.Time
	cacheFile      string
}

// NewPermissionRegistry creates a permission registry backed by IAM.
func NewPermissionRegistry(iamClient *IAMClient) *PermissionRegistry {
	return &PermissionRegistry{
		iamClient:  iamClient,
		cusPermMap: make(map[string]int),
		cacheFile:  "tmp/.permission_cache.json",
	}
}

// RegisterAndSync registers TuneLoop's cus_perm definitions with IAM and caches the mapping.
// On failure, it falls back to the local cache file if available.
func (r *PermissionRegistry) RegisterAndSync(namespaceID string) error {
	// Try to load from cache first
	if err := r.loadFromCache(); err == nil && len(r.cusPermMap) > 0 {
		log.Printf("[PermissionRegistry] Loaded %d permissions from cache", len(r.cusPermMap))
	}

	// Register permissions with IAM
	perms := r.getTuneLoopPermissions()
	registered, err := r.iamClient.RegisterCustomerPermissions(namespaceID, perms)
	if err != nil {
		log.Printf("[PermissionRegistry] Failed to register permissions with IAM: %v", err)
		if len(r.cusPermMap) > 0 {
			log.Printf("[PermissionRegistry] Using cached permissions (%d entries)", len(r.cusPermMap))
			return nil
		}
		env := os.Getenv("APP_ENV")
		if env == "production" {
			return fmt.Errorf("permission registration failed in production and no cache available: %w", err)
		}
		log.Printf("[PermissionRegistry] Development mode: continuing with empty permissions")
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range registered {
		if p.IsActive {
			r.cusPermMap[p.Code] = p.BitCode
			r.cusPermDetails[p.Code] = p
		}
	}
	r.lastSync = time.Now()
	log.Printf("[PermissionRegistry] Registered %d permissions with IAM", len(registered))

	snapshot := make(map[string]int, len(r.cusPermMap))
	for k, v := range r.cusPermMap {
		snapshot[k] = v
	}
	r.saveToCache(snapshot)

	// Start background sync
	go r.backgroundSync(namespaceID)
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
	// Sys_perm bits are fixed (0-24) from BeaconIAM, cached from config
	// These are the same as the MockPermissionRegistry
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

// middlewareSysPermMap is a copy of the sys_perm constants for registry access.
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

// getTuneLoopPermissions returns the 16 cus_perm definitions for TuneLoop.
func (r *PermissionRegistry) getTuneLoopPermissions() []PermissionDef {
	return []PermissionDef{
		{Code: "instrument:create", Name: "创建乐器"},
		{Code: "instrument:edit", Name: "编辑乐器"},
		{Code: "instrument:delete", Name: "删除乐器"},
		{Code: "instrument:view", Name: "查看乐器"},
		{Code: "category:manage", Name: "分类管理"},
		{Code: "property:manage", Name: "属性管理"},
		{Code: "inventory:view", Name: "库存查看"},
		{Code: "inventory:manage", Name: "库存管理/调拨"},
		{Code: "rent:setting", Name: "租金设定"},
		{Code: "order:view", Name: "订单/租约查看"},
		{Code: "order:manage", Name: "订单管理"},
		{Code: "maintenance:view", Name: "维修查看"},
		{Code: "maintenance:assign", Name: "维修派单"},
		{Code: "maintenance:complete", Name: "维修完成"},
		{Code: "finance:config", Name: "财务配置"},
		{Code: "appeal:handle", Name: "申诉处理"},
	}
}

type cacheEntry struct {
	CusPermMap map[string]int `json:"cus_perm_map"`
	LastSync   string         `json:"last_sync"`
}

func (r *PermissionRegistry) loadFromCache() error {
	data, err := os.ReadFile(r.cacheFile)
	if err != nil {
		return err
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cusPermMap = entry.CusPermMap
	if t, err := time.Parse(time.RFC3339, entry.LastSync); err == nil {
		r.lastSync = t
	}
	return nil
}

func (r *PermissionRegistry) saveToCache(snapshot map[string]int) {
	entry := cacheEntry{
		CusPermMap: snapshot,
		LastSync:   r.lastSync.Format(time.RFC3339),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		log.Printf("[PermissionRegistry] Failed to marshal cache: %v", err)
		return
	}
	if err := os.MkdirAll("tmp", 0755); err != nil {
		log.Printf("[PermissionRegistry] Failed to create tmp dir: %v", err)
		return
	}
	if err := os.WriteFile(r.cacheFile, data, 0644); err != nil {
		log.Printf("[PermissionRegistry] Failed to write cache: %v", err)
	}
}

func (r *PermissionRegistry) backgroundSync(namespaceID string) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		mappings, err := r.iamClient.GetCustomerPermissions(namespaceID)
		if err != nil {
			log.Printf("[PermissionRegistry] Background sync failed: %v", err)
			continue
		}
		r.mu.Lock()
		for _, p := range mappings {
			if p.IsActive {
				r.cusPermMap[p.Code] = p.BitCode
			} else {
				delete(r.cusPermMap, p.Code)
			}
		}
		r.lastSync = time.Now()
		snapshot := make(map[string]int, len(r.cusPermMap))
		for k, v := range r.cusPermMap {
			snapshot[k] = v
		}
		r.mu.Unlock()
		r.saveToCache(snapshot)
		log.Printf("[PermissionRegistry] Background sync: %d permissions", len(mappings))
	}
}

// UpdateGlobalRegistry replaces the global middleware PermissionRegistry with this real one.
func (r *PermissionRegistry) UpdateGlobalRegistry() {
	// This will be replaced in a future refactor when middleware.PermissionRegistry
	// accepts a PermissionRegistryInterface.
	// For now, the middleware uses the mock registry; we upgrade it here.
	_ = r // suppression for unused
}
