package handlers

// #2598：IAM 上游失败的统一响应出口。
//
// 背景（#2597 同批扫描判定）：同一类「上游 IAM 失败」在不同 handler 里被压成
// 不同三元组——
//   - 中转建组织：持久 4xx（父组织非法 / 权限缺失）与瞬时 5xx 同为 502/50200
//     +「请稍后再试」，用户被引导去重试一件永远失败的事；
//   - 微信绑定/合并：瞬时故障（IAM 不可达/超时）被写成 409/40900，前端按
//     「数据冲突」处理（`数据冲突，请刷新后重试`、重试按钮/监控口径全错）。
//
// 本文件把「分类 → 三元组 → 文案」收敛成唯一出口，分类判据见
// services.ClassifyIAMFailure（只用 errors.As，禁止字符串匹配）。
//
//	transient  → 502 / 50200 → 文案含「请稍后再试」
//	conflict   → 409 / 40900 → 指明冲突对象 + 可纠正动作
//	userAction → 400 / 40002 → 指明用户动作（重新扫码/重新发起），不含「稍后再试」
//	config     → 500 / 50015 → 「联系平台管理员」，不含「稍后再试」

import (
	"log"
	"net/http"

	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
)

const (
	// iamConfigErrorMessage 必须与 PC / mobile `ERROR_CODE_MAP[50015]` 逐字一致
	// （#2497 规则：code >= 50000 且含 CJK 的 message 走 code_map，不透传）。
	iamConfigErrorMessage = "身份服务配置异常，请联系平台管理员处理"
	// iamConflictFallbackMessage 仅在调用点没有更具体的冲突文案时使用。
	iamConflictFallbackMessage = "操作与当前数据状态冲突，请刷新后重试"
	// iamGenericFailureMessage 是防御性兜底（调用点漏填文案时不得返回空串）。
	iamGenericFailureMessage = "操作失败，请重试"
)

// iamFailureTexts 由调用点提供业务化文案；config 分支固定使用
// iamConfigErrorMessage，防止各调用点文案漂移。
type iamFailureTexts struct {
	Transient  string // 502/50200
	Conflict   string // 409/40900（留空则 iamConflictFallbackMessage）
	UserAction string // 400/40002
	// UserActionAsConfig=true：服务端代发起的上游请求，400/422 只能是我们的
	// 请求/配置缺陷（父组织非法、参数契约不符），因此归持久配置（500/50015），
	// 不得提示用户「重做动作」。
	UserActionAsConfig bool
}

// respondIAMFailure 按失败类别写响应，并额外记录 kind/upstream_status 便于线上
// 区分「IAM 抖动」与「真冲突/真配置问题」。调用点保留自己的 log.Printf 上下文
// （user id / org name 等），细节只进日志，不进响应。
func respondIAMFailure(c *gin.Context, err error, t iamFailureTexts) {
	kind, upstream := services.ClassifyIAMFailure(err)
	if kind == services.IAMFailureUserAction && t.UserActionAsConfig {
		kind = services.IAMFailureConfig
	}
	log.Printf("[IAMFailure] kind=%s upstream_status=%d route=%s", kind, upstream, c.FullPath())

	switch kind {
	case services.IAMFailureConflict:
		msg := t.Conflict
		if msg == "" {
			msg = iamConflictFallbackMessage
		}
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": msg})
	case services.IAMFailureUserAction:
		msg := t.UserAction
		if msg == "" {
			msg = iamGenericFailureMessage
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": msg})
	case services.IAMFailureConfig:
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50015, "message": iamConfigErrorMessage})
	default: // services.IAMFailureTransient
		msg := t.Transient
		if msg == "" {
			msg = iamGenericFailureMessage
		}
		c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "message": msg})
	}
}
