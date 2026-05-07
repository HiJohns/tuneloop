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

	// Sync sys_perm from role_templates.go to IAM functional_role_templates
	log.Printf("[Bootstrap] Syncing role template sys_perm to IAM")
	for code, template := range AllRoleTemplates {
		if len(template.SysPermBits) > 0 {
			if err := iamClient.SyncRoleTemplateSysPerm(namespaceID, code, template.SysPermBits); err != nil {
				log.Printf("[Bootstrap] Warning: failed to sync sys_perm for role %s: %v", code, err)
			}
		}
	}

	// After successful registration, set default role permissions.
	// Note: role template IDs are derived from the role code names defined in IAM.
	// The exact IDs depend on the IAM setup and may need to be configured.
	log.Printf("[Bootstrap] Customer permissions registered. Syncing default role permissions to IAM...")

	defaultPerms := GetDefaultRolePermissions()
	roleTemplates, err := iamClient.ListRoleTemplates(namespaceID)
	if err != nil {
		log.Printf("[Bootstrap] Warning: failed to list role templates: %v", err)
	} else {
		for _, rp := range defaultPerms {
			for _, rt := range roleTemplates {
				if rt.Code == rp.RoleCode {
					if err := iamClient.SetRoleCustomerPermissions(namespaceID, rt.ID, rp.Perms); err != nil {
						log.Printf("[Bootstrap] Warning: failed to set cus_perms for role %s: %v", rp.RoleCode, err)
					} else {
						log.Printf("[Bootstrap] Synced cus_perms for role %s: %v", rp.RoleCode, rp.Perms)
					}
					break
				}
			}
		}
		// Increment perm_version to notify clients
		iamClient.IncrementPermVersion()
	}

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
				"rent:setting",
				"order:view", "order:manage",
				"maintenance:view", "maintenance:assign", "maintenance:complete",
				"appeal:handle",
			},
		},
		{
			RoleCode: "staff", // Site staff (group member)
			Perms: []string{
				"instrument:view",
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
