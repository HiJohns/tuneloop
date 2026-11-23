package handlers

// #2453：通知/收据/action_data 金额必须使用执行净额 NetCash，
// 毛额（CashRefundable/PayableShortfall）仅供分账户展示。

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/models"
)

// R1：#2339 形状（毛退 500 / 毛补 460 / net 40 / 赠点 160）。
func TestBuildRefundReceipt_NetCash_2453(t *testing.T) {
	db := setupRefundNotifyTables(t)
	order := models.Order{ID: uuid.New().String()}
	s := &settlementResult{
		CashRefundable:        500,
		CashRefundableGross:   500,
		PayableShortfall:      460,
		PayableShortfallGross: 460,
		NetCash:               40,
		GiftPointsRefunded:    160,
		ActualDays:            28,
	}
	receipt := buildRefundReceipt(db, order, s)

	require.Contains(t, receipt, "退回微信：¥40.00", "实际退款 = net_cash")
	require.Contains(t, receipt, "实际退款合计：¥200.00", "net 40 + 赠点面值 160")
	require.NotContains(t, receipt, "退回微信：¥500.00", "毛额不得冒充实际退款")
	require.NotContains(t, receipt, "实际退款合计：¥660.00", "毛额合计不得冒充实际退款")
}

// R2：action_data 退款/补缴金额 = 净额；正负 net 均可判别。
func TestBuildRefundActionData_NetCash_2453(t *testing.T) {
	db := setupRefundNotifyTables(t)
	order := models.Order{ID: uuid.New().String()}

	positive := &settlementResult{
		CashRefundable:     500,
		PayableShortfall:   460,
		NetCash:            40,
		GiftPointsRefunded: 160,
	}
	var ad map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(buildRefundActionData(db, order, positive)), &ad))
	require.EqualValues(t, 4000, ad["refund_cents"], "refund = net 40 元 = 4000 分")
	require.EqualValues(t, 0, ad["shortfall_cents"])
	require.EqualValues(t, 16000, ad["gift_refund_cents"])

	negative := &settlementResult{
		CashRefundable:   0,
		PayableShortfall: 1.00,
		NetCash:          -0.28,
	}
	require.NoError(t, json.Unmarshal([]byte(buildRefundActionData(db, order, negative)), &ad))
	require.EqualValues(t, 0, ad["refund_cents"])
	require.EqualValues(t, 28, ad["shortfall_cents"], "补缴 = −net 0.28 元 = 28 分")
	require.EqualValues(t, 0, ad["gift_refund_cents"])
}

// R3：补缴文本 = −NetCash，不是毛额 PayableShortfall。
func TestBuildShortfallBreakdownText_NetCash_2453(t *testing.T) {
	order := models.Order{}
	s := &settlementResult{
		RentPayable:        0.36,
		PayableShortfall:   1.00,
		NetCash:            -0.28,
		GiftPointsRefunded: 0,
	}
	text := buildShortfallBreakdownText(order, s)
	require.Contains(t, text, "需补缴：¥0.28", "补缴结论 = −net_cash")
	require.NotContains(t, text, "需补缴：¥1.00", "毛额不得冒充实际补缴")
}
