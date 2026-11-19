package services

import "testing"

func TestWxAuthErrorExpiredToken_2425(t *testing.T) {
	status, code, msg := WxAuthError(&IAMAPIError{StatusCode: 400, ErrorCode: "invalid or expired exchange_token"})
	if status != 401 || code != 40110 {
		t.Fatalf("expired exchange_token = (%d,%d), want (401,40110)", status, code)
	}
	if msg == "" {
		t.Fatal("message must not be empty")
	}
}

func TestWxAuthErrorExistingMappings_2425(t *testing.T) {
	cases := []struct {
		code   string
		status int
	}{
		{"account not active", 403},
		{"wx_user_not_found", 404},
		{"invalid wechat code", 400},
	}
	for _, tc := range cases {
		status, _, _ := WxAuthError(&IAMAPIError{StatusCode: 400, ErrorCode: tc.code})
		if status != tc.status {
			t.Fatalf("%s -> status %d, want %d", tc.code, status, tc.status)
		}
	}
	if status, code, _ := WxAuthError(errPlain{}); status != 500 || code != 50000 {
		t.Fatalf("unknown -> (%d,%d), want (500,50000)", status, code)
	}
}

type errPlain struct{}

func (errPlain) Error() string { return "plain" }
