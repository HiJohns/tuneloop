// #2388: 创建成功反馈的统一分支判定。
//
// 后端契约（#2372/#2376）：
//   - auto_generate=true → 返回 initial_password（一次性展示）；
//   - 手动 password → 不返回密码（管理员自己设置的，不再展示）；
//   - 无密码 + 有邮箱 → IAM 邀请邮件；
//   - 无密码 + 无邮箱 → 不应到达（前端已拦截）。
//
// 页面必须共用本函数，禁止各自复制分支逻辑。
export function resolveCreateFeedback({ autoGenerate, password, email, username } = {}) {
  const identifier = String(username || '').trim() || String(email || '').trim()
  if (autoGenerate) return { kind: 'password_auto', identifier }
  if (password) return { kind: 'password_manual', identifier }
  if (email && String(email).trim()) return { kind: 'invite', identifier }
  return { kind: 'blocked', identifier }
}
