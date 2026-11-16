// #2370 F1：密码强度校验（唯一实现，供所有创建入口复用）。
// 规则与后端 backend/handlers/user_staff.go validatePassword 逐条一致；
// 后端仍为最终权威，前端只做即时反馈。
export function validatePasswordStrength(pwd) {
  const value = pwd || ''
  if (value.length < 8) return { ok: false, message: '密码长度不能少于 8 位' }
  if (!/[A-Z]/.test(value)) return { ok: false, message: '密码必须包含大写字母' }
  if (!/[a-z]/.test(value)) return { ok: false, message: '密码必须包含小写字母' }
  if (!/[0-9]/.test(value)) return { ok: false, message: '密码必须包含数字' }
  return { ok: true, message: '' }
}
