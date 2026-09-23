package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// #2034: EffectiveRoles 优先返回多重角色集，缺失时回退主角色。
func TestSiteMember_EffectiveRoles(t *testing.T) {
	cases := []struct {
		name string
		m    SiteMember
		want []string
	}{
		{"roles set", SiteMember{Role: "site_member", Roles: []string{"site_member", "repair_technician"}},
			[]string{"site_member", "repair_technician"}},
		{"roles empty falls back to primary", SiteMember{Role: "repair_technician"}, []string{"repair_technician"}},
		{"both empty", SiteMember{}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.m.EffectiveRoles())
		})
	}
}
