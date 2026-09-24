package handlers

import "testing"

// #2049 整改回归：仅 technician_{id}.webp 推导 _thumb.jpg；存量 /upload 键 → ""（前端回退原图）。
func TestTechnicianThumbURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"/uploads/media/1727000000_deadbeef.webp", ""}, // 存量旧 /upload 键：无 _thumb 变体
		{"/uploads/media/technician_abc.webp", "/uploads/media/technician_abc_thumb.jpg"},
		{"https://cdn.example.com/media/technician_z.webp", "https://cdn.example.com/media/technician_z_thumb.jpg"},
		{"/uploads/media/technician_noext", ""},
		{"/uploads/media/avatar_user1.webp", ""}, // 其他前缀同样不推导
	}
	for _, c := range cases {
		if got := technicianThumbURL(c.in); got != c.want {
			t.Errorf("technicianThumbURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
