// #2516：API 结果终判（唯一 owner）。
//
// `request()` 对 404 / 409 是「返回对象、不抛错」（见 services/api.js），
// 因此调用方若只 `try/catch` 就会把失败当成功（历史缺陷：租户创建 404 仍提示
// 「租户创建成功」）。所有页面/组件一律用这里的终判，禁止各自手写 `res.code` 判断
// 或拼接提示文案（D4）。
export const API_OK_CODE = 20000

// 损坏输入（null / undefined / 非对象 / 非数字 code）一律 false，不抛错。
export function isApiOk(res) {
  if (!res || typeof res !== 'object') return false
  return Number(res.code) === API_OK_CODE
}

// 提示文案：后端非空字符串 message 优先，否则用调用点兜底文案
// （禁止把 undefined / 对象渲染进用户提示）。
export function apiFailureMessage(res, fallback = '请求失败，请重试') {
  const raw = res && typeof res === 'object' ? res.message : ''
  if (typeof raw === 'string' && raw.trim() !== '') return raw
  return fallback
}
