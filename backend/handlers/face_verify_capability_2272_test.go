package handlers

// #2272: 核身能力 env 表驱动测试。

import "testing"

func TestFaceVerifyCapability_2272(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		provider string
		auto     bool
	}{
		{"no env", nil, "manual", false},
		{"eid complete", map[string]string{"FACE_VERIFY_PROVIDER": "eid", "TENCENTCLOUD_SECRET_ID": "id", "TENCENTCLOUD_SECRET_KEY": "key", "EID_MERCHANT_ID": "m"}, "eid", true},
		{"eid missing merchant", map[string]string{"FACE_VERIFY_PROVIDER": "eid", "TENCENTCLOUD_SECRET_ID": "id", "TENCENTCLOUD_SECRET_KEY": "key"}, "manual", false},
		{"faceid", map[string]string{"FACE_VERIFY_PROVIDER": "faceid", "TENCENTCLOUD_SECRET_ID": "id", "TENCENTCLOUD_SECRET_KEY": "key"}, "faceid", true},
		{"faceid no secrets", map[string]string{"FACE_VERIFY_PROVIDER": "faceid"}, "faceid", false},
		{"invalid provider", map[string]string{"FACE_VERIFY_PROVIDER": "xxx", "TENCENTCLOUD_SECRET_ID": "id", "TENCENTCLOUD_SECRET_KEY": "key"}, "manual", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"FACE_VERIFY_PROVIDER", "TENCENTCLOUD_SECRET_ID", "TENCENTCLOUD_SECRET_KEY", "EID_MERCHANT_ID"} {
				t.Setenv(k, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			p, a := FaceVerifyCapability()
			if p != tc.provider || a != tc.auto {
				t.Fatalf("got (%s,%v), want (%s,%v)", p, a, tc.provider, tc.auto)
			}
		})
	}
}
