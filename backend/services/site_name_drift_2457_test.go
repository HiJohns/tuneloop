package services

// #2457 R10：存量漂移只读清单的纯函数。3 条不一致 → 3 行（含 org_id 与两侧名）；
// 全部一致 → 0 行；本地无 org_id / IAM 缺该组织不产出差异行（不误报）。

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComputeSiteNameDrift_ThreeInconsistent_2457(t *testing.T) {
	local := []SiteNameRef{
		{SiteID: "s1", OrgID: "org1", Name: "卡丹萨乐器"},
		{SiteID: "s2", OrgID: "org2", Name: "卡丹萨乐器"},
		{SiteID: "s3", OrgID: "org3", Name: "北京中转站"},
	}
	iam := []OrgNameRef{
		{OrgID: "org1", Name: "海淀店"},
		{OrgID: "org2", Name: "卡丹萨西铁营"},
		{OrgID: "org3", Name: "tuneloop"},
	}

	rows := ComputeSiteNameDrift(local, iam)
	require.Len(t, rows, 3)
	for i, want := range []DriftRow{
		{SiteID: "s1", OrgID: "org1", LocalName: "卡丹萨乐器", IAMName: "海淀店"},
		{SiteID: "s2", OrgID: "org2", LocalName: "卡丹萨乐器", IAMName: "卡丹萨西铁营"},
		{SiteID: "s3", OrgID: "org3", LocalName: "北京中转站", IAMName: "tuneloop"},
	} {
		require.Equal(t, want, rows[i])
		require.NotEmpty(t, rows[i].OrgID)
		require.NotEmpty(t, rows[i].LocalName)
		require.NotEmpty(t, rows[i].IAMName)
	}
	t.Logf("DRIFT_ROWS=%d", len(rows))
}

func TestComputeSiteNameDrift_EmptyWhenConsistent_2457(t *testing.T) {
	local := []SiteNameRef{{SiteID: "s1", OrgID: "org1", Name: "海淀店"}}
	iam := []OrgNameRef{{OrgID: "org1", Name: "海淀店"}}

	rows := ComputeSiteNameDrift(local, iam)
	require.Empty(t, rows)
	t.Logf("DRIFT_ROWS_EMPTY=%d", len(rows))
}

// 负例：本地无 org_id / IAM 缺组织 → 不误报。
func TestComputeSiteNameDrift_SkipsUnboundAndMissingOrgs_2457(t *testing.T) {
	local := []SiteNameRef{
		{SiteID: "s1", OrgID: "", Name: "老网点"},
		{SiteID: "s2", OrgID: "org-missing", Name: "已删组织"},
	}
	iam := []OrgNameRef{{OrgID: "org1", Name: "海淀店"}}

	require.Empty(t, ComputeSiteNameDrift(local, iam))
}
