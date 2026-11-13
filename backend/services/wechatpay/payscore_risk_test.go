package wechatpay

import (
	"testing"

	"tuneloop-backend/models"
)

// TestComputeRiskAmount 覆盖三档风险金额口径（#2327 矩阵 R1）。
func TestComputeRiskAmount(t *testing.T) {
	cases := []struct {
		name    string
		tier    string
		full    int64
		reduced int64
		price   int64
		want    int64
		wantErr bool
	}{
		{"full 档无需风险金额", TierFull, 5000, 2000, 12000, 0, false},
		{"reduced 档=全额-差额", TierReduced, 5000, 2000, 12000, 3000, false},
		{"reduced 档差额等于全额", TierReduced, 5000, 5000, 12000, 0, false},
		{"zero 档=乐器原价", TierZero, 0, 0, 12000, 12000, false},
		// 负例：差额大于全额必须报错（不得静默取 0 或负数）
		{"negative reduced 超过 full", TierReduced, 2000, 5000, 12000, 0, true},
		// 负例：zero 档缺原价
		{"negative zero 档缺原价", TierZero, 0, 0, 0, 0, true},
		// 负例：非法档位
		{"negative 非法档位", "vip", 5000, 2000, 12000, 0, true},
		// 负例：负金额
		{"negative 负金额", TierReduced, -100, 0, 12000, 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ComputeRiskAmount(tc.tier, tc.full, tc.reduced, tc.price)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got amount=%d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("risk amount = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestTierConstantsMatchModels 防止本地常量与 models 漂移（同根扫描：档位取值只允许一处定义语义）。
func TestTierConstantsMatchModels(t *testing.T) {
	if TierFull != models.DepositTierFull {
		t.Fatalf("TierFull=%q != models.DepositTierFull=%q", TierFull, models.DepositTierFull)
	}
	if TierReduced != models.DepositTierReduced {
		t.Fatalf("TierReduced=%q != models.DepositTierReduced=%q", TierReduced, models.DepositTierReduced)
	}
	if TierZero != models.DepositTierZero {
		t.Fatalf("TierZero=%q != models.DepositTierZero=%q", TierZero, models.DepositTierZero)
	}
}
