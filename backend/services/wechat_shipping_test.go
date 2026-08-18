package services

import "testing"

func TestResolveExpressCompanyCode(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"顺丰速运", "SF"},
		{"顺丰", "SF"},
		{"圆通速递", "YTO"},
		{"中通快递", "ZTO"},
		{"申通快递", "STO"},
		{"韵达快递", "YUNDA"},
		{"德邦快递", "DB"},
		{"京东快递", "JDL"},
		{"极兔快递", "JTSD"},
		{"EMS", "EMS"},
		// Unknown names pass through unchanged (best-effort match).
		{"神秘物流", "神秘物流"},
	}
	for _, c := range cases {
		if got := ResolveExpressCompanyCode(c.name); got != c.want {
			t.Errorf("ResolveExpressCompanyCode(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
