package services

// #2459 R9：商户名漂移只读清单的纯函数。2 条不一致 → 2 行（含 org_id 与两侧名）；
// 全部一致 → 0 行；本地无 org_id / IAM 缺该组织不产出差异行（不误报）。
// 与网点漂移（#2457）共用同一比较核心与 DriftRow，禁止第二套比对逻辑。

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComputeMerchantNameDrift_TwoInconsistent_2459(t *testing.T) {
	local := []MerchantNameRef{
		{MerchantID: "m1", OrgID: "org1", Name: "卡丹萨乐器"},
		{MerchantID: "m2", OrgID: "org2", Name: "旧商户名"},
		{MerchantID: "m3", OrgID: "org3", Name: "一致商户"},
	}
	iam := []OrgNameRef{
		{OrgID: "org1", Name: "卡丹萨乐器（北京）有限公司"},
		{OrgID: "org2", Name: "新商户名"},
		{OrgID: "org3", Name: "一致商户"},
	}

	rows := ComputeMerchantNameDrift(local, iam)
	require.Len(t, rows, 2)
	for i, want := range []DriftRow{
		{SiteID: "m1", OrgID: "org1", LocalName: "卡丹萨乐器", IAMName: "卡丹萨乐器（北京）有限公司"},
		{SiteID: "m2", OrgID: "org2", LocalName: "旧商户名", IAMName: "新商户名"},
	} {
		require.Equal(t, want.SiteID, rows[i].SiteID, "DriftRow.SiteID 必须是 merchants.id")
		require.Equal(t, want, rows[i])
		require.NotEmpty(t, rows[i].OrgID)
		require.NotEmpty(t, rows[i].LocalName)
		require.NotEmpty(t, rows[i].IAMName)
	}
	t.Logf("MERCHANT_DRIFT_ROWS=%d", len(rows))
}

func TestComputeMerchantNameDrift_EmptyWhenConsistent_2459(t *testing.T) {
	local := []MerchantNameRef{{MerchantID: "m1", OrgID: "org1", Name: "卡丹萨乐器"}}
	iam := []OrgNameRef{{OrgID: "org1", Name: "卡丹萨乐器"}}

	rows := ComputeMerchantNameDrift(local, iam)
	require.Empty(t, rows)
	t.Logf("MERCHANT_DRIFT_EMPTY=%d", len(rows))
}

// 负例：本地无 org_id / IAM 缺组织 → 不误报。
func TestComputeMerchantNameDrift_SkipsUnboundAndMissingOrgs_2459(t *testing.T) {
	local := []MerchantNameRef{
		{MerchantID: "m1", OrgID: "", Name: "无组织商户"},
		{MerchantID: "m2", OrgID: "org-missing", Name: "已删组织"},
	}
	iam := []OrgNameRef{{OrgID: "org1", Name: "卡丹萨乐器"}}

	require.Empty(t, ComputeMerchantNameDrift(local, iam))
}
