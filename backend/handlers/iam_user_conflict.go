package handlers

// #2614: IAM `PUT /api/v1/users/{id}` 409 → 冲突对象中文文案。
//
// 分类唯一 owner 仍是 #2598 的 services.ClassifyIAMFailure + respondIAMFailure；
// 本文件只做「冲突对象细化」，不参与 transient / userAction / config 判定。
//
// 判据：先用 errors.As 命中 *services.IAMClientHTTPError 且 Status == 409
// （禁止对整个 err.Error() 做子串匹配 —— 上游正文里的任意文本都可能伪造出
// 「username already exists」），再用既有的 iamConflictField（#2290）取冲突
// 字段；未知原因返回 ""，由调用点或统一出口的冲突文案兜底。

import (
	"errors"
	"net/http"

	"tuneloop-backend/services"
)

// iamUserUpdateConflictText 返回 IAM UpdateUser 409 的冲突对象文案。
func iamUserUpdateConflictText(err error) string {
	var httpErr *services.IAMClientHTTPError
	if !errors.As(err, &httpErr) || httpErr == nil || httpErr.Status != http.StatusConflict {
		return ""
	}
	switch iamConflictField(err) {
	case "username":
		// 与 #2446 既有用例逐字一致（update_current_user_2446_test.go / api.md）。
		return "用户名已被占用，请换一个"
	case "email":
		return "邮箱已被其他账户使用，请更换后重试"
	case "phone":
		return "手机号已被其他账户使用，请更换后重试"
	default:
		return ""
	}
}
