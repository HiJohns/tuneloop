package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

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

// TestUploadShippingInfoWithRetry_RetriesOnSyncDelay verifies the retry helper
// retries after WeChat's 10060001 (payment-order sync delay) and succeeds on
// the second attempt.
func TestUploadShippingInfoWithRetry_RetriesOnSyncDelay(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cgi-bin/token") {
			fmt.Fprint(w, `{"access_token":"tok","expires_in":7200}`)
			return
		}
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			// First attempt: payment-order index not yet synced.
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"errcode":10060001,"errmsg":"支付单不存在"}`)
			return
		}
		fmt.Fprint(w, `{"errcode":0,"errmsg":"ok"}`)
	}))
	defer ts.Close()

	origBase := wxAPIBaseURL
	origBackoffs := retryBackoffs
	origToken := cachedToken
	wxAPIBaseURL = ts.URL
	retryBackoffs = []time.Duration{0, 10 * time.Millisecond}
	cachedToken = ""
	t.Cleanup(func() {
		wxAPIBaseURL = origBase
		retryBackoffs = origBackoffs
		cachedToken = origToken
	})

	os.Setenv("WX_APPID", "wx_test_appid")
	os.Setenv("WX_SECRET", "wx_test_secret")

	UploadShippingInfoWithRetry("oTestOpenid", "out_trade_1", "tx_1", "", "", "会员费", 3, "virtual membership")

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 upload attempts, got %d", got)
	}
}

// TestUploadShippingInfoWithRetry_GivesUpAfterExhaustion verifies the helper
// stops after len(retryBackoffs) attempts when the upstream keeps failing.
func TestUploadShippingInfoWithRetry_GivesUpAfterExhaustion(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cgi-bin/token") {
			fmt.Fprint(w, `{"access_token":"tok","expires_in":7200}`)
			return
		}
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"errcode":10060001,"errmsg":"支付单不存在"}`)
	}))
	defer ts.Close()

	origBase := wxAPIBaseURL
	origBackoffs := retryBackoffs
	origToken := cachedToken
	wxAPIBaseURL = ts.URL
	retryBackoffs = []time.Duration{0, 10 * time.Millisecond}
	cachedToken = ""
	t.Cleanup(func() {
		wxAPIBaseURL = origBase
		retryBackoffs = origBackoffs
		cachedToken = origToken
	})

	os.Setenv("WX_APPID", "wx_test_appid")
	os.Setenv("WX_SECRET", "wx_test_secret")

	UploadShippingInfoWithRetry("oTestOpenid", "out_trade_2", "tx_2", "", "", "会员费", 3, "virtual membership")

	if got := atomic.LoadInt32(&calls); got != int32(len(retryBackoffs)) {
		t.Fatalf("expected %d upload attempts, got %d", len(retryBackoffs), got)
	}
}

// TestNotifyConfirmReceive_ReceivedTimeIsRFC3339String is the #1953 regression:
// WeChat rejects an int64 received_time with 47001 data format error — the
// field must serialize as an RFC3339 string (same as upload_shipping_info's
// upload_time).
func TestNotifyConfirmReceive_ReceivedTimeIsRFC3339String(t *testing.T) {
	var captured []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cgi-bin/token") {
			fmt.Fprint(w, `{"access_token":"tok","expires_in":7200}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		captured = b
		fmt.Fprint(w, `{"errcode":0,"errmsg":"ok"}`)
	}))
	defer ts.Close()

	origBase := wxAPIBaseURL
	origToken := cachedToken
	wxAPIBaseURL = ts.URL
	cachedToken = ""
	t.Cleanup(func() {
		wxAPIBaseURL = origBase
		cachedToken = origToken
	})

	os.Setenv("WX_APPID", "wx_test_appid")
	os.Setenv("WX_SECRET", "wx_test_secret")

	received := time.Date(2026, 9, 17, 10, 30, 0, 0, time.UTC)
	if err := NotifyConfirmReceive("rent_out_trade_1", received); err != nil {
		t.Fatalf("NotifyConfirmReceive returned error: %v", err)
	}
	if captured == nil {
		t.Fatal("notify_confirm_receive endpoint was not called")
	}

	var body map[string]interface{}
	if err := json.Unmarshal(captured, &body); err != nil {
		t.Fatalf("notify body is not valid JSON: %v (%s)", err, captured)
	}

	rt, present := body["received_time"]
	if !present {
		t.Fatalf("received_time missing from body: %s", captured)
	}
	rtStr, isString := rt.(string)
	if !isString {
		t.Fatalf("received_time must serialize as a JSON string (int64 → WeChat 47001), got %T (%v)", rt, rt)
	}
	if _, err := time.Parse(time.RFC3339, rtStr); err != nil {
		t.Fatalf("received_time %q is not RFC3339: %v", rtStr, err)
	}
	if want := received.Format(time.RFC3339); rtStr != want {
		t.Fatalf("received_time = %q, want %q", rtStr, want)
	}

	orderKey, hasKey := body["order_key"].(map[string]interface{})
	if !hasKey {
		t.Fatalf("order_key missing from body: %s", captured)
	}
	if got := orderKey["out_trade_no"]; got != "rent_out_trade_1" {
		t.Fatalf("order_key.out_trade_no = %v, want rent_out_trade_1", got)
	}
}
