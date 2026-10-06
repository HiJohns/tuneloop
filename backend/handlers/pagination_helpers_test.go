package handlers

import "testing"

func TestClampPageSize(t *testing.T) {
	cases := []struct {
		name string
		raw  int
		def  int
		max  int
		want int
	}{
		{"raw<1 取默认", 0, 20, 100, 20},
		{"raw<1 负值取默认", -5, 10, 100, 10},
		{"raw 区间内取原值", 50, 20, 100, 50},
		{"raw 下界 1", 1, 20, 100, 1},
		{"raw 上界 100", 100, 20, 100, 100},
		{"raw>max 截断为 max", 101, 20, 100, 100},
		{"raw>max 大值截断为 max", 1000, 20, 100, 100},
	}
	for _, c := range cases {
		if got := clampPageSize(c.raw, c.def, c.max); got != c.want {
			t.Errorf("%s: clampPageSize(%d, %d, %d) = %d, want %d", c.name, c.raw, c.def, c.max, got, c.want)
		}
	}
}
