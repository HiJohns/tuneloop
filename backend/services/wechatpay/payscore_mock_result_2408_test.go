package wechatpay

// #2408：mock provider 的评估结果可由 PAYSCORE_MOCK_RESULT 控制（仅 QA/开发）。
//   approved（默认）→ approved
//   rejected        → rejected
//   pending         → 永远 pending（验证「未 approved」禁用态）
// 真实 provider 不受影响；未识别值保持历史行为（approved）。
import (
	"context"
	"testing"
)

func TestMockPayscoreResult_2408(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want string
	}{
		{"default-approved", "", mockStateApproved},
		{"explicit-approved", "approved", mockStateApproved},
		{"rejected", "rejected", mockStateRejected},
		{"always-pending", "pending", mockStatePending},
		{"unknown-falls-back", "wat", mockStateApproved},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PAYSCORE_MOCK_RESULT", tc.env)
			m := NewMockPayscoreProvider()
			m.ApproveAfter = 0 // 立即出结果，避免等待 30s

			ctx := context.Background()
			if _, err := m.CreateOrder(ctx, PayScoreCreateParams{OutOrderNo: "o-2408"}); err != nil {
				t.Fatalf("CreateOrder: %v", err)
			}
			got, err := m.QueryOrder(ctx, "", "o-2408")
			if err != nil {
				t.Fatalf("QueryOrder: %v", err)
			}
			if got.State != tc.want {
				t.Fatalf("PAYSCORE_MOCK_RESULT=%q → state %q, want %q", tc.env, got.State, tc.want)
			}
		})
	}
}

// pending 模式连续查询都必须保持 pending（不能因为时间流逝自行通过）。
func TestMockPayscoreResult_PendingStaysPending_2408(t *testing.T) {
	t.Setenv("PAYSCORE_MOCK_RESULT", "pending")
	m := NewMockPayscoreProvider()
	m.ApproveAfter = 0

	ctx := context.Background()
	if _, err := m.CreateOrder(ctx, PayScoreCreateParams{OutOrderNo: "o-2408-p"}); err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	for i := 0; i < 3; i++ {
		got, err := m.QueryOrder(ctx, "", "o-2408-p")
		if err != nil {
			t.Fatalf("QueryOrder #%d: %v", i, err)
		}
		if got.State != mockStatePending {
			t.Fatalf("query #%d: state %q, want pending", i, got.State)
		}
	}
}
