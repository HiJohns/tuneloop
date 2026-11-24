// #2480 v2（D5/D7）：业务可纠正冲突的判定与文案（唯一口径）。
//
// 「业务可纠正」= 用户改一个输入就能重试的 4xx 冲突。这类错误必须用**持久提示**
// （FormConflictAlert）而非 3 秒 toast，否则用户读不完就消失（#2480 Owner 追加期望 ②）。
//
// 40901/40902 明确**排除**：一人一号 / 既有账户需要用户「选择」而非「改输入」，
// 且两页已有 Modal 引导分支（D4）。
export const BUSINESS_CORRECTABLE_CODES = [40002, 40004, 40005, 40900]

// 损坏输入（null / undefined / 非对象 / 非数字 code）一律 false，不抛错。
export function isBusinessCorrectableConflict(result) {
  if (!result || typeof result !== 'object') return false
  const code = Number(result.code)
  if (!Number.isFinite(code)) return false
  return BUSINESS_CORRECTABLE_CODES.includes(code)
}

// 文案：后端具体 message 优先（非空字符串），否则用调用点给的兜底文案。
export function conflictText(result, fallback = '请修改后重新提交') {
  const raw = result && typeof result === 'object' ? result.message : ''
  const message = typeof raw === 'string' ? raw.trim() : ''
  return message || fallback
}
