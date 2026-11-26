// #2480：错误消息解析「纯核心」（无 i18n / Taro 依赖，供 node --test 与平价脚本使用）。
//
// 背景：后端 409 常携带面向用户的中文文案（如「网点名称已存在」），而 code 家族通用文案
// （40900「数据冲突，请刷新后重试」）会先命中并遮蔽具体原因。本模块把解析顺序收敛为唯一口径：
//   L1 message_map   — 精确英文键翻译（#1675）
//   L2 human_message — 后端可读中文文案原样透传（#2480，先于 code 家族）
//   L3 code_map      — code 家族通用中文（仅 L2 未命中时兜底）
//   L4 raw_message   — **已不可达**（#2504 D8）：含 CJK 的文案在 L2 已返回，本层不再有
//                      返回分支；层名保留仅为兼容历史断言/日志口径
//   L5 fallback      — 调用点兜底
//
// #2504：ASCII 机器串（如后端硬编码 `access denied`）不直出；未知码走 fallback。
// #2560：5xx 的中文串改按**形态**判定（isInternalDiagnosticMessage，PC 逐字一致）——
// 内部诊断形态走 code_map/fallback，固定用户文案（可操作提示）原样透传。
//
// PC（frontend-pc/src/services/errorMessages.js）与 mobile（errorMessages.js）各有一份 map，
// 但顺序与判定必须一致；跨包漂移由 scripts/check-error-message-parity-2480.mjs 兜住（D5）。

// 面向用户的「可读文案」＝ 字符串且含 CJK 统一表意文字（含扩展 A）。
// ASCII 串视为机器码（仍走 map），避免把后端技术串直接抛给用户。
const CJK_RE = /[\u3400-\u4dbf\u4e00-\u9fff]/

export function isHumanReadableMessage(message) {
  return typeof message === 'string' && CJK_RE.test(message)
}

// #2560：内部诊断形态表（唯一 owner 见 Accepted Plan D2；PC 与 mobile 逐字一致）。
// 小写化后 substring 匹配：后端把 err.Error() 拼进 5xx message 时几乎必然命中其一。
export const INTERNAL_DIAGNOSTIC_MARKERS = [
  'pq:',
  'sqlstate',
  'rpc error',
  'grpc:',
  'dial tcp',
  'connection refused',
  'connection reset',
  'no such host',
  'i/o timeout',
  'context deadline exceeded',
  'context canceled',
  'unexpected eof',
  'invalid memory address',
  'nil pointer',
  'panic:',
  'runtime error',
  'unsupported protocol scheme',
  'x509:',
  'cannot unmarshal',
  'record not found',
]

// 「中文前缀 + 冒号 + ASCII 尾串」= 拼接诊断的特征形态（固定用户文案不会长这样）。
// 方括号用 \u005B/\u005D 表示：字符类里的裸 [ ] 会被解析器当作类结束符（且 ESLint
// no-useless-escape 不允许简单转义）。语义与原字符类逐一等价（已用样本比对）。
const DIAGNOSTIC_TAIL_RE = /[：:]\s*[A-Za-z][A-Za-z0-9_.:@/'"()\u005B\u005D{}<>,;=+\-* ]{2,}$/

// #2560：5xx message 是否为「内部诊断形态」（命中则必须屏蔽，未命中才可透传）。
export function isInternalDiagnosticMessage(message) {
  if (typeof message !== 'string' || message.trim() === '') return false
  const lowered = message.toLowerCase()
  if (INTERNAL_DIAGNOSTIC_MARKERS.some((marker) => lowered.includes(marker))) return true
  return DIAGNOSTIC_TAIL_RE.test(message.trim())
}

function hasMessage(message) {
  return typeof message === 'string' && message.trim() !== ''
}

/**
 * 解析后端错误载荷。
 * @param {{code?: number|string, message?: string}|null|undefined} result
 * @param {{codeMap?: Record<string|number, string>, messageMap?: Record<string, string>, fallback?: string}} [options]
 * @returns {{text: string, layer: 'message_map'|'human_message'|'code_map'|'raw_message'|'fallback'}}（'raw_message' 仅保留层名，不再产生）
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
  // #2497/#2560：5xx（code >= 50000）的中文串按**形态**判定——
  // 内部诊断（拼接 err.Error()）必须走 code_map/fallback 屏蔽；固定用户文案（可操作提示）
  // 原样透传，否则用户会失去处置指引（#2560 主诉）。
  const numericCode = Number(code)
  const isServerError = Number.isFinite(numericCode) && numericCode >= 50000
  if (isServerError && hasMessage(message) && isHumanReadableMessage(message)) {
    if (isInternalDiagnosticMessage(message)) {
      const mapped = codeMap[code] || codeMap[numericCode]
      return mapped ? { text: mapped, layer: 'code_map' } : { text: fallback, layer: 'fallback' }
    }
    return { text: message, layer: 'human_message' }
  }
  if (isHumanReadableMessage(message)) return { text: message, layer: 'human_message' }
  if (code && codeMap[code]) return { text: codeMap[code], layer: 'code_map' }
  // #2504 D8：L4 raw_message 返回分支已删除（不可达：含 CJK 的文案在 L2 已返回）。
  return { text: fallback, layer: 'fallback' }
}

export default resolveErrorCore
