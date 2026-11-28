package services

import (
	"fmt"
	"reflect"
	"testing"
)

// #2626 R1：DeriveRoles 表驱动用例（单值 / 多值 / 平台角色 / 空）。
func TestDeriveRoles2626(t *testing.T) {
	cases := []struct {
		name        string
		orgLevel    string
		relation    string
		functional  []string
		wantPrimary string
		wantAll     []string
	}{
		{
			name:        "single site admin",
			orgLevel:    OrgLevelSite,
			relation:    IAMRoleAdmin,
			functional:  []string{"site_admin"},
			wantPrimary: "site_admin",
			wantAll:     []string{"site_admin"},
		},
		{
			name:        "multi functional roles keep priority",
			orgLevel:    OrgLevelMerchant,
			relation:    IAMRoleStaff,
			functional:  []string{"site_member", "merchant_staff"},
			wantPrimary: "merchant_staff",
			wantAll:     []string{"site_member", "merchant_staff"},
		},
		{
			name:        "platform role kept verbatim",
			orgLevel:    OrgLevelNamespace,
			relation:    IAMRoleOwner,
			functional:  []string{"namespace_admin"},
			wantPrimary: "namespace_admin",
			wantAll:     []string{"namespace_admin"},
		},
		{
			name:        "empty functional roles derive from relation + level",
			orgLevel:    OrgLevelSite,
			relation:    IAMRoleStaff,
			functional:  nil,
			wantPrimary: "site_member",
			wantAll:     []string{"site_member"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveRoles(tc.orgLevel, tc.relation, tc.functional)
			if got.Primary != tc.wantPrimary {
				t.Fatalf("primary = %q, want %q", got.Primary, tc.wantPrimary)
			}
			if !reflect.DeepEqual(got.All, tc.wantAll) {
				t.Fatalf("all = %v, want %v", got.All, tc.wantAll)
			}
		})
	}
}

// TestDeriveRoles2626Tokens 打印机器可判定 token（#2626 R1 证据脚本 grep 这些行）。
func TestDeriveRoles2626Tokens(t *testing.T) {
	site := DeriveRoles(OrgLevelSite, IAMRoleAdmin, []string{"site_admin"})
	multi := DeriveRoles(OrgLevelMerchant, IAMRoleStaff, []string{"site_member", "merchant_staff"})
	platform := DeriveRoles(OrgLevelNamespace, IAMRoleOwner, []string{"namespace_admin"})
	fmt.Printf("DERIVE_PRIMARY_SITE_ADMIN=%s\n", site.Primary)
	fmt.Printf("DERIVE_PRIMARY_MULTI=%s\n", multi.Primary)
	fmt.Printf("DERIVE_ALL_LEN=%d\n", len(multi.All))
	kept := 0
	if platform.Primary == "namespace_admin" {
		kept = 1
	}
	fmt.Printf("DERIVE_NAMESPACE_ADMIN_KEPT=%d\n", kept)
}

// 负例：多值不能只取第一个；平台角色不能被改写成业务角色；空输入不报错。
func TestDeriveRoles2626EdgeCases(t *testing.T) {
	multi := DeriveRoles(OrgLevelMerchant, IAMRoleStaff, []string{"site_member", "merchant_staff", "repair_technician"})
	if len(multi.All) != 3 {
		t.Fatalf("all length = %d, want 3 (%v)", len(multi.All), multi.All)
	}
	if multi.Primary != "repair_technician" {
		t.Fatalf("primary = %q, want repair_technician (priority)", multi.Primary)
	}

	platform := DeriveRoles(OrgLevelNamespace, IAMRoleOwner, []string{"sys_admin", "merchant_admin"})
	if platform.Primary != "sys_admin" {
		t.Fatalf("platform role must be kept, got %q", platform.Primary)
	}

	empty := DeriveRoles("", "", nil)
	if empty.Primary != "" || len(empty.All) != 0 {
		t.Fatalf("empty input must not panic or invent a role, got %+v", empty)
	}
}
