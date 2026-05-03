package services

import (
	"testing"
)

func TestGetRoleTemplate(t *testing.T) {
	tests := []struct {
		code    string
		wantOk  bool
		wantName string
	}{
		{"namespace_admin", true, "命名空间管理员"},
		{"merchant_admin", true, "商户管理员"},
		{"site_admin", true, "网点管理员"},
		{"site_member", true, "网点员工"},
		{"worker", true, "维修工程师"},
		{"customer", true, "顾客"},
		{"invalid_role", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			template, ok := GetRoleTemplate(tt.code)
			if ok != tt.wantOk {
				t.Errorf("GetRoleTemplate(%q) ok = %v, want %v", tt.code, ok, tt.wantOk)
			}
			if ok && template.Name != tt.wantName {
				t.Errorf("GetRoleTemplate(%q) name = %q, want %q", tt.code, template.Name, tt.wantName)
			}
		})
	}
}

func TestValidateRoleTemplate(t *testing.T) {
	if err := ValidateRoleTemplate("namespace_admin"); err != nil {
		t.Errorf("ValidateRoleTemplate('namespace_admin') = %v, want nil", err)
	}
	if err := ValidateRoleTemplate("invalid"); err == nil {
		t.Error("ValidateRoleTemplate('invalid') = nil, want error")
	}
}

func TestComputeSysPermBitmap(t *testing.T) {
	tests := []struct {
		bits []int
		want int64
	}{
		{[]int{0, 1, 2}, 0b111},
		{[]int{5}, 1 << 5},
		{[]int{}, 0},
		{[]int{-1, 0, 1}, 0b11}, // -1 should be ignored
	}

	for _, tt := range tests {
		got := ComputeSysPermBitmap(tt.bits)
		if got != tt.want {
			t.Errorf("ComputeSysPermBitmap(%v) = %b, want %b", tt.bits, got, tt.want)
		}
	}
}

func TestGetRoleTemplateSysPerm(t *testing.T) {
	// namespace_admin should have all bits 0-24 set
	sysPerm := GetRoleTemplateSysPerm("namespace_admin")
	var expected int64
	for i := 0; i <= 24; i++ {
		expected |= 1 << i
	}
	if sysPerm != expected {
		t.Errorf("GetRoleTemplateSysPerm('namespace_admin') = %b, want %b", sysPerm, expected)
	}

	// customer should have 0
	if GetRoleTemplateSysPerm("customer") != 0 {
		t.Error("GetRoleTemplateSysPerm('customer') should be 0")
	}
}

func TestGetAllValidRoleTemplateCodes(t *testing.T) {
	codes := GetAllValidRoleTemplateCodes()
	if len(codes) != len(AllRoleTemplates) {
		t.Errorf("GetAllValidRoleTemplateCodes() len = %d, want %d", len(codes), len(AllRoleTemplates))
	}
}
