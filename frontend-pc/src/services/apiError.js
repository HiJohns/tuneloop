// #2563：PC 客户端「非 2xx 失败」映射的唯一 owner（纯函数，便于 node --test）。
//
// 契约（docs/spec/api/api.md §1.3）：
//   404 → 返回对象 { code: 40400, data: null }（与改动前逐字一致）
//   409 → 返回对象 { code: 后端 code || 40900 }（40900 等业务可纠正冲突由此可达）
//   其余非 2xx（400 / 403 / 429 / 5xx）→ 抛 Error（name=ApiError），并把 code/status/data
//     挂到 Error 上，让上层 rubric（isBusinessCorrectableConflict）能在 catch 里判定。
//
// 为什么不把 400 也改成返回对象：40002 在后端同时表示「缺少必填参数」与「重名冲突」，
// 整族改成返回对象会让 256 个调用点中无 else 分支者变成静默成功（红线：禁止静默吞错）。
import { resolveErrorMessage } from './errorMessages.js'

function isObjectPayload(payload) {
  return Boolean(payload) && typeof payload === 'object' && !Array.isArray(payload)
}

// 只接受可表示为有限数字的 code（number 或数字字符串）；其余视为「无 code」（不写入非法值）。
function toFiniteCode(value) {
  const numeric = typeof value === 'string' && value.trim() !== '' ? Number(value) : value
  return typeof numeric === 'number' && Number.isFinite(numeric) ? numeric : undefined
}

export function resolveHttpFailure(status, payload) {
  // 损坏 payload（null / 字符串 / 数组）一律安全降级为空对象，不抛解析异常。
  const body = isObjectPayload(payload) ? payload : {}

  if (status === 404) {
    return {
      kind: 'result',
      value: {
        code: 40400,
        message: resolveErrorMessage({ code: 40400, message: body.message }, '未找到相关数据'),
        data: null,
        status: 404,
      },
    }
  }

  if (status === 409) {
    const code = body.code || 40900
    return {
      kind: 'result',
      value: {
        code,
        message: resolveErrorMessage({ code, message: body.message }, '数据冲突，请刷新后重试'),
        data: body.data,
        status: 409,
      },
    }
  }

  const code = toFiniteCode(body.code)
  const error = new Error(resolveErrorMessage({ code, message: body.message }, '请求失败，请重试'))
  error.name = 'ApiError'
  error.status = status
  if (code !== undefined) error.code = code
  error.data = body.data
  return { kind: 'throw', value: error }
}
