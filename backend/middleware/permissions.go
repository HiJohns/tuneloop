package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

// SysPerm bit codes (0-24, defined by BeaconIAM)
const (
	SysPermNamespaceView       = 0
	SysPermNamespaceList       = 1
	SysPermNamespaceCreate     = 2
	SysPermNamespaceUpdate     = 3
	SysPermNamespaceDelete     = 4
	SysPermTenantView          = 5
	SysPermTenantList          = 6
	SysPermTenantCreate        = 7
	SysPermTenantUpdate        = 8
	SysPermTenantDelete        = 9
	SysPermOrganizationView    = 10
	SysPermOrganizationList    = 11
	SysPermOrganizationCreate  = 12
	SysPermOrganizationUpdate  = 13
	SysPermOrganizationDelete  = 14
	SysPermUserView            = 15
	SysPermUserList            = 16
	SysPermUserCreate          = 17
	SysPermUserUpdate          = 18
	SysPermUserDelete          = 19
	SysPermRoleView            = 20
	SysPermRoleList            = 21
	SysPermRoleCreate          = 22
	SysPermRoleUpdate          = 23
	SysPermRoleDelete          = 24
)

// SysPermBitNames maps bit position to permission code name
var SysPermBitNames = map[int]string{
	SysPermNamespaceView:       "namespace_view",
	SysPermNamespaceList:       "namespace_list",
	SysPermNamespaceCreate:     "namespace_create",
	SysPermNamespaceUpdate:     "namespace_update",
	SysPermNamespaceDelete:     "namespace_delete",
	SysPermTenantView:          "tenant_view",
	SysPermTenantList:          "tenant_list",
	SysPermTenantCreate:        "tenant_create",
	SysPermTenantUpdate:        "tenant_update",
	SysPermTenantDelete:        "tenant_delete",
	SysPermOrganizationView:    "organization_view",
	SysPermOrganizationList:    "organization_list",
	SysPermOrganizationCreate:  "organization_create",
	SysPermOrganizationUpdate:  "organization_update",
	SysPermOrganizationDelete:  "organization_delete",
	SysPermUserView:            "user_view",
	SysPermUserList:            "user_list",
	SysPermUserCreate:          "user_create",
	SysPermUserUpdate:          "user_update",
	SysPermUserDelete:          "user_delete",
	SysPermRoleView:            "role_view",
	SysPermRoleList:            "role_list",
	SysPermRoleCreate:          "role_create",
	SysPermRoleUpdate:          "role_update",
	SysPermRoleDelete:          "role_delete",
}

// PermissionRegistry is the global permission registry (mock by default, real in Sub-task C).
var PermissionRegistry PermissionRegistryInterface = NewMockPermissionRegistry()

// PermissionRegistryInterface defines the contract for permission lookups.
type PermissionRegistryInterface interface {
	GetSysPermBit(name string) (int, bool)
	GetCusPermBit(name string) int
	GetCusPermMapping() map[string]int
}

// MockPermissionRegistry provides hardcoded sys_perm mappings.
// cus_perm returns -1 (not yet available) until Sub-task C replaces it.
type MockPermissionRegistry struct {
	mu         sync.RWMutex
	SysPermMap map[string]int
}

// NewMockPermissionRegistry creates a mock registry with hardcoded sys_perm bits.
func NewMockPermissionRegistry() *MockPermissionRegistry {
	m := &MockPermissionRegistry{
		SysPermMap: make(map[string]int),
	}
	for bit, name := range SysPermBitNames {
		m.SysPermMap[name] = bit
	}
	return m
}

func (r *MockPermissionRegistry) GetCusPermBit(name string) int {
	return -1
}

func (r *MockPermissionRegistry) GetSysPermBit(name string) (int, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	bit, ok := r.SysPermMap[name]
	return bit, ok
}

func (r *MockPermissionRegistry) GetCusPermMapping() map[string]int {
	return map[string]int{}
}

// RequireSysPerm checks whether the user has a specific sys_perm bit set.
// When sys_perm is 0 (no bitmap from IAM yet), the check passes through
// to maintain backward compatibility with existing JWT tokens.
func RequireSysPerm(bit int) gin.HandlerFunc {
	return func(c *gin.Context) {
		sysPerm := GetSysPerm(c.Request.Context())
		if sysPerm == 0 {
			c.Next()
			return
		}
		if sysPerm&(1<<bit) == 0 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    40303,
				"message": "insufficient system permission",
			})
			return
		}
		c.Next()
	}
}

// RequireCusPerm checks whether the user has a specific cus_perm bit set.
// Uses the global PermissionRegistry to resolve name to bit position.
// When cus_perm is 0 (no bitmap from IAM yet), passes through for compatibility.
// If the bit is -1 (mock mode), the permission check is skipped (pass-through).
func RequireCusPerm(name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		bit := PermissionRegistry.GetCusPermBit(name)
		if bit < 0 {
			c.Next()
			return
		}
		cusPerm := GetCusPerm(c.Request.Context())
		if cusPerm == 0 {
			c.Next()
			return
		}
		if cusPerm&(1<<bit) == 0 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    40305,
				"message": "insufficient customer permission",
			})
			return
		}
		c.Next()
	}
}

