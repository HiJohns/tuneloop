package services

import "errors"

// WxAuthError maps an IAM wx-auth error to (HTTP status, business code, message).
// #2425：invalid/expired exchange_token 必须是 401/40110（可刷新），不得落 500。
func WxAuthError(err error) (status int, code int, msg string) {
	var iamErr *IAMAPIError
	if errors.As(err, &iamErr) {
		switch iamErr.ErrorCode {
		case "invalid or expired exchange_token":
			return 401, 40110, "登录凭证已过期，请重新进入"
		case "account not active":
			return 403, 40300, "账户已停用，请联系管理员"
		case "wx_user_not_found":
			return 404, 40400, "该微信号尚未注册，请先注册"
		case "invalid wechat code":
			return 400, 40002, "微信登录凭证失效，请重试"
		}
	}
	return 500, 50000, "微信登录失败，请稍后重试"
}
