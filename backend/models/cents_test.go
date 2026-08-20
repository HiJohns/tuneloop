package models

import (
	"encoding/json"
	"testing"
)

// #1726 P1 (audit R6): money conversion boundary tests that were claimed in
// the 7e8ebfe3 commit message but lost in the stash incident — restored.

func TestFromYuan_RoundingBoundaries(t *testing.T) {
	cases := []struct {
		yuan float64
		want Cents
	}{
		{0, 0},
		{0.01, 1},
		{0.1, 10},
		{0.2, 20},
		{0.29, 29}, // audit R2: 0.29*100 = 28.9999... must round to 29, not 28
		{0.57, 57}, // audit R2: same off-by-one guard
		{0.1 + 0.2, 30},
		// 注：1.005 等三/四位小数的元在 float64 中无法精确表示（1.005*100 =
		// 100.4999...）——两位小数以内（业务金额）保证精确；此处仅覆盖
		// 可精确表示的进位边界。
		{1.25, 125},
		{1.35, 135},
		{12.34, 1234},
		{12.35, 1235}, // 12.35 元 → 1235 分（精确进位）
		{-0.29, -29},
		{-12.34, -1234},
		{12345.67, 1234567},
	}
	for _, c := range cases {
		if got := FromYuan(c.yuan); got != c.want {
			t.Errorf("FromYuan(%v) = %d, want %d", c.yuan, got, c.want)
		}
	}
}

func TestToYuan_RoundTrip(t *testing.T) {
	for _, yuan := range []float64{0, 0.01, 1.5, 12.34, 999.99, 0.29, 0.57} {
		cents := FromYuan(yuan)
		if back := cents.ToYuan(); back != yuan {
			t.Errorf("ToYuan(FromYuan(%v)) = %v, want %v", yuan, back, yuan)
		}
	}
}

func TestCents_MarshalJSON_OutputsCents(t *testing.T) {
	// P3 (#1728): JSON outputs integer cents.
	b, err := json.Marshal(Cents(1234))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "1234" {
		t.Errorf("MarshalJSON = %s, want 1234", b)
	}

	var got Cents
	if err := json.Unmarshal([]byte("5678"), &got); err != nil {
		t.Fatal(err)
	}
	if got != 5678 {
		t.Errorf("UnmarshalJSON = %d, want 5678", got)
	}
	// null → 0
	if err := json.Unmarshal([]byte("null"), &got); err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("UnmarshalJSON(null) = %d, want 0", got)
	}
}

func TestCents_ScanCompat(t *testing.T) {
	// P2 (#1727): DB BIGINT (int64) read directly as cents.
	var c Cents
	if err := c.Scan(int64(1234)); err != nil {
		t.Fatal(err)
	}
	if c != 1234 {
		t.Errorf("Scan(int64) = %d, want 1234", c)
	}
	// Legacy DECIMAL yuan float64 → ×100.
	if err := c.Scan(12.34); err != nil {
		t.Fatal(err)
	}
	if c != 1234 {
		t.Errorf("Scan(float64 12.34) = %d, want 1234", c)
	}
	// []byte "12.34" (legacy DECIMAL text) → ×100.
	if err := c.Scan([]byte("12.34")); err != nil {
		t.Fatal(err)
	}
	if c != 1234 {
		t.Errorf("Scan([]byte) = %d, want 1234", c)
	}
	// nil → 0.
	if err := c.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if c != 0 {
		t.Errorf("Scan(nil) = %d, want 0", c)
	}
}

func TestCents_Value_WritesCents(t *testing.T) {
	v, err := Cents(1234).Value()
	if err != nil {
		t.Fatal(err)
	}
	if v != int64(1234) {
		t.Errorf("Value() = %v, want int64(1234)", v)
	}
}

func TestCents_MulPercent(t *testing.T) {
	if got := Cents(100).Mul(3); got != 300 {
		t.Errorf("Mul(3) = %d, want 300", got)
	}
	// 5000 分 × 15% = 750 分
	if got := Cents(5000).Percent(15); got != 750 {
		t.Errorf("Percent(15%%) = %d, want 750", got)
	}
	// 333 分 × 33% = 109.89 → integer division truncates 109
	if got := Cents(333).Percent(33); got != 109 {
		t.Errorf("Percent(33%%) = %d, want 109", got)
	}
}

func TestToCentsPtr_ToYuanPtr(t *testing.T) {
	yuan := 25.5
	cp := ToCentsPtr(&yuan)
	if cp == nil || *cp != 2550 {
		t.Errorf("ToCentsPtr = %v, want 2550", cp)
	}
	if ToCentsPtr(nil) != nil {
		t.Error("ToCentsPtr(nil) should be nil")
	}
	c := Cents(2550)
	yp := ToYuanPtr(&c)
	if yp == nil || *yp != 25.5 {
		t.Errorf("ToYuanPtr = %v, want 25.5", yp)
	}
	if ToYuanPtr(nil) != nil {
		t.Error("ToYuanPtr(nil) should be nil")
	}
}
