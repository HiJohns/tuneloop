package services

import (
	"fmt"
	"log"
)

// BootstrapDefaultPermissions registers customer permissions with IAM and initializes
// default role assignments for TuneLoop's business roles.
func BootstrapDefaultPermissions(iamClient *IAMClient, namespaceID string) error {
	registry := NewPermissionRegistry(iamClient)

	// Register permissions with IAM
	log.Printf("[Bootstrap] Registering customer permissions for namespace %s", namespaceID)
	if err := registry.RegisterAndSync(namespaceID); err != nil {
		return fmt.Errorf("failed to register customer permissions: %w", err)
	}

	// After successful registration, set default role permissions.
	// Note: role template IDs are derived from the role code names defined in IAM.
	// The exact IDs depend on the IAM setup and may need to be configured.
	log.Printf("[Bootstrap] Customer permissions registered. Default role permissions should be set via IAM admin UI or API.")

	return nil
}

// DefaultRolePermissions defines which cus_perm codes each business role gets by default.
type DefaultRolePermissions struct {
	RoleCode string
	Perms    []string
}

// GetDefaultRolePermissions returns the predefined default permission sets.
func GetDefaultRolePermissions() []DefaultRolePermissions {
	return []DefaultRolePermissions{
		{
			RoleCode: "owner", // Merchant admin (tenant admin)
			Perms: []string{
				"instrument:create", "instrument:edit", "instrument:delete",
				"category:manage", "property:manage",
				"inventory:view", "inventory:manage", "rent:setting",
				"order:view", "order:manage",
				"maintenance:view", "maintenance:assign", "maintenance:complete",
				"finance:config", "appeal:handle",
			},
		},
		{
			RoleCode: "admin", // Site admin (group admin)
			Perms: []string{
				"inventory:view", "inventory:manage",
				"order:view", "order:manage",
				"maintenance:view", "maintenance:assign", "maintenance:complete",
				"appeal:handle",
			},
		},
		{
			RoleCode: "staff", // Site staff (group member)
			Perms: []string{
				"maintenance:view", "maintenance:complete",
			},
		},
		{
			RoleCode: "worker", // Maintenance worker
			Perms: []string{
				"maintenance:view", "maintenance:complete",
			},
		},
	}
}
