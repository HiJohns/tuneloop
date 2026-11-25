// #2480：错误消息解析「纯核心」（无 i18n / Taro 依赖，供 node --test 与平价脚本使用）。
//
// 背景：后端 409 常携带面向用户的中文文案（如「网点名称已存在」），而 code 家族通用文案
// （40900「数据冲突，请刷新后重试」）会先命中并遮蔽具体原因。本模块把解析顺序收敛为唯一口径：
//   L1 message_map   — 精确英文键翻译（#1675）
//   L2 human_message — 后端可读中文文案原样透传（#2480，先于 code 家族）
//   L3 code_map      — code 家族通用中文（仅 L2 未命中时兜底）
//   L4 raw_message   — 其它 message 透传（#2104）
//   L5 fallback      — 调用点兜底
//
// PC（frontend-pc/src/services/errorMessages.js）与 mobile（errorMessages.js）各有一份 map，
// 但顺序与判定必须一致；跨包漂移由 scripts/check-error-message-parity-2480.mjs 兜住（D5）。

// 面向用户的「可读文案」＝ 字符串且含 CJK 统一表意文字（含扩展 A）。
// ASCII 串视为机器码（仍走 map），避免把后端技术串直接抛给用户。
const CJK_RE = /[\u3400-\u4dbf\u4e00-\u9fff]/

export function isHumanReadableMessage(message) {
  return typeof message === 'string' && CJK_RE.test(message)
}

function hasMessage(message) {
  return typeof message === 'string' && message.trim() !== ''
}

/**
 * 解析后端错误载荷。
 * @param {{code?: number|string, message?: string}|null|undefined} result
 * @param {{codeMap?: Record<string|number, string>, messageMap?: Record<string, string>, fallback?: string}} [options]
 * @returns {{text: string, layer: 'message_map'|'human_message'|'code_map'|'raw_message'|'fallback'}}
 */
export function resolveErrorCore(result, options = {}) {
  const {
    codeMap = {},
    messageMap = {},
    fallback = '操作失败，请重试',
  } = options
  if (!result) return { text: fallback, layer: 'fallback' }
  const { code, message } = result
  if (hasMessage(message) && messageMap[message]) return { text: messageMap[message], layer: 'message_map' }
  // #2497：5xx（code >= 50000）的中文串是后端内部诊断（中文前缀 + err.Error() 拼接），
  // 不得作为 human_message 透传给用户；有 code 家族文案走 code_map，否则走 fallback。
  const numericCode = Number(code)
  const isServerError = Number.isFinite(numericCode) && numericCode >= 50000
  if (isServerError && hasMessage(message) && isHumanReadableMessage(message)) {
    const mapped = codeMap[code] || codeMap[numericCode]
    return mapped ? { text: mapped, layer: 'code_map' } : { text: fallback, layer: 'fallback' }
  }
  if (isHumanReadableMessage(message)) return { text: message, layer: 'human_message' }
  if (code && codeMap[code]) return { text: codeMap[code], layer: 'code_map' }
  if (hasMessage(message)) return { text: message, layer: 'raw_message' }
  return { text: fallback, layer: 'fallback' }
}

export default resolveErrorCore
